package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/lamooof/clickshield/internal/bots"
	"github.com/lamooof/clickshield/internal/config"
	"github.com/lamooof/clickshield/internal/domain"
	"github.com/lamooof/clickshield/internal/feature"
	"github.com/lamooof/clickshield/internal/integration"
	"github.com/lamooof/clickshield/internal/scoring"
	"github.com/lamooof/clickshield/internal/store"
)

type Service struct {
	store  *store.Store
	sender *integration.Sender
	cfg    config.Config
	log    *slog.Logger
}

func New(st *store.Store, sender *integration.Sender, cfg config.Config, log *slog.Logger) *Service {
	return &Service{st, sender, cfg, log}
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
	c, e := s.store.SessionContext(ctx, id)
	if e != nil {
		return e
	}
	bot, _ := bots.Identify(c.UserAgent, c.IP)
	c.KnownBot = bot.Declared()
	events, e := s.store.Events(ctx, id)
	if e != nil {
		return e
	}
	f := feature.Compute(id, events, c.StartedAt, c.LastSeenAt)
	f.SessionsPerIP = c.SessionsPerIP
	f.SessionsPerFingerprint = c.SessionsPerFingerprint
	f.PaidVisitFrequency = c.PaidVisitFrequency
	score := scoring.Score(c, f, scoring.Parse(c.ProjectScoringConfig))
	_, e = s.store.SaveAutomatic(ctx, f, score)
	return e
}
func (s *Service) Run(ctx context.Context) {
	go s.finalizer(ctx)
	go s.deliveries(ctx)
	go s.cleanup(ctx)
	go s.monitor(ctx)
}

func (s *Service) monitor(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	lastAlert := time.Time{}
	check := func() {
		state, err := s.store.Operations(ctx)
		if err != nil {
			s.log.Error("monitor operations", "error", err)
			return
		}
		if state.UnscoredOlderThanHour == 0 && state.OutboxPendingOlderHour == 0 && state.OutboxFailed == 0 {
			lastAlert = time.Time{}
			return
		}
		if time.Since(lastAlert) < time.Hour {
			return
		}
		lastAlert = time.Now()
		s.log.Warn("ClickShield operational alert", "unscored_older_than_hour", state.UnscoredOlderThanHour, "outbox_pending_older_hour", state.OutboxPendingOlderHour, "outbox_failed", state.OutboxFailed)
		if s.cfg.AlertWebhookURL == "" {
			return
		}
		body, _ := json.Marshal(state)
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.AlertWebhookURL, bytes.NewReader(body))
		if err != nil {
			s.log.Error("build alert request", "error", err)
			return
		}
		request.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 5 * time.Second}
		response, err := client.Do(request)
		if err != nil {
			s.log.Error("send alert", "error", err)
			return
		}
		response.Body.Close()
		if response.StatusCode >= 300 {
			s.log.Error("alert webhook rejected", "status", response.StatusCode)
		}
	}
	check()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
}
func (s *Service) finalizer(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.WorkerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ids, e := s.store.DueSessions(ctx, s.cfg.FinishGrace, s.cfg.StaleSessionAfter, 50)
			if e != nil {
				s.log.Error("find sessions to finalize", "error", e)
				continue
			}
			for _, id := range ids {
				if e = s.Finalize(ctx, id); e != nil {
					s.log.Error("finalize session", "session_id", id, "error", e)
				}
			}
		}
	}
}
func (s *Service) deliveries(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	_ = s.store.ResetStuckOutbox(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for i := 0; i < 20; i++ {
				id, ok, e := s.store.ClaimOutbox(ctx)
				if e != nil {
					s.log.Error("claim outbox", "error", e)
					break
				}
				if !ok {
					break
				}
				d, e := s.store.Delivery(ctx, id)
				if e != nil {
					_ = s.store.FinishDelivery(ctx, id, "RETRY", e.Error(), time.Minute)
					continue
				}
				out := s.sender.Send(ctx, d)
				retry := time.Duration(0)
				status := out.Status
				if out.Retryable {
					retry = backoff(d.Attempts)
					status = "RETRY"
				}
				if d.Attempts >= 8 && out.Retryable {
					status = "FAILED"
					retry = 0
				}
				if e = s.store.FinishDelivery(ctx, id, status, out.Message, retry); e != nil {
					s.log.Error("finish delivery", "outbox_id", id, "error", e)
				}
			}
		}
	}
}
func (s *Service) cleanup(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if e := s.store.Cleanup(ctx, s.cfg.RawEventsTTL, s.cfg.AggregateTTL); e != nil {
				s.log.Error("cleanup", "error", e)
			}
		}
	}
}
func backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := time.Second * time.Duration(1<<min(attempt, 10))
	if d > time.Hour {
		d = time.Hour
	}
	return d
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
