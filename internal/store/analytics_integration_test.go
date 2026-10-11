package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/lamooof/clickshield/internal/domain"
	"github.com/lamooof/clickshield/internal/scoring"
	"os"
	"testing"
	"time"
)

func TestAnalyticsReplayLifecycle(t *testing.T) {
	dsn := os.Getenv("CLICKSHIELD_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("dedicated database required")
	}
	ctx := context.Background()
	st, e := New(ctx, dsn, []byte("0123456789abcdef0123456789abcdef"))
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	if e = st.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	cfg, _ := json.Marshal(scoring.DefaultConfig())
	p, e := st.CreateProject(ctx, "Analytics test", []string{"https://example.test"}, "MONITORING", cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer st.Pool.Exec(ctx, `DELETE FROM projects WHERE id=$1`, p.ID)
	sid, _ := UUID()
	vid, _ := UUID()
	bid, _ := UUID()
	now := time.Now().UnixMilli()
	b := domain.EventBatch{SessionID: sid, VisitorID: vid, BatchID: bid, PageURL: "https://example.test/landing", LandingURL: "https://example.test/landing?yclid=entry-click&utm_campaign=entry", Referrer: "https://yandex.ru/search", UTM: domain.UTM{Source: "yandex", Medium: "cpc", Campaign: "entry"}, YCLID: "entry-click", PageIndex: 1, Events: []domain.Event{{Type: "pageview", TS: now, PageIndex: 1, Extra: map[string]any{"page_url": "https://example.test/landing"}}}}
	if _, e = st.Ingest(ctx, p, b, domain.RemoteMeta{}); e != nil {
		t.Fatal(e)
	}
	b.BatchID, _ = UUID()
	b.Referrer = "https://example.test/landing"
	b.UTM.Campaign = "overwrite"
	b.PageIndex = 2
	b.PageURL = "https://example.test/contact"
	b.Events = []domain.Event{{Type: "pageview", TS: now + 1, PageIndex: 2, Extra: map[string]any{"page_url": b.PageURL}}}
	if _, e = st.Ingest(ctx, p, b, domain.RemoteMeta{}); e != nil {
		t.Fatal(e)
	}
	detail, e := st.SessionDetail(ctx, sid)
	if e != nil {
		t.Fatal(e)
	}
	if detail.Context.Referrer != "https://yandex.ru/search" || detail.Session.UTMCampaign != "entry" || detail.Session.TrafficSource != "yandex" || detail.Session.TrafficChannel != "paid" || len(detail.Pages) != 2 {
		t.Fatalf("incorrect detail: %+v", detail)
	}
	rows, e := st.TrafficReport(ctx, SessionFilter{ProjectID: p.ID})
	if e != nil || len(rows) != 1 || rows[0].Visits != 1 {
		t.Fatalf("report: %+v %v", rows, e)
	}
	filtered, e := st.ListSessions(ctx, SessionFilter{ProjectID: p.ID, LandingURL: "https://example.test/landing", Campaign: "entry", CampaignSet: true}, 100, 0)
	if e != nil || len(filtered) != 1 {
		t.Fatalf("landing drilldown: %+v %v", filtered, e)
	}
	if rows[0].LandingURL != "https://example.test/landing" {
		t.Fatalf("landing grouping included click ID: %+v", rows[0])
	}
	if e = st.UpdateReplaySettings(ctx, p.ID, true, 100, 7); e != nil {
		t.Fatal(e)
	}
	pid, _ := UUID()
	raw := []byte(`[{"type":4,"timestamp":` + jsonNumber(now) + `,"data":{"width":900,"height":500}},{"type":2,"timestamp":` + jsonNumber(now) + `,"data":{"node":{"type":0,"id":1,"childNodes":[]}}}]`)
	split := len(raw) / 2
	part := ReplayPart{SessionID: sid, VisitorID: vid, PageID: pid, PageIndex: 1, PageURL: b.LandingURL, Sequence: 0, PartCount: 2, StartedAt: now, EndedAt: now, Data: raw[:split]}
	if e = st.SaveReplayPart(ctx, p.ID, part); e != nil {
		t.Fatal(e)
	}
	if e = st.SaveReplayPart(ctx, p.ID, part); e != nil {
		t.Fatalf("retry: %v", e)
	}
	data, e := st.Replay(ctx, sid)
	if e != nil || len(data.Pages) != 1 || !data.Incomplete || len(data.Pages[0].Events) != 0 {
		t.Fatalf("partial recording: %+v %v", data, e)
	}
	wrong := part
	wrong.Data = bytes.Repeat([]byte("x"), len(wrong.Data))
	if e = st.SaveReplayPart(ctx, p.ID, wrong); !errors.Is(e, ErrReplayConflict) {
		t.Fatalf("conflict=%v", e)
	}
	part.PartIndex = 1
	part.Data = raw[split:]
	if e = st.SaveReplayPart(ctx, p.ID, part); e != nil {
		t.Fatal(e)
	}
	data, e = st.Replay(ctx, sid)
	if e != nil || data.Incomplete || len(data.Pages[0].Events) != 2 {
		t.Fatalf("complete recording: %+v %v", data, e)
	}
	var size int64
	if e = st.Pool.QueryRow(ctx, `SELECT total_bytes FROM replay_recordings WHERE id=$1`, sid).Scan(&size); e != nil || size != int64(len(raw)) {
		t.Fatalf("retry changed quota: %d %v", size, e)
	}
	other, _ := st.CreateProject(ctx, "Other", []string{"https://other.test"}, "MONITORING", cfg)
	defer st.Pool.Exec(ctx, `DELETE FROM projects WHERE id=$1`, other.ID)
	if e = st.SaveReplayPart(ctx, other.ID, part); !errors.Is(e, ErrNotFound) {
		t.Fatalf("cross-project access: %v", e)
	}
	if _, e = st.Pool.Exec(ctx, `UPDATE sessions SET replay_deleted_at=now() WHERE id=$1`, sid); e != nil {
		t.Fatal(e)
	}
	if e = st.SaveReplayPart(ctx, p.ID, part); !errors.Is(e, ErrNotFound) {
		t.Fatalf("deleted recording recaptured: %v", e)
	}
	if _, e = st.Pool.Exec(ctx, `UPDATE sessions SET replay_deleted_at=NULL WHERE id=$1`, sid); e != nil {
		t.Fatal(e)
	}
	if e = st.UpdateReplaySettings(ctx, p.ID, false, 100, 7); e != nil {
		t.Fatal(e)
	}
	if e = st.SaveReplayPart(ctx, p.ID, part); !errors.Is(e, ErrNotFound) {
		t.Fatalf("disabled project: %v", e)
	}
	if _, e = st.Pool.Exec(ctx, `UPDATE replay_recordings SET expires_at=now()-interval '1 second' WHERE id=$1`, sid); e != nil {
		t.Fatal(e)
	}
	if e = st.Cleanup(ctx, 7*24*time.Hour, 180*24*time.Hour); e != nil {
		t.Fatal(e)
	}
	data, e = st.Replay(ctx, sid)
	if e != nil || len(data.Pages) != 0 {
		t.Fatalf("expired recording: %+v %v", data, e)
	}
}
func jsonNumber(v int64) string { b, _ := json.Marshal(v); return string(b) }
