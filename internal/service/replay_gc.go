package service

import (
	"context"
	"github.com/lamooof/clickshield/internal/replay"
	"time"
)

type WorkerOptions struct {
	Replay  *replay.Repository
	Objects replay.Objects
}

func (s *Service) replayGC(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		for i := 0; i < 100 && ctx.Err() == nil; i++ {
			job, cancel := context.WithTimeout(ctx, 30*time.Second)
			ok, err := s.options.Replay.CollectOne(job, s.options.Objects)
			cancel()
			if err != nil {
				s.log.Error("collect replay object", "error", err)
				break
			}
			if !ok {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
