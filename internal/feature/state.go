package feature

import (
	"github.com/lamooof/clickshield/internal/domain"
	"time"
)

// State preserves only the data needed by scoring. Coordinates are retained
// to keep exact percentiles and permit batches arriving out of timestamp order.
// DOM, URLs, business properties and other raw payloads are never retained here.
type State struct {
	FirstTS    int64          `json:"first_ts"`
	LastTS     int64          `json:"last_ts"`
	ActiveMS   int64          `json:"active_ms"`
	IdleMS     int64          `json:"idle_ms"`
	Heartbeats int            `json:"heartbeats"`
	Touches    int            `json:"touches"`
	Visibility int            `json:"visibility"`
	Pages      int            `json:"pages"`
	Points     []domain.Event `json:"points,omitempty"`
}

func (s *State) Add(events []domain.Event) {
	for _, e := range events {
		if e.TS > 0 && (s.FirstTS == 0 || e.TS < s.FirstTS) {
			s.FirstTS = e.TS
		}
		if e.TS > s.LastTS {
			s.LastTS = e.TS
		}
		switch e.Type {
		case "heartbeat":
			s.Heartbeats++
			if e.ActiveMS > 0 {
				s.ActiveMS += e.ActiveMS
			}
			if e.IdleMS > 0 {
				s.IdleMS += e.IdleMS
			}
		case "touch":
			s.Touches++
		case "visibility":
			s.Visibility++
		case "pageview":
			if e.PageIndex > s.Pages {
				s.Pages = e.PageIndex
			}
		case "mousemove", "scroll", "click":
			s.Points = append(s.Points, domain.Event{Type: e.Type, TS: e.TS, X: e.X, Y: e.Y, ScrollY: e.ScrollY, Depth: e.Depth})
		}
	}
}

func (s State) Compute(id string, started, last time.Time) domain.Features {
	events := make([]domain.Event, 0, len(s.Points)+3)
	events = append(events, s.Points...)
	events = append(events, domain.Event{Type: "pageview", TS: s.FirstTS, PageIndex: s.Pages}, domain.Event{Type: "heartbeat", TS: s.LastTS, ActiveMS: s.ActiveMS, IdleMS: s.IdleMS})
	f := Compute(id, events, started, last)
	f.HeartbeatEvents, f.TouchEvents, f.VisibilityEvents = s.Heartbeats, s.Touches, s.Visibility
	return f
}
