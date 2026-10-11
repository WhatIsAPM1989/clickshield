package antifraud

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/lamooof/clickshield/internal/bots"
	"github.com/lamooof/clickshield/internal/domain"
	"github.com/lamooof/clickshield/internal/feature"
	"github.com/lamooof/clickshield/internal/scoring"
)

const AlgorithmVersion = "rules-v1"

type ScoreClaim struct{ ID, Lease string }

var (
	ErrChanged = errors.New("scoring input changed")
	ErrBusy    = errors.New("session is already being scored")
)

type ScoreInput struct {
	Context  domain.SessionContext
	Events   []domain.Event
	State    *feature.State
	Revision int64
	Lease    string
}

type ScoreWrite struct {
	Input     ScoreInput
	Features  domain.Features
	Score     domain.Score
	Config    json.RawMessage
	Version   string
	EventName string
}

type ScoreRepository interface {
	ClaimScore(context.Context, string) (string, error)
	LoadScore(context.Context, string, string) (ScoreInput, error)
	SaveScore(context.Context, ScoreWrite) (int64, error)
	ReleaseScore(context.Context, string, string) error
}

type Scorer struct{ repository ScoreRepository }

func NewScorer(repository ScoreRepository) *Scorer { return &Scorer{repository} }

func Notification(mode, label, classification, source string) string {
	if mode == domain.ModeMonitoring || (source == "AUTO" && label != "") {
		return ""
	}
	switch classification {
	case domain.Bot, domain.KnownBot:
		return "antifraud_bot"
	case domain.Human:
		return "antifraud_human"
	default:
		return ""
	}
}

func (s *Scorer) Finalize(ctx context.Context, id string) error {
	lease, err := s.repository.ClaimScore(ctx, id)
	if err != nil {
		return err
	}
	return s.FinalizeClaimed(ctx, id, lease)
}

func (s *Scorer) FinalizeClaimed(ctx context.Context, id, lease string) error {
	defer func() {
		release, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.repository.ReleaseScore(release, id, lease)
	}()
	input, err := s.repository.LoadScore(ctx, id, lease)
	if err != nil {
		return err
	}
	c := input.Context
	bot, _ := bots.Identify(c.UserAgent, c.IP)
	c.KnownBot = bot.Declared()
	input.Context = c
	var f domain.Features
	if input.State != nil {
		f = input.State.Compute(id, c.StartedAt, c.LastSeenAt)
	} else {
		f = feature.Compute(id, input.Events, c.StartedAt, c.LastSeenAt)
	}
	f.SessionsPerIP, f.SessionsPerFingerprint, f.PaidVisitFrequency = c.SessionsPerIP, c.SessionsPerFingerprint, c.PaidVisitFrequency
	cfg := scoring.Parse(c.ProjectScoringConfig)
	score := scoring.Score(c, f, cfg)
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = s.repository.SaveScore(ctx, ScoreWrite{Input: input, Features: f, Score: score, Config: raw, Version: AlgorithmVersion, EventName: Notification(c.ProjectMode, c.ManualLabel, score.Classification, "AUTO")})
	return err
}
