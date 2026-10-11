package store

import (
	"context"
	"github.com/lamooof/clickshield/internal/domain"
	"github.com/lamooof/clickshield/internal/traffic"
	"time"
)

func (s *Store) SessionPages(ctx context.Context, id string) ([]domain.SessionPage, error) {
	rows, e := s.Pool.Query(ctx, `SELECT page_index,page_url,viewed_at FROM session_pages WHERE session_id=$1 ORDER BY viewed_at,page_index`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.SessionPage{}
	for rows.Next() {
		var p domain.SessionPage
		if e = rows.Scan(&p.Index, &p.URL, &p.ViewedAt); e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Existing records cannot recover a referrer that earlier tracker versions overwrote.
// Mark the attribution as historical rather than presenting it as a fresh observation.
func (s *Store) BackfillTraffic(ctx context.Context) error {
	for {
		rows, e := s.Pool.Query(ctx, `SELECT s.id::text,s.referrer,s.landing_url,s.utm_source,s.utm_medium,s.utm_campaign,s.yclid,s.gclid,s.rb_clickid,p.allowed_origins FROM sessions s JOIN projects p ON p.id=s.project_id WHERE s.traffic_channel='' LIMIT 500`)
		if e != nil {
			return e
		}
		type item struct {
			id string
			a  traffic.Attribution
		}
		items := []item{}
		for rows.Next() {
			var b domain.EventBatch
			var id string
			var origins []string
			if e = rows.Scan(&id, &b.Referrer, &b.LandingURL, &b.UTM.Source, &b.UTM.Medium, &b.UTM.Campaign, &b.YCLID, &b.GCLID, &b.RBClickID, &origins); e != nil {
				rows.Close()
				return e
			}
			a := traffic.Classify(b, origins)
			a.Basis = "historical_" + a.Basis
			items = append(items, item{id, a})
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(items) == 0 {
			return nil
		}
		for _, i := range items {
			if _, e = s.Pool.Exec(ctx, `UPDATE sessions SET traffic_source=$2,traffic_channel=$3,referrer_host=$4,attribution_basis=$5 WHERE id=$1 AND traffic_channel=''`, i.id, i.a.Source, i.a.Channel, i.a.ReferrerHost, i.a.Basis); e != nil {
				return e
			}
		}
	}
}

func (s *Store) UpdateReplaySettings(ctx context.Context, id string, enabled bool, rate, days int) error {
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	tag, e := tx.Exec(ctx, `UPDATE projects SET replay_enabled=$2,replay_sample_rate=$3,replay_retention_days=$4,updated_at=now() WHERE id=$1`, id, enabled, rate, days)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if _, e = tx.Exec(ctx, `UPDATE replay_recordings SET expires_at=LEAST(expires_at,started_at+make_interval(days=>$2)) WHERE project_id=$1`, id, days); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

var ErrNotFound = &notFoundError{}

type notFoundError struct{}

func (*notFoundError) Error() string { return "not found" }
func replayExpiry(days int) time.Time {
	return time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour)
}
