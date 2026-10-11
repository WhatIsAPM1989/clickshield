package feature

import (
	"math"
	"testing"
	"time"

	"github.com/lamooof/clickshield/internal/domain"
)

func TestCompute(t *testing.T) {
	start := time.UnixMilli(1000)
	events := []domain.Event{{Type: "pageview", TS: 1000, PageIndex: 1}, {Type: "heartbeat", TS: 2000, ActiveMS: 800, IdleMS: 200}, {Type: "mousemove", TS: 1100, X: 0, Y: 0}, {Type: "mousemove", TS: 2100, X: 3, Y: 4}, {Type: "click", TS: 2500}, {Type: "click", TS: 3000}, {Type: "scroll", TS: 2000, ScrollY: 0, Depth: .2}, {Type: "scroll", TS: 3000, ScrollY: 500, Depth: .8}}
	f := Compute("s", events, start, start.Add(3*time.Second))
	if math.Abs(f.MouseDistance-5) > .001 {
		t.Fatalf("distance=%v", f.MouseDistance)
	}
	if f.ClickCount != 2 || len(f.ClickIntervalsMS) != 1 || f.ClickIntervalsMS[0] != 500 {
		t.Fatalf("clicks=%+v", f)
	}
	if f.ScrollDepth != .8 {
		t.Fatalf("depth=%v", f.ScrollDepth)
	}
	if f.ActiveTimeMS != 800 {
		t.Fatalf("active=%d", f.ActiveTimeMS)
	}
}

func TestMouseP95IgnoresSingleSpike(t *testing.T) {
	start := time.UnixMilli(1000)
	events := make([]domain.Event, 40)
	for i := range events {
		x := float64(i)
		if i == len(events)-1 {
			x += 500
		}
		events[i] = domain.Event{Type: "mousemove", TS: 1000 + int64(i)*100, X: x}
	}
	f := Compute("s", events, start, start.Add(4*time.Second))
	if f.MouseMaxVelocity <= 3500 || f.MouseP95Velocity >= 3500 {
		t.Fatalf("single spike: max=%v p95=%v", f.MouseMaxVelocity, f.MouseP95Velocity)
	}
}

func TestCountsLifecycleEvents(t *testing.T) {
	start := time.UnixMilli(1000)
	events := []domain.Event{{Type: "pageview", TS: 1000}, {Type: "touch", TS: 1500}, {Type: "heartbeat", TS: 6000, ActiveMS: 5000}, {Type: "visibility", TS: 6100}}
	f := Compute("s", events, start, start.Add(5*time.Second))
	if f.TouchEvents != 1 || f.HeartbeatEvents != 1 || f.VisibilityEvents != 1 {
		t.Fatalf("lifecycle counts %+v", f)
	}
}
