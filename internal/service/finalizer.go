package service

import (
	"context"
	"errors"
	"github.com/lamooof/clickshield/internal/antifraud"
	"time"
)

func (s *Service) finalizer(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.WorkerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for i := 0; i < 50 && ctx.Err() == nil; i++ {
				claim, ok, err := s.store.ClaimDueScore(ctx, s.cfg.FinishGrace, s.cfg.StaleSessionAfter)
				if err != nil {
					s.log.Error("claim scoring", "error", err)
					break
				}
				if !ok {
					break
				}
				job, cancel := context.WithTimeout(ctx, 30*time.Second)
				err = s.scorer.FinalizeClaimed(job, claim.ID, claim.Lease)
				cancel()
				if err != nil && !errors.Is(err, antifraud.ErrChanged) {
					s.log.Error("finalize session", "session_id", claim.ID, "error", err)
					break
				}
			}
		}
	}
}
