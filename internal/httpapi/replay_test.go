package httpapi

import (
	"github.com/lamooof/clickshield/internal/domain"
	"github.com/lamooof/clickshield/internal/replay"
	"strings"
	"testing"
	"time"
)

func TestReplayValidation(t *testing.T) {
	id := "12345678-1234-4123-8123-123456789abc"
	now := time.Now().UnixMilli()
	p := replay.Part{SessionID: id, VisitorID: id, PageID: id, PageIndex: 1, PageURL: "https://example.com/?email=secret&yclid=x", PartCount: 1, StartedAt: now, EndedAt: now, Data: []byte("[]")}
	if e := validateReplayPart(&p); e != "" || strings.Contains(p.PageURL, "secret") {
		t.Fatalf("validation=%q URL=%q", e, p.PageURL)
	}
	p.PartCount = 0
	if validateReplayPart(&p) == "" {
		t.Fatal("accepted invalid fragment count")
	}
	p.PartCount = 1
	p.Data = make([]byte, replay.MaxPart+1)
	if validateReplayPart(&p) == "" {
		t.Fatal("accepted oversized fragment")
	}
}
func TestPageviewURLsSanitized(t *testing.T) {
	id := "12345678-1234-4123-8123-123456789abc"
	b := validBatchForReplayTest(id)
	if msg := validateBatch(&b); msg != "" {
		t.Fatal(msg)
	}
	raw, _ := b.Events[0].Extra["page_url"].(string)
	if strings.Contains(raw, "secret") || strings.Contains(raw, "fragment") {
		t.Fatalf("unsafe URL: %s", raw)
	}
}

func validBatchForReplayTest(id string) domain.EventBatch {
	return domain.EventBatch{SessionID: id, VisitorID: id, BatchID: id, Events: []domain.Event{{Type: "pageview", TS: time.Now().UnixMilli(), Extra: map[string]any{"page_url": "https://example.com/a?email=secret&utm_source=yandex#fragment"}}}}
}
