package httpapi

import (
	"encoding/csv"
	"encoding/json"
	"github.com/lamooof/clickshield/internal/analytics"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/lamooof/clickshield/internal/bots"
	"github.com/lamooof/clickshield/internal/domain"
	"github.com/lamooof/clickshield/internal/scoring"
	"github.com/lamooof/clickshield/internal/store"
)

var googleMeasurementID = regexp.MustCompile(`^G-[A-Z0-9]{4,32}$`)

type projectRequest struct {
	Name           string          `json:"name"`
	AllowedOrigins []string        `json:"allowed_origins"`
	Mode           string          `json:"mode"`
	ScoringConfig  json.RawMessage `json:"scoring_config"`
}

func (s *Server) projects(w http.ResponseWriter, r *http.Request) {
	items, e := s.store.Projects(r.Context())
	if e != nil {
		writeError(w, 500, "could not load projects")
		return
	}
	if p, ok := principalFrom(r.Context()); ok && p.ProjectID != "" {
		items = slices.DeleteFunc(items, func(x domain.Project) bool { return x.ID != p.ProjectID })
	}
	writeJSON(w, 200, map[string]any{"projects": items})
}
func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	var req projectRequest
	if e := readJSON(w, r, &req, 128<<10); e != nil {
		writeError(w, 400, e.Error())
		return
	}
	req.Name = truncate(req.Name, 120)
	if req.Name == "" {
		writeError(w, 422, "name is required")
		return
	}
	req.AllowedOrigins = normalizeOrigins(req.AllowedOrigins)
	if len(req.AllowedOrigins) == 0 {
		writeError(w, 422, "at least one allowed origin is required")
		return
	}
	if len(req.ScoringConfig) == 0 {
		req.ScoringConfig, _ = json.Marshal(scoring.DefaultConfig())
	}
	if e := scoring.Validate(req.ScoringConfig); e != nil {
		writeError(w, 422, e.Error())
		return
	}
	p, e := s.store.CreateProject(r.Context(), req.Name, req.AllowedOrigins, req.Mode, req.ScoringConfig)
	if e != nil {
		writeError(w, 422, e.Error())
		return
	}
	s.projectKeys.Reset()
	a, _ := adminFrom(r.Context())
	s.store.Audit(r.Context(), a.UserID, "CREATE", "project", p.ID, map[string]any{"name": p.Name})
	writeJSON(w, 201, p)
}
func (s *Server) updateProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !store.ValidUUID(id) {
		writeError(w, 400, "invalid project id")
		return
	}
	var req projectRequest
	if e := readJSON(w, r, &req, 128<<10); e != nil {
		writeError(w, 400, e.Error())
		return
	}
	req.Name = truncate(req.Name, 120)
	req.AllowedOrigins = normalizeOrigins(req.AllowedOrigins)
	if req.Name == "" || len(req.AllowedOrigins) == 0 {
		writeError(w, 422, "name and allowed_origins are required")
		return
	}
	if len(req.ScoringConfig) == 0 {
		writeError(w, 422, "scoring_config is required")
		return
	}
	if e := scoring.Validate(req.ScoringConfig); e != nil {
		writeError(w, 422, e.Error())
		return
	}
	if e := s.store.UpdateProject(r.Context(), id, req.Name, req.AllowedOrigins, req.Mode, req.ScoringConfig); e != nil {
		writeError(w, 422, e.Error())
		return
	}
	s.projectKeys.Reset()
	a, _ := adminFrom(r.Context())
	s.store.Audit(r.Context(), a.UserID, "UPDATE", "project", id, map[string]any{"name": req.Name, "mode": req.Mode})
	p, _ := s.store.Project(r.Context(), id)
	writeJSON(w, 200, p)
}
func normalizeOrigins(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range values {
		raw = strings.TrimSpace(raw)
		if raw == "*" {
			if !seen[raw] {
				out = append(out, raw)
				seen[raw] = true
			}
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || u.RawPath != "" {
			continue
		}
		scheme := strings.ToLower(u.Scheme)
		host := strings.ToLower(u.Hostname())
		localHTTP := scheme == "http" && (host == "localhost" || host == "127.0.0.1" || host == "::1")
		if scheme != "https" && !localHTTP {
			continue
		}
		normalized := scheme + "://" + strings.ToLower(u.Host)
		if seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, normalized)
		if len(out) >= 50 {
			break
		}
	}
	return out
}

func (s *Server) integrations(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !store.ValidUUID(id) {
		writeError(w, 400, "invalid project id")
		return
	}
	items, e := s.store.Integrations(r.Context(), id)
	if e != nil {
		writeError(w, 500, "could not load integrations")
		return
	}
	for i := range items {
		items[i].SecretEncrypted = nil
	}
	writeJSON(w, 200, map[string]any{"integrations": items})
}

type integrationRequest struct {
	PixelID          string `json:"pixel_id"`
	Enabled          bool   `json:"enabled"`
	CounterID        string `json:"counter_id"`
	MeasurementToken string `json:"measurement_token"`
	MeasurementID    string `json:"measurement_id"`
	APISecret        string `json:"api_secret"`
}

func (s *Server) updateIntegration(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	provider := strings.ToLower(r.PathValue("provider"))
	if !store.ValidUUID(projectID) {
		writeError(w, 400, "invalid project id")
		return
	}
	var req integrationRequest
	if e := readJSON(w, r, &req, 64<<10); e != nil {
		writeError(w, 400, e.Error())
		return
	}
	var dbProvider string
	var cfg any
	var secret []byte
	var e error
	req.MeasurementToken = truncate(req.MeasurementToken, 2048)
	req.APISecret = truncate(req.APISecret, 512)
	switch provider {
	case "yandex", "yandex_metrica":
		dbProvider = "YANDEX_METRICA"
		req.CounterID = truncate(req.CounterID, 32)
		if req.CounterID != "" && !yandexID.MatchString(req.CounterID) {
			writeError(w, 422, "counter_id must contain digits only")
			return
		}
		if req.Enabled && req.CounterID == "" {
			writeError(w, 422, "counter_id is required when Yandex integration is enabled")
			return
		}
		cfg = map[string]any{"counter_id": req.CounterID}
		if req.MeasurementToken != "" {
			secret, e = s.cipher.EncryptJSON(map[string]any{"measurement_token": req.MeasurementToken})
		}
	case "google", "google_analytics":
		dbProvider = "GOOGLE_ANALYTICS"
		req.MeasurementID = strings.ToUpper(truncate(req.MeasurementID, 64))
		if req.MeasurementID != "" && !googleMeasurementID.MatchString(req.MeasurementID) {
			writeError(w, 422, "measurement_id must look like G-XXXXXXXXXX")
			return
		}
		if req.Enabled && req.MeasurementID == "" {
			writeError(w, 422, "measurement_id is required when Google integration is enabled")
			return
		}
		cfg = map[string]any{"measurement_id": req.MeasurementID}
		if req.APISecret != "" {
			secret, e = s.cipher.EncryptJSON(map[string]any{"api_secret": req.APISecret})
		}
	case "vk", "vk_ads":
		dbProvider = "VK_ADS"
		req.PixelID = strings.TrimSpace(req.PixelID)
		if (req.PixelID != "" && !yandexID.MatchString(req.PixelID)) || (req.Enabled && req.PixelID == "") {
			writeError(w, 422, "numeric pixel_id is required when VK integration is enabled")
			return
		}
		cfg = map[string]any{"pixel_id": req.PixelID}
	default:
		writeError(w, 400, "unknown provider")
		return
	}
	if e != nil {
		writeError(w, 500, "could not encrypt secret")
		return
	}
	hasSecret := len(secret) > 0
	if req.Enabled && dbProvider != "VK_ADS" && !hasSecret {
		items, loadErr := s.store.Integrations(r.Context(), projectID)
		if loadErr != nil {
			writeError(w, 500, "could not inspect existing integration")
			return
		}
		for _, item := range items {
			if item.Provider == dbProvider && item.HasSecret {
				hasSecret = true
				break
			}
		}
	}
	if req.Enabled && dbProvider != "VK_ADS" && !hasSecret {
		writeError(w, 422, "a backend secret is required when the integration is enabled")
		return
	}
	if e = s.store.UpsertIntegration(r.Context(), projectID, dbProvider, req.Enabled, cfg, secret); e != nil {
		writeError(w, 422, e.Error())
		return
	}
	a, _ := adminFrom(r.Context())
	s.store.Audit(r.Context(), a.UserID, "UPDATE_INTEGRATION", "project", projectID, map[string]any{"provider": dbProvider, "enabled": req.Enabled})
	writeJSON(w, 200, map[string]any{"ok": true})
}
func (s *Server) testIntegration(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if !store.ValidUUID(projectID) {
		writeError(w, 400, "invalid project id")
		return
	}
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := readJSON(w, r, &req, 4096); err != nil || !store.ValidUUID(req.SessionID) {
		writeError(w, 422, "valid session_id required")
		return
	}
	if pid, e := s.store.SessionProject(r.Context(), req.SessionID); e != nil || pid != projectID {
		writeError(w, 404, "session not found in this project")
		return
	}
	provider := strings.ToLower(r.PathValue("provider"))
	providerName := map[string]string{"google": "GOOGLE_ANALYTICS", "yandex": "YANDEX_METRICA", "vk": "VK_ADS", "vk_ads": "VK_ADS"}[provider]
	if providerName == "" {
		writeError(w, 400, "unknown provider")
		return
	}
	items, err := s.store.Integrations(r.Context(), projectID)
	if err != nil {
		writeError(w, 500, "could not load integration")
		return
	}
	for _, item := range items {
		if item.Provider == providerName {
			outcome, err := s.service.TestIntegration(r.Context(), item, req.SessionID)
			if err != nil {
				writeError(w, 422, err.Error())
				return
			}
			a, _ := adminFrom(r.Context())
			s.store.Audit(r.Context(), a.UserID, "TEST_INTEGRATION", "project", projectID, map[string]any{"provider": providerName, "session_id": req.SessionID, "status": outcome.Status})
			writeJSON(w, 200, map[string]any{"status": outcome.Status, "message": outcome.Message})
			return
		}
	}
	writeError(w, 404, "integration not configured")
}
func (s *Server) sessions(w http.ResponseWriter, r *http.Request) {
	filter, ok := sessionFilters(w, r)
	if !ok {
		return
	}
	limit := intParam(r, "limit", 100, 1, 200)
	offset := intParam(r, "offset", 0, 0, 1000000)
	items, e := s.store.ListSessions(r.Context(), filter, limit, offset)
	if e != nil {
		writeError(w, 500, "could not load sessions")
		return
	}
	writeJSON(w, 200, map[string]any{"sessions": items, "limit": limit, "offset": offset})
}

func sessionFilters(w http.ResponseWriter, r *http.Request) (store.SessionFilter, bool) {
	q := r.URL.Query()
	f := store.SessionFilter{ProjectID: q.Get("project_id"), Class: strings.ToUpper(q.Get("classification")), BotName: truncate(q.Get("bot"), 100), BotCategory: q.Get("bot_category")}
	if f.ProjectID != "" && !store.ValidUUID(f.ProjectID) {
		writeError(w, 400, "invalid project_id")
		return store.SessionFilter{}, false
	}
	switch f.Class {
	case "", domain.Human, domain.Suspicious, domain.Bot, domain.KnownBot, "UNCLASSIFIED", "NO_BOTS":
	default:
		writeError(w, 400, "invalid classification")
		return store.SessionFilter{}, false
	}
	switch f.BotCategory {
	case "", bots.Search, bots.AI, bots.Social, bots.SEO, bots.Ads, bots.Monitoring, bots.Feeds, bots.Security, bots.Other, bots.Automation:
	default:
		writeError(w, 400, "invalid bot_category")
		return store.SessionFilter{}, false
	}
	switch q.Get("paid_only") {
	case "", "false":
	case "true":
		f.PaidOnly = true
	default:
		writeError(w, 400, "invalid paid_only")
		return store.SessionFilter{}, false
	}
	f.Source = truncate(q.Get("source"), 256)
	f.Channel = truncate(q.Get("channel"), 80)
	f.Campaign = truncate(q.Get("campaign"), 256)
	f.CampaignSet = q.Has("campaign")
	f.Device = truncate(q.Get("device"), 80)
	f.DeviceSet = q.Has("device")
	f.LandingURL = truncate(q.Get("landing_url"), 2048)
	for key, values := range q {
		if analytics.FilterableDimension(key) && len(values) > 0 {
			if f.Dims == nil {
				f.Dims = map[string]string{}
			}
			f.Dims[key] = truncate(values[0], 2048)
		}
	}
	switch q.Get("replay_only") {
	case "", "false":
	case "true":
		f.ReplayOnly = true
	default:
		writeError(w, 400, "invalid replay_only")
		return f, false
	}
	for _, pair := range []struct {
		name string
		dst  **time.Time
	}{{"from", &f.From}, {"to", &f.To}} {
		if raw := q.Get(pair.name); raw != "" {
			t, e := time.Parse(time.RFC3339, raw)
			if e != nil {
				writeError(w, 400, "dates must use RFC3339")
				return f, false
			}
			*pair.dst = &t
		}
	}
	if f.From != nil && f.To != nil && !f.From.Before(*f.To) {
		writeError(w, 400, "from must be before to")
		return f, false
	}
	return f, true
}

func (s *Server) exportSessions(w http.ResponseWriter, r *http.Request) {
	filter, ok := sessionFilters(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="clickshield-sessions.csv"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte{0xef, 0xbb, 0xbf})
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"session_id", "started_at", "project", "classification", "risk_score", "duration_ms", "click_count", "scroll_depth", "source", "utm_medium", "utm_campaign", "ip", "asn", "as_org", "fingerprint", "yclid", "gclid", "yandex_client_id", "google_client_id", "device_type", "manual_label", "bot_name", "bot_category", "rb_clickid", "channel", "referrer_host", "landing_url", "has_replay"})
	rows := 0
	err := s.store.IterateSessions(r.Context(), filter, 0, 0, func(x domain.SessionItem) error {
		rows++
		source := x.TrafficSource
		fields := []string{x.ID, x.StartedAt.UTC().Format(time.RFC3339), x.ProjectName, x.Classification, strconv.Itoa(x.RiskScore), strconv.FormatInt(x.DurationMS, 10), strconv.Itoa(x.ClickCount), strconv.FormatFloat(x.ScrollDepth, 'f', -1, 64), source, x.UTMMedium, x.UTMCampaign, x.IP, x.ASN, x.ASOrg, x.Fingerprint, x.YCLID, x.GCLID, x.YandexClientID, x.GoogleClientID, x.DeviceType, x.ManualLabel, x.BotName, x.BotCategory, x.RBClickID, x.TrafficChannel, x.ReferrerHost, x.LandingURL, strconv.FormatBool(x.HasReplay)}
		for i := range fields {
			fields[i] = safeCSVCell(fields[i])
		}
		return cw.Write(fields)
	})
	cw.Flush()
	if err == nil {
		err = cw.Error()
	}
	if err != nil {
		s.log.Error("session csv export", "error", err)
	}
	// The export holds personal data (IP, fingerprint, advertising IDs), so
	// every download is recorded with its filter and size.
	a, _ := adminFrom(r.Context())
	s.store.Audit(r.Context(), a.UserID, "EXPORT", "sessions", "csv", map[string]any{"project_id": filter.ProjectID, "classification": filter.Class, "bot": filter.BotName, "bot_category": filter.BotCategory, "paid_only": filter.PaidOnly, "rows": rows, "ip": IPFromHTTP(r, s.cfg.TrustProxyHeaders).String(), "complete": err == nil})
}
func safeCSVCell(value string) string {
	if value != "" && strings.ContainsRune("=+-@\t\r\n", rune(value[0])) {
		return "'" + value
	}
	return value
}
func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !store.ValidUUID(id) {
		writeError(w, 400, "invalid session id")
		return
	}
	d, e := s.store.SessionDetail(r.Context(), id)
	if e != nil {
		if store.IsNotFound(e) {
			writeError(w, 404, "session not found")
			return
		}
		s.log.Error("session detail", "error", e)
		writeError(w, 500, "could not load session")
		return
	}
	writeJSON(w, 200, d)
}
func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	filter, ok := sessionFilters(w, r)
	if !ok {
		return
	}
	st, e := s.analytics.FilteredStats(r.Context(), filter)
	if e != nil {
		writeError(w, 500, "could not load stats")
		return
	}
	writeJSON(w, 200, st)
}

// botStats counts sessions per known bot and category for the dashboard.
func (s *Server) botStats(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project_id")
	if project != "" && !store.ValidUUID(project) {
		writeError(w, 400, "invalid project_id")
		return
	}
	items, e := s.store.BotStats(r.Context(), project)
	if e != nil {
		s.log.Error("bot stats", "error", e)
		writeError(w, 500, "could not load bot stats")
		return
	}
	categories := map[string]int64{}
	for _, x := range items {
		categories[x.Category] += x.Sessions
	}
	writeJSON(w, 200, map[string]any{"bots": items, "categories": categories, "catalog_size": len(bots.Catalog())})
}

type labelRequest struct {
	Label string `json:"label"`
	Note  string `json:"note"`
}

func (s *Server) label(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !store.ValidUUID(id) {
		writeError(w, 400, "invalid session id")
		return
	}
	var req labelRequest
	if e := readJSON(w, r, &req, 16<<10); e != nil {
		writeError(w, 400, e.Error())
		return
	}
	req.Label = strings.ToUpper(req.Label)
	if req.Label != domain.Bot && req.Label != domain.Human {
		writeError(w, 422, "label must be BOT or HUMAN")
		return
	}
	req.Note = truncate(req.Note, 500)
	a, _ := adminFrom(r.Context())
	classID, e := s.service.Label(r.Context(), id, req.Label, req.Note, a.UserID)
	if e != nil {
		writeError(w, 422, e.Error())
		return
	}
	s.store.Audit(r.Context(), a.UserID, "LABEL", "session", id, map[string]any{"label": req.Label, "note": req.Note})
	writeJSON(w, 200, map[string]any{"classification_id": classID, "label": req.Label})
}
func (s *Server) outbox(w http.ResponseWriter, r *http.Request) {
	counts, e := s.store.OutboxCounts(r.Context())
	if e != nil {
		writeError(w, 500, "could not load outbox")
		return
	}
	writeJSON(w, 200, map[string]any{"counts": counts})
}
func (s *Server) operations(w http.ResponseWriter, r *http.Request) {
	state, err := s.store.Operations(r.Context())
	if err != nil {
		writeError(w, 500, "could not load operational status")
		return
	}
	writeJSON(w, 200, state)
}
func intParam(r *http.Request, name string, def, min, max int) int {
	v, e := strconv.Atoi(r.URL.Query().Get(name))
	if e != nil {
		return def
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
