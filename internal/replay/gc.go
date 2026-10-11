package replay

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
)

// GC entries are retained across failures and delayed past the upload deadline.
// A delete racing an upload cannot leave an object behind after collection.
func (r *Repository) CollectOne(ctx context.Context, objects Objects) (bool, error) {
	if objects == nil {
		return false, nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var key string
	err = tx.QueryRow(ctx, `SELECT object_key FROM replay_object_gc WHERE not_before<=now() ORDER BY not_before FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err = objects.Delete(ctx, key); err != nil {
		if _, saveErr := tx.Exec(ctx, `UPDATE replay_object_gc SET attempts=attempts+1,last_error=$2,not_before=now()+interval '1 minute' WHERE object_key=$1`, key, err.Error()); saveErr != nil {
			return false, saveErr
		}
		if saveErr := tx.Commit(ctx); saveErr != nil {
			return false, saveErr
		}
		return false, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM replay_object_gc WHERE object_key=$1`, key); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
