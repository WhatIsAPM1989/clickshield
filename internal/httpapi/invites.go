package httpapi

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/lamooof/clickshield/internal/store"
)

const inviteTTL = 7 * 24 * time.Hour

func (s *Server) invitePage(w http.ResponseWriter, r *http.Request) {
	b, e := assets.ReadFile("assets/invite.html")
	if e != nil {
		http.Error(w, "invite unavailable", 500)
		return
	}
	if !s.store.InviteValid(r.Context(), r.PathValue("token")) && r.URL.Query().Get("error") == "" {
		http.Redirect(w, r, r.URL.Path+"?error=invalid", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b)
}

func (s *Server) acceptInvite(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "Недопустимый Origin", http.StatusForbidden)
		return
	}
	ip := IPFromHTTP(r, s.cfg.TrustProxyHeaders).String()
	if !s.loginLimiter.Allow(ip) {
		http.Error(w, "Слишком много попыток. Повторите позже.", http.StatusTooManyRequests)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if e := r.ParseForm(); e != nil {
		http.Error(w, "Некорректная форма", 400)
		return
	}
	fail := func(code string) { http.Redirect(w, r, r.URL.Path+"?error="+code, http.StatusSeeOther) }
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	pass := r.FormValue("password")
	if a, e := mail.ParseAddress(email); e != nil || a.Address != email {
		fail("email")
		return
	}
	if len(pass) < 12 {
		fail("short")
		return
	}
	if pass != r.FormValue("password2") {
		fail("mismatch")
		return
	}
	uid, e := s.store.AcceptInvite(r.Context(), r.PathValue("token"), email, pass)
	switch {
	case errors.Is(e, store.ErrInviteInvalid):
		fail("invalid")
		return
	case errors.Is(e, store.ErrUserExists):
		fail("exists")
		return
	case e != nil:
		http.Error(w, "Не удалось создать пользователя", 500)
		return
	}
	s.store.Audit(r.Context(), uid, "INVITE_ACCEPTED", "admin_user", uid, map[string]any{"ip": ip})
	if s.startSession(w, r, uid, sessionTTL) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func (s *Server) invites(w http.ResponseWriter, r *http.Request) {
	list, e := s.store.ListInvites(r.Context())
	if e != nil {
		writeError(w, 500, e.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"invites": list})
}

func (s *Server) createInvite(w http.ResponseWriter, r *http.Request) {
	a, _ := adminFrom(r.Context())
	raw, exp, e := s.store.CreateInvite(r.Context(), a.UserID, inviteTTL)
	if e != nil {
		writeError(w, 500, e.Error())
		return
	}
	s.store.Audit(r.Context(), a.UserID, "INVITE_CREATED", "admin_invite", "", nil)
	writeJSON(w, 201, map[string]any{"path": "/invite/" + raw, "expires_at": exp})
}

func (s *Server) deleteInvite(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !store.ValidUUID(id) {
		writeError(w, 400, "invalid invite id")
		return
	}
	a, _ := adminFrom(r.Context())
	if e := s.store.DeleteInvite(r.Context(), id); e != nil {
		writeError(w, 500, e.Error())
		return
	}
	s.store.Audit(r.Context(), a.UserID, "INVITE_REVOKED", "admin_invite", id, nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) users(w http.ResponseWriter, r *http.Request) {
	list, e := s.store.ListUsers(r.Context())
	if e != nil {
		writeError(w, 500, e.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"users": list})
}

func (s *Server) setUserActive(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in struct {
		Active bool `json:"active"`
	}
	if !store.ValidUUID(id) || readJSON(w, r, &in, 1<<10) != nil {
		writeError(w, 400, "invalid request")
		return
	}
	name, e := s.store.UserName(r.Context(), id)
	if store.IsNotFound(e) {
		writeError(w, 404, "user not found")
		return
	}
	if e != nil {
		writeError(w, 500, e.Error())
		return
	}
	if strings.EqualFold(name, s.cfg.AdminUsername) {
		writeError(w, 400, "Главного администратора отключить нельзя")
		return
	}
	a, _ := adminFrom(r.Context())
	if e = s.store.SetUserActive(r.Context(), id, in.Active); e != nil {
		writeError(w, 500, e.Error())
		return
	}
	s.store.Audit(r.Context(), a.UserID, "USER_ACTIVE", "admin_user", id, map[string]any{"active": in.Active})
	writeJSON(w, 200, map[string]any{"ok": true})
}
