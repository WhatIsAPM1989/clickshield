package analytics

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type blockingReports struct {
	ReportReader
	active, max atomic.Int64
	entered     chan struct{}
}

func (r *blockingReports) AggregateReport(ctx context.Context, _ string, _ string, _ SessionFilter) (AggregateReport, error) {
	active := r.active.Add(1)
	defer r.active.Add(-1)
	for {
		old := r.max.Load()
		if old >= active || r.max.CompareAndSwap(old, active) {
			break
		}
	}
	r.entered <- struct{}{}
	<-ctx.Done()
	return AggregateReport{}, ctx.Err()
}
func TestReportBudgetAndValidationWithoutDatabase(t *testing.T) {
	reader := &blockingReports{entered: make(chan struct{}, 10)}
	svc := &Service{reader: reader, slots: make(chan struct{}, 3)}
	for _, q := range []Query{{Report: "unknown"}, {Report: "properties"}, {Report: "events", Timezone: "unknown"}, {Report: "events", Filter: SessionFilter{Class: "unknown"}}} {
		if _, err := svc.Report(context.Background(), q); err == nil {
			t.Fatalf("invalid query accepted: %+v", q)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Report(ctx, Query{Report: "sources"})
			if !errors.Is(err, context.Canceled) {
				t.Errorf("report cancellation: %v", err)
			}
		}()
	}
	for i := 0; i < 3; i++ {
		select {
		case <-reader.entered:
		case <-time.After(time.Second):
			t.Fatal("report did not start")
		}
	}
	if reader.max.Load() != 3 {
		t.Fatalf("parallelism: %d", reader.max.Load())
	}
	cancel()
	wg.Wait()
	if reader.max.Load() > 3 || len(svc.slots) != 0 {
		t.Fatal("report budget leaked")
	}
}
