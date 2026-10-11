package replay

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"time"
)

var ErrNotFound = errors.New("replay not found")

const MaxPart = 128 << 10
const MaxBytes = 20 << 20

var ErrQuota = errors.New("recording size limit reached")
var ErrConflict = errors.New("conflicting replay fragment")

type Part struct {
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
	b, e := io.ReadAll(io.LimitReader(z, MaxPart+1))
	if len(b) > MaxPart {
		return nil, ErrQuota
	}
	return b, e
}

type Page struct {
	ID         string            `json:"id"`
	Index      int               `json:"index"`
	URL        string            `json:"url"`
	Events     []json.RawMessage `json:"events"`
	Incomplete bool              `json:"incomplete"`
}
type Data struct {
	Pages      []Page     `json:"pages"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	Incomplete bool       `json:"incomplete"`
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
