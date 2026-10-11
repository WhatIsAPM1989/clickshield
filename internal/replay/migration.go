package replay

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"time"
)

func (r *Repository) CheckLegacy(ctx context.Context) error {
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT to_regclass('replay_chunks_legacy') IS NOT NULL`).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("legacy replay payloads require clickshield backfill replay before serve/worker")
	}
	return nil
}

func (r *Repository) MigrateLegacy(ctx context.Context, objects Objects) error {
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT to_regclass('replay_chunks_legacy') IS NOT NULL`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if objects == nil {
		return fmt.Errorf("S3 must be configured to migrate replay")
	}
	for {
		var p Part
		var project string
		var payload []byte
		err := r.pool.QueryRow(ctx, `SELECT c.recording_id::text,c.page_id::text,c.sequence,c.part_index,c.part_count,c.page_url,c.page_index,c.started_at,c.ended_at,c.payload,rr.project_id::text FROM replay_chunks_legacy c JOIN replay_recordings rr ON rr.id=c.recording_id ORDER BY c.recording_id,c.page_id,c.sequence,c.part_index LIMIT 1`).Scan(&p.SessionID, &p.PageID, &p.Sequence, &p.PartIndex, &p.PartCount, &p.PageURL, &p.PageIndex, &p.StartedAt, &p.EndedAt, &payload, &project)
		if errors.Is(err, pgx.ErrNoRows) {
			break
		}
		if err != nil {
			return err
		}
		raw, err := decompressReplay(payload)
		if err != nil {
			return err
		}
		digest := checksum(raw)
		key := fmt.Sprintf("replay/%s/%s/%s/%d/%d-%s.gz", project, p.SessionID, p.PageID, p.Sequence, p.PartIndex, digest)
		upload, cancel := context.WithTimeout(ctx, 30*time.Second)
		err = objects.Put(upload, key, payload)
		cancel()
		if err != nil {
			return err
		}
		tx, err := r.pool.Begin(ctx)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `INSERT INTO replay_chunks(recording_id,page_id,sequence,part_index,part_count,page_url,page_index,started_at,ended_at,object_key,checksum,raw_bytes,ready) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,true) ON CONFLICT(recording_id,page_id,sequence,part_index) DO UPDATE SET ready=true WHERE replay_chunks.checksum=EXCLUDED.checksum AND replay_chunks.object_key=EXCLUDED.object_key AND replay_chunks.part_count=EXCLUDED.part_count AND replay_chunks.page_url=EXCLUDED.page_url AND replay_chunks.page_index=EXCLUDED.page_index AND replay_chunks.started_at=EXCLUDED.started_at AND replay_chunks.ended_at=EXCLUDED.ended_at`, p.SessionID, p.PageID, p.Sequence, p.PartIndex, p.PartCount, p.PageURL, p.PageIndex, p.StartedAt, p.EndedAt, key, digest, len(raw))
		if err == nil && tag.RowsAffected() != 1 {
			err = ErrConflict
		}
		if err == nil {
			_, err = tx.Exec(ctx, `DELETE FROM replay_chunks_legacy WHERE recording_id=$1 AND page_id=$2 AND sequence=$3 AND part_index=$4`, p.SessionID, p.PageID, p.Sequence, p.PartIndex)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	_, err := r.pool.Exec(ctx, `DROP TABLE replay_chunks_legacy`)
	return err
}
