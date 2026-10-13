package httpapi

import (
	"net/http"

	"github.com/lamooof/clickshield/internal/access"
	"github.com/lamooof/clickshield/internal/store"
)

// API tokens are managed only from the panel (cookie + CSRF), never by a token.
func (s *Server) apiTokens(w http.ResponseWriter, r *http.Request) {
	items, e := s.store.APITokens(r.Context())
	if e != nil {
		writeError(w, 500, "could not load tokens")
		return
	}
	writeJSON(w, 200, map[string]any{"tokens": items, "scopes": access.All})
}
func (s *Server) createAPIToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string   `json:"name"`
		ProjectID string   `json:"project_id"`
		Scopes    []string `json:"scopes"`
		Days      int      `json:"days"`
	}
	if e := readJSON(w, r, &req, 4096); e != nil {
		writeError(w, 400, "invalid request")
		return
	}
	scopes, e := access.Normalize(req.Scopes)
	if e != nil {
		writeError(w, 422, e.Error())
		return
	}
	if req.Days == 0 {
		req.Days = 90
	}
	if req.ProjectID != "" && !s.projectExists(r, req.ProjectID) {
		writeError(w, 404, "project not found")
		return
	}
	a, _ := adminFrom(r.Context())
	t, raw, e := s.store.CreateAPIToken(r.Context(), a.UserID, req.Name, req.ProjectID, scopes, req.Days)
	if e != nil {
		writeError(w, 422, "name and lifetime 1–365 days required")
		return
	}
	s.store.Audit(r.Context(), a.UserID, "CREATE_API_TOKEN", "api_token", t.ID, map[string]any{"project_id": t.ProjectID, "scopes": t.Scopes})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 201, map[string]any{"token": t, "raw": raw})
}
func (s *Server) projectExists(r *http.Request, id string) bool {
	if !store.ValidUUID(id) {
		return false
	}
	_, e := s.store.Project(r.Context(), id)
	return e == nil
}
func (s *Server) revokeAPIToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !store.ValidUUID(id) {
		writeError(w, 400, "invalid id")
		return
	}
	if e := s.store.RevokeAPIToken(r.Context(), id); e != nil {
		toolError(w, e)
		return
	}
	a, _ := adminFrom(r.Context())
	s.store.Audit(r.Context(), a.UserID, "REVOKE_API_TOKEN", "api_token", id, nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}
