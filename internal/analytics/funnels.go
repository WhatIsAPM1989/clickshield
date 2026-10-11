package analytics

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"time"
)

func (s *Repository) FunnelReport(ctx context.Context, d AnalyticsDefinition, f SessionFilter) (FunnelReport, error) {
	f.ProjectID = d.ProjectID
	reports, err := s.FunnelReports(ctx, []AnalyticsDefinition{d}, f)
	if err != nil {
		return FunnelReport{}, err
	}
	return reports[0], nil
}

// Read activity once for all definitions, streaming one session at a time.
func (s *Repository) FunnelReports(ctx context.Context, defs []AnalyticsDefinition, f SessionFilter) ([]FunnelReport, error) {
	out := make([]FunnelReport, len(defs))
	if len(defs) == 0 {
		return out, nil
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	where, args := sessionWhere(f)
	var eligible int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM analytics.sessions s WHERE `+where, args...).Scan(&eligible); err != nil {
		return nil, err
	}
	latest := make([][]time.Time, len(defs))
	maxSteps := make([]int, len(defs))
	for i, d := range defs {
		out[i] = FunnelReport{Definition: d, EligibleVisits: eligible, Counts: make([]int64, len(d.Steps))}
		latest[i] = make([]time.Time, len(d.Steps))
	}
	rows, e := tx.Query(ctx, `WITH selected AS (SELECT s.id FROM analytics.sessions s WHERE `+where+`), activity AS (
 SELECT p.session_id,'page' AS type,split_part(p.page_url,'?',1) AS value,'{}'::jsonb AS properties,p.viewed_at AS at,0 AS rank,p.page_index::bigint AS seq FROM analytics.session_pages p JOIN selected s ON s.id=p.session_id
 UNION ALL SELECT b.session_id,'event',b.name,b.properties,b.event_time,1,b.id FROM analytics.business_events b JOIN selected s ON s.id=b.session_id
 UNION ALL SELECT o.session_id,'outcome','SUBMITTED','{}'::jsonb,o.created_at,2,o.id FROM analytics.lead_outcomes o JOIN selected s ON s.id=o.session_id
 UNION ALL SELECT o.session_id,'outcome',o.status,'{}'::jsonb,o.updated_at,3,o.id FROM analytics.lead_outcomes o JOIN selected s ON s.id=o.session_id WHERE o.status<>'SUBMITTED'
 ) SELECT session_id::text,type,value,properties,at FROM activity ORDER BY session_id,at,rank,seq`, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	finish := func() {
		for i := range defs {
			for step := 0; step < maxSteps[i]; step++ {
				out[i].Counts[step]++
			}
			maxSteps[i] = 0
			clear(latest[i])
		}
	}
	var current string
	for rows.Next() {
		var sid string
		var a Activity
		var raw []byte
		if e = rows.Scan(&sid, &a.Type, &a.Value, &raw, &a.At); e != nil {
			return nil, e
		}
		if sid != current {
			finish()
			current = sid
		}
		if e = json.Unmarshal(raw, &a.Properties); e != nil {
			return nil, e
		}
		for i, d := range defs {
			if n := advanceFunnel(d.Steps, latest[i], a, time.Duration(d.WindowMinutes)*time.Minute); n > maxSteps[i] {
				maxSteps[i] = n
			}
		}
	}
	finish()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	return out, tx.Commit(ctx)
}
