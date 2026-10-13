package access

import (
	"slices"
	"testing"
)

func TestHas(t *testing.T) {
	granted := []string{AnalyticsWrite, SessionsRead}
	for need, want := range map[string]bool{
		AnalyticsRead:  true,
		AnalyticsWrite: true,
		SessionsRead:   true,
		SessionsWrite:  false,
		ReplayRead:     false,
		ProjectsRead:   false,
	} {
		if got := Has(granted, need); got != want {
			t.Errorf("Has(%q) = %v, want %v", need, got, want)
		}
	}
}

func TestNormalize(t *testing.T) {
	got, err := Normalize([]string{ProjectsRead, AnalyticsRead, ProjectsRead})
	if err != nil || !slices.Equal(got, []string{AnalyticsRead, ProjectsRead}) {
		t.Fatalf("Normalize = %v, %v", got, err)
	}
	for _, bad := range [][]string{nil, {}, {"admin"}, {AnalyticsRead, "users:write"}} {
		if _, err := Normalize(bad); err == nil {
			t.Errorf("Normalize(%v) accepted", bad)
		}
	}
}
