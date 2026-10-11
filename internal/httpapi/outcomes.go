package httpapi

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/lamooof/clickshield/internal/store"
)

type outcomeRequest struct {
	ProjectID  string `json:"project_id"`
	SessionID  string `json:"session_id"`
	ExternalID string `json:"external_id"`
	Status     string `json:"status"`
}

func validOutcome(r *outcomeRequest) bool {
	r.ExternalID = strings.TrimSpace(r.ExternalID)
	r.Status = strings.ToUpper(strings.TrimSpace(r.Status))
	return store.ValidUUID(r.SessionID) && (r.ProjectID == "" || store.ValidUUID(r.ProjectID)) && len(r.ExternalID) > 0 && len(r.ExternalID) <= 128
}

func (s *Server) serverOutcome(w http.ResponseWriter, r *http.Request) {
	provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if s.cfg.OutcomeAPIToken == "" || len(provided) != len(s.cfg.OutcomeAPIToken) || subtle.ConstantTimeCompare([]byte(provided), []byte(s.cfg.OutcomeAPIToken)) != 1 {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var req outcomeRequest
	if err := readJSON(w, r, &req, 16<<10); err != nil || !validOutcome(&req) || req.Status != "SUBMITTED" {
		writeError(w, 422, "valid session_id, external_id and SUBMITTED status required")
		return
	}
	if err := s.store.SaveLeadOutcome(r.Context(), req.ProjectID, req.SessionID, req.ExternalID, req.Status, "SERVER", ""); err != nil {
		s.outcomeError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) quality(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	if projectID != "" && !store.ValidUUID(projectID) {
		writeError(w, 400, "invalid project_id")
		return
	}
	rows, err := s.store.Quality(r.Context(), projectID)
	if err != nil {
		s.log.Error("quality report", "error", err)
		writeError(w, 500, "could not load quality report")
		return
	}
	writeJSON(w, 200, map[string]any{"rows": rows})
}

func (s *Server) adminOutcome(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !store.ValidUUID(sessionID) {
		writeError(w, 400, "invalid session id")
		return
	}
	var req outcomeRequest
	if err := readJSON(w, r, &req, 16<<10); err != nil || !validOutcome(&req) || req.SessionID != sessionID {
		writeError(w, 422, "valid session_id and external_id required")
		return
	}
	if req.Status != "QUALIFIED" && req.Status != "SPAM" && req.Status != "LOST" {
		writeError(w, 422, "status must be QUALIFIED, SPAM or LOST")
		return
	}
	a, _ := adminFrom(r.Context())
	if err := s.store.SaveLeadOutcome(r.Context(), req.ProjectID, sessionID, req.ExternalID, req.Status, "MANUAL", a.UserID); err != nil {
		s.outcomeError(w, err)
		return
	}
	s.store.Audit(r.Context(), a.UserID, "LEAD_OUTCOME", "session", sessionID, map[string]any{"external_id": req.ExternalID, "status": req.Status})
	writeJSON(w, 200, map[string]any{"ok": true})
}

// outcomeError reports a lead conflict as is and hides database errors.
func (s *Server) outcomeError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrLeadConflict) {
		writeError(w, 422, err.Error())
		return
	}
	s.log.Error("save lead outcome", "error", err)
	writeError(w, 500, "could not save lead outcome")
}
