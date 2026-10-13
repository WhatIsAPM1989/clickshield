package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/lamooof/clickshield/internal/replay"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ReplayS3                replay.S3Config
	HTTPAddr                string
	DatabaseURL             string
	AdminUsername           string
	AdminPassword           string
	SessionSecret           []byte
	EncryptionKey           []byte
	CookieSecure            bool
	TrustProxyHeaders       bool
	OutcomeAPIToken         string
	AlertWebhookURL         string
	BootstrapProjectName    string
	BootstrapProjectKey     string
	BootstrapAllowedOrigins []string
	YandexCounterID         string
	YandexMeasurementToken  string
	GoogleMeasurementID     string
	GoogleAPISecret         string
	GeoCityDatabase         string
	MCPEnabled              bool
	ASNDatabase             string
	RawEventsTTL            time.Duration
	AggregateTTL            time.Duration
	StaleSessionAfter       time.Duration
	FinishGrace             time.Duration
	WorkerInterval          time.Duration
}

func Load() (Config, error) { return LoadFor("serve") }

func LoadFor(command string) (Config, error) {
	cfg := Config{
		ReplayS3:                replay.S3Config{Endpoint: strings.TrimSpace(os.Getenv("REPLAY_S3_ENDPOINT")), Bucket: strings.TrimSpace(os.Getenv("REPLAY_S3_BUCKET")), AccessKey: os.Getenv("REPLAY_S3_ACCESS_KEY"), SecretKey: os.Getenv("REPLAY_S3_SECRET_KEY"), Region: getenv("REPLAY_S3_REGION", "us-east-1")},
		HTTPAddr:                getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:             strings.TrimSpace(os.Getenv("DATABASE_URL")),
		AdminUsername:           strings.TrimSpace(getenv("ADMIN_USERNAME", "admin")),
		AdminPassword:           os.Getenv("ADMIN_PASSWORD"),
		CookieSecure:            boolEnv("COOKIE_SECURE", false),
		TrustProxyHeaders:       boolEnv("TRUST_PROXY_HEADERS", false),
		OutcomeAPIToken:         strings.TrimSpace(os.Getenv("OUTCOME_API_TOKEN")),
		AlertWebhookURL:         strings.TrimSpace(os.Getenv("ALERT_WEBHOOK_URL")),
		BootstrapProjectName:    strings.TrimSpace(getenv("BOOTSTRAP_PROJECT_NAME", "Demo site")),
		BootstrapProjectKey:     strings.TrimSpace(os.Getenv("BOOTSTRAP_PROJECT_KEY")),
		BootstrapAllowedOrigins: csv(os.Getenv("BOOTSTRAP_ALLOWED_ORIGINS")),
		YandexCounterID:         strings.TrimSpace(os.Getenv("YANDEX_COUNTER_ID")),
		YandexMeasurementToken:  strings.TrimSpace(os.Getenv("YANDEX_MEASUREMENT_TOKEN")),
		GoogleMeasurementID:     strings.TrimSpace(os.Getenv("GOOGLE_MEASUREMENT_ID")),
		GoogleAPISecret:         strings.TrimSpace(os.Getenv("GOOGLE_API_SECRET")),
		GeoCityDatabase:         strings.TrimSpace(os.Getenv("GEOIP_CITY_DATABASE")),
		MCPEnabled:              boolEnv("MCP_ENABLED", true),
		ASNDatabase:             getenv("ASN_DATABASE", "/app/ip2asn.tsv.gz"),
		WorkerInterval:          5 * time.Second,
	}
	if cfg.ReplayS3.Endpoint != "" && (cfg.ReplayS3.Bucket == "" || cfg.ReplayS3.AccessKey == "" || cfg.ReplayS3.SecretKey == "") {
		return Config{}, errors.New("REPLAY_S3_BUCKET, REPLAY_S3_ACCESS_KEY and REPLAY_S3_SECRET_KEY are required with REPLAY_S3_ENDPOINT")
	}
	var err error
	if cfg.RawEventsTTL, err = durationEnv("RAW_EVENTS_TTL", 7*24*time.Hour); err != nil {
		return Config{}, err
	}
	if cfg.AggregateTTL, err = durationEnv("AGGREGATE_TTL", 180*24*time.Hour); err != nil {
		return Config{}, err
	}
	if cfg.StaleSessionAfter, err = durationEnv("STALE_SESSION_AFTER", 30*time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.FinishGrace, err = durationEnv("FINISH_GRACE", 20*time.Second); err != nil {
		return Config{}, err
	}
	cfg.SessionSecret = []byte(os.Getenv("SESSION_SECRET"))
	if key := os.Getenv("ENCRYPTION_KEY"); key != "" {
		cfg.EncryptionKey, err = encryptionKey(key)
		if err != nil {
			return Config{}, err
		}

	}
	if command == "migrate" || command == "backfill" {
		if cfg.DatabaseURL == "" {
			return Config{}, errors.New("DATABASE_URL is required")
		}
		return cfg, nil
	}
	switch {
	case command == "bootstrap" && cfg.BootstrapProjectKey == "":
		return Config{}, errors.New("BOOTSTRAP_PROJECT_KEY is required for repeatable bootstrap")
	case cfg.DatabaseURL == "":
		return Config{}, errors.New("DATABASE_URL is required")
	case command == "bootstrap" && cfg.AdminUsername == "":
		return Config{}, errors.New("ADMIN_USERNAME is required")
	case command == "bootstrap" && len(cfg.AdminPassword) < 12:
		return Config{}, errors.New("ADMIN_PASSWORD must contain at least 12 characters")
	case len(cfg.SessionSecret) < 32:
		return Config{}, errors.New("SESSION_SECRET must contain at least 32 bytes")
	case (command == "serve" || command == "worker" || command == "bootstrap") && len(cfg.EncryptionKey) != 32:
		return Config{}, errors.New("ENCRYPTION_KEY must decode to exactly 32 bytes")
	case cfg.RawEventsTTL < time.Hour:
		return Config{}, errors.New("RAW_EVENTS_TTL must be at least 1h")
	case cfg.AggregateTTL < 0:
		return Config{}, errors.New("AGGREGATE_TTL cannot be negative")
	case cfg.StaleSessionAfter < time.Minute:
		return Config{}, errors.New("STALE_SESSION_AFTER must be at least 1m")
	case cfg.FinishGrace < 0:
		return Config{}, errors.New("FINISH_GRACE cannot be negative")
	}
	return cfg, nil
}

func getenv(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
func boolEnv(k string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
func durationEnv(k string, fallback time.Duration) (time.Duration, error) {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", k, err)
	}
	return d, nil
}
func csv(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
func encryptionKey(v string) ([]byte, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, errors.New("ENCRYPTION_KEY is required")
	}
	if b, err := base64.StdEncoding.DecodeString(v); err == nil && len(b) == 32 {
		return b, nil
	}
	if len(v) == 32 {
		return []byte(v), nil
	}
	return nil, errors.New("ENCRYPTION_KEY must be base64 of 32 bytes or a raw 32-byte string")
}
