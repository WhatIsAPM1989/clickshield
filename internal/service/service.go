package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/lamooof/clickshield/internal/antifraud"
	"github.com/lamooof/clickshield/internal/config"
	"github.com/lamooof/clickshield/internal/domain"
	"github.com/lamooof/clickshield/internal/integration"
	"sync"
)

type Service struct {
	store   Repository
	sender  Sender
	cfg     config.Config
	log     *slog.Logger
	scorer  *antifraud.Scorer
	workers sync.WaitGroup
	options WorkerOptions
}

func New(st Repository, sender Sender, cfg config.Config, log *slog.Logger, options ...WorkerOptions) *Service {
	s := &Service{store: st, sender: sender, cfg: cfg, log: log, scorer: antifraud.NewScorer(st)}
	if len(options) > 0 {
		s.options = options[0]
	}
	return s
}
func (s *Service) TestIntegration(ctx context.Context, item domain.Integration, sessionID string) (integration.Outcome, error) {
	session, err := s.store.SessionContext(ctx, sessionID)
	if err != nil {
		return integration.Outcome{}, err
	}
	if session.ProjectID != item.ProjectID {
		return integration.Outcome{}, errors.New("session belongs to another project")
	}
	d := domain.Delivery{Integration: item, Session: session, Classification: domain.Classification{RiskScore: 0, Classification: domain.Human, CreatedAt: time.Now()}}
	return s.sender.Test(ctx, d), nil
}
func (s *Service) Finalize(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return s.scorer.Finalize(ctx, id)
}

func (s *Service) Run(ctx context.Context) {
	runs := []func(context.Context){s.finalizer, s.deliveries, s.cleanup, s.monitor, s.projections}
	if s.options.Replay != nil {
		runs = append(runs, s.replayGC)
	}
	for _, run := range runs {
		s.workers.Add(1)
		go func() { defer s.workers.Done(); run(ctx) }()
	}
}

func (s *Service) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() { s.workers.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) Ingest(ctx context.Context, p domain.Project, b domain.EventBatch, r domain.RemoteMeta) (antifraud.IngestResult, error) {
	return antifraud.NewIngestor(s.store).Ingest(ctx, p, b, r)
}

func (s *Service) Label(ctx context.Context, id, label, note, user string) (int64, error) {
	return antifraud.Label(ctx, s.store, id, label, note, user)
}
