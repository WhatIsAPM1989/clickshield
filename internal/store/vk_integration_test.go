package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/lamooof/clickshield/internal/domain"
	"github.com/lamooof/clickshield/internal/scoring"
)

// Run against a dedicated test database with CLICKSHIELD_TEST_DATABASE_URL.
func TestVKIngestionAndOutbox(t *testing.T) {
	dsn := os.Getenv("CLICKSHIELD_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CLICKSHIELD_TEST_DATABASE_URL is not set")
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
	if err = st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	cfg, _ := json.Marshal(scoring.DefaultConfig())
	p, err := st.CreateProject(ctx, "VK integration test", []string{"https://example.test"}, "NOTIFY", cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Pool.Exec(ctx, "DELETE FROM projects WHERE id=$1", p.ID)
	if err = st.UpsertIntegration(ctx, p.ID, "VK_ADS", true, map[string]string{"pixel_id": "3229418"}, nil); err != nil {
		t.Fatal(err)
	}
	items, err := st.Integrations(ctx, p.ID)
	if err != nil || len(items) != 1 || items[0].HasSecret {
		t.Fatalf("integrations = %+v, err = %v", items, err)
	}
	sid, _ := UUID()
	visitor, _ := UUID()
	batchID, _ := UUID()
	batch := domain.EventBatch{SessionID: sid, VisitorID: visitor, BatchID: batchID, RBClickID: "first-click", PageURL: "https://example.test/", LandingURL: "https://example.test/", Events: []domain.Event{{Type: "pageview", TS: time.Now().UnixMilli()}}, Device: domain.Device{Type: "desktop"}}
	if _, err = st.Ingest(ctx, p, batch, domain.RemoteMeta{UserAgent: "test-browser"}); err != nil {
		t.Fatal(err)
	}
	// Navigation without a new click preserves attribution; a new click replaces it.
	for _, click := range []string{"", "last-click"} {
		batch.RBClickID = click
		batch.BatchID, _ = UUID()
		if _, err = st.Ingest(ctx, p, batch, domain.RemoteMeta{UserAgent: "test-browser"}); err != nil {
			t.Fatal(err)
		}
		c, err := st.SessionContext(ctx, sid)
		want := "first-click"
		if click != "" {
			want = click
		}
		if err != nil || c.RBClickID != want || c.PaidVisitFrequency != 1 {
			t.Fatalf("context = %+v, err = %v", c, err)
		}
	}
	stats, err := st.Stats(ctx, p.ID)
	if err != nil || stats.PaidVisits != 1 {
		t.Fatalf("stats = %+v, err = %v", stats, err)
	}
	count := 0
	err = st.IterateSessions(ctx, SessionFilter{ProjectID: p.ID, PaidOnly: true}, 0, 0, func(s domain.SessionItem) error {
		count++
		if s.RBClickID != "last-click" {
			t.Errorf("click = %q", s.RBClickID)
		}
		return nil
	})
	if err != nil || count != 1 {
		t.Fatalf("paid sessions = %d, err = %v", count, err)
	}
	detail, err := st.SessionDetail(ctx, sid)
	if err != nil || detail.Session.RBClickID != "last-click" {
		t.Fatalf("detail err = %v", err)
	}
	f := domain.Features{SessionID: sid, ComputedAt: time.Now(), ClickIntervalsMS: []int64{}}
	score := domain.Score{Classification: domain.Bot, RiskScore: 90}
	for i := 0; i < 2; i++ {
		if _, err = st.SaveAutomatic(ctx, f, score); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err = st.Pool.QueryRow(ctx, "SELECT count(*) FROM integration_outbox WHERE project_id=$1", p.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("outbox count = %d, err = %v", n, err)
	}
	var id int64
	if err = st.Pool.QueryRow(ctx, "SELECT id FROM integration_outbox WHERE project_id=$1", p.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	delivery, err := st.Delivery(ctx, id)
	if err != nil || delivery.Session.RBClickID != "last-click" || delivery.EventName != "antifraud_bot" || delivery.Integration.Provider != "VK_ADS" {
		t.Fatalf("delivery = %+v, err = %v", delivery, err)
	}
}
