package domain

import (
	"encoding/json"
	"net"
	"time"
)

const (
	Human      = "HUMAN"
	Suspicious = "SUSPICIOUS"
	Bot        = "BOT"
	KnownBot   = "KNOWN_BOT"
)

// Project modes. Every mode except MONITORING sends classifications to the
// enabled integrations; the two blocking modes also return BLOCK decisions.
const (
	ModeMonitoring   = "MONITORING"
	ModeNotify       = "NOTIFY"
	ModeBlocking     = "BLOCKING"      // blocks BOT and KNOWN_BOT visitors
	ModeBlockingBots = "BLOCKING_BOTS" // blocks BOT visitors, declared crawlers pass
)

// NormalizeMode maps unknown modes to MONITORING.
func NormalizeMode(mode string) string {
	switch mode {
	case ModeNotify, ModeBlocking, ModeBlockingBots:
		return mode
	default:
		return ModeMonitoring
	}
}

type Project struct {
	ReplayEnabled       bool            `json:"replay_enabled"`
	ReplaySampleRate    int             `json:"replay_sample_rate"`
	ReplayRetentionDays int             `json:"replay_retention_days"`
	ID                  string          `json:"id"`
	Name                string          `json:"name"`
	PublicKey           string          `json:"public_key"`
	AllowedOrigins      []string        `json:"allowed_origins"`
	Mode                string          `json:"mode"`
	ScoringConfig       json.RawMessage `json:"scoring_config"`
	CreatedAt           time.Time       `json:"created_at"`
}

type UTM struct {
	Source   string `json:"source,omitempty"`
	Medium   string `json:"medium,omitempty"`
	Campaign string `json:"campaign,omitempty"`
	Term     string `json:"term,omitempty"`
	Content  string `json:"content,omitempty"`
}
type Device struct {
	Type                string  `json:"type,omitempty"`
	Platform            string  `json:"platform,omitempty"`
	Language            string  `json:"language,omitempty"`
	Timezone            string  `json:"timezone,omitempty"`
	ScreenWidth         int     `json:"screen_width,omitempty"`
	ScreenHeight        int     `json:"screen_height,omitempty"`
	ColorDepth          int     `json:"color_depth,omitempty"`
	HardwareConcurrency int     `json:"hardware_concurrency,omitempty"`
	DeviceMemory        float64 `json:"device_memory,omitempty"`
	MaxTouchPoints      int     `json:"max_touch_points,omitempty"`
}
type Event struct {
	PageURL        string         `json:"page_url,omitempty"`
	Name           string         `json:"name,omitempty"`
	Properties     map[string]any `json:"properties,omitempty"`
	DocumentHeight float64        `json:"document_height,omitempty"`
	Type           string         `json:"type"`
	TS             int64          `json:"ts"`
	X              float64        `json:"x,omitempty"`
	Y              float64        `json:"y,omitempty"`
	VW             float64        `json:"vw,omitempty"`
	VH             float64        `json:"vh,omitempty"`
	ScrollY        float64        `json:"scroll_y,omitempty"`
	Depth          float64        `json:"depth,omitempty"`
	Visible        *bool          `json:"visible,omitempty"`
	ActiveMS       int64          `json:"active_ms,omitempty"`
	IdleMS         int64          `json:"idle_ms,omitempty"`
	PageIndex      int            `json:"page_index,omitempty"`
	Pointer        string         `json:"pointer,omitempty"`
	Button         int            `json:"button,omitempty"`
	Extra          map[string]any `json:"extra,omitempty"`
}
type EventBatch struct {
	SessionID      string          `json:"session_id"`
	VisitorID      string          `json:"visitor_id"`
	BatchID        string          `json:"batch_id"`
	SentAt         int64           `json:"sent_at"`
	PageURL        string          `json:"page_url"`
	LandingURL     string          `json:"landing_url"`
	Referrer       string          `json:"referrer"`
	YandexClientID string          `json:"yandex_client_id"`
	GoogleClientID string          `json:"google_client_id"`
	YCLID          string          `json:"yclid"`
	GCLID          string          `json:"gclid"`
	RBClickID      string          `json:"rb_clickid"`
	UTM            UTM             `json:"utm"`
	Fingerprint    string          `json:"fingerprint"`
	Device         Device          `json:"device"`
	Automation     map[string]bool `json:"automation"`
	PageIndex      int             `json:"page_index"`
	Finish         bool            `json:"finish,omitempty"`
	Events         []Event         `json:"events"`
}
type FinishRequest struct {
	SessionID string `json:"session_id"`
	VisitorID string `json:"visitor_id"`
	BatchID   string `json:"batch_id"`
	Reason    string `json:"reason"`
	SentAt    int64  `json:"sent_at"`
}
type RemoteMeta struct {
	IP        net.IP
	ASN       string
	ASOrg     string
	Region    string
	City      string
	Country   string
	UserAgent string
	Origin    string
}

type Features struct {
	SessionID              string    `json:"session_id"`
	DurationMS             int64     `json:"duration_ms"`
	ActiveTimeMS           int64     `json:"active_time_ms"`
	IdleTimeMS             int64     `json:"idle_time_ms"`
	MouseDistance          float64   `json:"mouse_distance"`
	MouseVelocity          float64   `json:"mouse_velocity"`
	MouseMaxVelocity       float64   `json:"mouse_max_velocity"`
	MouseP95Velocity       float64   `json:"mouse_p95_velocity"`
	MouseAcceleration      float64   `json:"mouse_acceleration"`
	MouseCurvature         float64   `json:"mouse_curvature"`
	MousePauses            int       `json:"mouse_pauses"`
	MouseMoves             int       `json:"mouse_moves"`
	ScrollDepth            float64   `json:"scroll_depth"`
	ScrollVelocity         float64   `json:"scroll_velocity"`
	ScrollReversals        int       `json:"scroll_reversals"`
	ScrollEvents           int       `json:"scroll_events"`
	ClickCount             int       `json:"click_count"`
	TouchEvents            int       `json:"touch_events"`
	HeartbeatEvents        int       `json:"heartbeat_events"`
	VisibilityEvents       int       `json:"visibility_events"`
	ClickIntervalsMS       []int64   `json:"click_intervals_ms"`
	MeanClickIntervalMS    float64   `json:"mean_click_interval_ms"`
	TimeToFirstClickMS     int64     `json:"time_to_first_click_ms"`
	PageCount              int       `json:"page_count"`
	InteractionDensity     float64   `json:"interaction_density"`
	ActiveRatio            float64   `json:"active_ratio"`
	PaidVisitFrequency     int       `json:"paid_visit_frequency"`
	SessionsPerIP          int       `json:"sessions_per_ip"`
	SessionsPerFingerprint int       `json:"sessions_per_fingerprint"`
	ComputedAt             time.Time `json:"computed_at"`
}
type Reason struct {
	Code    string         `json:"code"`
	Weight  int            `json:"weight"`
	Details map[string]any `json:"details,omitempty"`
}
type Score struct {
	RiskScore      int      `json:"risk_score"`
	Classification string   `json:"classification"`
	Reasons        []Reason `json:"reasons"`
}

type SessionContext struct {
	ID                     string
	ProjectID              string
	VisitorDBID            string
	VisitorKey             string
	Fingerprint            string
	IP                     string
	ASN                    string
	ASOrg                  string
	UserAgent              string
	DeviceType             string
	YandexClientID         string
	GoogleClientID         string
	YCLID                  string
	GCLID                  string
	RBClickID              string
	UTMSource              string
	UTMMedium              string
	UTMCampaign            string
	Referrer               string
	LandingURL             string
	LastPageURL            string
	Automation             map[string]bool
	StartedAt              time.Time
	LastSeenAt             time.Time
	ManualLabel            string
	ProjectMode            string
	ProjectScoringConfig   json.RawMessage
	SessionsPerIP          int
	SessionsPerFingerprint int
	VisitorsPerFingerprint int
	NearbyVisitorSessions  int
	NearbyVisitorPages     int
	PaidVisitFrequency     int
	KnownBot               bool
}

type SessionItem struct {
	TrafficSource    string    `json:"traffic_source"`
	TrafficChannel   string    `json:"traffic_channel"`
	ReferrerHost     string    `json:"referrer_host"`
	AttributionBasis string    `json:"attribution_basis"`
	LandingURL       string    `json:"landing_url"`
	HasReplay        bool      `json:"has_replay"`
	ID               string    `json:"id"`
	ProjectID        string    `json:"project_id"`
	ProjectName      string    `json:"project_name"`
	StartedAt        time.Time `json:"started_at"`
	LastSeenAt       time.Time `json:"last_seen_at"`
	DurationMS       int64     `json:"duration_ms"`
	Classification   string    `json:"classification"`
	RiskScore        int       `json:"risk_score"`
	YandexClientID   string    `json:"yandex_client_id"`
	GoogleClientID   string    `json:"google_client_id"`
	YCLID            string    `json:"yclid"`
	GCLID            string    `json:"gclid"`
	RBClickID        string    `json:"rb_clickid"`
	UTMSource        string    `json:"utm_source"`
	UTMMedium        string    `json:"utm_medium"`
	UTMCampaign      string    `json:"utm_campaign"`
	ClickCount       int       `json:"click_count"`
	ScrollDepth      float64   `json:"scroll_depth"`
	Fingerprint      string    `json:"fingerprint"`
	IP               string    `json:"ip"`
	ASN              string    `json:"asn"`
	ASOrg            string    `json:"as_org"`
	DeviceType       string    `json:"device_type"`
	ManualLabel      string    `json:"manual_label"`
	BotName          string    `json:"bot_name"`
	BotCategory      string    `json:"bot_category"`
}
type Stats struct {
	Total        int64   `json:"total"`
	Human        int64   `json:"human"`
	Suspicious   int64   `json:"suspicious"`
	Bot          int64   `json:"bot"`
	KnownBot     int64   `json:"known_bot"`
	Unclassified int64   `json:"unclassified"`
	AverageRisk  float64 `json:"average_risk"`
	PaidVisits   int64   `json:"paid_visits"`
}
type Classification struct {
	ID             int64     `json:"id"`
	SessionID      string    `json:"session_id"`
	RiskScore      int       `json:"risk_score"`
	Classification string    `json:"classification"`
	Source         string    `json:"source"`
	CreatedAt      time.Time `json:"created_at"`
	Reasons        []Reason  `json:"reasons"`
}
type TrajectoryPoint struct {
	Type  string  `json:"type"`
	TS    int64   `json:"ts"`
	X     float64 `json:"x,omitempty"`
	Y     float64 `json:"y,omitempty"`
	VW    float64 `json:"vw,omitempty"`
	VH    float64 `json:"vh,omitempty"`
	Depth float64 `json:"depth,omitempty"`
}
type SessionPage struct {
	Index    int       `json:"index"`
	URL      string    `json:"url"`
	ViewedAt time.Time `json:"viewed_at"`
}
type SessionDetail struct {
	Pages           []SessionPage     `json:"pages"`
	Session         SessionItem       `json:"session"`
	Context         SessionContext    `json:"context"`
	Features        *Features         `json:"features,omitempty"`
	Classifications []Classification  `json:"classifications"`
	Trajectory      []TrajectoryPoint `json:"trajectory"`
	Outcomes        []LeadOutcome     `json:"outcomes"`
}
type LeadOutcome struct {
	ExternalID string    `json:"external_id"`
	SessionID  string    `json:"session_id"`
	Status     string    `json:"status"`
	Source     string    `json:"source"`
	UpdatedAt  time.Time `json:"updated_at"`
}
type Integration struct {
	ID              string          `json:"id"`
	ProjectID       string          `json:"project_id"`
	Provider        string          `json:"provider"`
	Enabled         bool            `json:"enabled"`
	Config          json.RawMessage `json:"config"`
	SecretEncrypted []byte          `json:"-"`
	HasSecret       bool            `json:"has_secret"`
	LastStatus      string          `json:"last_status"`
	LastError       string          `json:"last_error"`
	LastSentAt      *time.Time      `json:"last_sent_at,omitempty"`
}
type Delivery struct {
	OutboxID       int64
	Integration    Integration
	Session        SessionContext
	Classification Classification
	EventName      string
	Attempts       int
}

type OutboxClaim struct {
	ID       int64
	Lease    string
	Attempts int
}

type Operations struct {
	AnalyticsPending       int64 `json:"analytics_pending"`
	AnalyticsLagSeconds    int64 `json:"analytics_lag_seconds"`
	AnalyticsRetries       int64 `json:"analytics_retries"`
	ReplayGCFailed         int64 `json:"replay_gc_failed"`
	EventsLastHour         int64 `json:"events_last_hour"`
	UnscoredOlderThanHour  int64 `json:"unscored_older_than_hour"`
	OutboxPendingOlderHour int64 `json:"outbox_pending_older_hour"`
	OutboxFailed           int64 `json:"outbox_failed"`
	OutboxSentLastDay      int64 `json:"outbox_sent_last_day"`
}
