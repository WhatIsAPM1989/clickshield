package service

import (
	"context"
	"github.com/lamooof/clickshield/internal/domain"
	"time"
)

func (s *Service) deliveries(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	s.deliveryTick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.deliveryTick(ctx)
		}
	}
}

func (s *Service) deliveryTick(ctx context.Context) {
	if err := s.store.ResetStuckOutbox(ctx); err != nil {
		s.log.Error("restore outbox", "error", err)
		return
	}
	for i := 0; i < 20 && ctx.Err() == nil; i++ {
		job, cancel := context.WithTimeout(ctx, 30*time.Second)
		claim, ok, err := s.store.ClaimOutbox(job)
		if err != nil {
			cancel()
			s.log.Error("claim outbox", "error", err)
			break
		}
		if !ok {
			cancel()
			break
		}
		d, err := s.store.Delivery(job, claim.ID)
		if err != nil {
			cancel()
			s.finishDelivery(claim, "RETRY", err.Error(), time.Minute)
			continue
		}
		out := s.sender.Send(job, d)
		cancel()
		status, retry := out.Status, time.Duration(0)
		if out.Retryable {
			status, retry = "RETRY", backoff(claim.Attempts)
		}
		s.finishDelivery(claim, status, out.Message, retry)
	}
}

func (s *Service) finishDelivery(claim domain.OutboxClaim, status, message string, retry time.Duration) {
	if status == "RETRY" && claim.Attempts >= 8 {
		status, retry = "FAILED", 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.store.FinishDelivery(ctx, claim, status, message, retry); err != nil {
		s.log.Error("finish delivery", "outbox_id", claim.ID, "error", err)
	}
}

func backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	return time.Second * time.Duration(1<<min(attempt, 10))
}
