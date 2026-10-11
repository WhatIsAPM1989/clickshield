package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestIPFromHTTPUsesOnlyProxyReplacedHeader(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/events", nil)
	r.RemoteAddr = "172.18.0.1:40123"
	r.Header.Set("CF-Connecting-IP", "203.0.113.1")
	r.Header.Set("X-Forwarded-For", "203.0.113.2")
	r.Header.Set("X-Real-IP", "198.51.100.3")
	if got := IPFromHTTP(r, true).String(); got != "198.51.100.3" {
		t.Fatalf("trusted proxy IP = %s", got)
	}
	if got := IPFromHTTP(r, false).String(); got != "172.18.0.1" {
		t.Fatalf("untrusted proxy IP = %s", got)
	}
	r.Header.Del("X-Real-IP")
	if got := IPFromHTTP(r, true).String(); got != "172.18.0.1" {
		t.Fatalf("fallback IP = %s", got)
	}
}
