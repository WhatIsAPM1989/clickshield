package store

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lamooof/clickshield/internal/analytics"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/lamooof/clickshield/internal/domain"
)

type contextReader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Store) SessionContext(ctx context.Context, id string) (domain.SessionContext, error) {
	return readSessionContext(ctx, s.Pool, id)
}

func readSessionContext(ctx context.Context, db contextReader, id string) (domain.SessionContext, error) {
	var c domain.SessionContext
	var auto, cfg []byte
	err := db.QueryRow(ctx, `SELECT s.id::text,s.project_id::text,s.visitor_id::text,s.visitor_key,s.fingerprint,COALESCE(host(s.ip),''),s.asn,s.as_org,s.user_agent,s.device_type,s.yandex_client_id,s.google_client_id,s.yclid,s.gclid,s.rb_clickid,s.utm_source,s.utm_medium,s.utm_campaign,s.referrer,s.landing_url,s.last_page_url,s.automation,s.started_at,s.last_seen_at,s.manual_label,p.mode,p.scoring_config,
 (SELECT count(*) FROM sessions x WHERE x.project_id=s.project_id AND x.ip=s.ip AND x.started_at BETWEEN s.last_seen_at-interval '24 hours' AND s.last_seen_at),
 fp.sessions,fp.visitors,nearby.sessions,nearby.pages,
 (SELECT count(*) FROM sessions x WHERE x.project_id=s.project_id AND x.visitor_id=s.visitor_id AND x.started_at BETWEEN s.last_seen_at-interval '24 hours' AND s.last_seen_at AND (x.yclid<>'' OR x.gclid<>'' OR x.rb_clickid<>'' OR lower(x.utm_medium) IN ('cpc','ppc','paid','paidsearch','display')))
 FROM sessions s JOIN projects p ON p.id=s.project_id
 CROSS JOIN LATERAL (SELECT count(*) AS sessions,count(DISTINCT x.visitor_id) AS visitors FROM sessions x WHERE s.fingerprint<>'' AND x.project_id=s.project_id AND x.fingerprint=s.fingerprint AND x.started_at BETWEEN s.last_seen_at-interval '24 hours' AND s.last_seen_at) fp
 CROSS JOIN LATERAL (SELECT count(*) AS sessions,count(DISTINCT nullif(split_part(x.landing_url,'?',1),'')) AS pages FROM sessions x WHERE x.project_id=s.project_id AND x.visitor_id=s.visitor_id AND x.ip=s.ip AND x.started_at BETWEEN s.started_at-interval '30 seconds' AND s.started_at+interval '30 seconds') nearby
 WHERE s.id=$1`, id).Scan(&c.ID, &c.ProjectID, &c.VisitorDBID, &c.VisitorKey, &c.Fingerprint, &c.IP, &c.ASN, &c.ASOrg, &c.UserAgent, &c.DeviceType, &c.YandexClientID, &c.GoogleClientID, &c.YCLID, &c.GCLID, &c.RBClickID, &c.UTMSource, &c.UTMMedium, &c.UTMCampaign, &c.Referrer, &c.LandingURL, &c.LastPageURL, &auto, &c.StartedAt, &c.LastSeenAt, &c.ManualLabel, &c.ProjectMode, &cfg, &c.SessionsPerIP, &c.SessionsPerFingerprint, &c.VisitorsPerFingerprint, &c.NearbyVisitorSessions, &c.NearbyVisitorPages, &c.PaidVisitFrequency)
	if err != nil {
		return c, err
	}
	if string(auto) == "null" || (len(auto) > 0 && auto[0] == '[') {
		auto = []byte("{}")
	}
	if err = json.Unmarshal(auto, &c.Automation); err != nil {
		return c, err
	}
	c.ProjectScoringConfig = cfg
	return c, nil
}

func (s *Store) Events(ctx context.Context, sessionID string) ([]domain.Event, error) {
	rows, e := s.Pool.Query(ctx, `SELECT payload FROM raw_events WHERE session_id=$1 ORDER BY event_time,id`, sessionID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []domain.Event
	for rows.Next() {
		var raw []byte
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		var event domain.Event
		if json.Unmarshal(raw, &event) == nil {
			out = append(out, event)
		}
	}
	return out, rows.Err()
}

type SessionFilter = analytics.SessionFilter

func (s *Store) ListSessions(ctx context.Context, filter SessionFilter, limit, offset int) ([]domain.SessionItem, error) {
	var out []domain.SessionItem
	err := s.IterateSessions(ctx, filter, limit, offset, func(x domain.SessionItem) error {
		out = append(out, x)
		return nil
	})
	return out, err
}

func (s *Store) IterateSessions(ctx context.Context, filter SessionFilter, limit, offset int, visit func(domain.SessionItem) error) error {
	where, args := sessionWhere(filter)
	q := `SELECT s.id::text,s.project_id::text,p.name,s.started_at,s.last_seen_at,COALESCE(f.duration_ms,0),s.current_classification,s.current_risk_score,s.yandex_client_id,s.google_client_id,s.yclid,s.gclid,s.rb_clickid,s.utm_source,s.utm_medium,s.utm_campaign,COALESCE(f.click_count,0),COALESCE(f.scroll_depth,0),s.fingerprint,COALESCE(host(s.ip),''),s.asn,s.as_org,s.device_type,s.manual_label,COALESCE(s.bot_name,''),s.bot_category,s.traffic_source,s.traffic_channel,s.referrer_host,s.attribution_basis,s.landing_url,EXISTS(SELECT 1 FROM replay_recordings rr WHERE rr.session_id=s.id AND rr.expires_at>now()) FROM sessions s JOIN projects p ON p.id=s.project_id LEFT JOIN session_features f ON f.session_id=s.id WHERE ` + where + ` ORDER BY s.started_at DESC, s.id DESC`
	if limit > 0 {
		args = append(args, limit, offset)
		q += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	}
	rows, e := s.Pool.Query(ctx, q, args...)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var x domain.SessionItem
		if e = rows.Scan(&x.ID, &x.ProjectID, &x.ProjectName, &x.StartedAt, &x.LastSeenAt, &x.DurationMS, &x.Classification, &x.RiskScore, &x.YandexClientID, &x.GoogleClientID, &x.YCLID, &x.GCLID, &x.RBClickID, &x.UTMSource, &x.UTMMedium, &x.UTMCampaign, &x.ClickCount, &x.ScrollDepth, &x.Fingerprint, &x.IP, &x.ASN, &x.ASOrg, &x.DeviceType, &x.ManualLabel, &x.BotName, &x.BotCategory, &x.TrafficSource, &x.TrafficChannel, &x.ReferrerHost, &x.AttributionBasis, &x.LandingURL, &x.HasReplay); e != nil {
			return e
		}
		if e = visit(x); e != nil {
			return e
		}
	}
	return rows.Err()
}

func (s *Store) Stats(ctx context.Context, projectID string) (domain.Stats, error) {
	var st domain.Stats
	var arg any
	if projectID != "" {
		arg = projectID
	}
	e := s.Pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE current_classification='HUMAN'),count(*) FILTER(WHERE current_classification='SUSPICIOUS'),count(*) FILTER(WHERE current_classification='BOT'),count(*) FILTER(WHERE current_classification='KNOWN_BOT'),count(*) FILTER(WHERE current_classification=''),COALESCE(avg(current_risk_score),0),count(*) FILTER(WHERE yclid<>'' OR gclid<>'' OR rb_clickid<>'' OR lower(utm_medium) IN ('cpc','ppc','paid','paidsearch','display')) FROM sessions WHERE ($1::uuid IS NULL OR project_id=$1)`, arg).Scan(&st.Total, &st.Human, &st.Suspicious, &st.Bot, &st.KnownBot, &st.Unclassified, &st.AverageRisk, &st.PaidVisits)
	return st, e
}

func sessionWhere(f SessionFilter) (string, []any) {
	where, args := analytics.Where(f)
	return strings.ReplaceAll(where, "analytics.replay_recordings", "replay_recordings"), args
}
