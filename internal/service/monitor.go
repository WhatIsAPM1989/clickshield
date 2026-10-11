package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"
)

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
		if state.UnscoredOlderThanHour == 0 && state.OutboxPendingOlderHour == 0 && state.OutboxFailed == 0 && state.AnalyticsLagSeconds < 300 && state.AnalyticsRetries == 0 && state.ReplayGCFailed == 0 {
			lastAlert = time.Time{}
			return
		}
		if time.Since(lastAlert) < time.Hour {
			return
		}
		lastAlert = time.Now()
		s.log.Warn("ClickShield operational alert", "unscored_older_than_hour", state.UnscoredOlderThanHour, "outbox_pending_older_hour", state.OutboxPendingOlderHour, "outbox_failed", state.OutboxFailed, "analytics_lag_seconds", state.AnalyticsLagSeconds, "analytics_retries", state.AnalyticsRetries, "replay_gc_failed", state.ReplayGCFailed)
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
