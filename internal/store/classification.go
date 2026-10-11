package store

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/lamooof/clickshield/internal/antifraud"
	"github.com/lamooof/clickshield/internal/domain"
	"reflect"
)

func equalJSON(a, b []byte) bool {
	var x, y any
	return json.Unmarshal(a, &x) == nil && json.Unmarshal(b, &y) == nil && reflect.DeepEqual(x, y)
}

func (s *Store) SaveScore(ctx context.Context, write antifraud.ScoreWrite) (int64, error) {
	return s.saveClassification(ctx, write.Features, write.Score, "AUTO", "", "", &write, nil)
}

func (s *Store) SaveAutomatic(ctx context.Context, f domain.Features, score domain.Score) (int64, error) {
	return s.saveClassification(ctx, f, score, "AUTO", "", "", nil, nil)
}
func (s *Store) SaveManual(ctx context.Context, id, label, note, user string) (int64, error) {
	return antifraud.Label(ctx, s, id, label, note, user)
}

func (s *Store) SaveManualScore(ctx context.Context, w antifraud.ManualWrite) (int64, error) {
	return s.saveClassification(ctx, w.Features, w.Score, "MANUAL", w.UserID, w.Note, nil, &w)
}

func (s *Store) saveClassification(ctx context.Context, f domain.Features, score domain.Score, source, userID, note string, write *antifraud.ScoreWrite, manual *antifraud.ManualWrite) (int64, error) {
	tx, e := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if e != nil {
		return 0, e
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var revision int64
	var lease, mode, label string
	var config []byte
	var validLease bool
	e = tx.QueryRow(ctx, `SELECT s.data_revision,COALESCE(s.scoring_lease::text,''),COALESCE(s.scoring_lease_until>now(),false),p.mode,s.manual_label,p.scoring_config FROM sessions s JOIN projects p ON p.id=s.project_id WHERE s.id=$1 FOR UPDATE OF s FOR SHARE OF p`, f.SessionID).Scan(&revision, &lease, &validLease, &mode, &label, &config)
	if e != nil {
		return 0, e
	}
	if write != nil && (revision != write.Input.Revision || lease != write.Input.Lease || !validLease || !equalJSON(config, write.Input.Context.ProjectScoringConfig) || mode != write.Input.Context.ProjectMode || label != write.Input.Context.ManualLabel) {
		return 0, antifraud.ErrChanged
	}

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
	if write != nil {
		if write.Input.State != nil {
			state, err := json.Marshal(write.Input.State)
			if err != nil {
				return 0, err
			}
			if _, err = tx.Exec(ctx, `UPDATE sessions SET feature_state=COALESCE(feature_state,$2) WHERE id=$1`, f.SessionID, state); err != nil {
				return 0, err
			}
		}
		contextJSON, err := json.Marshal(write.Input.Context)
		if err != nil {
			return 0, err
		}
		featuresJSON, err := json.Marshal(f)
		if err != nil {
			return 0, err
		}
		if _, e = tx.Exec(ctx, `UPDATE classifications SET algorithm_version=$2,input_revision=$3,config_snapshot=$4,context_snapshot=$5,features_snapshot=$6 WHERE id=$1`, classID, write.Version, revision, write.Config, contextJSON, featuresJSON); e != nil {
			return 0, e
		}
		if _, e = tx.Exec(ctx, `UPDATE sessions SET scored_revision=$2,scoring_lease=NULL,scoring_lease_until=NULL WHERE id=$1`, f.SessionID, revision); e != nil {
			return 0, e
		}
	}
	name := antifraud.Notification(mode, label, score.Classification, source)
	if write != nil {
		name = write.EventName
	}
	if manual != nil {
		if mode != manual.Context.ProjectMode {
			return 0, antifraud.ErrChanged
		}
		name = manual.EventName
	}
	if name != "" {
		_, e = tx.Exec(ctx, `INSERT INTO integration_outbox(integration_id,project_id,session_id,classification_id,provider,event_name) SELECT i.id,i.project_id,$1,$2,i.provider,$3 FROM integrations i JOIN sessions s ON s.project_id=i.project_id JOIN projects p ON p.id=s.project_id WHERE s.id=$1 AND i.enabled ON CONFLICT(session_id,provider,event_name) DO NOTHING`, f.SessionID, classID, name)
		if e != nil {
			return 0, e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return 0, e
	}
	return classID, nil
}
