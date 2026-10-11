package analytics

import (
	"context"
	"github.com/lamooof/clickshield/internal/domain"
)

type TrafficRow struct {
	Source     string `json:"source"`
	Channel    string `json:"channel"`
	Campaign   string `json:"campaign"`
	LandingURL string `json:"landing_url"`
	Visits     int64  `json:"visits"`
	Visitors   int64  `json:"visitors"`
	Bots       int64  `json:"bots"`
	KnownBots  int64  `json:"known_bots"`
	Leads      int64  `json:"leads"`
	Qualified  int64  `json:"qualified"`
	Replays    int64  `json:"replays"`
}

func (s *Repository) TrafficReport(ctx context.Context, f SessionFilter) ([]TrafficRow, error) {
	where, args := sessionWhere(f)
	rows, e := s.Pool.Query(ctx, `SELECT s.traffic_source,s.traffic_channel,s.utm_campaign,split_part(s.landing_url,'?',1),count(*),count(DISTINCT s.visitor_id),count(*) FILTER(WHERE s.current_classification='BOT'),count(*) FILTER(WHERE s.current_classification='KNOWN_BOT'),COALESCE(sum(o.leads),0)::bigint,COALESCE(sum(o.qualified),0)::bigint,count(*) FILTER(WHERE EXISTS(SELECT 1 FROM analytics.replay_recordings rr WHERE rr.session_id=s.id AND rr.expires_at>now())) FROM analytics.sessions s LEFT JOIN LATERAL(SELECT count(*) AS leads,count(*) FILTER(WHERE status='QUALIFIED') AS qualified FROM analytics.lead_outcomes WHERE session_id=s.id) o ON true WHERE `+where+` GROUP BY s.traffic_source,s.traffic_channel,s.utm_campaign,split_part(s.landing_url,'?',1) ORDER BY count(*) DESC,s.traffic_source,s.traffic_channel,s.utm_campaign,split_part(s.landing_url,'?',1)`, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []TrafficRow{}
	for rows.Next() {
		var r TrafficRow
		if e = rows.Scan(&r.Source, &r.Channel, &r.Campaign, &r.LandingURL, &r.Visits, &r.Visitors, &r.Bots, &r.KnownBots, &r.Leads, &r.Qualified, &r.Replays); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *Repository) FilteredStats(ctx context.Context, f SessionFilter) (domain.Stats, error) {
	where, args := sessionWhere(f)
	var st domain.Stats
	e := s.Pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE s.current_classification='HUMAN'),count(*) FILTER(WHERE s.current_classification='SUSPICIOUS'),count(*) FILTER(WHERE s.current_classification='BOT'),count(*) FILTER(WHERE s.current_classification='KNOWN_BOT'),count(*) FILTER(WHERE s.current_classification=''),COALESCE(avg(s.current_risk_score),0),count(*) FILTER(WHERE s.traffic_channel IN ('paid','paid_social','display')) FROM analytics.sessions s WHERE `+where, args...).Scan(&st.Total, &st.Human, &st.Suspicious, &st.Bot, &st.KnownBot, &st.Unclassified, &st.AverageRisk, &st.PaidVisits)
	return st, e
}
