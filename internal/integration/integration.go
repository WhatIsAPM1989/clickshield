package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/lamooof/clickshield/internal/cryptox"
	"github.com/lamooof/clickshield/internal/domain"
)

type Outcome struct {
	Status    string
	Message   string
	Retryable bool
}

type Sender struct {
	cipher                      *cryptox.Cipher
	client                      *http.Client
	yandexURL, googleURL, vkURL string
}

func New(cipher *cryptox.Cipher) *Sender {
	return &Sender{
		cipher:    cipher,
		client:    &http.Client{Timeout: 10 * time.Second},
		yandexURL: "https://mc.yandex.ru/collect",
		googleURL: "https://www.google-analytics.com/mp/collect",
		vkURL:     "https://top-fwz1.mail.ru/tracker",
	}
}

func (s *Sender) Send(ctx context.Context, delivery domain.Delivery) Outcome {
	if !delivery.Integration.Enabled {
		return Outcome{Status: "SKIPPED", Message: "integration is disabled"}
	}
	switch delivery.Integration.Provider {
	case "YANDEX_METRICA":
		return s.yandex(ctx, delivery)
	case "GOOGLE_ANALYTICS":
		return s.google(ctx, delivery)
	case "VK_ADS":
		return s.vk(ctx, delivery)
	default:
		return Outcome{Status: "FAILED", Message: "unknown provider"}
	}
}

// Test validates Google payloads without adding an event to reports. Yandex has
// no equivalent validation endpoint, so its test sends an antifraud_test event.
// VK also sends a real antifraud_test event using the session rb_clickid.
func (s *Sender) Test(ctx context.Context, d domain.Delivery) Outcome {
	if d.Integration.Provider == "VK_ADS" {
		d.Integration.Enabled = true
		d.EventName = "antifraud_test"
		return s.vk(ctx, d)
	}
	if d.Integration.Provider == "YANDEX_METRICA" {
		d.Integration.Enabled = true
		d.EventName = "antifraud_test"
		return s.yandex(ctx, d)
	}
	if d.Integration.Provider != "GOOGLE_ANALYTICS" {
		return Outcome{Status: "FAILED", Message: "unknown provider"}
	}
	var cfg googleConfig
	var secret googleSecret
	if json.Unmarshal(d.Integration.Config, &cfg) != nil || s.cipher.DecryptJSON(d.Integration.SecretEncrypted, &secret) != nil || cfg.MeasurementID == "" || secret.APISecret == "" {
		return Outcome{Status: "FAILED", Message: "Google configuration is incomplete"}
	}
	if d.Session.GoogleClientID == "" {
		return Outcome{Status: "FAILED", Message: "session has no Google client_id"}
	}
	body, _ := json.Marshal(map[string]any{
		"client_id":           d.Session.GoogleClientID,
		"validation_behavior": "ENFORCE_RECOMMENDATIONS",
		"events":              []any{map[string]any{"name": "antifraud_test", "params": map[string]any{"engagement_time_msec": 1, "risk_score": d.Classification.RiskScore}}},
	})
	endpoint := "https://www.google-analytics.com/debug/mp/collect?measurement_id=" + url.QueryEscape(cfg.MeasurementID) + "&api_secret=" + url.QueryEscape(secret.APISecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Outcome{Status: "FAILED", Message: "could not build Google validation request"}
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(req)
	if err != nil {
		return Outcome{Status: "FAILED", Message: safeTransportError("Google validation failed", err)}
	}
	defer response.Body.Close()
	var result struct {
		ValidationMessages []struct {
			FieldPath   string `json:"fieldPath"`
			Description string `json:"description"`
		} `json:"validationMessages"`
	}
	if response.StatusCode != 200 || json.NewDecoder(io.LimitReader(response.Body, 8192)).Decode(&result) != nil {
		return Outcome{Status: "FAILED", Message: fmt.Sprintf("Google validation HTTP %d or invalid response", response.StatusCode)}
	}
	if len(result.ValidationMessages) > 0 {
		messages := make([]string, 0, len(result.ValidationMessages))
		for _, item := range result.ValidationMessages {
			messages = append(messages, item.FieldPath+": "+item.Description)
		}
		return Outcome{Status: "FAILED", Message: strings.Join(messages, "; ")}
	}
	return Outcome{Status: "VALID", Message: "Google payload is valid; the validation endpoint does not verify API secret or measurement ID"}
}

type yandexConfig struct {
	CounterID string `json:"counter_id"`
}
type yandexSecret struct {
	MeasurementToken string `json:"measurement_token"`
}

var digits = regexp.MustCompile(`^[0-9]+$`)

func (s *Sender) yandex(ctx context.Context, d domain.Delivery) Outcome {
	var cfg yandexConfig
	var secret yandexSecret
	if json.Unmarshal(d.Integration.Config, &cfg) != nil || s.cipher.DecryptJSON(d.Integration.SecretEncrypted, &secret) != nil {
		return Outcome{Status: "FAILED", Message: "invalid encrypted Yandex configuration"}
	}
	if !digits.MatchString(cfg.CounterID) || secret.MeasurementToken == "" {
		return Outcome{Status: "FAILED", Message: "counter_id or Measurement Protocol token is missing"}
	}
	if !digits.MatchString(d.Session.YandexClientID) {
		return Outcome{Status: "SKIPPED", Message: "session has no numeric Yandex ClientID"}
	}

	reasons := make([]string, 0, len(d.Classification.Reasons))
	for _, reason := range d.Classification.Reasons {
		reasons = append(reasons, reason.Code)
	}
	params, _ := json.Marshal(map[string]any{"antifraud": map[string]any{
		"classification": d.Classification.Classification,
		"risk_score":     d.Classification.RiskScore,
		"session_id":     d.Session.ID,
		"reasons":        strings.Join(reasons, ","),
	}})
	form := url.Values{
		"tid": {cfg.CounterID}, "cid": {d.Session.YandexClientID}, "t": {"event"},
		"ea": {d.EventName}, "ms": {secret.MeasurementToken}, "dl": {d.Session.LastPageURL},
		"params": {string(params)},
	}
	if age := time.Since(d.Classification.CreatedAt); age >= 0 && age < 11*time.Hour {
		form.Set("et", fmt.Sprint(d.Classification.CreatedAt.Unix()))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.yandexURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Outcome{Status: "RETRY", Message: err.Error(), Retryable: true}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "ClickShield/1.0")
	resp, err := s.client.Do(req)
	if err != nil {
		return Outcome{Status: "RETRY", Message: safeTransportError("Yandex request failed", err), Retryable: true}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return Outcome{Status: "SENT"}
	}
	retry := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
	status := "FAILED"
	if retry {
		status = "RETRY"
	}
	message := strings.ReplaceAll(strings.TrimSpace(string(body)), secret.MeasurementToken, "[REDACTED]")
	return Outcome{Status: status, Message: fmt.Sprintf("Yandex HTTP %d: %s", resp.StatusCode, message), Retryable: retry}
}

type googleConfig struct {
	MeasurementID string `json:"measurement_id"`
}
type googleSecret struct {
	APISecret string `json:"api_secret"`
}

func (s *Sender) google(ctx context.Context, d domain.Delivery) Outcome {
	var cfg googleConfig
	var secret googleSecret
	if json.Unmarshal(d.Integration.Config, &cfg) != nil || s.cipher.DecryptJSON(d.Integration.SecretEncrypted, &secret) != nil {
		return Outcome{Status: "FAILED", Message: "invalid encrypted Google configuration"}
	}
	if cfg.MeasurementID == "" || secret.APISecret == "" {
		return Outcome{Status: "FAILED", Message: "measurement_id or api_secret is missing"}
	}
	if d.Session.GoogleClientID == "" {
		return Outcome{Status: "SKIPPED", Message: "session has no Google client_id"}
	}

	reasons := make([]string, 0, len(d.Classification.Reasons))
	for _, reason := range d.Classification.Reasons {
		reasons = append(reasons, reason.Code)
	}
	reasonText := strings.Join(reasons, ",")
	if len(reasonText) > 100 {
		reasonText = reasonText[:100]
	}
	payload := map[string]any{
		"client_id": d.Session.GoogleClientID,
		"user_properties": map[string]any{
			"antifraud_classification": map[string]any{"value": d.Classification.Classification},
		},
		"events": []any{map[string]any{
			"name": d.EventName,
			"params": map[string]any{
				"risk_score":           d.Classification.RiskScore,
				"classification":       d.Classification.Classification,
				"antifraud_session_id": d.Session.ID,
				"reasons":              reasonText,
				"engagement_time_msec": 1,
			},
		}},
	}
	if age := time.Since(d.Classification.CreatedAt); age >= 0 && age < 71*time.Hour {
		payload["timestamp_micros"] = d.Classification.CreatedAt.UnixMicro()
	}
	body, _ := json.Marshal(payload)
	endpoint := s.googleURL + "?measurement_id=" + url.QueryEscape(cfg.MeasurementID) + "&api_secret=" + url.QueryEscape(secret.APISecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Outcome{Status: "FAILED", Message: "could not build Google Measurement Protocol request"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ClickShield/1.0")
	resp, err := s.client.Do(req)
	if err != nil {
		return Outcome{Status: "RETRY", Message: safeTransportError("Google request failed", err), Retryable: true}
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return Outcome{Status: "SENT"}
	}
	retry := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
	status := "FAILED"
	if retry {
		status = "RETRY"
	}
	message := strings.ReplaceAll(strings.TrimSpace(string(responseBody)), secret.APISecret, "[REDACTED]")
	return Outcome{Status: status, Message: fmt.Sprintf("Google HTTP %d: %s", resp.StatusCode, message), Retryable: retry}
}

func safeTransportError(prefix string, err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		return prefix + ": " + urlErr.Err.Error()
	}
	return prefix
}
