package httpapi

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/lamooof/clickshield/internal/config"
	"github.com/lamooof/clickshield/internal/cryptox"
	"github.com/lamooof/clickshield/internal/integration"
	"github.com/lamooof/clickshield/internal/service"
	"github.com/lamooof/clickshield/internal/store"
)

//go:embed assets/*
var assets embed.FS

type Server struct {
	cfg            config.Config
	store          *store.Store
	service        *service.Service
	cipher         *cryptox.Cipher
	log            *slog.Logger
	mux            *http.ServeMux
	loginLimiter   *limiter
	userLimiter    *limiter
	publicLimiter  *limiter
	eventLimiter   *limiter
	replayLimiter  *limiter
	metrikaLimiter *limiter
	metrikaReader  *integration.MetrikaReader
	projectKeys    *projectCache
}
type limiter struct {
	mu      sync.Mutex
	entries map[string]limitEntry
	max     int
	window  time.Duration
}
type limitEntry struct {
	count int
	reset time.Time
}

func newLimiter(max int, window time.Duration) *limiter {
	return &limiter{entries: map[string]limitEntry{}, max: max, window: window}
}

// maxLimiterEntries bounds the memory of one limiter. Past it, expired
// entries are dropped first and the whole window is forgotten only when the
// keys are still live, so a flood of distinct addresses cannot exhaust memory.
const maxLimiterEntries = 100000

func (l *limiter) Allow(key string) bool { return l.AllowN(key, 1) }

// AllowN spends n units of the key's budget for the current window.
func (l *limiter) AllowN(key string, n int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	e := l.entries[key]
	if now.After(e.reset) {
		e = limitEntry{0, now.Add(l.window)}
	}
	e.count += n
	l.entries[key] = e
	if len(l.entries) > maxLimiterEntries {
		for k, v := range l.entries {
			if now.After(v.reset) {
				delete(l.entries, k)
			}
		}
		if len(l.entries) > maxLimiterEntries {
			l.entries = map[string]limitEntry{key: e}
		}
	}
	return e.count <= l.max
}

func New(cfg config.Config, st *store.Store, svc *service.Service, c *cryptox.Cipher, log *slog.Logger) *Server {
	s := &Server{
		cfg: cfg, store: st, service: svc, cipher: c, log: log, mux: http.NewServeMux(),
		// Login attempts per address and per user name, so a botnet cannot
		// spread a password guess over many addresses.
		loginLimiter: newLimiter(10, 15*time.Minute),
		userLimiter:  newLimiter(50, 15*time.Minute),
		// Public requests per address (an IPv6 /64), checked before the
		// project key is looked up, and stored events per project and address.
		publicLimiter:  newLimiter(600, time.Minute),
		eventLimiter:   newLimiter(20000, time.Minute),
		replayLimiter:  newLimiter(32<<20, time.Minute),
		metrikaLimiter: newLimiter(30, time.Minute),
		metrikaReader:  integration.NewMetrikaReader(),
		projectKeys:    newProjectCache(st, 30*time.Second),
	}
	s.routes()
	return s
}
func (s *Server) Handler() http.Handler { return s.recover(s.security(s.logRequests(s.mux))) }
func (s *Server) routes() {
	if s.cfg.MCPEnabled {
		s.mux.Handle("/mcp", s.mcpHandler())
	}
	s.mux.HandleFunc("GET /l/{slug}", s.shortLink)
	s.mux.HandleFunc("GET /p/{pixel}", s.pixel)
	s.mux.HandleFunc("GET /v1/projects/{id}/analytics-keys", s.requireAPI(s.analyticsKeys))
	s.mux.HandleFunc("POST /v1/projects/{id}/analytics-keys", s.requireAPI(s.csrf(s.analyticsKeys)))
	s.mux.HandleFunc("DELETE /v1/projects/{id}/analytics-keys/{key}", s.requireAPI(s.csrf(s.revokeAnalyticsKey)))
	s.mux.HandleFunc("GET /v1/projects/{id}/tools/{kind}", s.requireAnalytics(s.analyticsTools))
	s.mux.HandleFunc("POST /v1/projects/{id}/tools/{kind}", s.requireAPI(s.csrf(s.analyticsTools)))
	s.mux.HandleFunc("PUT /v1/projects/{id}/tools/{kind}/{tool}", s.requireAPI(s.csrf(s.analyticsTools)))
	s.mux.HandleFunc("DELETE /v1/projects/{id}/tools/{kind}/{tool}", s.requireAPI(s.csrf(s.analyticsTools)))
	s.mux.HandleFunc("GET /v1/analytics/{kind}", s.requireAnalytics(s.productReport))
	s.mux.HandleFunc("GET /v1/projects/{id}/definitions", s.requireAnalytics(s.definitions))
	s.mux.HandleFunc("POST /v1/projects/{id}/definitions", s.requireAPI(s.csrf(s.saveDefinition)))
	s.mux.HandleFunc("PUT /v1/projects/{id}/definitions/{definition}", s.requireAPI(s.csrf(s.saveDefinition)))
	s.mux.HandleFunc("DELETE /v1/projects/{id}/definitions/{definition}", s.requireAPI(s.csrf(s.deleteDefinition)))
	s.mux.HandleFunc("GET /healthz", s.health)
	s.mux.HandleFunc("GET /v1/projects/{id}/metrika-read", s.requireAPI(s.metrikaConnection))
	s.mux.HandleFunc("PUT /v1/projects/{id}/metrika-read", s.requireAPI(s.csrf(s.saveMetrikaConnection)))
	s.mux.HandleFunc("DELETE /v1/projects/{id}/metrika-read", s.requireAPI(s.csrf(s.deleteMetrikaConnection)))
	s.mux.HandleFunc("GET /v1/projects/{id}/metrika-report", s.requireAPI(s.metrikaReport))
	s.mux.HandleFunc("GET /v1/tracker/config", s.trackerConfig)
	s.mux.HandleFunc("OPTIONS /v1/replay", s.preflight)
	s.mux.HandleFunc("POST /v1/replay", s.replayIngest)
	s.mux.HandleFunc("GET /v1/traffic", s.requireAnalytics(s.trafficReport))
	s.mux.HandleFunc("GET /v1/reports", s.requireAnalytics(s.aggregateReport))
	s.mux.HandleFunc("GET /v1/reports/timeline", s.requireAnalytics(s.reportTimeline))
	s.mux.HandleFunc("PUT /v1/projects/{id}/replay", s.requireAPI(s.csrf(s.replaySettings)))
	s.mux.HandleFunc("GET /v1/sessions/{id}/replay", s.requireAPI(s.replayData))
	s.mux.HandleFunc("DELETE /v1/sessions/{id}/replay", s.requireAPI(s.csrf(s.deleteReplay)))
	s.mux.HandleFunc("GET /tracker.js", s.tracker)
	s.mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/assets/favicon.svg", http.StatusFound)
	})
	s.mux.HandleFunc("OPTIONS /v1/events", s.preflight)
	s.mux.HandleFunc("POST /v1/events", s.events)
	s.mux.HandleFunc("OPTIONS /v1/session/finish", s.preflight)
	s.mux.HandleFunc("POST /v1/session/finish", s.finish)
	s.mux.HandleFunc("POST /v1/outcomes", s.serverOutcome)
	s.mux.HandleFunc("GET /login", s.loginPage)
	s.mux.HandleFunc("POST /login", s.login)
	s.mux.HandleFunc("GET /{$}", s.requirePage(s.index))
	s.mux.HandleFunc("GET /analytics", s.requirePage(s.analyticsPage))
	s.mux.HandleFunc("GET /assets/{name}", s.asset)
	s.mux.HandleFunc("GET /v1/me", s.requireAPI(s.me))
	s.mux.HandleFunc("POST /logout", s.requireAPI(s.csrf(s.logout)))
	s.mux.HandleFunc("GET /v1/projects", s.requireAPI(s.projects))
	s.mux.HandleFunc("POST /v1/projects", s.requireAPI(s.csrf(s.createProject)))
	s.mux.HandleFunc("PUT /v1/projects/{id}", s.requireAPI(s.csrf(s.updateProject)))
	s.mux.HandleFunc("GET /v1/projects/{id}/integrations", s.requireAPI(s.integrations))
	s.mux.HandleFunc("PUT /v1/projects/{id}/integrations/{provider}", s.requireAPI(s.csrf(s.updateIntegration)))
	s.mux.HandleFunc("POST /v1/projects/{id}/integrations/{provider}/test", s.requireAPI(s.csrf(s.testIntegration)))
	s.mux.HandleFunc("GET /v1/sessions", s.requireAPI(s.sessions))
	s.mux.HandleFunc("GET /v1/sessions/export.csv", s.requireAPI(s.exportSessions))
	s.mux.HandleFunc("GET /v1/sessions/{id}", s.requireAPI(s.session))
	s.mux.HandleFunc("POST /v1/sessions/{id}/label", s.requireAPI(s.csrf(s.label)))
	s.mux.HandleFunc("GET /v1/stats", s.requireAPI(s.stats))
	s.mux.HandleFunc("GET /v1/bots", s.requireAPI(s.botStats))
	s.mux.HandleFunc("GET /v1/quality", s.requireAPI(s.quality))
	s.mux.HandleFunc("POST /v1/sessions/{id}/outcomes", s.requireAPI(s.csrf(s.adminOutcome)))
	s.mux.HandleFunc("GET /v1/outbox", s.requireAPI(s.outbox))
	s.mux.HandleFunc("GET /v1/operations", s.requireAPI(s.operations))
}
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if e := s.store.Pool.Ping(ctx); e != nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}
func (s *Server) tracker(w http.ResponseWriter, r *http.Request) {
	b, e := assets.ReadFile("assets/tracker.js")
	if e != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(b)
}
func (s *Server) asset(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if strings.Contains(name, "/") || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	b, e := assets.ReadFile("assets/" + name)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	switch {
	case strings.HasSuffix(name, ".js"):
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case strings.HasSuffix(name, ".css"):
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case strings.HasSuffix(name, ".html"):
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case strings.HasSuffix(name, ".svg"):
		w.Header().Set("Content-Type", "image/svg+xml")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(b)
}
func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	b, e := assets.ReadFile("assets/admin.html")
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b)
}
func (s *Server) analyticsPage(w http.ResponseWriter, r *http.Request) {
	b, e := assets.ReadFile("assets/analytics.html")
	if e != nil {
		http.Error(w, "could not load analytics", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b)
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}
func readJSON(w http.ResponseWriter, r *http.Request, dst any, max int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, max)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if e := dec.Decode(dst); e != nil {
		return e
	}
	var extra any
	e := dec.Decode(&extra)
	if e == nil {
		return fmt.Errorf("multiple JSON values")
	}
	if !errors.Is(e, io.EOF) {
		return e
	}
	return nil
}
func (s *Server) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("panic", "value", rec, "stack", string(debug.Stack()))
				writeError(w, 500, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
func (s *Server) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		if r.URL.Path == "/assets/replay.html" {
			w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		}
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if strings.HasPrefix(r.URL.Path, "/v1/") || r.URL.Path == "/login" || r.URL.Path == "/" {
			w.Header().Set("Cache-Control", "no-store")
		}
		if s.cfg.CookieSecure && r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		if r.URL.Path != "/tracker.js" && !strings.HasPrefix(r.URL.Path, "/v1/events") && !strings.HasPrefix(r.URL.Path, "/v1/session/finish") {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		}

		if r.URL.Path == "/assets/replay.html" {
			w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline' https:; img-src https: data:; font-src https: data:; frame-src 'self' blob:; connect-src 'none'; frame-ancestors 'self'; base-uri 'none'; form-action 'none'")
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if r.URL.Path != "/healthz" {
			s.log.Info("http", "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(start).Milliseconds())
		}
	})
}
