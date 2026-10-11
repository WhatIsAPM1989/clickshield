package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestVKOfflinePixel(t *testing.T) {
	for _, event := range []string{"antifraud_bot", "antifraud_human", "antifraud_test"} {
		t.Run(event, func(t *testing.T) {
			var got string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("method = %s", r.Method)
				}
				got = r.URL.RawQuery
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			sender := New(testCipher(t))
			sender.vkURL = server.URL
			d := testDelivery("VK_ADS", json.RawMessage(`{"pixel_id":"3229418"}`), nil)
			d.EventName = event
			d.Session.RBClickID = "click;id=other&x=/+"
			out := sender.Send(context.Background(), d)
			if out.Status != "SENT" {
				t.Fatalf("outcome = %+v", out)
			}
			want := "id=3229418;e=" + url.QueryEscape("RG:/"+event) + ";rb_clickid=" + url.QueryEscape(d.Session.RBClickID)
			if got != want || len(strings.Split(got, ";")) != 3 {
				t.Fatalf("query = %q, want %q", got, want)
			}
		})
	}
}

func TestVKOutcomes(t *testing.T) {
	for _, tc := range []struct {
		code   int
		status string
		retry  bool
	}{
		{204, "SENT", false}, {400, "FAILED", false}, {401, "FAILED", false}, {429, "RETRY", true}, {503, "RETRY", true},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.code) }))
		sender := New(testCipher(t))
		sender.vkURL = server.URL
		d := testDelivery("VK_ADS", json.RawMessage(`{"pixel_id":"3229418"}`), nil)
		d.Session.RBClickID = "123-456-789"
		out := sender.Send(context.Background(), d)
		server.Close()
		if out.Status != tc.status || out.Retryable != tc.retry {
			t.Errorf("HTTP %d: %+v", tc.code, out)
		}
	}
}

func TestVKSkipsAndValidatesWithoutRequest(t *testing.T) {
	sender := New(testCipher(t))
	sender.vkURL = ":invalid"
	d := testDelivery("VK_ADS", json.RawMessage(`{"pixel_id":"3229418"}`), nil)
	if out := sender.Send(context.Background(), d); out.Status != "SKIPPED" {
		t.Fatal(out)
	}
	d.Session.RBClickID = "123"
	d.Integration.Config = json.RawMessage(`{"pixel_id":"not-numeric"}`)
	if out := sender.Send(context.Background(), d); out.Status != "FAILED" {
		t.Fatal(out)
	}
	d.Integration.Enabled = false
	if out := sender.Send(context.Background(), d); out.Status != "SKIPPED" {
		t.Fatal(out)
	}
}

func TestVKTestSendsRealTestEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "antifraud_test") {
			t.Errorf("query = %q", r.URL.RawQuery)
		}
	}))
	defer server.Close()
	sender := New(testCipher(t))
	sender.vkURL = server.URL
	d := testDelivery("VK_ADS", json.RawMessage(`{"pixel_id":"3229418"}`), nil)
	d.Session.RBClickID = "123"
	d.Integration.Enabled = false
	if out := sender.Test(context.Background(), d); out.Status != "SENT" {
		t.Fatal(out)
	}
}
