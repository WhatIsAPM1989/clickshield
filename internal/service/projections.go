package service

import (
	"context"
	"time"
)

func (s *Service) projections(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for i := 0; i < 100 && ctx.Err() == nil; i++ {
				job, cancel := context.WithTimeout(ctx, 20*time.Second)
				ok, err := s.store.ProjectAnalytics(job)
				cancel()
				if err != nil {
					s.log.Error("project analytics", "error", err)
					break
				}
				if !ok {
					break
				}
			}
		}
	}
}
