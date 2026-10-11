package store

import (
	"context"
	"github.com/lamooof/clickshield/internal/domain"
)

type Operations = domain.Operations

func (s *Store) Operations(ctx context.Context) (Operations, error) {
	var o Operations
	err := s.Pool.QueryRow(ctx, `SELECT
	(SELECT count(*) FROM raw_events WHERE created_at>now()-interval '1 hour'),
	(SELECT count(*) FROM sessions WHERE finished_at IS NULL AND last_seen_at<now()-interval '1 hour'),
	(SELECT count(*) FROM integration_outbox WHERE status IN ('PENDING','RETRY','SENDING') AND created_at<now()-interval '1 hour'),
	(SELECT count(*) FROM integration_outbox WHERE status='FAILED'),
	(SELECT count(*) FROM integration_outbox WHERE status='SENT' AND sent_at>now()-interval '1 day'),
	(SELECT count(*) FROM analytics_projection_outbox),
	(SELECT COALESCE(extract(epoch FROM now()-min(created_at))::bigint,0) FROM analytics_projection_outbox),
	(SELECT count(*) FROM analytics_projection_outbox WHERE attempts>0),
	(SELECT count(*) FROM replay_object_gc WHERE attempts>0)`).Scan(
		&o.EventsLastHour, &o.UnscoredOlderThanHour, &o.OutboxPendingOlderHour, &o.OutboxFailed, &o.OutboxSentLastDay, &o.AnalyticsPending, &o.AnalyticsLagSeconds, &o.AnalyticsRetries, &o.ReplayGCFailed)
	return o, err
}
