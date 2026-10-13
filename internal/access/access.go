// Package access defines API token scopes. A write scope implies the read
// scope of the same area.
package access

import (
	"errors"
	"slices"
	"strings"
)

const (
	AnalyticsRead  = "analytics:read"
	AnalyticsWrite = "analytics:write"
	SessionsRead   = "sessions:read"
	SessionsWrite  = "sessions:write"
	ReplayRead     = "replay:read"
	ReplayWrite    = "replay:write"
	ProjectsRead   = "projects:read"
	ProjectsWrite  = "projects:write"
)

// All lists every scope in display order.
var All = []string{AnalyticsRead, AnalyticsWrite, SessionsRead, SessionsWrite, ReplayRead, ReplayWrite, ProjectsRead, ProjectsWrite}

// Has reports whether granted scopes allow need.
func Has(granted []string, need string) bool {
	if slices.Contains(granted, need) {
		return true
	}
	area, ok := strings.CutSuffix(need, ":read")
	return ok && slices.Contains(granted, area+":write")
}

// Normalize validates scopes and returns them deduplicated in display order.
func Normalize(scopes []string) ([]string, error) {
	out := []string{}
	for _, s := range All {
		if slices.Contains(scopes, s) {
			out = append(out, s)
		}
	}
	for _, s := range scopes {
		if !slices.Contains(All, s) {
			return nil, errors.New("unknown scope: " + s)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("at least one scope is required")
	}
	return out, nil
}
