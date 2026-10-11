package httpapi

import (
	"context"
	"github.com/lamooof/clickshield/internal/analytics"
	"net/http"
	"strconv"
	"time"

	"github.com/lamooof/clickshield/internal/store"
)

func (s *Server) productReport(w http.ResponseWriter, r *http.Request) {
	f, ok := sessionFilters(w, r)
	if !ok {
		return
	}
	width := 0
	if raw := r.URL.Query().Get("width"); raw != "" {
		var err error
		width, err = strconv.Atoi(raw)
		if err != nil || width < 1 || width > 20000 {
			writeError(w, 400, "width must be 1–20000")
			return
		}
	}
	q := analytics.Query{Report: r.PathValue("kind"), Filter: f, Timezone: r.URL.Query().Get("timezone"), Event: r.URL.Query().Get("event"), PageURL: r.URL.Query().Get("page_url"), Width: width}
	if err := q.Validate(); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	data, err := s.analytics.Report(ctx, q)
	if err != nil {
		s.log.Error("product report", "error", err)
		writeError(w, 500, "could not load report")
		return
	}
	writeJSON(w, 200, map[string]any{"data": data})
}

func (s *Server) definitions(w http.ResponseWriter, r *http.Request) {
	pid := r.PathValue("id")
	if !store.ValidUUID(pid) {
		writeError(w, 400, "invalid project")
		return
	}
	defs, e := s.analytics.Repository.Definitions(r.Context(), pid)
	if e != nil {
		writeError(w, 500, "could not load definitions")
		return
	}
	writeJSON(w, 200, map[string]any{"definitions": defs})
}
func (s *Server) saveDefinition(w http.ResponseWriter, r *http.Request) {
	var d analytics.AnalyticsDefinition
	if e := readJSON(w, r, &d, 32<<10); e != nil {
		writeError(w, 400, "invalid definition")
		return
	}
	d.ProjectID = r.PathValue("id")
	d.ID = r.PathValue("definition")
	if d.ID != "" && !store.ValidUUID(d.ID) {
		writeError(w, 400, "invalid definition id")
		return
	}
	if e := d.Validate(); e != nil {
		writeError(w, 422, e.Error())
		return
	}
	if _, err := s.store.Project(r.Context(), d.ProjectID); err != nil {
		writeError(w, 404, "project not found")
		return
	}
	result, e := s.analytics.Repository.SaveDefinition(r.Context(), d)
	if e != nil {
		if e == analytics.ErrNotFound {
			writeError(w, 404, "definition not found")
		} else {
			writeError(w, 500, "could not save definition")
		}
		return
	}
	a, _ := adminFrom(r.Context())
	s.store.Audit(r.Context(), a.UserID, "SAVE_ANALYTICS_DEFINITION", "definition", result.ID, nil)
	writeJSON(w, 200, result)
}
func (s *Server) deleteDefinition(w http.ResponseWriter, r *http.Request) {
	pid, id := r.PathValue("id"), r.PathValue("definition")
	if !store.ValidUUID(pid) || !store.ValidUUID(id) {
		writeError(w, 400, "invalid id")
		return
	}
	if e := s.analytics.Repository.DeleteDefinition(r.Context(), pid, id); e != nil {
		if e == analytics.ErrNotFound {
			writeError(w, 404, "definition not found")
		} else {
			writeError(w, 500, "could not delete definition")
		}
		return
	}
	a, _ := adminFrom(r.Context())
	s.store.Audit(r.Context(), a.UserID, "DELETE_ANALYTICS_DEFINITION", "definition", id, nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}
