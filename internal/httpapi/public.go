package httpapi

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lamooof/clickshield/internal/asn"
	"github.com/lamooof/clickshield/internal/domain"
	"github.com/lamooof/clickshield/internal/geo"
	"github.com/lamooof/clickshield/internal/store"
)

var yandexID = regexp.MustCompile(`^[0-9]{1,32}$`)
var googleID = regexp.MustCompile(`^[0-9]{1,32}\.[0-9]{1,32}$`)
var allowedEvents = map[string]bool{"pageview": true, "heartbeat": true, "mousemove": true, "click": true, "scroll": true, "visibility": true, "touch": true, "automation": true, "business": true}

func (s *Server) preflight(w http.ResponseWriter, r *http.Request) {
	p, ok := s.publicProject(w, r)
	if !ok {
		return
	}
	if !setCORS(w, r, p.AllowedOrigins) {
		writeError(w, http.StatusForbidden, "origin is not allowed")
		return
	}
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Max-Age", "600")
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	p, ok := s.publicProject(w, r)
	if !ok {
		return
	}
	if !setCORS(w, r, p.AllowedOrigins) {
		writeError(w, http.StatusForbidden, "origin is not allowed")
		return
	}
	ip := IPFromHTTP(r, s.cfg.TrustProxyHeaders)
	var b domain.EventBatch
	if e := readJSON(w, r, &b, 256<<10); e != nil {
		writeError(w, 400, "invalid JSON payload")
		return
	}
	if msg := validateBatch(&b); msg != "" {
		writeError(w, 422, msg)
		return
	}
	if !s.eventLimiter.AllowN(p.ID+"|"+limitKey(ip), len(b.Events)) {
		writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}
	meta := domain.RemoteMeta{IP: ip, UserAgent: truncate(r.UserAgent(), 512), Origin: r.Header.Get("Origin")}
	if s.cfg.TrustProxyHeaders {
		meta.ASN = truncate(firstNonEmpty(r.Header.Get("X-GeoIP-ASN"), r.Header.Get("CF-ASN")), 32)
		meta.ASOrg = truncate(r.Header.Get("X-GeoIP-Organization"), 160)
		meta.Country = truncate(r.Header.Get("CF-IPCountry"), 8)
		meta.Region = truncate(r.Header.Get("X-GeoIP-Region"), 120)
		meta.City = truncate(r.Header.Get("X-GeoIP-City"), 120)
	}
	if location, ok := geo.Lookup(ip); ok {
		if meta.Country == "" {
			meta.Country = location.Country
		}
		if meta.Region == "" {
			meta.Region = location.Region
		}
		if meta.City == "" {
			meta.City = location.City
		}
	}
	// The AS country is where the network is registered, not where the
	// visitor is, so only the AS itself is taken from the database.
	if meta.ASN == "" && ip != nil {
		if info, ok := asn.Lookup(ip.String()); ok {
			meta.ASN, meta.ASOrg = strconv.FormatUint(uint64(info.Number), 10), truncate(info.Org, 160)
		}
	}
	result, e := s.service.Ingest(r.Context(), p, b, meta)
	if e != nil {
		s.log.Error("ingest", "error", e)
		writeError(w, 500, "could not store events")
		return
	}
	writeJSON(w, 202, result)
}
func (s *Server) finish(w http.ResponseWriter, r *http.Request) {
	p, ok := s.publicProject(w, r)
	if !ok {
		return
	}
	if !setCORS(w, r, p.AllowedOrigins) {
		writeError(w, 403, "origin is not allowed")
		return
	}
	var req domain.FinishRequest
	if e := readJSON(w, r, &req, 32<<10); e != nil {
		writeError(w, 400, "invalid JSON payload")
		return
	}
	if !store.ValidUUID(req.SessionID) || !store.ValidUUID(req.VisitorID) || !store.ValidUUID(req.BatchID) {
		writeError(w, 422, "session_id, visitor_id and batch_id must be UUIDs")
		return
	}
	if e := s.store.RequestFinish(r.Context(), p.ID, req); e != nil {
		if store.IsNotFound(e) {
			writeError(w, 404, "session not found")
			return
		}
		writeError(w, 500, "could not finish session")
		return
	}
	writeJSON(w, 202, map[string]any{"accepted": true})
}

// publicProject rate-limits the address before the key is looked up, so
// requests with random keys cannot load the database either.
func (s *Server) publicProject(w http.ResponseWriter, r *http.Request) (domain.Project, bool) {
	if !s.publicLimiter.Allow(limitKey(IPFromHTTP(r, s.cfg.TrustProxyHeaders))) {
		writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return domain.Project{}, false
	}
	key := strings.TrimSpace(r.URL.Query().Get("key"))
	if key == "" || len(key) > 128 {
		writeError(w, 401, "project key is required")
		return domain.Project{}, false
	}
	p, found, e := s.projectKeys.Get(r.Context(), key)
	if e != nil {
		s.log.Error("project lookup", "error", e)
		writeError(w, http.StatusServiceUnavailable, "project lookup failed")
		return domain.Project{}, false
	}
	if !found {
		writeError(w, 401, "invalid project key")
		return domain.Project{}, false
	}
	return p, true
}

// limitKey groups addresses the way one client holds them: an IPv4 address,
// or the /64 network an IPv6 host can rotate addresses within.
func limitKey(ip net.IP) string {
	if ip == nil {
		return ""
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
}
func setCORS(w http.ResponseWriter, r *http.Request, allowed []string) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	ok := false
	for _, a := range allowed {
		if a == "*" || strings.EqualFold(strings.TrimRight(a, "/"), strings.TrimRight(origin, "/")) {
			ok = true
			break
		}
	}
	if ok {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Add("Vary", "Origin")
	}
	return ok
}
func validateBatch(b *domain.EventBatch) string {
	if !store.ValidUUID(b.SessionID) || !store.ValidUUID(b.VisitorID) || !store.ValidUUID(b.BatchID) {
		return "session_id, visitor_id and batch_id must be UUIDs"
	}
	if len(b.Events) > 200 {
		return "no more than 200 events per batch"
	}
	if len(b.Events) == 0 {
		return "events cannot be empty"
	}
	b.PageURL = sanitizeURL(b.PageURL)
	b.LandingURL = sanitizeURL(b.LandingURL)
	b.Referrer = sanitizeURL(b.Referrer)
	b.Fingerprint = truncate(b.Fingerprint, 128)
	b.YCLID = truncate(b.YCLID, 256)
	b.GCLID = truncate(b.GCLID, 256)
	b.RBClickID = truncate(b.RBClickID, 256)
	b.UTM.Source = truncate(b.UTM.Source, 256)
	b.UTM.Medium = truncate(b.UTM.Medium, 256)
	b.UTM.Campaign = truncate(b.UTM.Campaign, 256)
	b.UTM.Term = truncate(b.UTM.Term, 256)
	b.UTM.Content = truncate(b.UTM.Content, 256)
	if b.YandexClientID != "" && !yandexID.MatchString(b.YandexClientID) {
		b.YandexClientID = ""
	}
	if b.GoogleClientID != "" && !googleID.MatchString(b.GoogleClientID) {
		b.GoogleClientID = ""
	}
	if b.PageIndex < 1 {
		b.PageIndex = 1
	}
	if b.PageIndex > 1000 {
		b.PageIndex = 1000
	}
	b.Device.Type = truncate(b.Device.Type, 16)
	b.Device.Platform = truncate(b.Device.Platform, 80)
	b.Device.Language = truncate(b.Device.Language, 32)
	b.Device.Timezone = truncate(b.Device.Timezone, 80)
	if b.Device.ScreenWidth < 0 || b.Device.ScreenWidth > 20000 {
		b.Device.ScreenWidth = 0
	}
	if b.Device.ScreenHeight < 0 || b.Device.ScreenHeight > 20000 {
		b.Device.ScreenHeight = 0
	}
	if b.Device.ColorDepth < 0 || b.Device.ColorDepth > 128 {
		b.Device.ColorDepth = 0
	}
	if b.Device.HardwareConcurrency < 0 || b.Device.HardwareConcurrency > 1024 {
		b.Device.HardwareConcurrency = 0
	}
	if b.Device.DeviceMemory < 0 || b.Device.DeviceMemory > 1024 {
		b.Device.DeviceMemory = 0
	}
	if b.Device.MaxTouchPoints < 0 || b.Device.MaxTouchPoints > 100 {
		b.Device.MaxTouchPoints = 0
	}
	knownAutomation := map[string]bool{"webdriver": true, "selenium": true, "phantom": true, "playwright": true, "puppeteer": true}
	cleanAutomation := make(map[string]bool, len(b.Automation))
	for key, value := range b.Automation {
		if knownAutomation[key] && value {
			cleanAutomation[key] = true
		}
	}
	b.Automation = cleanAutomation
	nowMS := time.Now().UnixMilli()
	minTS := nowMS - int64((7*24*time.Hour)/time.Millisecond)
	maxTS := nowMS + int64((10*time.Minute)/time.Millisecond)
	for i := range b.Events {
		e := &b.Events[i]
		if !allowedEvents[e.Type] {
			return "unsupported event type: " + e.Type
		}
		if e.TS < minTS || e.TS > maxTS {
			e.TS = nowMS
		}
		if e.Type == "mousemove" || e.Type == "click" {
			e.X = clamp(e.X, -10000, 100000)
			e.Y = clamp(e.Y, -10000, 100000)
			e.VW = clamp(e.VW, 0, 20000)
			e.VH = clamp(e.VH, 0, 20000)
		}
		e.Pointer = truncate(e.Pointer, 16)
		if e.Button < 0 || e.Button > 5 {
			e.Button = 0
		}
		if e.PageIndex < 0 {
			e.PageIndex = 0
		} else if e.PageIndex > 1000 {
			e.PageIndex = 1000
		}
		e.VW = clamp(e.VW, 0, 20000)
		e.VH = clamp(e.VH, 0, 20000)
		e.ScrollY = clamp(e.ScrollY, -1000000, 10000000)
		e.Depth = clamp(e.Depth, 0, 1)
		if e.ActiveMS < 0 {
			e.ActiveMS = 0
		}
		if e.IdleMS < 0 {
			e.IdleMS = 0
		}
		if e.ActiveMS > 600000 {
			e.ActiveMS = 600000
		}
		if e.IdleMS > 600000 {
			e.IdleMS = 600000
		}
		e.PageURL = sanitizeURL(e.PageURL)
		e.DocumentHeight = clamp(e.DocumentHeight, 0, 10000000)
		if e.Type == "business" {
			e.Name = strings.TrimSpace(e.Name)
			if e.Name == "" || utf8.RuneCountInString(e.Name) > 80 || len(e.Name) > 320 {
				return "business event name must contain 1–80 characters"
			}
			if len(e.Properties) > 32 {
				return "no more than 32 event properties"
			}
			for k, v := range e.Properties {
				if strings.TrimSpace(k) == "" || len(k) > 80 {
					return "invalid property name"
				}
				switch val := v.(type) {
				case nil, bool, float64:
				case string:
					if len(val) > 1024 {
						return "property value too long"
					}
				default:
					return "event properties must be scalar values"
				}
			}
			raw, err := json.Marshal(e.Properties)
			if err != nil || len(raw) > 8192 {
				return "event properties exceed 8 KiB"
			}
		} else {
			e.Name = ""
			e.Properties = nil
		}
		switch e.Type {
		case "pageview":
			raw, _ := e.Extra["page_url"].(string)
			e.Extra = map[string]any{"page_url": sanitizeURL(raw)}
		case "touch":
			touches := 0
			if raw, ok := e.Extra["touches"].(float64); ok && raw > 0 {
				touches = int(raw)
			}
			if touches > 10 {
				touches = 10
			}
			e.Extra = map[string]any{"touches": touches}
		default:
			e.Extra = nil
		}
	}
	return ""
}
func IPFromHTTP(r *http.Request, trust bool) net.IP {
	if trust {
		// The production reverse proxy replaces X-Real-IP with its own $remote_addr.
		// Never use client supplied CF-Connecting-IP or X-Forwarded-For here.
		if ip := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); ip != nil {
			return ip
		}
	}
	return store.IPFromRequest(r.RemoteAddr)
}
func sanitizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Opaque != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	u.User = nil
	u.Fragment = ""
	input := u.Query()
	clean := url.Values{}
	for _, key := range []string{"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content", "yclid", "gclid", "rb_clickid"} {
		if value := input.Get(key); value != "" {
			clean.Set(key, truncate(value, 256))
		}
	}
	u.RawQuery = clean.Encode()
	return truncate(u.String(), 2048)
}

func truncate(v string, n int) string {
	runes := []rune(strings.TrimSpace(v))
	if len(runes) > n {
		runes = runes[:n]
	}
	return string(runes)
}
func firstNonEmpty(v ...string) string {
	for _, x := range v {
		if strings.TrimSpace(x) != "" {
			return x
		}
	}
	return ""
}
func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
