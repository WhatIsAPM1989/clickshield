package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/lamooof/clickshield/internal/domain"
)

// VK's offline pixel protocol uses semicolon-separated parameters, not an
// OAuth API: https://ads.vk.ru/help/general/sites/offline_events.
func (s *Sender) vk(ctx context.Context, d domain.Delivery) Outcome {
	var cfg struct {
		PixelID string `json:"pixel_id"`
	}
	if json.Unmarshal(d.Integration.Config, &cfg) != nil || !digits.MatchString(cfg.PixelID) || len(cfg.PixelID) > 32 {
		return Outcome{Status: "FAILED", Message: "numeric VK pixel_id is required"}
	}
	if d.Session.RBClickID == "" {
		return Outcome{Status: "SKIPPED", Message: "session has no VK rb_clickid"}
	}
	if len(d.Session.RBClickID) > 256 {
		return Outcome{Status: "FAILED", Message: "VK rb_clickid is too long"}
	}
	switch d.EventName {
	case "antifraud_bot", "antifraud_human", "antifraud_test":
	default:
		return Outcome{Status: "FAILED", Message: "unsupported VK event"}
	}
	// An empty GOAL_VALUE records the event without assigning revenue.
	endpoint := s.vkURL + "?id=" + url.QueryEscape(cfg.PixelID) + ";e=" + url.QueryEscape("RG:/"+d.EventName) + ";rb_clickid=" + url.QueryEscape(d.Session.RBClickID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Outcome{Status: "FAILED", Message: "could not build VK pixel request"}
	}
	req.Header.Set("User-Agent", "ClickShield/1.0")
	resp, err := s.client.Do(req)
	if err != nil {
		return Outcome{Status: "RETRY", Message: safeTransportError("VK pixel request failed", err), Retryable: true}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 2048))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return Outcome{Status: "SENT", Message: "VK pixel accepted the HTTP request; verify attribution in VK Ads"}
	}
	retry := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
	status := "FAILED"
	if retry {
		status = "RETRY"
	}
	return Outcome{Status: status, Message: fmt.Sprintf("VK pixel HTTP %d", resp.StatusCode), Retryable: retry}
}
