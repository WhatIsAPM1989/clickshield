package antifraud

import (
	"context"
	"github.com/lamooof/clickshield/internal/domain"
	"testing"
	"time"
)

type batchMemory struct {
	batch    PreparedBatch
	inserted int
	blocked  bool
}

func (r *batchMemory) WriteBatch(_ context.Context, b PreparedBatch) (int, bool, error) {
	r.batch = b
	return r.inserted, r.blocked, nil
}

func TestIngestPolicyAndSPAPageResolution(t *testing.T) {
	r := &batchMemory{inserted: 3, blocked: true}
	p := domain.Project{Mode: domain.ModeBlockingBots, AllowedOrigins: []string{"https://example.test"}}
	b := domain.EventBatch{PageIndex: 2, PageURL: "https://example.test/new", Events: []domain.Event{
		{Type: "click", TS: 1, PageIndex: 1},
		{Type: "business", TS: 1, PageIndex: 2},
		{Type: "pageview", TS: 1, PageIndex: 2, Extra: map[string]any{"page_url": "https://example.test/new?gclid=x"}},
	}}
	result, err := NewIngestor(r).Ingest(context.Background(), p, b, domain.RemoteMeta{})
	if err != nil || result.Decision != "BLOCK" || result.InsertedEvents != 3 || len(r.batch.BlockedClasses) != 1 || r.batch.BlockedClasses[0] != domain.Bot {
		t.Fatalf("policy: %+v %v", result, err)
	}
	ResolvePages(&r.batch, map[int]string{1: "https://example.test/old?utm_source=x"})
	if r.batch.Events[0].PageURL != "https://example.test/old" || r.batch.Events[1].PageURL != "https://example.test/new" || r.batch.Events[2].PageURL != "https://example.test/new?gclid=x" {
		t.Fatalf("SPA pages: %+v", r.batch.Events)
	}
	if r.batch.Events[0].Event.TS <= 1 || time.Since(r.batch.Events[0].At) > time.Minute {
		t.Fatal("invalid event time not normalized")
	}
}
