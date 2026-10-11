package httpapi

import (
	"github.com/lamooof/clickshield/internal/domain"
	"strings"
	"testing"
	"time"
)

func TestBusinessEventValidation(t *testing.T) {
	makeBatch := func() domain.EventBatch {
		return domain.EventBatch{SessionID: "12345678-1234-1234-1234-123456789abc", VisitorID: "12345678-1234-1234-1234-123456789abc", BatchID: "12345678-1234-1234-1234-123456789abc", PageURL: "https://example.test", Events: []domain.Event{{Type: "business", TS: time.Now().UnixMilli(), Name: "Звонок", Properties: map[string]any{"count": float64(2), "success": true, "optional": nil}}}}
	}
	b := makeBatch()
	if e := validateBatch(&b); e != "" {
		t.Fatal(e)
	}
	for _, name := range []string{"", strings.Repeat("я", 81)} {
		b = makeBatch()
		b.Events[0].Name = name
		if validateBatch(&b) == "" {
			t.Fatal("invalid name accepted")
		}
	}
	b = makeBatch()
	b.Events[0].Properties["nested"] = map[string]any{"x": true}
	if validateBatch(&b) == "" {
		t.Fatal("nested properties accepted")
	}
	b = makeBatch()
	b.Events[0].Properties["long"] = strings.Repeat("x", 1025)
	if validateBatch(&b) == "" {
		t.Fatal("oversize value accepted")
	}
}
