package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lamooof/clickshield/internal/antifraud"
	"github.com/lamooof/clickshield/internal/domain"
	"github.com/lamooof/clickshield/internal/feature"
)

type ScoreClaim = antifraud.ScoreClaim

func (s *Store) ScoreHistory(ctx context.Context, id int64) (antifraud.ScoreWrite, error) {
	var w antifraud.ScoreWrite
	var contextJSON, featuresJSON []byte
	err := s.Pool.QueryRow(ctx, `SELECT algorithm_version,COALESCE(input_revision,0),config_snapshot,context_snapshot,features_snapshot FROM classifications WHERE id=$1`, id).Scan(&w.Version, &w.Input.Revision, &w.Config, &contextJSON, &featuresJSON)
	if err != nil {
		return w, err
	}
	if len(featuresJSON) == 0 || len(contextJSON) == 0 {
		return w, fmt.Errorf("classification %d has no saved scoring inputs (%s)", id, w.Version)
	}
	if err = json.Unmarshal(contextJSON, &w.Input.Context); err != nil {
		return w, err
	}
	if err = json.Unmarshal(featuresJSON, &w.Features); err != nil {
		return w, err
	}
	return w, nil
}

func (s *Store) ClaimDueScore(ctx context.Context, grace, stale time.Duration) (ScoreClaim, bool, error) {
	lease, err := UUID()
	if err != nil {
		return ScoreClaim{}, false, err
	}
	claim := ScoreClaim{Lease: lease}
	err = s.Pool.QueryRow(ctx, `WITH due AS (
 SELECT id FROM sessions WHERE score_next_attempt_at<=now() AND (scoring_lease_until IS NULL OR scoring_lease_until<=now()) AND
 ((finish_requested_at IS NOT NULL AND finish_requested_at<now()-$1::interval AND last_seen_at<now()-$1::interval)
 OR (finished_at IS NULL AND last_seen_at<now()-$2::interval)
 OR (current_classification='' AND started_at<now()-$2::interval))
 ORDER BY COALESCE(finish_requested_at,last_seen_at),id FOR UPDATE SKIP LOCKED LIMIT 1
 ) UPDATE sessions s SET scoring_lease=$3,scoring_lease_until=now()+interval '2 minutes' FROM due WHERE s.id=due.id RETURNING s.id::text`, grace.String(), stale.String(), lease).Scan(&claim.ID)
	if IsNotFound(err) {
		return ScoreClaim{}, false, nil
	}
	return claim, err == nil, err
}

func (s *Store) ClaimScore(ctx context.Context, id string) (string, error) {
	lease, err := UUID()
	if err != nil {
		return "", err
	}
	tag, err := s.Pool.Exec(ctx, `UPDATE sessions SET scoring_lease=$2,scoring_lease_until=now()+interval '2 minutes' WHERE id=$1 AND (scoring_lease_until IS NULL OR scoring_lease_until<=now())`, id, lease)
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() == 0 {
		return "", antifraud.ErrBusy
	}
	return lease, nil
}

func (s *Store) ReleaseScore(ctx context.Context, id, lease string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE sessions SET scoring_lease=NULL,scoring_lease_until=NULL,score_next_attempt_at=now()+interval '30 seconds' WHERE id=$1 AND scoring_lease=$2`, id, lease)
	return err
}

func (s *Store) LoadScore(ctx context.Context, id, lease string) (antifraud.ScoreInput, error) {
	input := antifraud.ScoreInput{Lease: lease}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return input, err
	}
	defer tx.Rollback(ctx)
	var stateRaw []byte
	err = tx.QueryRow(ctx, `SELECT data_revision,feature_state FROM sessions WHERE id=$1 AND scoring_lease=$2 AND scoring_lease_until>now()`, id, lease).Scan(&input.Revision, &stateRaw)
	if IsNotFound(err) {
		return input, antifraud.ErrChanged
	}
	if err != nil {
		return input, err
	}
	input.Context, err = readSessionContext(ctx, tx, id)
	if err != nil {
		return input, err
	}
	if stateRaw != nil {
		var state feature.State
		if err = json.Unmarshal(stateRaw, &state); err != nil {
			return input, err
		}
		input.State = &state
		return input, tx.Commit(ctx)
	}
	rows, err := tx.Query(ctx, `SELECT payload FROM raw_events WHERE session_id=$1 ORDER BY event_time,id`, id)
	if err != nil {
		return input, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var event domain.Event
		if err = rows.Scan(&raw); err != nil {
			return input, err
		}
		if err = json.Unmarshal(raw, &event); err != nil {
			return input, err
		}
		input.Events = append(input.Events, event)
	}
	if err = rows.Err(); err != nil {
		return input, err
	}
	rows.Close()
	if len(input.Events) == 0 {
		var scored bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM session_features WHERE session_id=$1)`, id).Scan(&scored); err != nil {
			return input, err
		}
		if scored {
			return input, fmt.Errorf("legacy session %s has no retained raw events; exact feature backfill is unavailable", id)
		}
	}
	var state feature.State
	state.Add(input.Events)
	input.State = &state
	input.Events = nil
	return input, tx.Commit(ctx)
}
