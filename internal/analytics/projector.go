package analytics

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
)

type Projector struct{ pool *pgxpool.Pool }

func NewProjector(pool *pgxpool.Pool) *Projector { return &Projector{pool} }

var projectionColumns = map[string]string{
	"sessions":          "id,project_id,visitor_id,started_at,last_seen_at,traffic_source,traffic_channel,referrer_host,attribution_basis,landing_url,last_page_url,utm_source,utm_medium,utm_campaign,utm_content,utm_term,device_type,device,user_agent,country,region,city,asn,as_org,browser,os,page_count,current_classification,current_risk_score,bot_name,bot_category",
	"session_features":  "session_id,duration_ms,active_time_ms,page_count",
	"session_pages":     "session_id,page_index,page_url,viewed_at",
	"business_events":   "id,project_id,session_id,event_time,page_url,name,properties",
	"heatmap_events":    "session_id,batch_id,event_index,event_time,page_url,event_type,viewport_width,viewport_height,document_height,x,y,depth",
	"lead_outcomes":     "id,project_id,session_id,status,created_at,updated_at",
	"replay_recordings": "id,project_id,session_id,expires_at",
}

// ProjectOne commits the projection and acknowledgement together. A per-session
// advisory lock prevents concurrent workers from overwriting a newer snapshot.
func (p *Projector) ProjectOne(ctx context.Context) (bool, error) {
	var sid string
	ok, err := p.projectOne(ctx, &sid)
	if err != nil && sid != "" && ctx.Err() == nil {
		_, retryErr := p.pool.Exec(ctx, `UPDATE analytics_projection_outbox SET attempts=attempts+1,last_error=$2,next_attempt_at=now()+interval '1 minute' WHERE session_id=$1`, sid, err.Error())
		err = errors.Join(err, retryErr)
	}
	return ok, err
}

func (p *Projector) projectOne(ctx context.Context, selected *string) (bool, error) {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var id int64
	var sid, pid string
	err = tx.QueryRow(ctx, `SELECT id,session_id::text,project_id::text FROM analytics_projection_outbox WHERE next_attempt_at<=now() ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&id, &sid, &pid)
	if IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	*selected = sid
	var locked bool
	if err = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended('analytics:'||$1,0))`, sid).Scan(&locked); err != nil {
		return false, err
	}
	if !locked {
		return false, nil
	}
	if err = tx.QueryRow(ctx, `SELECT max(id) FROM analytics_projection_outbox WHERE session_id=$1`, sid).Scan(&id); err != nil {
		return false, err
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM projects WHERE id=$1)`, pid).Scan(&exists); err != nil {
		return false, err
	}
	if !exists {
		return true, deleteProject(ctx, tx, pid)
	}
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sessions WHERE id=$1)`, sid).Scan(&exists); err != nil {
		return false, err
	}
	if !exists {
		if _, err = tx.Exec(ctx, `DELETE FROM analytics.sessions WHERE id=$1`, sid); err != nil {
			return false, err
		}
	} else {
		for _, table := range []string{"sessions", "session_features", "session_pages", "business_events", "heatmap_events", "lead_outcomes", "replay_recordings"} {
			if err = projectTable(ctx, tx, table, sid); err != nil {
				return false, err
			}
		}
	}
	// Only acknowledge rows already visible before the source reads. Later writes
	// always retain a queue entry, even if they race this transaction.
	if _, err = tx.Exec(ctx, `DELETE FROM analytics_projection_outbox WHERE session_id=$1 AND id<=$2`, sid, id); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func deleteProject(ctx context.Context, tx pgx.Tx, pid string) error {
	for _, table := range []string{"sessions", "analytics_definitions", "analytics_api_keys", "tracked_links", "tracking_pixels", "tracking_hits", "analytics_annotations"} {
		if _, err := tx.Exec(ctx, "DELETE FROM analytics."+table+" WHERE project_id=$1", pid); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM analytics_projection_outbox WHERE project_id=$1`, pid); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func projectTable(ctx context.Context, tx pgx.Tx, table, sid string) error {
	columns := strings.Split(projectionColumns[table], ",")
	key := "session_id"
	conflict := "session_id"
	switch table {
	case "sessions":
		key, conflict = "id", "id"
	case "session_pages":
		conflict = "session_id,page_index"
	case "heatmap_events":
		conflict = "session_id,batch_id,event_index"
	case "business_events", "lead_outcomes", "replay_recordings":
		conflict = "id"
	}
	var updates []string
	for _, column := range columns {
		updates = append(updates, column+"=EXCLUDED."+column)
	}
	query := fmt.Sprintf("INSERT INTO analytics.%s(%s) SELECT %s FROM %s WHERE %s=$1 ON CONFLICT(%s) DO UPDATE SET %s", table, strings.Join(columns, ","), strings.Join(columns, ","), table, key, conflict, strings.Join(updates, ","))
	if _, err := tx.Exec(ctx, query, sid); err != nil {
		return err
	}
	// Remove expired/deleted child rows from the snapshot as well.
	if table != "sessions" {
		query = fmt.Sprintf("DELETE FROM analytics.%s a WHERE a.session_id=$1 AND NOT EXISTS(SELECT 1 FROM %s s WHERE %s)", table, table, projectionIdentity(table))
		_, err := tx.Exec(ctx, query, sid)
		return err
	}
	return nil
}

func projectionIdentity(table string) string {
	switch table {
	case "session_pages":
		return "s.session_id=a.session_id AND s.page_index=a.page_index"
	case "heatmap_events":
		return "s.session_id=a.session_id AND s.batch_id=a.batch_id AND s.event_index=a.event_index"
	case "session_features":
		return "s.session_id=a.session_id"
	default:
		return "s.id=a.id"
	}
}
