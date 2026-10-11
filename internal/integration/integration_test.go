package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/lamooof/clickshield/internal/cryptox"
	"github.com/lamooof/clickshield/internal/domain"
)

func testCipher(t *testing.T) *cryptox.Cipher {
	t.Helper()
	cipher, err := cryptox.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	return cipher
}

func testDelivery(provider string, config json.RawMessage, secret []byte) domain.Delivery {
	return domain.Delivery{
		Integration: domain.Integration{Provider: provider, Enabled: true, Config: config, SecretEncrypted: secret},
		Session: domain.SessionContext{
			ID: "56b94a88-1555-4b0c-bf97-72cbd8229790", YandexClientID: "1234567890123456789",
			GoogleClientID: "1234567890.1700000000", LastPageURL: "https://example.test/landing",
		},
		Classification: domain.Classification{
			RiskScore: 82, Classification: domain.Bot, CreatedAt: time.Now().UTC(),
			Reasons: []domain.Reason{{Code: "AUTOMATION_DETECTED"}, {Code: "REPEATED_PAID_VISITS"}},
		},
		EventName: "antifraud_bot",
	}
}

func TestYandexPayload(t *testing.T) {
	cipher := testCipher(t)
	secret, err := cipher.EncryptJSON(map[string]string{"measurement_token": "private-token"})
	if err != nil {
		t.Fatal(err)
	}

	var got url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sender := New(cipher)
	sender.yandexURL = server.URL
	delivery := testDelivery("YANDEX_METRICA", json.RawMessage(`{"counter_id":"12345"}`), secret)
	outcome := sender.Send(context.Background(), delivery)
	if outcome.Status != "SENT" {
		t.Fatalf("outcome = %+v", outcome)
	}
	for key, want := range map[string]string{
		"tid": "12345", "cid": delivery.Session.YandexClientID, "t": "event",
		"ea": "antifraud_bot", "ms": "private-token",
	} {
		if got.Get(key) != want {
			t.Errorf("%s = %q, want %q", key, got.Get(key), want)
		}
	}
	if !strings.Contains(got.Get("params"), "AUTOMATION_DETECTED") {
		t.Errorf("params do not contain reasons: %s", got.Get("params"))
	}
}

func TestGooglePayload(t *testing.T) {
	cipher := testCipher(t)
	secret, err := cipher.EncryptJSON(map[string]string{"api_secret": "private-api-secret"})
	if err != nil {
		t.Fatal(err)
	}

	var query url.Values
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	sender := New(cipher)
	sender.googleURL = server.URL
	delivery := testDelivery("GOOGLE_ANALYTICS", json.RawMessage(`{"measurement_id":"G-TEST123"}`), secret)
	outcome := sender.Send(context.Background(), delivery)
	if outcome.Status != "SENT" {
		t.Fatalf("outcome = %+v", outcome)
	}
	if query.Get("measurement_id") != "G-TEST123" || query.Get("api_secret") != "private-api-secret" {
		t.Errorf("unexpected query: %v", query)
	}
	if got, _ := payload["client_id"].(string); got != delivery.Session.GoogleClientID {
		t.Errorf("client_id = %q", got)
	}
	events, ok := payload["events"].([]any)
	if !ok || len(events) != 1 {
		t.Fatalf("events = %#v", payload["events"])
	}
	event, _ := events[0].(map[string]any)
	if event["name"] != "antifraud_bot" {
		t.Errorf("event name = %#v", event["name"])
	}
}

func TestMissingClientIDsAreSkipped(t *testing.T) {
	cipher := testCipher(t)
	yandexSecret, _ := cipher.EncryptJSON(map[string]string{"measurement_token": "token"})
	googleSecret, _ := cipher.EncryptJSON(map[string]string{"api_secret": "secret"})

	yandex := testDelivery("YANDEX_METRICA", json.RawMessage(`{"counter_id":"1"}`), yandexSecret)
	yandex.Session.YandexClientID = ""
	if out := New(cipher).Send(context.Background(), yandex); out.Status != "SKIPPED" {
		t.Fatalf("Yandex outcome = %+v", out)
	}

	google := testDelivery("GOOGLE_ANALYTICS", json.RawMessage(`{"measurement_id":"G-X"}`), googleSecret)
	google.Session.GoogleClientID = ""
	if out := New(cipher).Send(context.Background(), google); out.Status != "SKIPPED" {
		t.Fatalf("Google outcome = %+v", out)
	}
}

func TestDisabledIntegrationIsSkipped(t *testing.T) {
	cipher := testCipher(t)
	delivery := testDelivery("GOOGLE_ANALYTICS", json.RawMessage(`{"measurement_id":"G-TEST123"}`), nil)
	delivery.Integration.Enabled = false
	if out := New(cipher).Send(context.Background(), delivery); out.Status != "SKIPPED" {
		t.Fatalf("disabled integration outcome = %+v", out)
	}
}
