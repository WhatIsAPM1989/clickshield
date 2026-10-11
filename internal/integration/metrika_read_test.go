package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetrikaReporting(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "OAuth secret-test" {
			t.Error("missing OAuth header")
		}
		q := r.URL.Query()
		if q.Get("ids") != "123" || q.Get("offset") != "501" || q.Get("accuracy") != "full" || q.Get("dimensions") != "ym:s:lastDirectPlatformType,ym:s:lastDirectPlatform" || q.Get("filters") != "ym:s:lastAdvEngine=='ya_direct'" {
			t.Errorf("incorrect query: %v", q)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"dimensions":[{"id":"network","name":"РСЯ"},{"id":"site","name":"example.com"}],"metrics":[2,1,50,30]}],"total_rows":501,"sampled":false,"totals":[2,1,50,30]}`))
	}))
	defer server.Close()
	reader := NewMetrikaReader()
	reader.Endpoint = server.URL
	result, e := reader.Report(context.Background(), "secret-test", "123", "platforms", "2026-10-01", "2026-10-11", 501)
	if e != nil || len(result.Data) != 1 || result.Data[0].Dimensions[1].Name != "example.com" || result.TotalRows != 501 {
		t.Fatalf("result=%+v err=%v", result, e)
	}
}
func TestMetrikaFailureDoesNotExposeToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401); w.Write([]byte("secret-test")) }))
	defer server.Close()
	reader := NewMetrikaReader()
	reader.Endpoint = server.URL
	_, e := reader.Report(context.Background(), "secret-test", "123", "sources", "2026-10-01", "2026-10-11", 1)
	if e == nil || strings.Contains(e.Error(), "secret-test") {
		t.Fatalf("unsafe error: %v", e)
	}
}
