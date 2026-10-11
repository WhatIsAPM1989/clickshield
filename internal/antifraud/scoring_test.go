package antifraud

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lamooof/clickshield/internal/domain"
)

type scoreMemory struct {
	input             ScoreInput
	write             ScoreWrite
	claimErr, saveErr error
	released          bool
}

func (r *scoreMemory) ClaimScore(context.Context, string) (string, error) { return "lease", r.claimErr }
func (r *scoreMemory) LoadScore(context.Context, string, string) (ScoreInput, error) {
	return r.input, nil
}
func (r *scoreMemory) SaveScore(_ context.Context, w ScoreWrite) (int64, error) {
	r.write = w
	return 1, r.saveErr
}
func (r *scoreMemory) ReleaseScore(context.Context, string, string) error {
	r.released = true
	return nil
}

func TestFinalizeProvenanceAndChangedInput(t *testing.T) {
	start := time.Date(2026, 10, 11, 10, 0, 0, 0, time.UTC)
	r := &scoreMemory{input: ScoreInput{Revision: 7, Lease: "lease", Context: domain.SessionContext{ID: "s", StartedAt: start, LastSeenAt: start.Add(time.Minute), ProjectMode: domain.ModeNotify}, Events: []domain.Event{{Type: "pageview", TS: start.UnixMilli(), PageIndex: 1}}}}
	if err := NewScorer(r).Finalize(context.Background(), "s"); err != nil {
		t.Fatal(err)
	}
	if !r.released || r.write.Version != AlgorithmVersion || r.write.Input.Revision != 7 || len(r.write.Config) == 0 || r.write.Features.DurationMS != 60000 {
		t.Fatalf("incomplete provenance: %+v", r)
	}
	r.saveErr = ErrChanged
	r.released = false
	if err := NewScorer(r).Finalize(context.Background(), "s"); !errors.Is(err, ErrChanged) || !r.released {
		t.Fatalf("stale score: %v; released=%v", err, r.released)
	}
	r.claimErr = ErrBusy
	r.released = false
	if err := NewScorer(r).Finalize(context.Background(), "s"); !errors.Is(err, ErrBusy) || r.released {
		t.Fatalf("busy score: %v; released=%v", err, r.released)
	}
}

func TestNotificationPolicy(t *testing.T) {
	for _, tc := range []struct{ mode, label, class, source, want string }{
		{domain.ModeMonitoring, "", domain.Bot, "AUTO", ""},
		{domain.ModeNotify, "", domain.Bot, "AUTO", "antifraud_bot"},
		{domain.ModeBlockingBots, "", domain.KnownBot, "AUTO", "antifraud_bot"},
		{domain.ModeNotify, domain.Human, domain.Bot, "AUTO", ""},
		{domain.ModeNotify, domain.Bot, domain.Human, "MANUAL", "antifraud_human"},
		{domain.ModeNotify, "", domain.Suspicious, "AUTO", ""},
	} {
		if got := Notification(tc.mode, tc.label, tc.class, tc.source); got != tc.want {
			t.Errorf("%+v: %q", tc, got)
		}
	}
}
