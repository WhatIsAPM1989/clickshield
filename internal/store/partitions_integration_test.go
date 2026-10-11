package store

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/lamooof/clickshield/internal/domain"
	"github.com/lamooof/clickshield/internal/replay"
	"github.com/lamooof/clickshield/internal/scoring"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPartitionUpgradePreservesHistoryAndSequence(t *testing.T) {
	dsn := os.Getenv("CLICKSHIELD_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("dedicated database required")
	}
	ctx := context.Background()
	root, err := NewDatabase(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	id, _ := UUID()
	schema := "upgrade_" + strings.ReplaceAll(id, "-", "")
	if _, err = root.Pool.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	defer root.Pool.Exec(ctx, "DROP DATABASE "+pgx.Identifier{schema}.Sanitize()+" WITH (FORCE)")
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + schema
	st, err := NewDatabase(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err = st.Pool.Exec(ctx, `CREATE TABLE schema_migrations(name text PRIMARY KEY,applied_at timestamptz DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	names, err := migrationFS.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range names {
		if entry.Name() > "018_scoring_lifecycle.sql" {
			continue
		}
		raw, err := migrationFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if _, err = st.Pool.Exec(ctx, string(raw)); err != nil {
			t.Fatalf("%s: %v", entry.Name(), err)
		}
		if _, err = st.Pool.Exec(ctx, `INSERT INTO schema_migrations(name) VALUES($1)`, entry.Name()); err != nil {
			t.Fatal(err)
		}
	}
	cfg, _ := json.Marshal(scoring.DefaultConfig())
	p, err := st.CreateProject(ctx, "Upgrade", []string{"https://example.test"}, domain.ModeMonitoring, cfg)
	if err != nil {
		t.Fatal(err)
	}
	sid, _ := UUID()
	vid, _ := UUID()
	bid, _ := UUID()
	old := time.Now().UTC().AddDate(0, 0, -10)
	if _, err = st.Pool.Exec(ctx, `INSERT INTO visitors(id,project_id,visitor_key) VALUES($1,$2,$3)`, vid, p.ID, vid); err != nil {
		t.Fatal(err)
	}
	if _, err = st.Pool.Exec(ctx, `INSERT INTO sessions(id,project_id,visitor_id,visitor_key) VALUES($1,$2,$3::uuid,$3::text)`, sid, p.ID, vid); err != nil {
		t.Fatal(err)
	}
	if _, err = st.Pool.Exec(ctx, `INSERT INTO raw_events(id,project_id,session_id,batch_id,event_index,event_time,event_type,payload,created_at) VALUES(12345,$1,$2,$3,0,$4,'pageview','{"type":"pageview","ts":1}',$4)`, p.ID, sid, bid, old); err != nil {
		t.Fatal(err)
	}

	pageID, _ := UUID()
	replayRaw := []byte(`[{"type":4,"timestamp":1,"data":{"href":"https://example.test/","width":800,"height":600}},{"type":2,"timestamp":2,"data":{"node":{"type":0,"id":1,"childNodes":[]}}}]`)
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	zw.Write(replayRaw)
	zw.Close()
	if _, err = st.Pool.Exec(ctx, `INSERT INTO replay_recordings(id,project_id,session_id,visitor_key,expires_at,total_bytes) VALUES($1,$2,$1,$3,now()+interval '7 days',$4)`, sid, p.ID, vid, len(replayRaw)); err != nil {
		t.Fatal(err)
	}
	if _, err = st.Pool.Exec(ctx, `INSERT INTO replay_chunks(recording_id,page_id,sequence,part_index,part_count,page_url,page_index,started_at,ended_at,payload,event_count) VALUES($1,$2,0,0,1,'https://example.test/',1,1,2,$3,2)`, sid, pageID, compressed.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err = st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	repo := replay.NewRepository(st.Pool)
	objects := testReplayObjects(t)
	if err = repo.CheckLegacy(ctx); err == nil {
		t.Fatal("legacy replay silently accepted")
	}
	if err = repo.MigrateLegacy(ctx, objects); err != nil {
		t.Fatal(err)
	}
	if err = repo.MigrateLegacy(ctx, objects); err != nil {
		t.Fatal("migration not idempotent", err)
	}
	if err = repo.CheckLegacy(ctx); err != nil {
		t.Fatal(err)
	}
	data, err := replay.NewService(repo, objects).Replay(ctx, sid)
	if err != nil || len(data.Pages) != 1 || len(data.Pages[0].Events) != 2 {
		t.Fatalf("replay upgrade lost data: %+v %v", data, err)
	}
	var hasPayload bool
	if err = st.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='replay_chunks' AND column_name='payload')`).Scan(&hasPayload); err != nil || hasPayload {
		t.Fatal("replay payload stayed in PG", err)
	}
	var preserved int
	if err = st.Pool.QueryRow(ctx, `SELECT count(*) FROM raw_events WHERE id=12345 AND created_at=$1 AND payload->>'type'='pageview'`, old).Scan(&preserved); err != nil || preserved != 1 {
		t.Fatalf("history: %d %v", preserved, err)
	}
	var generated int64
	if err = st.Pool.QueryRow(ctx, `INSERT INTO raw_events(project_id,session_id,batch_id,event_index,event_time,event_type,payload) VALUES($1,$2,$3,1,now(),'click','{"type":"click","ts":2}') RETURNING id`, p.ID, sid, bid).Scan(&generated); err != nil || generated <= 12345 {
		t.Fatalf("sequence: %d %v", generated, err)
	}
	if err = st.DropRawPartitions(ctx, time.Now().UTC().AddDate(0, 0, -7)); err != nil {
		t.Fatal(err)
	}
	if err = st.Pool.QueryRow(ctx, `SELECT count(*) FROM raw_events WHERE session_id=$1`, sid).Scan(&preserved); err != nil || preserved != 1 {
		t.Fatalf("partition retention: %d %v", preserved, err)
	}
}
