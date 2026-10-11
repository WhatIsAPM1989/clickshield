package store

import "context"

type Operations struct {
	EventsLastHour         int64 `json:"events_last_hour"`
	UnscoredOlderThanHour  int64 `json:"unscored_older_than_hour"`
	OutboxPendingOlderHour int64 `json:"outbox_pending_older_hour"`
	OutboxFailed           int64 `json:"outbox_failed"`
	OutboxSentLastDay      int64 `json:"outbox_sent_last_day"`
}

func (s *Store) Operations(ctx context.Context) (Operations, error) {
	var o Operations
	err := s.Pool.QueryRow(ctx, `SELECT
	(SELECT count(*) FROM raw_events WHERE created_at>now()-interval '1 hour'),
	(SELECT count(*) FROM sessions WHERE finished_at IS NULL AND last_seen_at<now()-interval '1 hour'),
	(SELECT count(*) FROM integration_outbox WHERE status IN ('PENDING','RETRY','SENDING') AND created_at<now()-interval '1 hour'),
	(SELECT count(*) FROM integration_outbox WHERE status='FAILED'),
	(SELECT count(*) FROM integration_outbox WHERE status='SENT' AND sent_at>now()-interval '1 day')`).Scan(
		&o.EventsLastHour, &o.UnscoredOlderThanHour, &o.OutboxPendingOlderHour, &o.OutboxFailed, &o.OutboxSentLastDay)
	return o, err
}
