package replay

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool} }

func (s *Repository) Reserve(ctx context.Context, projectID string, chunk Chunk) (bool, error) {
	p := chunk.Part
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return false, e
	}
	defer tx.Rollback(ctx)
	// The session must already exist. Recheck settings inside the transaction so
	// disabling capture takes effect without waiting for the public config cache.
	var owner string
	e = tx.QueryRow(ctx, `SELECT s.id::text FROM sessions s JOIN projects pr ON pr.id=s.project_id WHERE s.id=$1 AND s.project_id=$2 AND s.visitor_key=$3 AND s.replay_deleted_at IS NULL AND pr.replay_enabled AND pr.replay_sample_rate>0 FOR UPDATE OF s`, p.SessionID, projectID, p.VisitorID).Scan(&owner)
	if errors.Is(e, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if e != nil {
		return false, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO replay_recordings(id,project_id,session_id,visitor_key,expires_at) SELECT s.id,s.project_id,s.id,s.visitor_key,now()+make_interval(days=>pr.replay_retention_days) FROM sessions s JOIN projects pr ON pr.id=s.project_id WHERE s.id=$1 AND s.project_id=$2 AND s.visitor_key=$3 AND pr.replay_enabled AND pr.replay_sample_rate>0 ON CONFLICT DO NOTHING`, p.SessionID, projectID, p.VisitorID)
	if e != nil {
		return false, e
	}
	var total int64
	var enabled bool
	var expired bool
	e = tx.QueryRow(ctx, `SELECT rr.total_bytes,pr.replay_enabled,rr.expires_at<=now() FROM replay_recordings rr JOIN projects pr ON pr.id=rr.project_id WHERE rr.id=$1 AND rr.project_id=$2 AND rr.visitor_key=$3 FOR UPDATE OF rr`, p.SessionID, projectID, p.VisitorID).Scan(&total, &enabled, &expired)
	if errors.Is(e, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if e != nil {
		return false, e
	}
	if !enabled || expired {
		return false, ErrNotFound
	}

	var existingDigest string
	var ready, matching bool
	e = tx.QueryRow(ctx, `SELECT checksum,ready,(part_count=$5 AND page_url=$6 AND page_index=$7 AND started_at=$8 AND ended_at=$9) FROM replay_chunks WHERE recording_id=$1 AND page_id=$2 AND sequence=$3 AND part_index=$4`, p.SessionID, p.PageID, p.Sequence, p.PartIndex, p.PartCount, p.PageURL, p.PageIndex, p.StartedAt, p.EndedAt).Scan(&existingDigest, &ready, &matching)
	if e == nil {
		if existingDigest != chunk.Checksum || !matching {
			return false, ErrConflict
		}
		return ready, tx.Commit(ctx)
	}

	if !errors.Is(e, pgx.ErrNoRows) {
		return false, e
	}
	if total+int64(len(p.Data)) > MaxBytes {
		return false, ErrQuota
	}
	var mismatch bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM replay_chunks WHERE recording_id=$1 AND page_id=$2 AND sequence=$3 AND (part_count<>$4 OR page_url<>$5 OR page_index<>$6 OR started_at<>$7 OR ended_at<>$8))`, p.SessionID, p.PageID, p.Sequence, p.PartCount, p.PageURL, p.PageIndex, p.StartedAt, p.EndedAt).Scan(&mismatch)
	if e != nil {
		return false, e
	}
	if mismatch {
		return false, ErrConflict
	}
	_, e = tx.Exec(ctx, `INSERT INTO replay_chunks(recording_id,page_id,sequence,part_index,part_count,page_url,page_index,started_at,ended_at,object_key,checksum,raw_bytes) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, p.SessionID, p.PageID, p.Sequence, p.PartIndex, p.PartCount, p.PageURL, p.PageIndex, p.StartedAt, p.EndedAt, chunk.Key, chunk.Checksum, len(p.Data))
	if e != nil {
		return false, e
	}

	_, e = tx.Exec(ctx, `UPDATE replay_recordings SET total_bytes=total_bytes+$2,last_seen_at=now() WHERE id=$1`, p.SessionID, len(p.Data))
	if e != nil {
		return false, e
	}
	return false, tx.Commit(ctx)
}

func (r *Repository) MarkReady(ctx context.Context, c Chunk) error {
	tag, err := r.pool.Exec(ctx, `UPDATE replay_chunks c SET ready=true FROM replay_recordings rr JOIN sessions s ON s.id=rr.session_id WHERE c.recording_id=rr.id AND c.object_key=$1 AND c.checksum=$2 AND s.replay_deleted_at IS NULL AND rr.expires_at>now()`, c.Key, c.Checksum)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (r *Repository) Load(ctx context.Context, id string) (*time.Time, []Chunk, error) {
	var expires time.Time
	err := r.pool.QueryRow(ctx, `SELECT expires_at FROM replay_recordings WHERE session_id=$1 AND expires_at>now()`, id).Scan(&expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT c.page_id::text,c.page_index,c.page_url,c.sequence,c.part_index,c.part_count,c.object_key,c.checksum,c.ready FROM replay_chunks c JOIN replay_recordings rr ON rr.id=c.recording_id WHERE rr.session_id=$1 AND rr.expires_at>now() ORDER BY c.page_index,c.page_id,c.sequence,c.part_index`, id)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var chunks []Chunk
	for rows.Next() {
		var c Chunk
		if err = rows.Scan(&c.PageID, &c.PageIndex, &c.PageURL, &c.Sequence, &c.PartIndex, &c.PartCount, &c.Key, &c.Checksum, &c.Ready); err != nil {
			return nil, nil, err
		}
		chunks = append(chunks, c)
	}
	return &expires, chunks, rows.Err()
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `WITH mark AS (UPDATE sessions SET replay_deleted_at=now() WHERE id=$1 RETURNING id) DELETE FROM replay_recordings WHERE session_id IN(SELECT id FROM mark)`, id)
	return err
}
