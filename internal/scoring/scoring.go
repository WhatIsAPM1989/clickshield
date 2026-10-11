package scoring

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/lamooof/clickshield/internal/asn"
	"github.com/lamooof/clickshield/internal/bots"
	"github.com/lamooof/clickshield/internal/domain"
)

type Thresholds struct {
	Suspicious int `json:"suspicious"`
	Bot        int `json:"bot"`
}

type Limits struct {
	SessionsPerIP          int     `json:"sessions_per_ip"`
	SessionsPerFingerprint int     `json:"sessions_per_fingerprint"`
	PaidVisitFrequency     int     `json:"paid_visit_frequency"`
	FastFirstClickMS       int64   `json:"fast_first_click_ms"`
	FastClickIntervalMS    float64 `json:"fast_click_interval_ms"`
	HighInteractionDensity float64 `json:"high_interaction_density"`
	HighMouseVelocity      float64 `json:"high_mouse_velocity"`
	HighScrollVelocity     float64 `json:"high_scroll_velocity"`
}

type Config struct {
	Thresholds Thresholds     `json:"thresholds"`
	Limits     Limits         `json:"limits"`
	Weights    map[string]int `json:"weights"`
}

func DefaultConfig() Config {
	return Config{
		Thresholds: Thresholds{Suspicious: 35, Bot: 70},
		Limits: Limits{
			SessionsPerIP: 20, SessionsPerFingerprint: 8, PaidVisitFrequency: 4,
			FastFirstClickMS: 180, FastClickIntervalMS: 250, HighInteractionDensity: 30,
			HighMouseVelocity: 6000, HighScrollVelocity: 5000,
		},
		Weights: map[string]int{
			"AUTOMATION_DETECTED": 70, "HEADLESS_SIGNALS": 30,
			"EXCESSIVE_IP_VELOCITY": 25, "EXCESSIVE_FINGERPRINT_VELOCITY": 32,
			"RAPID_UNENGAGED_PAGE_BURST": 75, "ABANDONED_AFTER_FIRST_FLUSH": 35,
			"REPEATED_PAID_VISITS": 25, "FAST_FIRST_CLICK": 12,
			"ROBOTIC_CLICK_INTERVALS": 22, "HIGH_CLICK_RATE": 20,
			"NO_MOUSE_DESKTOP": 18, "ABNORMAL_MOUSE_PATTERN": 20,
			"EXTREME_MOUSE_VELOCITY": 15, "EXTREME_SCROLL_VELOCITY": 15,
			"LOW_ACTIVE_RATIO": 15, "HIGH_INTERACTION_DENSITY": 20,
			"MISSING_FINGERPRINT": 8, "DATACENTER_NETWORK": 18,
			"CLOUD_NO_INTERACTION": 55,
		},
	}
}

func Validate(raw []byte) error {
	if !json.Valid(raw) {
		return fmt.Errorf("scoring_config must be valid JSON")
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("invalid scoring_config: %w", err)
	}
	if cfg.Thresholds.Suspicious < 1 || cfg.Thresholds.Suspicious >= 100 {
		return fmt.Errorf("thresholds.suspicious must be between 1 and 99")
	}
	if cfg.Thresholds.Bot <= cfg.Thresholds.Suspicious || cfg.Thresholds.Bot > 100 {
		return fmt.Errorf("thresholds.bot must be greater than suspicious and no more than 100")
	}
	if cfg.Limits.SessionsPerIP < 1 || cfg.Limits.SessionsPerFingerprint < 1 || cfg.Limits.PaidVisitFrequency < 1 {
		return fmt.Errorf("session and paid-visit limits must be positive")
	}
	if cfg.Limits.FastFirstClickMS < 1 || cfg.Limits.FastClickIntervalMS <= 0 || cfg.Limits.HighInteractionDensity <= 0 || cfg.Limits.HighMouseVelocity <= 0 || cfg.Limits.HighScrollVelocity <= 0 {
		return fmt.Errorf("behavioral limits must be positive")
	}
	for code, weight := range cfg.Weights {
		if weight < 0 || weight > 100 {
			return fmt.Errorf("weight %s must be between 0 and 100", code)
		}
	}
	return nil
}

func Parse(raw []byte) Config {
	cfg := DefaultConfig()
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &cfg)
	}
	defaults := DefaultConfig()
	if cfg.Thresholds.Suspicious <= 0 {
		cfg.Thresholds.Suspicious = defaults.Thresholds.Suspicious
	}
	if cfg.Thresholds.Bot <= cfg.Thresholds.Suspicious {
		cfg.Thresholds.Bot = defaults.Thresholds.Bot
	}
	if cfg.Weights == nil {
		cfg.Weights = defaults.Weights
	} else {
		for code, weight := range defaults.Weights {
			if _, ok := cfg.Weights[code]; !ok {
				cfg.Weights[code] = weight
			}
		}
	}
	return cfg
}

func Score(ctx domain.SessionContext, f domain.Features, cfg Config) domain.Score {
	bot, _ := bots.Identify(ctx.UserAgent, ctx.IP)
	if ctx.KnownBot {
		return domain.Score{
			RiskScore: 100, Classification: domain.KnownBot,
			Reasons: []domain.Reason{{Code: "KNOWN_CRAWLER", Weight: 100, Details: map[string]any{"user_agent": ctx.UserAgent, "bot": bot.Name, "vendor": bot.Vendor, "category": bot.Category}}},
		}
	}

	var reasons []domain.Reason
	risk := 0
	add := func(code string, details map[string]any) {
		weight := cfg.Weights[code]
		if weight <= 0 {
			return
		}
		risk += weight
		reasons = append(reasons, domain.Reason{Code: code, Weight: weight, Details: details})
	}

	automationSignals := 0
	for key, enabled := range ctx.Automation {
		if enabled && (strings.Contains(key, "webdriver") || strings.Contains(key, "selenium") || strings.Contains(key, "playwright") || strings.Contains(key, "puppeteer") || strings.Contains(key, "phantom")) {
			automationSignals++
		}
	}
	// Browser automation and HTTP client libraries that identify themselves in
	// the user agent but are not declared crawlers.
	if bot.Category == bots.Automation {
		automationSignals++
	}
	if automationSignals > 0 {
		details := map[string]any{"signals": automationSignals}
		if bot.Name != "" {
			details["client"] = bot.Name
		}
		add("AUTOMATION_DETECTED", details)
	}
	if automationSignals >= 2 {
		add("HEADLESS_SIGNALS", map[string]any{"signals": automationSignals})
	}
	if ctx.SessionsPerIP >= cfg.Limits.SessionsPerIP {
		add("EXCESSIVE_IP_VELOCITY", map[string]any{"sessions_24h": ctx.SessionsPerIP})
	}
	if ctx.VisitorsPerFingerprint >= cfg.Limits.SessionsPerFingerprint {
		add("EXCESSIVE_FINGERPRINT_VELOCITY", map[string]any{"visitors_24h": ctx.VisitorsPerFingerprint, "sessions_24h": ctx.SessionsPerFingerprint})
	}
	strongBurst := ctx.NearbyVisitorSessions >= 5 && ctx.NearbyVisitorPages >= 5 && f.DurationMS <= 5000 && f.ClickCount == 0 && f.ScrollEvents == 0 && f.MouseMoves == 0
	if strongBurst {
		add("RAPID_UNENGAGED_PAGE_BURST", map[string]any{"sessions_60s": ctx.NearbyVisitorSessions, "distinct_pages_60s": ctx.NearbyVisitorPages, "duration_ms": f.DurationMS})
	}
	// The tracker delivered only its first 5-second batch, with no interaction
	// and no visibility change: the page was destroyed rather than closed by a
	// visitor, which is typical for headless renderers.
	if f.HeartbeatEvents == 1 && f.VisibilityEvents == 0 && f.DurationMS >= 4000 && f.DurationMS <= 6500 && f.MouseMoves == 0 && f.ClickCount == 0 && f.ScrollEvents == 0 && f.TouchEvents == 0 {
		add("ABANDONED_AFTER_FIRST_FLUSH", map[string]any{"duration_ms": f.DurationMS})
	}
	if ctx.PaidVisitFrequency >= cfg.Limits.PaidVisitFrequency {
		add("REPEATED_PAID_VISITS", map[string]any{"paid_visits_24h": ctx.PaidVisitFrequency})
	}
	if f.TimeToFirstClickMS >= 0 && f.TimeToFirstClickMS < cfg.Limits.FastFirstClickMS {
		add("FAST_FIRST_CLICK", map[string]any{"ms": f.TimeToFirstClickMS})
	}
	if f.ClickCount >= 4 && f.MeanClickIntervalMS > 0 && f.MeanClickIntervalMS < cfg.Limits.FastClickIntervalMS {
		add("ROBOTIC_CLICK_INTERVALS", map[string]any{"mean_ms": math.Round(f.MeanClickIntervalMS)})
	}
	if f.ClickCount >= 15 && f.DurationMS < 30000 {
		add("HIGH_CLICK_RATE", map[string]any{"clicks": f.ClickCount, "duration_ms": f.DurationMS})
	}
	if ctx.DeviceType == "desktop" && f.DurationMS > 15000 && f.ClickCount >= 3 && f.MouseMoves < 2 {
		add("NO_MOUSE_DESKTOP", map[string]any{"clicks": f.ClickCount})
	}
	if ctx.DeviceType == "desktop" && f.MouseMoves >= 20 && f.MouseCurvature < 0.035 && f.MouseVelocity > 900 {
		add("ABNORMAL_MOUSE_PATTERN", map[string]any{"curvature": f.MouseCurvature, "velocity": f.MouseVelocity})
	}
	if f.MouseMoves >= 40 && f.MouseP95Velocity > cfg.Limits.HighMouseVelocity {
		add("EXTREME_MOUSE_VELOCITY", map[string]any{"p95_velocity": f.MouseP95Velocity})
	}
	if f.ScrollDepth > 0.85 && f.ScrollVelocity > cfg.Limits.HighScrollVelocity {
		add("EXTREME_SCROLL_VELOCITY", map[string]any{"velocity": f.ScrollVelocity})
	}
	// Long sessions are mostly background tabs, where low activity is normal.
	if f.DurationMS > 15000 && f.DurationMS <= 10*60*1000 && f.ActiveRatio < 0.10 && (f.ClickCount+f.ScrollEvents) > 0 {
		add("LOW_ACTIVE_RATIO", map[string]any{"active_ratio": f.ActiveRatio})
	}
	if f.InteractionDensity > cfg.Limits.HighInteractionDensity {
		add("HIGH_INTERACTION_DENSITY", map[string]any{"density": f.InteractionDensity})
	}
	if ctx.Fingerprint == "" && (ctx.YCLID != "" || ctx.GCLID != "" || ctx.RBClickID != "") {
		add("MISSING_FINGERPRINT", nil)
	}
	network := asn.Classify(asn.ParseNumber(ctx.ASN))
	org := strings.ToLower(ctx.ASOrg)
	if network != asn.Residential || strings.Contains(org, "hosting") || strings.Contains(org, "cloud") || strings.Contains(org, "datacenter") || strings.Contains(org, "vps") {
		add("DATACENTER_NETWORK", map[string]any{"asn": ctx.ASN, "as_org": ctx.ASOrg})
	}
	// Scrapers on cloud platforms load pages without ever touching them. Real
	// visitors behind a VPN on rented servers do the same in background tabs,
	// so the rule is limited to cloud platforms and short sessions.
	if network == asn.Cloud && f.MouseMoves == 0 && f.ClickCount == 0 && f.TouchEvents == 0 && f.DurationMS <= 5*60*1000 {
		add("CLOUD_NO_INTERACTION", map[string]any{"asn": ctx.ASN, "as_org": ctx.ASOrg, "duration_ms": f.DurationMS})
	}

	if risk > 100 {
		risk = 100
	}
	classification := domain.Human
	// Automation markers and the rapid, unengaged multi-page burst are strong
	// signals on their own; other individual signals still need independent
	// corroboration for BOT.
	strong := (strongBurst && cfg.Weights["RAPID_UNENGAGED_PAGE_BURST"] > 0) || (automationSignals > 0 && cfg.Weights["AUTOMATION_DETECTED"] > 0)
	if risk >= cfg.Thresholds.Bot && (len(reasons) >= 2 || strong) {
		classification = domain.Bot
	} else if risk >= cfg.Thresholds.Suspicious {
		classification = domain.Suspicious
	}
	return domain.Score{RiskScore: risk, Classification: classification, Reasons: reasons}
}
