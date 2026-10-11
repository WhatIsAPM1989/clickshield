package analytics

import (
	"context"
	"github.com/lamooof/clickshield/internal/domain"
	"time"
)

func (s *Service) acquire(ctx context.Context) error {
	select {
	case s.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *Service) release() { <-s.slots }

func (s *Service) ReportTimeline(ctx context.Context, kind, interval, timezone string, series []string, totalOnly bool, filter SessionFilter) (Timeline, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := s.acquire(ctx); err != nil {
		return Timeline{}, err
	}
	defer s.release()
	return s.Repository.ReportTimeline(ctx, kind, interval, timezone, series, totalOnly, filter)
}
func (s *Service) TrafficReport(ctx context.Context, filter SessionFilter) ([]TrafficRow, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := s.acquire(ctx); err != nil {
		return nil, err
	}
	defer s.release()
	return s.Repository.TrafficReport(ctx, filter)
}
func (s *Service) FilteredStats(ctx context.Context, filter SessionFilter) (domain.Stats, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := s.acquire(ctx); err != nil {
		return domain.Stats{}, err
	}
	defer s.release()
	return s.Repository.FilteredStats(ctx, filter)
}
