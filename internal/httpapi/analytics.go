package httpapi

import (
	"context"
	"github.com/lamooof/clickshield/internal/analytics"
	"github.com/lamooof/clickshield/internal/store"
	"net/http"
	"time"
)

func (s *Server) aggregateReport(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("report")
	if kind == "" {
		kind = "sources"
	}
	if !analytics.ValidReport(kind) {
		writeError(w, 400, "unknown report")
		return
	}
	f, ok := sessionFilters(w, r)
	if !ok {
		return
	}
	zone := r.URL.Query().Get("timezone")
	if zone == "" {
		zone = "Europe/Moscow"
	}
	if _, e := time.LoadLocation(zone); e != nil {
		writeError(w, 400, "invalid timezone")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	data, e := s.analytics.Report(ctx, analytics.Query{Report: kind, Timezone: zone, Filter: f})
	if e != nil {
		s.log.Error("aggregate report", "error", e)
		writeError(w, 500, "could not load report")
		return
	}
	writeJSON(w, 200, data)
}

func (s *Server) reportTimeline(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	kind, interval := q.Get("report"), q.Get("interval")
	if kind == "" {
		kind = "sources"
	}
	if !analytics.ValidReport(kind) || !analytics.ValidInterval(interval) {
		writeError(w, 400, "unknown report or interval")
		return
	}
	f, ok := sessionFilters(w, r)
	if !ok {
		return
	}
	if f.From == nil || f.To == nil || !f.From.Before(*f.To) {
		writeError(w, 400, "from and to are required")
		return
	}
	zone := q.Get("timezone")
	if zone == "" {
		zone = "Europe/Moscow"
	}
	if _, e := time.LoadLocation(zone); e != nil {
		writeError(w, 400, "invalid timezone")
		return
	}
	series := q["series"]
	if len(series) > 20 {
		writeError(w, 400, "at most 20 series")
		return
	}
	for i := range series {
		series[i] = truncate(series[i], 2048)
	}
	if _, e := analytics.TimelineBuckets(interval, *f.From, *f.To, time.UTC); e != nil {
		writeError(w, 422, "too many points for this interval")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	data, e := s.analytics.ReportTimeline(ctx, kind, interval, zone, series, q.Get("total_only") == "true", f)
	if e != nil {
		s.log.Error("report timeline", "error", e)
		writeError(w, 500, "could not load timeline")
		return
	}
	writeJSON(w, 200, data)
}

func (s *Server) trafficReport(w http.ResponseWriter, r *http.Request) {
	f, ok := sessionFilters(w, r)
	if !ok {
		return
	}
	items, e := s.analytics.TrafficReport(r.Context(), f)
	if e != nil {
		s.log.Error("traffic report", "error", e)
		writeError(w, 500, "could not load traffic report")
		return
	}
	writeJSON(w, 200, map[string]any{"rows": items})
}
func (s *Server) replaySettings(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !store.ValidUUID(id) {
		writeError(w, 400, "invalid project id")
		return
	}
	var req struct {
		Enabled       bool `json:"enabled"`
		SampleRate    int  `json:"sample_rate"`
		RetentionDays int  `json:"retention_days"`
	}
	if e := readJSON(w, r, &req, 4096); e != nil {
		writeError(w, 400, "invalid settings")
		return
	}
	if req.SampleRate < 0 || req.SampleRate > 100 || req.RetentionDays < 1 || req.RetentionDays > 30 {
		writeError(w, 422, "sample rate must be 0–100 and retention 1–30 days")
		return
	}
	if req.Enabled && !s.replay.Configured() {
		writeError(w, 503, "configure REPLAY_S3_* before enabling replay")
		return
	}
	if e := s.store.UpdateReplaySettings(r.Context(), id, req.Enabled, req.SampleRate, req.RetentionDays); e != nil {
		writeError(w, 404, "project not found")
		return
	}
	s.projectKeys.Reset()
	a, _ := adminFrom(r.Context())
	s.store.Audit(r.Context(), a.UserID, "UPDATE_REPLAY", "project", id, map[string]any{"enabled": req.Enabled, "sample_rate": req.SampleRate, "retention_days": req.RetentionDays})
	writeJSON(w, 200, map[string]any{"ok": true})
}
func (s *Server) trackerConfig(w http.ResponseWriter, r *http.Request) {
	p, ok := s.publicProject(w, r)
	if !ok {
		return
	}
	if !setCORS(w, r, p.AllowedOrigins) {
		writeError(w, 403, "origin is not allowed")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"replay_enabled": p.ReplayEnabled && s.replay.Configured(), "replay_sample_rate": p.ReplaySampleRate, "max_chunk_bytes": 128 << 10})
}
