package httpapi

import (
	"context"
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
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	var data any
	var err error
	switch r.PathValue("kind") {
	case "events":
		data, err = s.store.BusinessReport(ctx, f)
	case "properties":
		if r.URL.Query().Get("event") == "" {
			writeError(w, 400, "event required")
			return
		}
		data, err = s.store.EventProperties(ctx, r.URL.Query().Get("event"), f)
	case "journey":
		data, err = s.store.Journey(ctx, f)
	case "retention":
		zone := r.URL.Query().Get("timezone")
		if zone == "" {
			zone = "Europe/Moscow"
		}
		if _, e := time.LoadLocation(zone); e != nil {
			writeError(w, 400, "invalid timezone")
			return
		}
		data, err = s.store.Retention(ctx, f, zone)
	case "heatmap":
		width := 0
		if raw := r.URL.Query().Get("width"); raw != "" {
			width, err = strconv.Atoi(raw)
			if err != nil || width < 1 || width > 20000 {
				writeError(w, 400, "width must be 1–20000")
				return
			}
		}
		data, err = s.store.Heatmap(ctx, f, r.URL.Query().Get("page_url"), width)
	case "goals", "funnels":
		if f.ProjectID == "" {
			writeError(w, 400, "select one project")
			return
		}
		defs, e := s.store.Definitions(ctx, f.ProjectID)
		if e != nil {
			err = e
			break
		}
		reports := []store.FunnelReport{}
		for _, d := range defs {
			if (r.PathValue("kind") == "goals" && d.Kind == "goal") || (r.PathValue("kind") == "funnels" && d.Kind == "funnel") {
				report, e := s.store.FunnelReport(ctx, d, f)
				if e != nil {
					err = e
					break
				}
				reports = append(reports, report)
			}
		}
		data = reports
	default:
		writeError(w, 404, "unknown report")
		return
	}
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
	defs, e := s.store.Definitions(r.Context(), pid)
	if e != nil {
		writeError(w, 500, "could not load definitions")
		return
	}
	writeJSON(w, 200, map[string]any{"definitions": defs})
}
func (s *Server) saveDefinition(w http.ResponseWriter, r *http.Request) {
	var d store.AnalyticsDefinition
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
	result, e := s.store.SaveDefinition(r.Context(), d)
	if e != nil {
		if e == store.ErrNotFound {
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
	if e := s.store.DeleteDefinition(r.Context(), pid, id); e != nil {
		if e == store.ErrNotFound {
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
