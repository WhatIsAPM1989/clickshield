package store

import (
	"context"
	"encoding/json"

	"github.com/lamooof/clickshield/internal/domain"
)

func (s *Store) SessionDetail(ctx context.Context, id string) (domain.SessionDetail, error) {
	var d domain.SessionDetail
	e := s.Pool.QueryRow(ctx, `SELECT s.id::text,s.project_id::text,p.name,s.started_at,s.last_seen_at,COALESCE(f.duration_ms,0),s.current_classification,s.current_risk_score,s.yandex_client_id,s.google_client_id,s.yclid,s.gclid,s.rb_clickid,s.utm_source,s.utm_medium,s.utm_campaign,COALESCE(f.click_count,0),COALESCE(f.scroll_depth,0),s.fingerprint,COALESCE(host(s.ip),''),s.asn,s.as_org,s.device_type,s.manual_label,COALESCE(s.bot_name,''),s.bot_category,s.traffic_source,s.traffic_channel,s.referrer_host,s.attribution_basis,s.landing_url,EXISTS(SELECT 1 FROM replay_recordings rr WHERE rr.session_id=s.id AND rr.expires_at>now()) FROM sessions s JOIN projects p ON p.id=s.project_id LEFT JOIN session_features f ON f.session_id=s.id WHERE s.id=$1`, id).Scan(&d.Session.ID, &d.Session.ProjectID, &d.Session.ProjectName, &d.Session.StartedAt, &d.Session.LastSeenAt, &d.Session.DurationMS, &d.Session.Classification, &d.Session.RiskScore, &d.Session.YandexClientID, &d.Session.GoogleClientID, &d.Session.YCLID, &d.Session.GCLID, &d.Session.RBClickID, &d.Session.UTMSource, &d.Session.UTMMedium, &d.Session.UTMCampaign, &d.Session.ClickCount, &d.Session.ScrollDepth, &d.Session.Fingerprint, &d.Session.IP, &d.Session.ASN, &d.Session.ASOrg, &d.Session.DeviceType, &d.Session.ManualLabel, &d.Session.BotName, &d.Session.BotCategory, &d.Session.TrafficSource, &d.Session.TrafficChannel, &d.Session.ReferrerHost, &d.Session.AttributionBasis, &d.Session.LandingURL, &d.Session.HasReplay)
	if e != nil {
		return d, e
	}
	if d.Context, e = s.SessionContext(ctx, id); e != nil {
		return d, e
	}
	var f domain.Features
	var intervals []byte
	e = s.Pool.QueryRow(ctx, `SELECT session_id::text,duration_ms,active_time_ms,idle_time_ms,mouse_distance,mouse_velocity,mouse_max_velocity,mouse_p95_velocity,mouse_acceleration,mouse_curvature,mouse_pauses,mouse_moves,scroll_depth,scroll_velocity,scroll_reversals,scroll_events,click_count,click_intervals_ms,mean_click_interval_ms,time_to_first_click_ms,page_count,interaction_density,active_ratio,paid_visit_frequency,sessions_per_ip,sessions_per_fingerprint,computed_at FROM session_features WHERE session_id=$1`, id).Scan(&f.SessionID, &f.DurationMS, &f.ActiveTimeMS, &f.IdleTimeMS, &f.MouseDistance, &f.MouseVelocity, &f.MouseMaxVelocity, &f.MouseP95Velocity, &f.MouseAcceleration, &f.MouseCurvature, &f.MousePauses, &f.MouseMoves, &f.ScrollDepth, &f.ScrollVelocity, &f.ScrollReversals, &f.ScrollEvents, &f.ClickCount, &intervals, &f.MeanClickIntervalMS, &f.TimeToFirstClickMS, &f.PageCount, &f.InteractionDensity, &f.ActiveRatio, &f.PaidVisitFrequency, &f.SessionsPerIP, &f.SessionsPerFingerprint, &f.ComputedAt)
	if e == nil {
		_ = json.Unmarshal(intervals, &f.ClickIntervalsMS)
		d.Features = &f
	} else if !IsNotFound(e) {
		return d, e
	}
	rows, e := s.Pool.Query(ctx, `SELECT id,risk_score,classification,source,created_at FROM classifications WHERE session_id=$1 ORDER BY created_at DESC,id DESC`, id)
	if e != nil {
		return d, e
	}
	for rows.Next() {
		var c domain.Classification
		c.SessionID = id
		if e = rows.Scan(&c.ID, &c.RiskScore, &c.Classification, &c.Source, &c.CreatedAt); e != nil {
			rows.Close()
			return d, e
		}
		rr, er := s.Pool.Query(ctx, `SELECT code,weight,details FROM classification_reasons WHERE classification_id=$1 ORDER BY id`, c.ID)
		if er != nil {
			rows.Close()
			return d, er
		}
		for rr.Next() {
			var r domain.Reason
			var raw []byte
			if er = rr.Scan(&r.Code, &r.Weight, &raw); er != nil {
				rr.Close()
				rows.Close()
				return d, er
			}
			_ = json.Unmarshal(raw, &r.Details)
			c.Reasons = append(c.Reasons, r)
		}
		if er = rr.Err(); er != nil {
			rr.Close()
			rows.Close()
			return d, er
		}
		rr.Close()
		d.Classifications = append(d.Classifications, c)
	}
	if e = rows.Err(); e != nil {
		rows.Close()
		return d, e
	}
	rows.Close()
	tr, e := s.Pool.Query(ctx, `SELECT event_type,(payload->>'ts')::bigint,COALESCE((payload->>'x')::double precision,0),COALESCE((payload->>'y')::double precision,0),COALESCE((payload->>'vw')::double precision,0),COALESCE((payload->>'vh')::double precision,0),COALESCE((payload->>'depth')::double precision,0) FROM raw_events WHERE session_id=$1 AND event_type IN ('mousemove','click','scroll') ORDER BY event_time,id LIMIT 1500`, id)
	if e != nil {
		return d, e
	}
	for tr.Next() {
		var p domain.TrajectoryPoint
		if e = tr.Scan(&p.Type, &p.TS, &p.X, &p.Y, &p.VW, &p.VH, &p.Depth); e != nil {
			tr.Close()
			return d, e
		}
		d.Trajectory = append(d.Trajectory, p)
	}
	if e = tr.Err(); e != nil {
		tr.Close()
		return d, e
	}
	tr.Close()
	d.Pages, e = s.SessionPages(ctx, id)
	if e != nil {
		return d, e
	}
	d.Outcomes, e = s.SessionOutcomes(ctx, id)
	if e != nil {
		return d, e
	}
	return d, nil
}
