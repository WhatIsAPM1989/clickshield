package store

import (
	"context"
	"errors"

	"github.com/lamooof/clickshield/internal/domain"
)

type QualityRow struct {
	DeviceType     string `json:"device_type"`
	TrafficSource  string `json:"traffic_source"`
	Reviewed       int64  `json:"reviewed"`
	Qualified      int64  `json:"qualified"`
	Spam           int64  `json:"spam"`
	FalsePositives int64  `json:"false_positives"`
	TruePositives  int64  `json:"true_positives"`
	FalseNegatives int64  `json:"false_negatives"`
}

// ErrLeadConflict means the session does not exist in the project or the lead
// is already linked to another session.
var ErrLeadConflict = errors.New("session not found or lead belongs to another session")

func (s *Store) SaveLeadOutcome(ctx context.Context, projectID, sessionID, externalID, status, source, adminID string) error {
	var actor any
	if adminID != "" {
		actor = adminID
	}
	tag, err := s.Pool.Exec(ctx, `INSERT INTO lead_outcomes(project_id,session_id,external_id,status,source,updated_by)
	SELECT s.project_id,s.id,$3,$4,$5,$6 FROM sessions s WHERE s.id=$2 AND ($1='' OR s.project_id=NULLIF($1,'')::uuid)
	ON CONFLICT(project_id,external_id) DO UPDATE SET status=EXCLUDED.status,source=EXCLUDED.source,updated_by=EXCLUDED.updated_by,updated_at=now()
	WHERE lead_outcomes.session_id=EXCLUDED.session_id AND (EXCLUDED.source='MANUAL' OR lead_outcomes.source<>'MANUAL')`, projectID, sessionID, externalID, status, source, actor)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	// A server retry of a lead already reviewed by hand keeps the manual status;
	// it is a duplicate delivery, not a conflict.
	var kept bool
	if err = s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lead_outcomes o JOIN sessions s ON s.project_id=o.project_id AND s.id=o.session_id
	WHERE o.session_id=$2 AND o.external_id=$3 AND o.source='MANUAL' AND $4<>'MANUAL' AND ($1='' OR s.project_id=NULLIF($1,'')::uuid))`, projectID, sessionID, externalID, source).Scan(&kept); err != nil {
		return err
	}
	if !kept {
		return ErrLeadConflict
	}
	return nil
}

func (s *Store) SessionOutcomes(ctx context.Context, sessionID string) ([]domain.LeadOutcome, error) {
	rows, err := s.Pool.Query(ctx, `SELECT external_id,session_id::text,status,source,updated_at FROM lead_outcomes WHERE session_id=$1 ORDER BY updated_at DESC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.LeadOutcome
	for rows.Next() {
		var row domain.LeadOutcome
		if err = rows.Scan(&row.ExternalID, &row.SessionID, &row.Status, &row.Source, &row.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) Quality(ctx context.Context, projectID string) ([]QualityRow, error) {
	rows, err := s.Pool.Query(ctx, `SELECT s.device_type,
	s.traffic_source AS traffic_source,
	count(*) AS reviewed,
	count(*) FILTER (WHERE o.status='QUALIFIED') AS qualified,
	count(*) FILTER (WHERE o.status='SPAM') AS spam,
	count(*) FILTER (WHERE o.status='QUALIFIED' AND auto.classification IN ('BOT','KNOWN_BOT')) AS false_positives,
	count(*) FILTER (WHERE o.status='SPAM' AND auto.classification IN ('BOT','KNOWN_BOT')) AS true_positives,
	count(*) FILTER (WHERE o.status='SPAM' AND auto.classification IN ('HUMAN','SUSPICIOUS')) AS false_negatives
	FROM lead_outcomes o JOIN sessions s ON s.id=o.session_id
	JOIN LATERAL (SELECT classification FROM classifications WHERE session_id=s.id AND source='AUTO' ORDER BY id DESC LIMIT 1) auto ON true
	WHERE o.status IN ('QUALIFIED','SPAM') AND ($1='' OR s.project_id=NULLIF($1,'')::uuid)
	GROUP BY s.device_type,traffic_source ORDER BY reviewed DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []QualityRow
	for rows.Next() {
		var row QualityRow
		if err = rows.Scan(&row.DeviceType, &row.TrafficSource, &row.Reviewed, &row.Qualified, &row.Spam, &row.FalsePositives, &row.TruePositives, &row.FalseNegatives); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
