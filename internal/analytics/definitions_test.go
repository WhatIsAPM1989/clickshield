package analytics

import (
	"testing"
	"time"
)

func TestFunnelAlternativeStartsAndRepeatedSteps(t *testing.T) {
	steps := []AnalyticsStep{{Type: "event", Value: "start"}, {Type: "event", Value: "finish"}}
	latest := make([]time.Time, 2)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, a := range []Activity{{Type: "event", Value: "start", At: base}, {Type: "event", Value: "start", At: base.Add(59 * time.Minute)}} {
		advanceFunnel(steps, latest, a, time.Hour)
	}
	if got := advanceFunnel(steps, latest, Activity{Type: "event", Value: "finish", At: base.Add(100 * time.Minute)}, time.Hour); got != 2 {
		t.Fatalf("later valid start ignored: %d", got)
	}
	steps = []AnalyticsStep{{Type: "event", Value: "same"}, {Type: "event", Value: "same"}}
	latest = make([]time.Time, 2)
	a := Activity{Type: "event", Value: "same", At: base}
	if n := advanceFunnel(steps, latest, a, time.Hour); n != 1 {
		t.Fatalf("one event completed repeated steps: %d", n)
	}
	if n := advanceFunnel(steps, latest, a, time.Hour); n != 2 {
		t.Fatalf("second occurrence failed: %d", n)
	}
}
func TestFunnelPageAndPropertyMatching(t *testing.T) {
	step := AnalyticsStep{Type: "page", Value: "/pricing", Match: "prefix"}
	if !matches(step, Activity{Type: "page", Value: "https://example.test/pricing/annual"}) {
		t.Fatal("relative path failed")
	}
	step = AnalyticsStep{Type: "event", Value: "signup", Properties: map[string]any{"plan": "pro", "paid": true}}
	if !matches(step, Activity{Type: "event", Value: "signup", Properties: map[string]any{"plan": "pro", "paid": true}}) {
		t.Fatal("property match failed")
	}
	if matches(step, Activity{Type: "event", Value: "signup", Properties: map[string]any{"plan": "pro", "paid": "true"}}) {
		t.Fatal("property type lost")
	}
	if matches(AnalyticsStep{Type: "event", Value: "signup", Properties: map[string]any{"missing": nil}}, Activity{Type: "event", Value: "signup"}) {
		t.Fatal("absent property matched null")
	}
}

func TestLeadSubmissionDoesNotCountStatusUpdateTwice(t *testing.T) {
	steps := []AnalyticsStep{{Type: "outcome", Value: "LEAD"}, {Type: "outcome", Value: "LEAD"}}
	latest := make([]time.Time, 2)
	at := time.Now()
	if n := advanceFunnel(steps, latest, Activity{Type: "outcome", Value: "SUBMITTED", At: at}, time.Hour); n != 1 {
		t.Fatal(n)
	}
	if n := advanceFunnel(steps, latest, Activity{Type: "outcome", Value: "QUALIFIED", At: at.Add(time.Minute)}, time.Hour); n != 0 {
		t.Fatalf("qualification created a second submission: %d", n)
	}
}
