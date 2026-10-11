package feature

import (
	"github.com/lamooof/clickshield/internal/domain"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

func TestStateMatchesRawIncludingLateEvents(t *testing.T) {
	start := time.Date(2026, 10, 11, 10, 0, 0, 0, time.UTC)
	rng := rand.New(rand.NewSource(42))
	events := []domain.Event{{Type: "business", TS: start.UnixMilli(), Properties: map[string]any{"private": "must not be retained"}}}
	kinds := []string{"mousemove", "scroll", "click", "heartbeat", "touch", "visibility", "pageview", "business"}
	for i := 0; i < 400; i++ {
		events = append(events, domain.Event{Type: kinds[i%len(kinds)], TS: start.UnixMilli() + int64(i+1)*100, X: rng.Float64() * 900, Y: rng.Float64() * 700, ScrollY: rng.Float64() * 2000, Depth: rng.Float64() * 100, PageIndex: i%4 + 1, ActiveMS: 50, IdleMS: 30})
	}
	var state State
	// A later batch arrives before an older one.
	state.Add(events[200:])
	state.Add(events[:200])
	want := Compute("s", append([]domain.Event(nil), events...), start, start.Add(time.Minute))
	got := state.Compute("s", start, start.Add(time.Minute))
	want.ComputedAt, got.ComputedAt = time.Time{}, time.Time{}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("state differs:\nwant %+v\ngot  %+v", want, got)
	}
	for _, point := range state.Points {
		if point.Properties != nil || point.PageURL != "" || point.Extra != nil {
			t.Fatal("raw payload retained")
		}
	}
}
