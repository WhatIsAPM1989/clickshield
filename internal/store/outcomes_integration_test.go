package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/lamooof/clickshield/internal/domain"
	"github.com/lamooof/clickshield/internal/scoring"
)

func TestSaveLeadOutcomeRetryAfterManualReview(t *testing.T) {
	dsn := os.Getenv("CLICKSHIELD_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("dedicated database required")
	}
	ctx := context.Background()
	s, e := New(ctx, dsn, []byte("0123456789abcdef0123456789abcdef"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	cfg, _ := json.Marshal(scoring.DefaultConfig())
	p, e := s.CreateProject(ctx, "Lead retry test", []string{"https://leads.test"}, "MONITORING", cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Pool.Exec(ctx, `DELETE FROM projects WHERE id=$1`, p.ID)
	var ids []string
	for i := 0; i < 2; i++ {
		sid, _ := UUID()
		vid, _ := UUID()
		bid, _ := UUID()
		b := domain.EventBatch{SessionID: sid, VisitorID: vid, BatchID: bid, PageIndex: 1, PageURL: "https://leads.test/", LandingURL: "https://leads.test/", Events: []domain.Event{{Type: "pageview", TS: time.Now().UnixMilli(), PageIndex: 1}}}
		if _, e = s.Ingest(ctx, p, b, domain.RemoteMeta{}); e != nil {
			t.Fatal(e)
		}
		ids = append(ids, sid)
	}
	if e = s.SaveLeadOutcome(ctx, p.ID, ids[0], "crm-1", "SUBMITTED", "SERVER", ""); e != nil {
		t.Fatal(e)
	}
	if e = s.SaveLeadOutcome(ctx, p.ID, ids[0], "crm-1", "QUALIFIED", "MANUAL", ""); e != nil {
		t.Fatal(e)
	}
	// The CRM webhook repeats the delivery: it succeeds and keeps the manual status.
	if e = s.SaveLeadOutcome(ctx, p.ID, ids[0], "crm-1", "SUBMITTED", "SERVER", ""); e != nil {
		t.Fatalf("retry after manual review: %v", e)
	}
	out, e := s.SessionOutcomes(ctx, ids[0])
	if e != nil || len(out) != 1 || out[0].Status != "QUALIFIED" || out[0].Source != "MANUAL" {
		t.Fatalf("outcomes: %+v %v", out, e)
	}
	// The same lead from another session is still a conflict.
	if e = s.SaveLeadOutcome(ctx, p.ID, ids[1], "crm-1", "SUBMITTED", "SERVER", ""); !errors.Is(e, ErrLeadConflict) {
		t.Fatalf("other session: %v", e)
	}
}
