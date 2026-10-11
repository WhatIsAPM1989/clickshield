package store

import (
	"context"
	"encoding/json"
	"github.com/lamooof/clickshield/internal/analytics"
	"os"
	"testing"
	"time"

	"github.com/lamooof/clickshield/internal/domain"
	"github.com/lamooof/clickshield/internal/scoring"
)

func TestProductAnalyticsLifecycle(t *testing.T) {
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
	p, e := st.CreateProject(ctx, "Product test", []string{"https://product.test"}, "MONITORING", cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer st.Pool.Exec(ctx, `DELETE FROM projects WHERE id=$1`, p.ID)
	sid, _ := UUID()
	vid, _ := UUID()
	bid, _ := UUID()
	now := time.Now().UnixMilli()
	b := domain.EventBatch{SessionID: sid, VisitorID: vid, BatchID: bid, PageIndex: 1, PageURL: "https://product.test/start", LandingURL: "https://product.test/start", Device: domain.Device{Type: "desktop", Language: "ru-RU"}, Events: []domain.Event{
		{Type: "pageview", TS: now, PageIndex: 1, Extra: map[string]any{"page_url": "https://product.test/start"}},
		{Type: "business", Name: "call", Properties: map[string]any{"plan": "pro", "count": float64(2)}, TS: now + 1, PageIndex: 1},
		{Type: "click", TS: now + 2, PageIndex: 1, X: 100, Y: 200, ScrollY: 300, VW: 1000, VH: 600, DocumentHeight: 2000},
		{Type: "scroll", TS: now + 3, PageIndex: 1, VW: 1000, VH: 600, DocumentHeight: 2000, Depth: .8},
		{Type: "pageview", TS: now + 4, PageIndex: 2, Extra: map[string]any{"page_url": "https://product.test/thanks"}},
	}}
	remote := domain.RemoteMeta{UserAgent: "Mozilla/5.0 (Windows NT 10.0) Chrome/140.0 Safari/537.36 Edg/140.0", Country: "RU", Region: "Moscow", City: "Moscow"}
	if _, e = st.Ingest(ctx, p, b, remote); e != nil {
		t.Fatal(e)
	}
	if r, e := st.Ingest(ctx, p, b, remote); e != nil || r.InsertedEvents != 0 {
		t.Fatalf("retry: %+v %v", r, e)
	}
	f := SessionFilter{ProjectID: p.ID}
	events, e := projectedReports(t, st).BusinessReport(ctx, f)
	if e != nil || len(events) != 1 || events[0].Events != 1 || events[0].Visitors != 1 {
		t.Fatalf("events: %+v %v", events, e)
	}
	props, e := projectedReports(t, st).EventProperties(ctx, "call", f)
	if e != nil || len(props) != 2 {
		t.Fatalf("properties: %+v %v", props, e)
	}
	d := AnalyticsDefinition{ProjectID: p.ID, Kind: "funnel", Name: "Call", WindowMinutes: 60, Steps: []AnalyticsStep{{Type: "page", Value: "/start"}, {Type: "event", Value: "call", Properties: map[string]any{"plan": "pro"}}, {Type: "page", Value: "/thanks"}}}
	d, e = projectedReports(t, st).SaveDefinition(ctx, d)
	if e != nil {
		t.Fatal(e)
	}
	report, e := projectedReports(t, st).FunnelReport(ctx, d, f)
	if e != nil || report.EligibleVisits != 1 || report.Counts[2] != 1 {
		t.Fatalf("funnel: %+v %v", report, e)
	}
	other, _ := UUID()
	if e = projectedReports(t, st).DeleteDefinition(ctx, other, d.ID); e != analytics.ErrNotFound {
		t.Fatalf("cross-project delete: %v", e)
	}
	d.ProjectID = other
	if _, e = projectedReports(t, st).SaveDefinition(ctx, d); e != analytics.ErrNotFound {
		t.Fatalf("cross-project update: %v", e)
	}
	d.ProjectID = p.ID
	edges, e := projectedReports(t, st).Journey(ctx, f)
	if e != nil || len(edges) != 3 {
		t.Fatalf("journey: %+v %v", edges, e)
	}
	heat, e := projectedReports(t, st).Heatmap(ctx, f, "", 0)
	if e != nil || heat.Visits != 1 || len(heat.Clicks) != 1 || heat.Clicks[0].X != 10 || heat.Clicks[0].Y != 25 || heat.Clicks[0].Count != 1 || heat.Scroll[8].Visits != 1 || heat.Scroll[9].Visits != 0 {
		t.Fatalf("heatmap: %+v %v", heat, e)
	}
	for _, kind := range []string{"geo", "languages", "browsers"} {
		r, e := projectedReports(t, st).AggregateReport(ctx, kind, "UTC", f)
		if e != nil || len(r.Rows) < 1 || r.Totals.Visits != 1 {
			t.Fatalf("audience %s: %+v %v", kind, r, e)
		}
	}
	// A return across a local midnight is day one despite being only an hour later.
	first := time.Now().UTC().Add(-72 * time.Hour).Truncate(24 * time.Hour).Add(23*time.Hour + 30*time.Minute)
	if _, e = st.Pool.Exec(ctx, `UPDATE sessions SET started_at=$2,last_seen_at=$2 WHERE id=$1`, sid, first); e != nil {
		t.Fatal(e)
	}
	b.SessionID, _ = UUID()
	b.BatchID, _ = UUID()
	b.Events = []domain.Event{{Type: "pageview", TS: now, PageIndex: 1}}
	if _, e = st.Ingest(ctx, p, b, remote); e != nil {
		t.Fatal(e)
	}
	if _, e = st.Pool.Exec(ctx, `UPDATE sessions SET started_at=$2,last_seen_at=$2 WHERE id=$1`, b.SessionID, first.Add(time.Hour)); e != nil {
		t.Fatal(e)
	}
	cells, e := projectedReports(t, st).Retention(ctx, f, "UTC")
	if e != nil || len(cells) != 31 || cells[0].Visitors != 1 || cells[1].Returned != 1 {
		t.Fatalf("retention: %+v %v", cells, e)
	}
	if _, e = st.Pool.Exec(ctx, `DELETE FROM raw_events WHERE project_id=$1`, p.ID); e != nil {
		t.Fatal(e)
	}
	events, e = projectedReports(t, st).BusinessReport(ctx, f)
	if e != nil || events[0].Events != 1 {
		t.Fatalf("business history removed with raw data: %+v %v", events, e)
	}
	report, e = projectedReports(t, st).FunnelReport(ctx, d, f)
	if e != nil || report.Counts[2] != 1 {
		t.Fatalf("funnel after raw deletion: %+v %v", report, e)
	}
	if e = projectedReports(t, st).DeleteDefinition(ctx, p.ID, d.ID); e != nil {
		t.Fatal(e)
	}
}
