package store

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"io"
	"time"
)

const MaxReplayPart = 128 << 10
const MaxReplayBytes = 20 << 20

var ErrReplayQuota = errors.New("recording size limit reached")
var ErrReplayConflict = errors.New("conflicting replay fragment")

type ReplayPart struct {
	SessionID string `json:"session_id"`
	VisitorID string `json:"visitor_id"`
	PageID    string `json:"page_id"`
	PageIndex int    `json:"page_index"`
	PageURL   string `json:"page_url"`
	Sequence  int    `json:"sequence"`
	PartIndex int    `json:"part_index"`
	PartCount int    `json:"part_count"`
	StartedAt int64  `json:"started_at"`
	EndedAt   int64  `json:"ended_at"`
	Data      []byte `json:"data"`
}

func compressReplay(data []byte) ([]byte, error) {
	var b bytes.Buffer
	z := gzip.NewWriter(&b)
	if _, e := z.Write(data); e != nil {
		return nil, e
	}
	if e := z.Close(); e != nil {
		return nil, e
	}
	return b.Bytes(), nil
}
func decompressReplay(data []byte) ([]byte, error) {
	z, e := gzip.NewReader(bytes.NewReader(data))
	if e != nil {
		return nil, e
	}
	defer z.Close()
	b, e := io.ReadAll(io.LimitReader(z, MaxReplayPart+1))
	if len(b) > MaxReplayPart {
		return nil, ErrReplayQuota
	}
	return b, e
}
func (s *Store) SaveReplayPart(ctx context.Context, projectID string, p ReplayPart) error {
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	// The session must already exist. Recheck settings inside the transaction so
	// disabling capture takes effect without waiting for the public config cache.
	var owner string
	e = tx.QueryRow(ctx, `SELECT s.id::text FROM sessions s JOIN projects pr ON pr.id=s.project_id WHERE s.id=$1 AND s.project_id=$2 AND s.visitor_key=$3 AND s.replay_deleted_at IS NULL AND pr.replay_enabled AND pr.replay_sample_rate>0 FOR UPDATE OF s`, p.SessionID, projectID, p.VisitorID).Scan(&owner)
	if errors.Is(e, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO replay_recordings(id,project_id,session_id,visitor_key,expires_at) SELECT s.id,s.project_id,s.id,s.visitor_key,now()+make_interval(days=>pr.replay_retention_days) FROM sessions s JOIN projects pr ON pr.id=s.project_id WHERE s.id=$1 AND s.project_id=$2 AND s.visitor_key=$3 AND pr.replay_enabled AND pr.replay_sample_rate>0 ON CONFLICT DO NOTHING`, p.SessionID, projectID, p.VisitorID)
	if e != nil {
		return e
	}
	var total int64
	var enabled bool
	var expired bool
	e = tx.QueryRow(ctx, `SELECT rr.total_bytes,pr.replay_enabled,rr.expires_at<=now() FROM replay_recordings rr JOIN projects pr ON pr.id=rr.project_id WHERE rr.id=$1 AND rr.project_id=$2 AND rr.visitor_key=$3 FOR UPDATE OF rr`, p.SessionID, projectID, p.VisitorID).Scan(&total, &enabled, &expired)
	if errors.Is(e, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if e != nil {
		return e
	}
	if !enabled || expired {
		return ErrNotFound
	}
	var existing []byte
	var count int
	e = tx.QueryRow(ctx, `SELECT payload,part_count FROM replay_chunks WHERE recording_id=$1 AND page_id=$2 AND sequence=$3 AND part_index=$4`, p.SessionID, p.PageID, p.Sequence, p.PartIndex).Scan(&existing, &count)
	if e == nil {
		raw, e := decompressReplay(existing)
		if e != nil {
			return e
		}
		if count != p.PartCount || !bytes.Equal(raw, p.Data) {
			return ErrReplayConflict
		}
		return tx.Commit(ctx)
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return e
	}
	if total+int64(len(p.Data)) > MaxReplayBytes {
		return ErrReplayQuota
	}
	var mismatch bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM replay_chunks WHERE recording_id=$1 AND page_id=$2 AND sequence=$3 AND (part_count<>$4 OR page_url<>$5 OR page_index<>$6 OR started_at<>$7 OR ended_at<>$8))`, p.SessionID, p.PageID, p.Sequence, p.PartCount, p.PageURL, p.PageIndex, p.StartedAt, p.EndedAt).Scan(&mismatch)
	if e != nil {
		return e
	}
	if mismatch {
		return ErrReplayConflict
	}
	compressed, e := compressReplay(p.Data)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO replay_chunks(recording_id,page_id,sequence,part_index,part_count,page_url,page_index,started_at,ended_at,payload,event_count) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,0)`, p.SessionID, p.PageID, p.Sequence, p.PartIndex, p.PartCount, p.PageURL, p.PageIndex, p.StartedAt, p.EndedAt, compressed)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE replay_recordings SET total_bytes=total_bytes+$2,last_seen_at=now() WHERE id=$1`, p.SessionID, len(p.Data))
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}

type ReplayPage struct {
	ID         string            `json:"id"`
	Index      int               `json:"index"`
	URL        string            `json:"url"`
	Events     []json.RawMessage `json:"events"`
	Incomplete bool              `json:"incomplete"`
}
type ReplayData struct {
	Pages      []ReplayPage `json:"pages"`
	ExpiresAt  *time.Time   `json:"expires_at,omitempty"`
	Incomplete bool         `json:"incomplete"`
}

func validReplayEvents(raw []byte) ([]json.RawMessage, error) {
	var events []json.RawMessage
	if e := json.Unmarshal(raw, &events); e != nil {
		return nil, e
	}
	if len(events) == 0 || len(events) > 20000 {
		return nil, errors.New("invalid event count")
	}
	for _, raw := range events {
		var event struct {
			Type      *int  `json:"type"`
			Timestamp int64 `json:"timestamp"`
		}
		if e := json.Unmarshal(raw, &event); e != nil {
			return nil, e
		}
		if event.Type == nil || *event.Type < 0 || *event.Type > 6 || event.Timestamp <= 0 {
			return nil, errors.New("invalid replay event")
		}
	}
	return events, nil
}
func (s *Store) Replay(ctx context.Context, id string) (ReplayData, error) {
	result := ReplayData{Pages: []ReplayPage{}}
	e := s.Pool.QueryRow(ctx, `SELECT expires_at FROM replay_recordings WHERE session_id=$1 AND expires_at>now()`, id).Scan(&result.ExpiresAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return result, nil
	}
	if e != nil {
		return result, e
	}
	rows, e := s.Pool.Query(ctx, `SELECT c.page_id::text,c.page_index,c.page_url,c.sequence,c.part_index,c.part_count,c.payload FROM replay_chunks c JOIN replay_recordings rr ON rr.id=c.recording_id WHERE rr.session_id=$1 AND rr.expires_at>now() ORDER BY c.page_index,c.page_id,c.sequence,c.part_index`, id)
	if e != nil {
		return result, e
	}
	defer rows.Close()
	var page *ReplayPage
	var group bytes.Buffer
	lastSeq := -1
	parts := 0
	expected := 0
	nextPart := 0
	finish := func() {
		if page == nil || lastSeq < 0 {
			return
		}
		if parts != expected || nextPart != expected {
			page.Incomplete = true
			return
		}
		events, e := validReplayEvents(group.Bytes())
		if e != nil {
			page.Incomplete = true
			return
		}
		page.Events = append(page.Events, events...)
	}
	for rows.Next() {
		var pid, url string
		var index, seq, part, count int
		var payload []byte
		if e = rows.Scan(&pid, &index, &url, &seq, &part, &count, &payload); e != nil {
			return result, e
		}
		if page == nil || page.ID != pid {
			finish()
			result.Pages = append(result.Pages, ReplayPage{ID: pid, Index: index, URL: url, Events: []json.RawMessage{}})
			page = &result.Pages[len(result.Pages)-1]
			lastSeq = -1
		}
		if seq != lastSeq {
			finish()
			if seq != lastSeq+1 {
				page.Incomplete = true
			}
			lastSeq = seq
			group.Reset()
			parts = 0
			expected = count
			nextPart = 0
		}
		raw, e := decompressReplay(payload)
		if e != nil {
			return result, e
		}
		if part != nextPart || count != expected {
			page.Incomplete = true
		}
		nextPart = part + 1
		parts++
		group.Write(raw)
	}
	if e = rows.Err(); e != nil {
		return result, e
	}
	finish()
	for _, p := range result.Pages {
		if p.Incomplete {
			result.Incomplete = true
		}
	}
	return result, nil
}
