package store

import (
	"context"
	"encoding/json"
)

func (s *Store) BackfillFeatures(ctx context.Context) error {
	for {
		tx, err := s.Pool.Begin(ctx)
		if err != nil {
			return err
		}
		var id string
		err = tx.QueryRow(ctx, `SELECT s.id::text FROM sessions s WHERE s.feature_state IS NULL AND
 (EXISTS(SELECT 1 FROM raw_events r WHERE r.session_id=s.id) OR NOT EXISTS(SELECT 1 FROM session_features f WHERE f.session_id=s.id))
 ORDER BY s.id FOR UPDATE OF s SKIP LOCKED LIMIT 1`).Scan(&id)
		if IsNotFound(err) {
			_ = tx.Rollback(ctx)
			return nil
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		state, err := loadFeatureState(ctx, tx, id)
		if err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		raw, err := json.Marshal(state)
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE sessions SET feature_state=$2 WHERE id=$1`, id, raw)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
}
