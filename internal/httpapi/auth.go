package httpapi

import (
	"context"
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lamooof/clickshield/internal/store"
)

type adminKey struct{}

func adminFrom(ctx context.Context) (store.Admin, bool) {
	a, ok := ctx.Value(adminKey{}).(store.Admin)
	return a, ok
}
func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	if c, e := r.Cookie("clickshield_session"); e == nil {
		if _, e = s.store.AdminSession(r.Context(), c.Value); e == nil {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
	}
	b, e := assets.ReadFile("assets/login.html")
	if e != nil {
		http.Error(w, "login unavailable", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b)
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "Недопустимый Origin", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	ip := IPFromHTTP(r, s.cfg.TrustProxyHeaders).String()
	if !s.loginLimiter.Allow(ip) {
		http.Error(w, "Слишком много попыток. Повторите позже.", http.StatusTooManyRequests)
		return
	}
	if e := r.ParseForm(); e != nil {
		http.Error(w, "Некорректная форма", 400)
		return
	}
	username := strings.ToLower(strings.TrimSpace(r.FormValue("username")))
	if !s.userLimiter.Allow(username) {
		http.Error(w, "Слишком много попыток. Повторите позже.", http.StatusTooManyRequests)
		return
	}
	a, e := s.store.Authenticate(r.Context(), username, r.FormValue("password"))
	if e != nil {
		s.store.Audit(r.Context(), "", "LOGIN_FAILED", "admin_user", truncate(username, 64), map[string]any{"ip": ip})
		http.Redirect(w, r, "/login?error=1", http.StatusSeeOther)
		return
	}
	ttl := sessionTTL
	if r.FormValue("remember") == "1" {
		ttl = rememberTTL
	}
	if !s.startSession(w, r, a.UserID, ttl) {
		return
	}
	s.store.Audit(r.Context(), a.UserID, "LOGIN", "admin_user", a.UserID, map[string]any{"ip": ip})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

const (
	sessionTTL  = 12 * time.Hour
	rememberTTL = 90 * 24 * time.Hour
)

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, userID string, ttl time.Duration) bool {
	raw, _, e := s.store.CreateAdminSession(r.Context(), userID, ttl)
	if e != nil {
		http.Error(w, "Не удалось создать сессию", 500)
		return false
	}
	http.SetCookie(w, &http.Cookie{Name: "clickshield_session", Value: raw, Path: "/", HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteStrictMode, MaxAge: int(ttl.Seconds())})
	return true
}
func sameOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && strings.EqualFold(u.Host, r.Host)
}
func (s *Server) isOwner(a store.Admin) bool {
	return strings.EqualFold(a.Username, s.cfg.AdminUsername)
}
func (s *Server) requireOwner(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a, _ := adminFrom(r.Context()); !s.isOwner(a) {
			writeError(w, http.StatusForbidden, "owner only")
			return
		}
		next(w, r)
	}
}
func (s *Server) requirePage(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, ok := s.authenticate(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), adminKey{}, a)))
	}
}
func (s *Server) requireAPI(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, ok := s.authenticate(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), adminKey{}, a)))
	}
}
func (s *Server) authenticate(r *http.Request) (store.Admin, bool) {
	c, e := r.Cookie("clickshield_session")
	if e != nil || strings.TrimSpace(c.Value) == "" {
		return store.Admin{}, false
	}
	a, e := s.store.AdminSession(r.Context(), c.Value)
	return a, e == nil
}
func (s *Server) csrf(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, ok := adminFrom(r.Context())
		token := r.Header.Get("X-CSRF-Token")
		if !ok || token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(a.CSRF)) != 1 {
			writeError(w, http.StatusForbidden, "invalid CSRF token")
			return
		}
		next(w, r)
	}
}
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	a, _ := adminFrom(r.Context())
	writeJSON(w, 200, map[string]any{"user_id": a.UserID, "username": a.Username, "csrf_token": a.CSRF, "expires_at": a.ExpiresAt, "is_owner": s.isOwner(a)})
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	a, _ := adminFrom(r.Context())
	if c, e := r.Cookie("clickshield_session"); e == nil {
		_ = s.store.DeleteAdminSession(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "clickshield_session", Value: "", Path: "/", HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	s.store.Audit(r.Context(), a.UserID, "LOGOUT", "admin_user", a.UserID, nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}
