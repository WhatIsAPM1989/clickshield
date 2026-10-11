package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lamooof/clickshield/internal/antifraud"
	"github.com/lamooof/clickshield/internal/domain"
)

func (s *Store) UpsertIntegration(ctx context.Context, projectID, provider string, enabled bool, cfg any, secret []byte) error {
	b, e := json.Marshal(cfg)
	if e != nil {
		return e
	}
	id, e := UUID()
	if e != nil {
		return e
	}
	_, e = s.Pool.Exec(ctx, `INSERT INTO integrations(id,project_id,provider,enabled,config,secret_encrypted) VALUES($1,$2,$3,$4,$5::jsonb,$6) ON CONFLICT(project_id,provider) DO UPDATE SET enabled=EXCLUDED.enabled,config=EXCLUDED.config,secret_encrypted=COALESCE(EXCLUDED.secret_encrypted,integrations.secret_encrypted),updated_at=now()`, id, projectID, provider, enabled, b, secret)
	return e
}
func (s *Store) Integrations(ctx context.Context, projectID string) ([]domain.Integration, error) {
	rows, e := s.Pool.Query(ctx, `SELECT id::text,project_id::text,provider,enabled,config,secret_encrypted,(secret_encrypted IS NOT NULL),last_status,last_error,last_sent_at FROM integrations WHERE project_id=$1 ORDER BY provider`, projectID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []domain.Integration
	for rows.Next() {
		var x domain.Integration
		var cfg []byte
		if e = rows.Scan(&x.ID, &x.ProjectID, &x.Provider, &x.Enabled, &cfg, &x.SecretEncrypted, &x.HasSecret, &x.LastStatus, &x.LastError, &x.LastSentAt); e != nil {
			return nil, e
		}
		x.Config = cfg
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *Store) ClaimOutbox(ctx context.Context) (domain.OutboxClaim, bool, error) {
	lease, err := UUID()
	if err != nil {
		return domain.OutboxClaim{}, false, err
	}
	claim := domain.OutboxClaim{Lease: lease}
	err = s.Pool.QueryRow(ctx, `WITH c AS (SELECT id FROM integration_outbox WHERE status IN ('PENDING','RETRY') AND next_attempt_at<=now() ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1) UPDATE integration_outbox o SET status='SENDING',locked_at=now(),lease=$1,attempts=attempts+1 FROM c WHERE o.id=c.id RETURNING o.id,o.attempts`, lease).Scan(&claim.ID, &claim.Attempts)
	if IsNotFound(err) {
		return domain.OutboxClaim{}, false, nil
	}
	return claim, err == nil, err
}
func (s *Store) Delivery(ctx context.Context, id int64) (domain.Delivery, error) {
	var d domain.Delivery
	var cfg []byte
	e := s.Pool.QueryRow(ctx, `SELECT o.id,o.event_name,o.attempts,i.id::text,i.project_id::text,i.provider,i.enabled,i.config,i.secret_encrypted,i.last_status,i.last_error,i.last_sent_at,c.id,c.session_id::text,c.risk_score,c.classification,c.source,c.created_at FROM integration_outbox o JOIN integrations i ON i.id=o.integration_id JOIN classifications c ON c.id=o.classification_id WHERE o.id=$1`, id).Scan(&d.OutboxID, &d.EventName, &d.Attempts, &d.Integration.ID, &d.Integration.ProjectID, &d.Integration.Provider, &d.Integration.Enabled, &cfg, &d.Integration.SecretEncrypted, &d.Integration.LastStatus, &d.Integration.LastError, &d.Integration.LastSentAt, &d.Classification.ID, &d.Classification.SessionID, &d.Classification.RiskScore, &d.Classification.Classification, &d.Classification.Source, &d.Classification.CreatedAt)
	if e != nil {
		return d, e
	}
	d.Integration.Config = cfg
	d.Session, e = s.SessionContext(ctx, d.Classification.SessionID)
	if e != nil {
		return d, e
	}
	rows, e := s.Pool.Query(ctx, `SELECT code,weight,details FROM classification_reasons WHERE classification_id=$1 ORDER BY id`, d.Classification.ID)
	if e != nil {
		return d, e
	}
	defer rows.Close()
	for rows.Next() {
		var r domain.Reason
		var raw []byte
		if e = rows.Scan(&r.Code, &r.Weight, &raw); e != nil {
			return d, e
		}
		_ = json.Unmarshal(raw, &r.Details)
		d.Classification.Reasons = append(d.Classification.Reasons, r)
	}
	return d, rows.Err()
}
func (s *Store) FinishDelivery(ctx context.Context, claim domain.OutboxClaim, status, lastError string, retryAfter time.Duration) error {
	var next any
	if retryAfter > 0 {
		next = time.Now().UTC().Add(retryAfter)
	}
	tag, e := s.Pool.Exec(ctx, `WITH u AS (UPDATE integration_outbox SET status=$2,last_error=$3,next_attempt_at=COALESCE($4,next_attempt_at),sent_at=CASE WHEN $2='SENT' THEN now() ELSE sent_at END WHERE id=$1 AND lease=$5 AND status='SENDING' AND locked_at>now()-interval '5 minutes' RETURNING integration_id) UPDATE integrations i SET last_status=$2,last_error=$3,last_sent_at=CASE WHEN $2='SENT' THEN now() ELSE i.last_sent_at END,updated_at=now() FROM u WHERE i.id=u.integration_id`, claim.ID, status, lastError, next, claim.Lease)
	if e == nil && tag.RowsAffected() == 0 {
		return antifraud.ErrChanged
	}
	return e
}
func (s *Store) ResetStuckOutbox(ctx context.Context) error {
	_, e := s.Pool.Exec(ctx, `UPDATE integration_outbox SET status='RETRY',lease=NULL,locked_at=NULL,next_attempt_at=now(),last_error='worker lock expired' WHERE status='SENDING' AND locked_at<now()-interval '5 minutes'`)
	return e
}
func (s *Store) OutboxCounts(ctx context.Context) (map[string]int64, error) {
	rows, e := s.Pool.Query(ctx, `SELECT status,count(*) FROM integration_outbox GROUP BY status`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var k string
		var v int64
		if e = rows.Scan(&k, &v); e != nil {
			return nil, e
		}
		out[k] = v
	}
	return out, rows.Err()
}
func (s *Store) AssertProjectSession(ctx context.Context, projectID, sessionID string) error {
	var ok bool
	e := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sessions WHERE id=$1 AND project_id=$2)`, sessionID, projectID).Scan(&ok)
	if e != nil {
		return e
	}
	if !ok {
		return fmt.Errorf("session not found")
	}
	return nil
}

// EnsureIntegration imports initial settings only; UI changes survive restarts.
func (s *Store) EnsureIntegration(ctx context.Context, projectID, provider string, cfg any, secret []byte) error {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	id, err := UUID()
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO integrations(id,project_id,provider,enabled,config,secret_encrypted) VALUES($1,$2,$3,true,$4,$5) ON CONFLICT(project_id,provider) DO NOTHING`, id, projectID, provider, raw, secret)
	return err
}
