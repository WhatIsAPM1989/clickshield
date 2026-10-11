package analytics

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

type AnalyticsStep struct {
	Type       string         `json:"type"`
	Value      string         `json:"value"`
	Match      string         `json:"match"`
	Properties map[string]any `json:"properties,omitempty"`
}
type AnalyticsDefinition struct {
	ID            string          `json:"id"`
	ProjectID     string          `json:"project_id"`
	Kind          string          `json:"kind"`
	Name          string          `json:"name"`
	Steps         []AnalyticsStep `json:"steps"`
	WindowMinutes int             `json:"window_minutes"`
}

func (d AnalyticsDefinition) Validate() error {
	if !ValidUUID(d.ProjectID) || strings.TrimSpace(d.Name) == "" || utf8.RuneCountInString(d.Name) > 120 {
		return errors.New("project and name are required (name up to 120 characters)")
	}
	if (d.Kind != "goal" && d.Kind != "funnel") || len(d.Steps) < 1 || len(d.Steps) > 12 || (d.Kind == "goal" && len(d.Steps) != 1) || (d.Kind == "funnel" && len(d.Steps) < 2) {
		return errors.New("goal requires one step; funnel requires 2–12 steps")
	}
	if d.WindowMinutes < 1 || d.WindowMinutes > 10080 {
		return errors.New("window must be 1–10080 minutes")
	}
	for _, s := range d.Steps {
		if s.Type != "page" && s.Type != "event" && s.Type != "outcome" {
			return errors.New("step type must be page, event or outcome")
		}
		if strings.TrimSpace(s.Value) == "" || len(s.Value) > 2048 || (s.Match != "" && s.Match != "exact" && s.Match != "prefix") {
			return errors.New("step value and exact/prefix match required")
		}
		if s.Type == "outcome" && s.Value != "LEAD" && s.Value != "QUALIFIED" && s.Value != "SPAM" && s.Value != "SUBMITTED" && s.Value != "LOST" {
			return errors.New("invalid outcome status")
		}
		if len(s.Properties) > 32 {
			return errors.New("too many property filters")
		}
		for k, v := range s.Properties {
			if len(k) == 0 || len(k) > 80 {
				return errors.New("invalid property key")
			}
			switch x := v.(type) {
			case nil, bool, float64:
			case string:
				if len(x) > 1024 {
					return errors.New("property too long")
				}
			default:
				return errors.New("property filters must be scalar")
			}
		}
	}
	return nil
}

type Activity struct {
	Type, Value string
	Properties  map[string]any
	At          time.Time
}

func matches(step AnalyticsStep, a Activity) bool {
	if step.Type != a.Type {
		return false
	}
	actual := a.Value
	if step.Type == "outcome" && step.Value == "LEAD" && a.Value == "SUBMITTED" {
		actual = "LEAD"
	}
	if step.Type == "page" && strings.HasPrefix(step.Value, "/") {
		if u, e := url.Parse(actual); e == nil {
			actual = u.Path
		}
	}
	if step.Match == "prefix" {
		if !strings.HasPrefix(actual, step.Value) {
			return false
		}
	} else if actual != step.Value {
		return false
	}
	for k, want := range step.Properties {
		got, ok := a.Properties[k]
		if !ok {
			return false
		}
		g, _ := json.Marshal(got)
		w, _ := json.Marshal(want)
		if string(g) != string(w) {
			return false
		}
	}
	return true
}

// Latest reachable timestamps preserve alternative starts when an earlier path expired.
// Reverse iteration prevents one occurrence from completing two identical steps.
func advanceFunnel(steps []AnalyticsStep, latest []time.Time, a Activity, window time.Duration) int {
	reached := 0
	for i := len(steps) - 1; i >= 0; i-- {
		if !matches(steps[i], a) {
			continue
		}
		if i == 0 || (!latest[i-1].IsZero() && !a.At.Before(latest[i-1]) && a.At.Sub(latest[i-1]) <= window) {
			latest[i] = a.At
			if reached < i+1 {
				reached = i + 1
			}
		}
	}
	return reached
}
