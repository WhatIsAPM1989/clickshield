package store

import (
	"context"
	"github.com/lamooof/clickshield/internal/analytics"
	"testing"
)

type AnalyticsDefinition = analytics.AnalyticsDefinition
type AnalyticsStep = analytics.AnalyticsStep
type ReportRow = analytics.ReportRow

func projectedReports(t *testing.T, st *Store) *analytics.Repository {
	t.Helper()
	projector := analytics.NewProjector(st.Pool)
	for i := 0; i < 10000; i++ {
		ok, err := projector.ProjectOne(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			return analytics.NewRepository(st.Pool)
		}
	}
	t.Fatal("projection queue did not drain")
	return nil
}
