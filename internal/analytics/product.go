package analytics

import (
	"context"
	"encoding/json"
	"strconv"
)

func (s *Repository) SaveDefinition(ctx context.Context, d AnalyticsDefinition) (AnalyticsDefinition, error) {
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
		_, err = s.Pool.Exec(ctx, `INSERT INTO analytics.analytics_definitions(id,project_id,kind,name,steps,window_minutes) VALUES($1,$2,$3,$4,$5,$6)`, d.ID, d.ProjectID, d.Kind, d.Name, raw, d.WindowMinutes)
	} else {
		tag, e := s.Pool.Exec(ctx, `UPDATE analytics.analytics_definitions SET name=$3,steps=$4,window_minutes=$5,updated_at=now() WHERE id=$1 AND project_id=$2 AND kind=$6`, d.ID, d.ProjectID, d.Name, raw, d.WindowMinutes, d.Kind)
		err = e
		if err == nil && tag.RowsAffected() == 0 {
			err = ErrNotFound
		}
	}
	return d, err
}
func (s *Repository) Definitions(ctx context.Context, pid string) ([]AnalyticsDefinition, error) {
	rows, e := s.Pool.Query(ctx, `SELECT id::text,project_id::text,kind,name,steps,window_minutes FROM analytics.analytics_definitions WHERE project_id=$1 ORDER BY created_at,id`, pid)
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
func (s *Repository) DeleteDefinition(ctx context.Context, pid, id string) error {
	tag, e := s.Pool.Exec(ctx, `DELETE FROM analytics.analytics_definitions WHERE id=$1 AND project_id=$2`, id, pid)
	if e == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return e
}

type FunnelReport struct {
	Definition     AnalyticsDefinition `json:"definition"`
	EligibleVisits int64               `json:"eligible_visits"`
	Counts         []int64             `json:"counts"`
}

type EventMetric struct {
	Name     string `json:"name"`
	Events   int64  `json:"events"`
	Visits   int64  `json:"visits"`
	Visitors int64  `json:"visitors"`
}

func (s *Repository) BusinessReport(ctx context.Context, f SessionFilter) ([]EventMetric, error) {
	where, args := sessionWhere(f)
	rows, e := s.Pool.Query(ctx, `SELECT b.name,count(*),count(DISTINCT s.id),count(DISTINCT s.visitor_id) FROM analytics.business_events b JOIN analytics.sessions s ON s.id=b.session_id WHERE `+where+` GROUP BY b.name ORDER BY count(*) DESC,b.name`, args...)
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

func (s *Repository) EventProperties(ctx context.Context, name string, f SessionFilter) ([]PropertyMetric, error) {
	where, args := sessionWhere(f)
	args = append(args, name)
	rows, e := s.Pool.Query(ctx, `SELECT p.key,p.value,count(*) FROM analytics.business_events b JOIN analytics.sessions s ON s.id=b.session_id CROSS JOIN LATERAL jsonb_each(b.properties) p WHERE `+where+` AND b.name=$`+strconv.Itoa(len(args))+` GROUP BY p.key,p.value ORDER BY count(*) DESC,p.key,p.value LIMIT 500`, args...)
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
