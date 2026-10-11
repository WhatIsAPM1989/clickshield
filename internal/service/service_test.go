package service

import (
	"context"
	"github.com/lamooof/clickshield/internal/config"
	"github.com/lamooof/clickshield/internal/domain"
	"github.com/lamooof/clickshield/internal/integration"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

type workerMemory struct {
	Repository
	resets atomic.Int64
	ready  chan struct{}
}

func (r *workerMemory) ResetStuckOutbox(context.Context) error {
	if r.resets.Add(1) == 1 && r.ready != nil {
		close(r.ready)
	}
	return nil
}
func (r *workerMemory) ClaimOutbox(context.Context) (domain.OutboxClaim, bool, error) {
	return domain.OutboxClaim{}, false, nil
}
func (r *workerMemory) Operations(context.Context) (domain.Operations, error) {
	return domain.Operations{}, nil
}
func (r *workerMemory) ProjectAnalytics(context.Context) (bool, error)              { return false, nil }
func (r *workerMemory) Cleanup(context.Context, time.Duration, time.Duration) error { return nil }

func TestWorkerRecoveryAndShutdownWithoutDatabase(t *testing.T) {
	r := &workerMemory{ready: make(chan struct{})}
	s := New(r, nil, config.Config{WorkerInterval: time.Hour}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.deliveryTick(context.Background())
	s.deliveryTick(context.Background())
	if r.resets.Load() != 2 {
		t.Fatal("recovery runs only at startup")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.Run(ctx)
	select {
	case <-r.ready:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	cancel()
	deadline, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := s.Wait(deadline); err != nil {
		t.Fatalf("workers not joined: %v", err)
	}
}

type deliveryMemory struct {
	workerMemory
	claimed  atomic.Bool
	finished chan string
}

func (r *deliveryMemory) ClaimOutbox(context.Context) (domain.OutboxClaim, bool, error) {
	return domain.OutboxClaim{ID: 1, Lease: "owner", Attempts: 1}, r.claimed.CompareAndSwap(false, true), nil
}
func (r *deliveryMemory) Delivery(context.Context, int64) (domain.Delivery, error) {
	return domain.Delivery{}, nil
}
func (r *deliveryMemory) FinishDelivery(ctx context.Context, _ domain.OutboxClaim, status, _ string, _ time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.finished <- status
	return nil
}

type waitingSender struct {
	Sender
	started chan struct{}
}

func (s *waitingSender) Send(ctx context.Context, _ domain.Delivery) integration.Outcome {
	close(s.started)
	<-ctx.Done()
	return integration.Outcome{Status: "RETRY", Retryable: true, Message: ctx.Err().Error()}
}
func TestShutdownRecordsInFlightDeliveryRetry(t *testing.T) {
	r := &deliveryMemory{finished: make(chan string, 1)}
	sender := &waitingSender{started: make(chan struct{})}
	svc := New(r, sender, config.Config{WorkerInterval: time.Hour}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.Run(ctx)
	select {
	case <-sender.started:
	case <-time.After(time.Second):
		t.Fatal("delivery did not start")
	}
	cancel()
	stop, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	if err := svc.Wait(stop); err != nil {
		t.Fatal(err)
	}
	select {
	case status := <-r.finished:
		if status != "RETRY" {
			t.Fatal(status)
		}
	default:
		t.Fatal("shutdown lost delivery result")
	}
}
