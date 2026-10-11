package feature

import (
	"math"
	"sort"
	"time"

	"github.com/lamooof/clickshield/internal/domain"
)

type point struct {
	t    int64
	x, y float64
}
type scrollPoint struct {
	t int64
	y float64
}

func Compute(sessionID string, events []domain.Event, started, last time.Time) domain.Features {
	f := domain.Features{SessionID: sessionID, TimeToFirstClickMS: -1, PageCount: 1, ComputedAt: time.Now().UTC()}
	sort.SliceStable(events, func(i, j int) bool { return events[i].TS < events[j].TS })
	duration := last.Sub(started).Milliseconds()
	if duration < 0 {
		duration = 0
	}
	var firstTS int64
	var mice []point
	var scrolls []scrollPoint
	var clicks []int64
	interactions := 0
	for _, e := range events {
		if e.TS > 0 && (firstTS == 0 || e.TS < firstTS) {
			firstTS = e.TS
		}
		switch e.Type {
		case "heartbeat":
			f.HeartbeatEvents++
			if e.ActiveMS > 0 {
				f.ActiveTimeMS += e.ActiveMS
			}
			if e.IdleMS > 0 {
				f.IdleTimeMS += e.IdleMS
			}
		case "mousemove":
			mice = append(mice, point{e.TS, e.X, e.Y})
			interactions++
		case "click":
			clicks = append(clicks, e.TS)
			interactions++
		case "scroll":
			scrolls = append(scrolls, scrollPoint{e.TS, e.ScrollY})
			if e.Depth > f.ScrollDepth {
				f.ScrollDepth = e.Depth
			}
			interactions++
		case "touch":
			f.TouchEvents++
		case "visibility":
			f.VisibilityEvents++
		case "pageview":
			if e.PageIndex > f.PageCount {
				f.PageCount = e.PageIndex
			}
		}
	}
	if len(events) > 1 {
		span := events[len(events)-1].TS - events[0].TS
		if span > duration && span < 7*24*60*60*1000 {
			duration = span
		}
	}
	f.DurationMS = duration
	if f.ActiveTimeMS+f.IdleTimeMS == 0 {
		f.ActiveTimeMS = duration
	} else if total := f.ActiveTimeMS + f.IdleTimeMS; total > duration+5000 {
		scale := float64(duration) / float64(total)
		f.ActiveTimeMS = int64(float64(f.ActiveTimeMS) * scale)
		f.IdleTimeMS = int64(float64(f.IdleTimeMS) * scale)
	}
	if duration > 0 {
		f.ActiveRatio = math.Min(1, float64(f.ActiveTimeMS)/float64(duration))
	}
	denom := float64(f.ActiveTimeMS) / 1000
	if denom < 1 {
		denom = float64(duration) / 1000
	}
	if denom < 1 {
		denom = 1
	}
	f.InteractionDensity = float64(interactions) / denom
	computeMouse(&f, mice)
	computeScroll(&f, scrolls)
	f.ScrollEvents = len(scrolls)
	f.ClickCount = len(clicks)
	if len(clicks) > 0 {
		base := firstTS
		if base == 0 {
			base = started.UnixMilli()
		}
		f.TimeToFirstClickMS = clicks[0] - base
		if f.TimeToFirstClickMS < 0 {
			f.TimeToFirstClickMS = 0
		}
		var sum int64
		for i := 1; i < len(clicks); i++ {
			gap := clicks[i] - clicks[i-1]
			if gap >= 0 && gap < 3600000 {
				f.ClickIntervalsMS = append(f.ClickIntervalsMS, gap)
				sum += gap
			}
		}
		if len(f.ClickIntervalsMS) > 0 {
			f.MeanClickIntervalMS = float64(sum) / float64(len(f.ClickIntervalsMS))
		}
	}
	return f
}

func computeMouse(f *domain.Features, p []point) {
	f.MouseMoves = len(p)
	if len(p) < 2 {
		return
	}
	var velocities []float64
	var accelSum float64
	var accelN int
	var angleSum float64
	var angleN int
	for i := 1; i < len(p); i++ {
		dx, dy := p[i].x-p[i-1].x, p[i].y-p[i-1].y
		dist := math.Hypot(dx, dy)
		dt := float64(p[i].t-p[i-1].t) / 1000
		if dt <= 0 || dt > 10 {
			continue
		}
		f.MouseDistance += dist
		v := dist / dt
		velocities = append(velocities, v)
		if v > f.MouseMaxVelocity {
			f.MouseMaxVelocity = v
		}
		if p[i].t-p[i-1].t > 700 {
			f.MousePauses++
		}
		if len(velocities) > 1 {
			prev := velocities[len(velocities)-2]
			accelSum += math.Abs(v-prev) / dt
			accelN++
		}
		if i >= 2 {
			ax, ay := p[i-1].x-p[i-2].x, p[i-1].y-p[i-2].y
			bx, by := dx, dy
			na, nb := math.Hypot(ax, ay), math.Hypot(bx, by)
			if na > 0 && nb > 0 {
				cos := (ax*bx + ay*by) / (na * nb)
				if cos > 1 {
					cos = 1
				}
				if cos < -1 {
					cos = -1
				}
				angleSum += math.Abs(math.Acos(cos))
				angleN++
			}
		}
	}
	var sum float64
	for _, v := range velocities {
		sum += v
	}
	if len(velocities) > 0 {
		f.MouseVelocity = sum / float64(len(velocities))
		sort.Float64s(velocities)
		index := int(math.Ceil(0.95*float64(len(velocities)))) - 1
		f.MouseP95Velocity = velocities[index]
	}
	if accelN > 0 {
		f.MouseAcceleration = accelSum / float64(accelN)
	}
	if angleN > 0 {
		f.MouseCurvature = angleSum / float64(angleN)
	}
}
func computeScroll(f *domain.Features, p []scrollPoint) {
	if len(p) < 2 {
		return
	}
	var sum float64
	var n int
	lastSign := 0
	for i := 1; i < len(p); i++ {
		dy := p[i].y - p[i-1].y
		dt := float64(p[i].t-p[i-1].t) / 1000
		if dt <= 0 || dt > 10 {
			continue
		}
		sum += math.Abs(dy) / dt
		n++
		sign := 0
		if dy > 1 {
			sign = 1
		} else if dy < -1 {
			sign = -1
		}
		if sign != 0 && lastSign != 0 && sign != lastSign {
			f.ScrollReversals++
		}
		if sign != 0 {
			lastSign = sign
		}
	}
	if n > 0 {
		f.ScrollVelocity = sum / float64(n)
	}
}
