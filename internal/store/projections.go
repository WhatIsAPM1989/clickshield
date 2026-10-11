package store

import (
	"context"
	"github.com/lamooof/clickshield/internal/analytics"
)

func (s *Store) ProjectAnalytics(ctx context.Context) (bool, error) {
	return analytics.NewProjector(s.Pool).ProjectOne(ctx)
}
