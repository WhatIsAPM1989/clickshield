package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lamooof/clickshield/internal/domain"
)

func (s *Store) DueSessions(ctx context.Context, finishGrace, stale time.Duration, limit int) ([]string, error) {
	finishBefore := time.Now().UTC().Add(-finishGrace)
	staleBefore := time.Now().UTC().Add(-stale)
	// Tabs left open keep sending heartbeats and never go stale, so sessions
	// still unscored after the stale window get a provisional classification.
	rows, e := s.Pool.Query(ctx, `SELECT id::text FROM sessions WHERE (finish_requested_at IS NOT NULL AND finish_requested_at<$1 AND last_seen_at<$1) OR (finished_at IS NULL AND last_seen_at<$2) OR (current_classification='' AND started_at<$2) ORDER BY COALESCE(finish_requested_at,last_seen_at) LIMIT $3`, finishBefore, staleBefore, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) SessionContext(ctx context.Context, id string) (domain.SessionContext, error) {
	var c domain.SessionContext
	var auto, cfg []byte
	e := s.Pool.QueryRow(ctx, `SELECT s.id::text,s.project_id::text,s.visitor_id::text,s.visitor_key,s.fingerprint,COALESCE(host(s.ip),''),s.asn,s.as_org,s.user_agent,s.device_type,s.yandex_client_id,s.google_client_id,s.yclid,s.gclid,s.rb_clickid,s.utm_source,s.utm_medium,s.utm_campaign,s.referrer,s.landing_url,s.last_page_url,s.automation,s.started_at,s.last_seen_at,s.manual_label,p.mode,p.scoring_config FROM sessions s JOIN projects p ON p.id=s.project_id WHERE s.id=$1`, id).Scan(&c.ID, &c.ProjectID, &c.VisitorDBID, &c.VisitorKey, &c.Fingerprint, &c.IP, &c.ASN, &c.ASOrg, &c.UserAgent, &c.DeviceType, &c.YandexClientID, &c.GoogleClientID, &c.YCLID, &c.GCLID, &c.RBClickID, &c.UTMSource, &c.UTMMedium, &c.UTMCampaign, &c.Referrer, &c.LandingURL, &c.LastPageURL, &auto, &c.StartedAt, &c.LastSeenAt, &c.ManualLabel, &c.ProjectMode, &cfg)
	if e != nil {
		return c, e
	}
	_ = json.Unmarshal(auto, &c.Automation)
	c.ProjectScoringConfig = cfg
	if c.IP != "" {
		if e = s.Pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE project_id=$1 AND ip=$2::inet AND started_at>now()-interval '24 hours'`, c.ProjectID, c.IP).Scan(&c.SessionsPerIP); e != nil {
			return c, fmt.Errorf("count sessions per IP: %w", e)
		}
	}
	if c.Fingerprint != "" {
		if e = s.Pool.QueryRow(ctx, `SELECT count(*),count(DISTINCT visitor_id) FROM sessions WHERE project_id=$1 AND fingerprint=$2 AND started_at>now()-interval '24 hours'`, c.ProjectID, c.Fingerprint).Scan(&c.SessionsPerFingerprint, &c.VisitorsPerFingerprint); e != nil {
			return c, fmt.Errorf("count sessions per fingerprint: %w", e)
		}
	}
	if c.IP != "" {
		if e = s.Pool.QueryRow(ctx, `SELECT count(*),count(DISTINCT nullif(split_part(landing_url,'?',1),'')) FROM sessions WHERE project_id=$1 AND visitor_id=$2 AND ip=$3::inet AND started_at BETWEEN $4::timestamptz-interval '30 seconds' AND $4::timestamptz+interval '30 seconds'`, c.ProjectID, c.VisitorDBID, c.IP, c.StartedAt).Scan(&c.NearbyVisitorSessions, &c.NearbyVisitorPages); e != nil {
			return c, fmt.Errorf("count nearby visitor sessions: %w", e)
		}
	}
	if e = s.Pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE project_id=$1 AND visitor_id=$2 AND started_at>now()-interval '24 hours' AND (yclid<>'' OR gclid<>'' OR rb_clickid<>'' OR lower(utm_medium) IN ('cpc','ppc','paid','paidsearch','display'))`, c.ProjectID, c.VisitorDBID).Scan(&c.PaidVisitFrequency); e != nil {
		return c, fmt.Errorf("count paid visits: %w", e)
	}
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

func eventName(classification string) string {
	switch classification {
	case domain.Bot, domain.KnownBot:
		return "antifraud_bot"
	case domain.Human:
		return "antifraud_human"
	default:
		return ""
	}
}

func (s *Store) SaveAutomatic(ctx context.Context, f domain.Features, score domain.Score) (int64, error) {
	return s.saveClassification(ctx, f, score, "AUTO", "", "")
}
func (s *Store) SaveManual(ctx context.Context, sessionID, label, note, userID string) (int64, error) {
	f := domain.Features{SessionID: sessionID, ComputedAt: time.Now().UTC()}
	score := domain.Score{Classification: label, RiskScore: 0, Reasons: []domain.Reason{{Code: "MANUAL_OVERRIDE", Weight: 0, Details: map[string]any{"note": note}}}}
	if label == domain.Bot {
		score.RiskScore = 100
	}
	return s.saveClassification(ctx, f, score, "MANUAL", userID, note)
}

func (s *Store) saveClassification(ctx context.Context, f domain.Features, score domain.Score, source, userID, note string) (int64, error) {
	tx, e := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if e != nil {
		return 0, e
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if source == "AUTO" {
		clicks, _ := json.Marshal(f.ClickIntervalsMS)
		_, e = tx.Exec(ctx, `INSERT INTO session_features(session_id,duration_ms,active_time_ms,idle_time_ms,mouse_distance,mouse_velocity,mouse_max_velocity,mouse_p95_velocity,mouse_acceleration,mouse_curvature,mouse_pauses,mouse_moves,scroll_depth,scroll_velocity,scroll_reversals,scroll_events,click_count,click_intervals_ms,mean_click_interval_ms,time_to_first_click_ms,page_count,interaction_density,active_ratio,paid_visit_frequency,sessions_per_ip,sessions_per_fingerprint,computed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18::jsonb,$19,$20,$21,$22,$23,$24,$25,$26,$27) ON CONFLICT(session_id) DO UPDATE SET duration_ms=EXCLUDED.duration_ms,active_time_ms=EXCLUDED.active_time_ms,idle_time_ms=EXCLUDED.idle_time_ms,mouse_distance=EXCLUDED.mouse_distance,mouse_velocity=EXCLUDED.mouse_velocity,mouse_max_velocity=EXCLUDED.mouse_max_velocity,mouse_p95_velocity=EXCLUDED.mouse_p95_velocity,mouse_acceleration=EXCLUDED.mouse_acceleration,mouse_curvature=EXCLUDED.mouse_curvature,mouse_pauses=EXCLUDED.mouse_pauses,mouse_moves=EXCLUDED.mouse_moves,scroll_depth=EXCLUDED.scroll_depth,scroll_velocity=EXCLUDED.scroll_velocity,scroll_reversals=EXCLUDED.scroll_reversals,scroll_events=EXCLUDED.scroll_events,click_count=EXCLUDED.click_count,click_intervals_ms=EXCLUDED.click_intervals_ms,mean_click_interval_ms=EXCLUDED.mean_click_interval_ms,time_to_first_click_ms=EXCLUDED.time_to_first_click_ms,page_count=EXCLUDED.page_count,interaction_density=EXCLUDED.interaction_density,active_ratio=EXCLUDED.active_ratio,paid_visit_frequency=EXCLUDED.paid_visit_frequency,sessions_per_ip=EXCLUDED.sessions_per_ip,sessions_per_fingerprint=EXCLUDED.sessions_per_fingerprint,computed_at=EXCLUDED.computed_at`, f.SessionID, f.DurationMS, f.ActiveTimeMS, f.IdleTimeMS, f.MouseDistance, f.MouseVelocity, f.MouseMaxVelocity, f.MouseP95Velocity, f.MouseAcceleration, f.MouseCurvature, f.MousePauses, f.MouseMoves, f.ScrollDepth, f.ScrollVelocity, f.ScrollReversals, f.ScrollEvents, f.ClickCount, clicks, f.MeanClickIntervalMS, f.TimeToFirstClickMS, f.PageCount, f.InteractionDensity, f.ActiveRatio, f.PaidVisitFrequency, f.SessionsPerIP, f.SessionsPerFingerprint, f.ComputedAt)
		if e != nil {
			return 0, e
		}
	}
	var classID int64
	var createdBy any
	if userID != "" {
		createdBy = userID
	}
	e = tx.QueryRow(ctx, `INSERT INTO classifications(session_id,risk_score,classification,source,created_by) VALUES($1,$2,$3,$4,$5) RETURNING id`, f.SessionID, score.RiskScore, score.Classification, source, createdBy).Scan(&classID)
	if e != nil {
		return 0, e
	}
	for _, r := range score.Reasons {
		d, _ := json.Marshal(r.Details)
		if _, e = tx.Exec(ctx, `INSERT INTO classification_reasons(classification_id,code,weight,details) VALUES($1,$2,$3,$4::jsonb)`, classID, r.Code, r.Weight, d); e != nil {
			return 0, e
		}
	}
	if source == "MANUAL" {
		_, e = tx.Exec(ctx, `UPDATE sessions SET manual_label=$2,manual_note=$3,current_classification_id=$4,current_classification=$2,current_risk_score=$5,finished_at=now(),finish_requested_at=NULL,updated_at=now() WHERE id=$1`, f.SessionID, score.Classification, note, classID, score.RiskScore)
	} else {
		_, e = tx.Exec(ctx, `UPDATE sessions SET current_classification_id=CASE WHEN manual_label='' THEN $2 ELSE current_classification_id END,current_classification=CASE WHEN manual_label='' THEN $3 ELSE current_classification END,current_risk_score=CASE WHEN manual_label='' THEN $4 ELSE current_risk_score END,finished_at=now(),finish_requested_at=NULL,updated_at=now() WHERE id=$1`, f.SessionID, classID, score.Classification, score.RiskScore)
	}
	if e != nil {
		return 0, e
	}
	name := eventName(score.Classification)
	if name != "" {
		_, e = tx.Exec(ctx, `INSERT INTO integration_outbox(integration_id,project_id,session_id,classification_id,provider,event_name) SELECT i.id,i.project_id,$1,$2,i.provider,$3 FROM integrations i JOIN sessions s ON s.project_id=i.project_id JOIN projects p ON p.id=s.project_id WHERE s.id=$1 AND i.enabled AND p.mode<>'MONITORING' AND ($4='MANUAL' OR s.manual_label='') ON CONFLICT(session_id,provider,event_name) DO NOTHING`, f.SessionID, classID, name, source)
		if e != nil {
			return 0, e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return 0, e
	}
	return classID, nil
}

// SessionFilter narrows session listings. Empty fields do not filter.
type SessionFilter struct {
	DeviceSet   bool
	Device      string
	CampaignSet bool
	Source      string
	Channel     string
	Campaign    string
	LandingURL  string
	ReplayOnly  bool
	From        *time.Time
	To          *time.Time
	ProjectID   string
	Class       string
	PaidOnly    bool
	BotName     string
	BotCategory string
	// Dims narrows by exact values of filterable report dimensions.
	Dims map[string]string
}

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

func sessionWhere(filter SessionFilter) (string, []any) {
	args := []any{}
	where := []string{"1=1"}
	if filter.ProjectID != "" {
		args = append(args, filter.ProjectID)
		where = append(where, fmt.Sprintf("s.project_id=$%d", len(args)))
	}
	if filter.Class == "UNCLASSIFIED" {
		where = append(where, "s.current_classification=''")
	} else if filter.Class == "NO_BOTS" {
		where = append(where, "s.current_classification NOT IN ('BOT','KNOWN_BOT')")
	} else if filter.Class != "" {
		args = append(args, filter.Class)
		where = append(where, fmt.Sprintf("s.current_classification=$%d", len(args)))
	}
	if filter.BotName != "" {
		args = append(args, filter.BotName)
		where = append(where, fmt.Sprintf("s.bot_name=$%d", len(args)))
	}
	if filter.BotCategory != "" {
		args = append(args, filter.BotCategory)
		where = append(where, fmt.Sprintf("s.bot_category=$%d AND s.bot_name<>''", len(args)))
	}
	if filter.PaidOnly {
		where = append(where, `s.traffic_channel IN ('paid','paid_social','display')`)
	}

	for _, pair := range []struct{ column, value string }{{"traffic_source", filter.Source}, {"traffic_channel", filter.Channel}} {
		if pair.value != "" {
			args = append(args, pair.value)
			where = append(where, fmt.Sprintf("s.%s=$%d", pair.column, len(args)))
		}
	}
	if filter.LandingURL != "" {
		args = append(args, filter.LandingURL)
		where = append(where, fmt.Sprintf("split_part(s.landing_url,'?',1)=$%d", len(args)))
	}
	if filter.CampaignSet {
		args = append(args, filter.Campaign)
		where = append(where, fmt.Sprintf("s.utm_campaign=$%d", len(args)))
	}
	if filter.DeviceSet {
		args = append(args, filter.Device)
		where = append(where, fmt.Sprintf("COALESCE(NULLIF(s.device_type,''),'unknown')=$%d", len(args)))
	}
	if filter.From != nil {
		args = append(args, *filter.From)
		where = append(where, fmt.Sprintf("s.started_at >= $%d", len(args)))
	}
	if filter.To != nil {
		args = append(args, *filter.To)
		where = append(where, fmt.Sprintf("s.started_at < $%d", len(args)))
	}
	if filter.ReplayOnly {
		where = append(where, "EXISTS(SELECT 1 FROM replay_recordings rr WHERE rr.session_id=s.id AND rr.expires_at>now())")
	}
	var dims []string
	dims, args = dimensionWhere(filter.Dims, args)
	where = append(where, dims...)
	return strings.Join(where, " AND "), args
}
