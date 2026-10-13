package httpapi

import (
	"context"
	"encoding/base64"
	"errors"
	"github.com/lamooof/clickshield/internal/analytics"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lamooof/clickshield/internal/store"
)

func toolError(w http.ResponseWriter, e error) {
	var p *pgconn.PgError
	switch {
	case errors.Is(e, analytics.ErrNotFound), store.IsNotFound(e):
		writeError(w, 404, "not found")
	case errors.As(e, &p) && p.Code == "23505":
		writeError(w, 409, "slug already exists")
	case errors.As(e, &p):
		writeError(w, 500, "could not save analytics tool")
	default:
		writeError(w, 422, e.Error())
	}
}
func (s *Server) analyticsTools(w http.ResponseWriter, r *http.Request) {
	pid, kind, id := r.PathValue("id"), r.PathValue("kind"), r.PathValue("tool")
	if !store.ValidUUID(pid) || (id != "" && !store.ValidUUID(id)) {
		writeError(w, 400, "invalid id")
		return
	}
	if kind != "links" && kind != "pixels" && kind != "annotations" {
		writeError(w, 404, "unknown tool")
		return
	}
	if r.Method != "GET" {
		if _, err := s.store.Project(r.Context(), pid); err != nil {
			writeError(w, 404, "project not found")
			return
		}
	}
	var result any
	var err error
	if r.Method == "GET" {
		f, ok := sessionFilters(w, r)
		if !ok {
			return
		}
		f.ProjectID = pid
		result, err = s.analytics.Report(r.Context(), analytics.Query{Report: kind, Filter: f})
	} else if r.Method == "DELETE" {
		err = s.analytics.Repository.DeleteAnalyticsTool(r.Context(), pid, id, kind)
		result = map[string]any{"ok": true}
	} else {
		switch kind {
		case "links":
			var l analytics.TrackedLink
			if e := readJSON(w, r, &l, 8192); e != nil {
				writeError(w, 400, "invalid link")
				return
			}
			l.ProjectID = pid
			l.ID = id
			result, err = s.analytics.Repository.SaveLink(r.Context(), l)
		case "pixels":
			var p analytics.TrackingPixel
			if e := readJSON(w, r, &p, 4096); e != nil {
				writeError(w, 400, "invalid pixel")
				return
			}
			p.ProjectID = pid
			p.ID = id
			result, err = s.analytics.Repository.SavePixel(r.Context(), p)
		case "annotations":
			var a analytics.Annotation
			if e := readJSON(w, r, &a, 8192); e != nil {
				writeError(w, 400, "invalid annotation")
				return
			}
			a.ProjectID = pid
			a.ID = id
			result, err = s.analytics.Repository.SaveAnnotation(r.Context(), a)
		}
	}
	if err != nil {
		toolError(w, err)
		return
	}
	if r.Method != "GET" {
		a, _ := adminFrom(r.Context())
		s.store.Audit(r.Context(), a.UserID, r.Method+"_ANALYTICS_TOOL", kind, id, nil)
	}
	writeJSON(w, 200, map[string]any{"data": result})
}
func (s *Server) trackingCountry(r *http.Request) string {
	if s.cfg.TrustProxyHeaders {
		return truncate(r.Header.Get("CF-IPCountry"), 8)
	}
	return ""
}
func (s *Server) shortLink(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.publicLimiter.Allow("link:" + limitKey(IPFromHTTP(r, s.cfg.TrustProxyHeaders))) {
		writeError(w, 429, "rate limit exceeded")
		return
	}
	// HEAD previews do not count; GET requests, including mail/link scanners, do.
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	destination, e := s.analytics.Repository.HitLink(ctx, r.PathValue("slug"), truncate(r.UserAgent(), 512), sanitizeURL(r.Referer()), s.trackingCountry(r), r.Method != "HEAD")
	if e != nil {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, destination, http.StatusFound)
}

var transparentPixel, _ = base64.StdEncoding.DecodeString("R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7")

func (s *Server) pixel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("pixel")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	if !store.ValidUUID(id) {
		http.NotFound(w, r)
		return
	}
	if !s.publicLimiter.Allow("pixel:" + limitKey(IPFromHTTP(r, s.cfg.TrustProxyHeaders))) {
		writeError(w, 429, "rate limit exceeded")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if e := s.analytics.Repository.HitPixel(ctx, id, truncate(r.UserAgent(), 512), sanitizeURL(r.Referer()), s.trackingCountry(r)); e != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/gif")
	w.Write(transparentPixel)
}
