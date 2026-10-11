package httpapi

import (
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/lamooof/clickshield/internal/store"
	"net/http"
	"strings"
	"time"
)

func (s *Server) metrikaConnection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !store.ValidUUID(id) {
		writeError(w, 400, "invalid project id")
		return
	}
	var counter string
	err := s.store.Pool.QueryRow(r.Context(), `SELECT counter_id FROM metrika_read_connections WHERE project_id=$1`, id).Scan(&counter)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 500, "could not load connection")
		return
	}
	writeJSON(w, 200, map[string]any{"counter_id": counter, "has_token": err == nil})
}
func (s *Server) saveMetrikaConnection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !store.ValidUUID(id) {
		writeError(w, 400, "invalid project id")
		return
	}
	var req struct {
		CounterID  string `json:"counter_id"`
		OAuthToken string `json:"oauth_token"`
	}
	if e := readJSON(w, r, &req, 8192); e != nil {
		writeError(w, 400, "invalid connection")
		return
	}
	req.OAuthToken = strings.TrimSpace(req.OAuthToken)
	if !yandexID.MatchString(req.CounterID) || len(req.OAuthToken) > 4096 || strings.ContainsAny(req.OAuthToken, "\r\n") {
		writeError(w, 422, "numeric counter and valid OAuth token required")
		return
	}
	var encrypted []byte
	var e error
	if req.OAuthToken != "" {
		encrypted, e = s.cipher.EncryptJSON(map[string]string{"oauth_token": req.OAuthToken})
		if e != nil {
			writeError(w, 500, "could not encrypt token")
			return
		}
	} else {
		e = s.store.Pool.QueryRow(r.Context(), `SELECT token_encrypted FROM metrika_read_connections WHERE project_id=$1`, id).Scan(&encrypted)
		if e != nil {
			writeError(w, 422, "OAuth token required for first setup")
			return
		}
	}
	if _, e = s.store.Pool.Exec(r.Context(), `INSERT INTO metrika_read_connections(project_id,counter_id,token_encrypted) VALUES($1,$2,$3) ON CONFLICT(project_id) DO UPDATE SET counter_id=EXCLUDED.counter_id,token_encrypted=EXCLUDED.token_encrypted,updated_at=now()`, id, req.CounterID, encrypted); e != nil {
		writeError(w, 500, "could not save connection")
		return
	}
	a, _ := adminFrom(r.Context())
	s.store.Audit(r.Context(), a.UserID, "UPDATE_METRIKA_READ", "project", id, map[string]any{"counter_id": req.CounterID})
	writeJSON(w, 200, map[string]any{"ok": true})
}
func (s *Server) deleteMetrikaConnection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !store.ValidUUID(id) {
		writeError(w, 400, "invalid project id")
		return
	}
	if _, e := s.store.Pool.Exec(r.Context(), `DELETE FROM metrika_read_connections WHERE project_id=$1`, id); e != nil {
		writeError(w, 500, "could not delete connection")
		return
	}
	a, _ := adminFrom(r.Context())
	s.store.Audit(r.Context(), a.UserID, "DELETE_METRIKA_READ", "project", id, nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}
func (s *Server) metrikaReport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !store.ValidUUID(id) {
		writeError(w, 400, "invalid project id")
		return
	}
	kind := r.URL.Query().Get("report")
	switch kind {
	case "sources", "campaigns", "platforms", "utm":
	default:
		writeError(w, 400, "unknown report")
		return
	}
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	f, e := time.Parse("2006-01-02", from)
	if e != nil {
		writeError(w, 400, "from must be YYYY-MM-DD")
		return
	}
	end, e := time.Parse("2006-01-02", to)
	if e != nil || end.Before(f) || end.Sub(f) > 366*24*time.Hour {
		writeError(w, 400, "invalid date range (up to 366 days)")
		return
	}
	a, _ := adminFrom(r.Context())
	if !s.metrikaLimiter.Allow(a.UserID) {
		writeError(w, 429, "too many Metrika requests")
		return
	}
	var counter string
	var encrypted []byte
	if e = s.store.Pool.QueryRow(r.Context(), `SELECT counter_id,token_encrypted FROM metrika_read_connections WHERE project_id=$1`, id).Scan(&counter, &encrypted); e != nil {
		writeError(w, 422, "configure a Metrika OAuth connection in Projects")
		return
	}
	var secret struct {
		Token string `json:"oauth_token"`
	}
	if e = s.cipher.DecryptJSON(encrypted, &secret); e != nil {
		writeError(w, 500, "could not decrypt connection")
		return
	}
	data, e := s.metrikaReader.Report(r.Context(), secret.Token, counter, kind, from, to, intParam(r, "offset", 1, 1, 100000))
	if e != nil {
		writeError(w, 502, e.Error())
		return
	}
	writeJSON(w, 200, data)
}
