package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/lamooof/clickshield/internal/antifraud"
	"github.com/lamooof/clickshield/internal/domain"
	"github.com/lamooof/clickshield/internal/feature"
	"github.com/lamooof/clickshield/internal/scoring"
)

func TestScoringLeaseRevisionAndProvenance(t *testing.T) {
	dsn := os.Getenv("CLICKSHIELD_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("dedicated database required")
	}
	ctx := context.Background()
	st, err := New(ctx, dsn, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err = st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	cfg, _ := json.Marshal(scoring.DefaultConfig())
	p, err := st.CreateProject(ctx, "Score lifecycle", []string{"https://example.test"}, domain.ModeNotify, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Pool.Exec(ctx, `DELETE FROM projects WHERE id=$1`, p.ID)
	sid, _ := UUID()
	vid, _ := UUID()
	bid, _ := UUID()
	b := domain.EventBatch{SessionID: sid, VisitorID: vid, BatchID: bid, PageURL: "https://example.test/", PageIndex: 1, Finish: true, Events: []domain.Event{{Type: "pageview", TS: time.Now().UnixMilli(), PageIndex: 1}}}
	if _, err = st.Ingest(ctx, p, b, domain.RemoteMeta{}); err != nil {
		t.Fatal(err)
	}
	lease, err := st.ClaimScore(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.ClaimScore(ctx, sid); !errors.Is(err, antifraud.ErrBusy) {
		t.Fatalf("second worker: %v", err)
	}
	input, err := st.LoadScore(ctx, sid, lease)
	if err != nil {
		t.Fatal(err)
	}
	f := feature.Compute(sid, input.Events, input.Context.StartedAt, input.Context.LastSeenAt)
	w := antifraud.ScoreWrite{Input: input, Features: f, Score: domain.Score{Classification: domain.Human}, Config: cfg, Version: antifraud.AlgorithmVersion}
	b.BatchID, _ = UUID()
	b.Events = []domain.Event{{Type: "click", TS: time.Now().UnixMilli()}}
	if _, err = st.Ingest(ctx, p, b, domain.RemoteMeta{}); err != nil {
		t.Fatal(err)
	}
	if _, err = st.SaveScore(ctx, w); !errors.Is(err, antifraud.ErrChanged) {
		t.Fatalf("late batch accepted: %v", err)
	}
	var count int
	if err = st.Pool.QueryRow(ctx, `SELECT count(*) FROM classifications WHERE session_id=$1`, sid).Scan(&count); err != nil || count != 0 {
		t.Fatalf("stale score persisted: %d %v", count, err)
	}
	if err = st.ReleaseScore(ctx, sid, lease); err != nil {
		t.Fatal(err)
	}
	if _, err = st.Pool.Exec(ctx, `UPDATE sessions SET score_next_attempt_at=now() WHERE id=$1`, sid); err != nil {
		t.Fatal(err)
	}
	claim, ok, err := st.ClaimDueScore(ctx, 0, time.Minute)
	if err != nil || !ok || claim.ID != sid {
		t.Fatalf("claim due: %+v %v %v", claim, ok, err)
	}
	if _, ok, err = st.ClaimDueScore(ctx, 0, time.Minute); err != nil || ok {
		t.Fatalf("duplicate due claim: %v %v", ok, err)
	}
	if _, err = st.Pool.Exec(ctx, `UPDATE sessions SET scoring_lease_until=now()-interval '1 second' WHERE id=$1`, sid); err != nil {
		t.Fatal(err)
	}
	newLease, err := st.ClaimScore(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.ReleaseScore(ctx, sid, claim.Lease); err != nil {
		t.Fatal(err)
	}
	input, err = st.LoadScore(ctx, sid, newLease)
	if err != nil {
		t.Fatalf("old worker released new lease: %v", err)
	}
	if err = antifraud.NewScorer(st).FinalizeClaimed(ctx, sid, newLease); err != nil {
		t.Fatal(err)
	}
	var version string
	var revision int64
	var contextRaw, featuresRaw []byte
	if err = st.Pool.QueryRow(ctx, `SELECT algorithm_version,input_revision,context_snapshot,features_snapshot FROM classifications WHERE session_id=$1`, sid).Scan(&version, &revision, &contextRaw, &featuresRaw); err != nil {
		t.Fatal(err)
	}
	if version != antifraud.AlgorithmVersion || revision != input.Revision || len(contextRaw) == 0 || len(featuresRaw) == 0 {
		t.Fatalf("missing provenance: %s %d", version, revision)
	}
}
