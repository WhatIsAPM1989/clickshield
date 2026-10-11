package store

import (
	"context"
	"time"
)

func (s *Store) Cleanup(ctx context.Context, rawTTL, aggregateTTL time.Duration) error {
	if _, e := s.Pool.Exec(ctx, `DELETE FROM replay_recordings WHERE id IN (SELECT id FROM replay_recordings WHERE expires_at<=now() LIMIT 500)`); e != nil {
		return e
	}
	rawBefore := time.Now().UTC().Add(-rawTTL)
	if e := s.EnsureRawPartitions(ctx); e != nil {
		return e
	}
	if e := s.DropRawPartitions(ctx, rawBefore); e != nil {
		return e
	}
	if aggregateTTL > 0 {
		before := time.Now().UTC().Add(-aggregateTTL)
		if _, e := s.Pool.Exec(ctx, `DELETE FROM analytics.tracking_hits WHERE id IN (SELECT id FROM analytics.tracking_hits WHERE occurred_at<$1 LIMIT 2000)`, before); e != nil {
			return e
		}
		if _, e := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE id IN (SELECT id FROM sessions WHERE last_seen_at<$1 LIMIT 500)`, before); e != nil {
			return e
		}
		if _, e := s.Pool.Exec(ctx, `DELETE FROM visitors WHERE id IN (SELECT v.id FROM visitors v WHERE v.last_seen_at<$1 AND NOT EXISTS (SELECT 1 FROM sessions s WHERE s.visitor_id=v.id) LIMIT 500)`, before); e != nil {
			return e
		}
	}
	receiptBefore := rawBefore
	if aggregateTTL == 0 {
		receiptBefore = time.Time{}
	} else if cutoff := time.Now().UTC().Add(-aggregateTTL); cutoff.Before(receiptBefore) {
		receiptBefore = cutoff
	}
	if _, err := s.Pool.Exec(ctx, `DELETE FROM ingest_batches WHERE (session_id,batch_id) IN (SELECT session_id,batch_id FROM ingest_batches WHERE created_at<$1 LIMIT 2000)`, receiptBefore); err != nil {
		return err
	}
	_, e := s.Pool.Exec(ctx, `DELETE FROM admin_sessions WHERE id IN (SELECT id FROM admin_sessions WHERE expires_at<now() LIMIT 500)`)
	return e
}
