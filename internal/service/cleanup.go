package service

import (
	"context"
	"time"
)

func (s *Service) cleanup(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			job, cancel := context.WithTimeout(ctx, 30*time.Second)
			e := s.store.Cleanup(job, s.cfg.RawEventsTTL, s.cfg.AggregateTTL)
			cancel()
			if e != nil {
				s.log.Error("cleanup", "error", e)
			}
		}
	}
}
