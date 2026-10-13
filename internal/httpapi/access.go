package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/lamooof/clickshield/internal/access"
	"github.com/lamooof/clickshield/internal/store"
)

// principal is the API token behind a request. Cookie admins have none.
type principal struct {
	UserID, TokenID, TokenName, ProjectID string
	Scopes                                []string
}
type principalKey struct{}

func principalFrom(ctx context.Context) (principal, bool) {
	p, ok := ctx.Value(principalKey{}).(principal)
	return p, ok
}

// target says how a route names its project, so a project token stays inside it.
type target int

const (
	byQuery   target = iota // ?project_id=, forced to the token project
	byPath                  // {id} is a project
	bySession               // {id} is a session
	global                  // only tokens for all projects
)

// tokenAuth checks the Bearer token and rate limits it.
func (s *Server) tokenAuth(w http.ResponseWriter, r *http.Request) (principal, bool) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") || len(auth) > 200 {
		writeError(w, 401, "API token required")
		return principal{}, false
	}
	// Rate limit before the lookup; a valid token gets its own quota.
	if !s.publicLimiter.Allow("token:" + limitKey(IPFromHTTP(r, s.cfg.TrustProxyHeaders))) {
		writeError(w, 429, "rate limit exceeded")
		return principal{}, false
	}
	t, e := s.store.APITokenByRaw(r.Context(), strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")))
	if e != nil {
		writeError(w, 401, "invalid, expired or revoked API token")
		return principal{}, false
	}
	if !s.eventLimiter.Allow("token:" + t.ID) {
		writeError(w, 429, "rate limit exceeded")
		return principal{}, false
	}
	return principal{UserID: t.CreatorID, TokenID: t.ID, TokenName: t.Name, ProjectID: t.ProjectID, Scopes: t.Scopes}, true
}

// guard admits a cookie admin (with CSRF for changes) or an API token with
// the scope, limited to the token project. MCP calls arrive with the
// principal already in the context.
func (s *Server) guard(scope string, t target, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := principalFrom(r.Context())
		if !ok {
			if r.Header.Get("Authorization") == "" {
				h := next
				if r.Method != "GET" && r.Method != "HEAD" {
					h = s.csrf(next)
				}
				s.requireAPI(h)(w, r)
				return
			}
			if p, ok = s.tokenAuth(w, r); !ok {
				return
			}
		}
		if !access.Has(p.Scopes, scope) {
			writeError(w, 403, "API token lacks scope "+scope)
			return
		}
		if r, ok = s.restrict(w, r, p, t); !ok {
			return
		}
		ctx := context.WithValue(r.Context(), principalKey{}, p)
		ctx = context.WithValue(ctx, adminKey{}, store.Admin{UserID: p.UserID})
		if r.Method != "GET" {
			s.store.Audit(ctx, p.UserID, "API_TOKEN_REQUEST", "api_token", p.TokenID, map[string]any{"method": r.Method, "path": r.URL.Path})
		}
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) restrict(w http.ResponseWriter, r *http.Request, p principal, t target) (*http.Request, bool) {
	if p.ProjectID == "" {
		return r, true
	}
	switch t {
	case global:
		writeError(w, 403, "this route needs a token for all projects")
		return r, false
	case byPath:
		if r.PathValue("id") != p.ProjectID {
			writeError(w, 403, "API token belongs to another project")
			return r, false
		}
	case bySession:
		pid := ""
		if id := r.PathValue("id"); store.ValidUUID(id) {
			pid, _ = s.store.SessionProject(r.Context(), id)
		}
		if pid != p.ProjectID {
			writeError(w, 404, "session not found")
			return r, false
		}
	case byQuery:
		q := r.URL.Query()
		if v := q["project_id"]; len(v) > 1 || (len(v) == 1 && v[0] != "" && v[0] != p.ProjectID) {
			writeError(w, 403, "API token belongs to another project")
			return r, false
		}
		q.Set("project_id", p.ProjectID)
		r = r.Clone(r.Context())
		r.URL.RawQuery = q.Encode()
	}
	return r, true
}
