package store

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type AnalyticsStep struct {
	Type       string         `json:"type"`
	Value      string         `json:"value"`
	Match      string         `json:"match"`
	Properties map[string]any `json:"properties,omitempty"`
}
type AnalyticsDefinition struct {
	ID            string          `json:"id"`
	ProjectID     string          `json:"project_id"`
	Kind          string          `json:"kind"`
	Name          string          `json:"name"`
	Steps         []AnalyticsStep `json:"steps"`
	WindowMinutes int             `json:"window_minutes"`
}

func (d AnalyticsDefinition) Validate() error {
	if !ValidUUID(d.ProjectID) || strings.TrimSpace(d.Name) == "" || utf8.RuneCountInString(d.Name) > 120 {
		return errors.New("project and name are required (name up to 120 characters)")
	}
	if (d.Kind != "goal" && d.Kind != "funnel") || len(d.Steps) < 1 || len(d.Steps) > 12 || (d.Kind == "goal" && len(d.Steps) != 1) || (d.Kind == "funnel" && len(d.Steps) < 2) {
		return errors.New("goal requires one step; funnel requires 2–12 steps")
	}
	if d.WindowMinutes < 1 || d.WindowMinutes > 10080 {
		return errors.New("window must be 1–10080 minutes")
	}
	for _, s := range d.Steps {
		if s.Type != "page" && s.Type != "event" && s.Type != "outcome" {
			return errors.New("step type must be page, event or outcome")
		}
		if strings.TrimSpace(s.Value) == "" || len(s.Value) > 2048 || (s.Match != "" && s.Match != "exact" && s.Match != "prefix") {
			return errors.New("step value and exact/prefix match required")
		}
		if s.Type == "outcome" && s.Value != "LEAD" && s.Value != "QUALIFIED" && s.Value != "SPAM" && s.Value != "SUBMITTED" && s.Value != "LOST" {
			return errors.New("invalid outcome status")
		}
		if len(s.Properties) > 32 {
			return errors.New("too many property filters")
		}
		for k, v := range s.Properties {
			if len(k) == 0 || len(k) > 80 {
				return errors.New("invalid property key")
			}
			switch x := v.(type) {
			case nil, bool, float64:
			case string:
				if len(x) > 1024 {
					return errors.New("property too long")
				}
			default:
				return errors.New("property filters must be scalar")
			}
		}
	}
	return nil
}
func (s *Store) SaveDefinition(ctx context.Context, d AnalyticsDefinition) (AnalyticsDefinition, error) {
	if err := d.Validate(); err != nil {
		return d, err
	}
	raw, err := json.Marshal(d.Steps)
	if err != nil {
		return d, err
	}
	if d.ID == "" {
		d.ID, err = UUID()
		if err != nil {
			return d, err
		}
		_, err = s.Pool.Exec(ctx, `INSERT INTO analytics_definitions(id,project_id,kind,name,steps,window_minutes) VALUES($1,$2,$3,$4,$5,$6)`, d.ID, d.ProjectID, d.Kind, d.Name, raw, d.WindowMinutes)
	} else {
		tag, e := s.Pool.Exec(ctx, `UPDATE analytics_definitions SET name=$3,steps=$4,window_minutes=$5,updated_at=now() WHERE id=$1 AND project_id=$2 AND kind=$6`, d.ID, d.ProjectID, d.Name, raw, d.WindowMinutes, d.Kind)
		err = e
		if err == nil && tag.RowsAffected() == 0 {
			err = ErrNotFound
		}
	}
	return d, err
}
func (s *Store) Definitions(ctx context.Context, pid string) ([]AnalyticsDefinition, error) {
	rows, e := s.Pool.Query(ctx, `SELECT id::text,project_id::text,kind,name,steps,window_minutes FROM analytics_definitions WHERE project_id=$1 ORDER BY created_at,id`, pid)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []AnalyticsDefinition{}
	for rows.Next() {
		var d AnalyticsDefinition
		var raw []byte
		if e = rows.Scan(&d.ID, &d.ProjectID, &d.Kind, &d.Name, &raw, &d.WindowMinutes); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(raw, &d.Steps); e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func (s *Store) DeleteDefinition(ctx context.Context, pid, id string) error {
	tag, e := s.Pool.Exec(ctx, `DELETE FROM analytics_definitions WHERE id=$1 AND project_id=$2`, id, pid)
	if e == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return e
}

type Activity struct {
	Type, Value string
	Properties  map[string]any
	At          time.Time
}

func matches(step AnalyticsStep, a Activity) bool {
	if step.Type != a.Type {
		return false
	}
	actual := a.Value
	if step.Type == "outcome" && step.Value == "LEAD" && a.Value == "SUBMITTED" {
		actual = "LEAD"
	}
	if step.Type == "page" && strings.HasPrefix(step.Value, "/") {
		if u, e := url.Parse(actual); e == nil {
			actual = u.Path
		}
	}
	if step.Match == "prefix" {
		if !strings.HasPrefix(actual, step.Value) {
			return false
		}
	} else if actual != step.Value {
		return false
	}
	for k, want := range step.Properties {
		got, ok := a.Properties[k]
		if !ok {
			return false
		}
		g, _ := json.Marshal(got)
		w, _ := json.Marshal(want)
		if string(g) != string(w) {
			return false
		}
	}
	return true
}

// Latest reachable timestamps preserve alternative starts when an earlier path expired.
// Reverse iteration prevents one occurrence from completing two identical steps.
func advanceFunnel(steps []AnalyticsStep, latest []time.Time, a Activity, window time.Duration) int {
	reached := 0
	for i := len(steps) - 1; i >= 0; i-- {
		if !matches(steps[i], a) {
			continue
		}
		if i == 0 || (!latest[i-1].IsZero() && !a.At.Before(latest[i-1]) && a.At.Sub(latest[i-1]) <= window) {
			latest[i] = a.At
			if reached < i+1 {
				reached = i + 1
			}
		}
	}
	return reached
}

type FunnelReport struct {
	Definition     AnalyticsDefinition `json:"definition"`
	EligibleVisits int64               `json:"eligible_visits"`
	Counts         []int64             `json:"counts"`
}

func (s *Store) FunnelReport(ctx context.Context, d AnalyticsDefinition, f SessionFilter) (FunnelReport, error) {
	out := FunnelReport{Definition: d, Counts: make([]int64, len(d.Steps))}
	f.ProjectID = d.ProjectID
	where, args := sessionWhere(f)
	if e := s.Pool.QueryRow(ctx, `SELECT count(*) FROM sessions s WHERE `+where, args...).Scan(&out.EligibleVisits); e != nil {
		return out, e
	}
	rows, e := s.Pool.Query(ctx, `WITH selected AS (SELECT s.id FROM sessions s WHERE `+where+`), activity AS (
 SELECT p.session_id,'page' AS type,split_part(p.page_url,'?',1) AS value,'{}'::jsonb AS properties,p.viewed_at AS at,0 AS rank,p.page_index::bigint AS seq FROM session_pages p JOIN selected s ON s.id=p.session_id
 UNION ALL SELECT b.session_id,'event',b.name,b.properties,b.event_time,1,b.id FROM business_events b JOIN selected s ON s.id=b.session_id
 UNION ALL SELECT o.session_id,'outcome','SUBMITTED','{}'::jsonb,o.created_at,2,o.id FROM lead_outcomes o JOIN selected s ON s.id=o.session_id
 UNION ALL SELECT o.session_id,'outcome',o.status,'{}'::jsonb,o.updated_at,3,o.id FROM lead_outcomes o JOIN selected s ON s.id=o.session_id WHERE o.status<>'SUBMITTED'
 ) SELECT session_id::text,type,value,properties,at FROM activity ORDER BY session_id,at,rank,seq`, args...)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	var current string
	latest := make([]time.Time, len(d.Steps))
	maxStep := 0
	finish := func() {
		for i := 0; i < maxStep; i++ {
			out.Counts[i]++
		}
	}
	for rows.Next() {
		var sid string
		var a Activity
		var raw []byte
		if e = rows.Scan(&sid, &a.Type, &a.Value, &raw, &a.At); e != nil {
			return out, e
		}
		if sid != current {
			finish()
			current = sid
			latest = make([]time.Time, len(d.Steps))
			maxStep = 0
		}
		if e = json.Unmarshal(raw, &a.Properties); e != nil {
			return out, e
		}
		if n := advanceFunnel(d.Steps, latest, a, time.Duration(d.WindowMinutes)*time.Minute); n > maxStep {
			maxStep = n
		}
	}
	finish()
	return out, rows.Err()
}

type EventMetric struct {
	Name     string `json:"name"`
	Events   int64  `json:"events"`
	Visits   int64  `json:"visits"`
	Visitors int64  `json:"visitors"`
}

func (s *Store) BusinessReport(ctx context.Context, f SessionFilter) ([]EventMetric, error) {
	where, args := sessionWhere(f)
	rows, e := s.Pool.Query(ctx, `SELECT b.name,count(*),count(DISTINCT s.id),count(DISTINCT s.visitor_id) FROM business_events b JOIN sessions s ON s.id=b.session_id WHERE `+where+` GROUP BY b.name ORDER BY count(*) DESC,b.name`, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []EventMetric{}
	for rows.Next() {
		var m EventMetric
		if e = rows.Scan(&m.Name, &m.Events, &m.Visits, &m.Visitors); e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

type PropertyMetric struct {
	Name   string `json:"property"`
	Value  any    `json:"value"`
	Events int64  `json:"events"`
}

func (s *Store) EventProperties(ctx context.Context, name string, f SessionFilter) ([]PropertyMetric, error) {
	where, args := sessionWhere(f)
	args = append(args, name)
	rows, e := s.Pool.Query(ctx, `SELECT p.key,p.value,count(*) FROM business_events b JOIN sessions s ON s.id=b.session_id CROSS JOIN LATERAL jsonb_each(b.properties) p WHERE `+where+` AND b.name=$`+strconv.Itoa(len(args))+` GROUP BY p.key,p.value ORDER BY count(*) DESC,p.key,p.value LIMIT 500`, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []PropertyMetric{}
	for rows.Next() {
		var m PropertyMetric
		var raw []byte
		if e = rows.Scan(&m.Name, &raw, &m.Events); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(raw, &m.Value); e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
