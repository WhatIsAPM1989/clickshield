package store

import (
	"context"
	"strconv"
	"time"

	"github.com/lamooof/clickshield/internal/asn"
	"github.com/lamooof/clickshield/internal/bots"
)

// LabelBots fills bot_name and bot_category for sessions stored before the
// labels existed or after a catalog change reset them. Recent automatic
// classifications that disagree with the new labels are queued for re-scoring
// while their raw events are still retained.
func (s *Store) LabelBots(ctx context.Context) (labeled, requeued int64, err error) {
	for {
		rows, e := s.Pool.Query(ctx, `SELECT id::text,user_agent,COALESCE(host(ip),'') FROM sessions WHERE bot_name IS NULL LIMIT 1000`)
		if e != nil {
			return labeled, 0, e
		}
		var ids, names, categories []string
		for rows.Next() {
			var id, ua, ip string
			if e = rows.Scan(&id, &ua, &ip); e != nil {
				rows.Close()
				return labeled, 0, e
			}
			bot, _ := bots.Identify(ua, ip)
			ids, names, categories = append(ids, id), append(names, bot.Name), append(categories, bot.Category)
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return labeled, 0, e
		}
		if len(ids) == 0 {
			break
		}
		tag, e := s.Pool.Exec(ctx, `UPDATE sessions s SET bot_name=v.name,bot_category=v.category,data_revision=s.data_revision+1 FROM unnest($1::uuid[],$2::text[],$3::text[]) AS v(id,name,category) WHERE s.id=v.id`, ids, names, categories)
		if e != nil {
			return labeled, 0, e
		}
		labeled += tag.RowsAffected()
	}
	if labeled == 0 {
		return 0, 0, nil
	}
	tag, e := s.Pool.Exec(ctx, `UPDATE sessions SET finished_at=NULL,finish_requested_at=NULL,updated_at=now()
WHERE manual_label='' AND started_at>now()-interval '6 days' AND current_classification<>''
  AND ((bot_name<>'' AND bot_category<>$1 AND current_classification<>'KNOWN_BOT')
    OR ((bot_name='' OR bot_category=$1) AND current_classification='KNOWN_BOT'))`, bots.Automation)
	if e != nil {
		return labeled, 0, e
	}
	return labeled, tag.RowsAffected(), nil
}

type BotStat struct {
	bots.Bot
	Sessions     int64     `json:"sessions"`
	Last24h      int64     `json:"last_24h"`
	Last7d       int64     `json:"last_7d"`
	Visitors     int64     `json:"visitors"`
	IPs          int64     `json:"ips"`
	KnownBot     int64     `json:"known_bot"`
	BotClass     int64     `json:"bot"`
	Suspicious   int64     `json:"suspicious"`
	Human        int64     `json:"human"`
	Unclassified int64     `json:"unclassified"`
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
}

// BotStats counts sessions per known bot, most active first.
func (s *Store) BotStats(ctx context.Context, projectID string) ([]BotStat, error) {
	var arg any
	if projectID != "" {
		arg = projectID
	}
	rows, e := s.Pool.Query(ctx, `SELECT bot_name,count(*),count(*) FILTER(WHERE started_at>now()-interval '24 hours'),count(*) FILTER(WHERE started_at>now()-interval '7 days'),count(DISTINCT visitor_id),count(DISTINCT ip),
count(*) FILTER(WHERE current_classification='KNOWN_BOT'),count(*) FILTER(WHERE current_classification='BOT'),count(*) FILTER(WHERE current_classification='SUSPICIOUS'),count(*) FILTER(WHERE current_classification='HUMAN'),count(*) FILTER(WHERE current_classification=''),
min(started_at),max(last_seen_at)
FROM sessions WHERE bot_name<>'' AND ($1::uuid IS NULL OR project_id=$1) GROUP BY bot_name ORDER BY count(*) DESC,bot_name`, arg)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []BotStat{}
	for rows.Next() {
		var x BotStat
		var name string
		if e = rows.Scan(&name, &x.Sessions, &x.Last24h, &x.Last7d, &x.Visitors, &x.IPs, &x.KnownBot, &x.BotClass, &x.Suspicious, &x.Human, &x.Unclassified, &x.FirstSeen, &x.LastSeen); e != nil {
			return nil, e
		}
		x.Bot = bots.Lookup(name)
		out = append(out, x)
	}
	return out, rows.Err()
}

// FillNetworks fills the autonomous system of sessions stored without one,
// because the proxy sent no ASN headers or the database was not loaded yet,
// and queues recent automatic classifications from datacenter networks for
// re-scoring.
func (s *Store) FillNetworks(ctx context.Context) (filled, requeued int64, err error) {
	last := "00000000-0000-0000-0000-000000000000"
	for {
		rows, e := s.Pool.Query(ctx, `SELECT id::text,host(ip) FROM sessions WHERE asn='' AND ip IS NOT NULL AND id>$1::uuid ORDER BY id LIMIT 1000`, last)
		if e != nil {
			return filled, 0, e
		}
		var ids, numbers, orgs []string
		n := 0
		for rows.Next() {
			var id, ip string
			if e = rows.Scan(&id, &ip); e != nil {
				rows.Close()
				return filled, 0, e
			}
			n, last = n+1, id
			if info, ok := asn.Lookup(ip); ok {
				ids, numbers, orgs = append(ids, id), append(numbers, strconv.FormatUint(uint64(info.Number), 10)), append(orgs, info.Org)
			}
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return filled, 0, e
		}
		if n == 0 {
			break
		}
		if len(ids) == 0 {
			continue
		}
		tag, e := s.Pool.Exec(ctx, `UPDATE sessions s SET asn=v.asn,as_org=left(v.org,160),data_revision=s.data_revision+1 FROM unnest($1::uuid[],$2::text[],$3::text[]) AS v(id,asn,org) WHERE s.id=v.id`, ids, numbers, orgs)
		if e != nil {
			return filled, 0, e
		}
		filled += tag.RowsAffected()
	}
	if filled == 0 {
		return 0, 0, nil
	}
	tag, e := s.Pool.Exec(ctx, `UPDATE sessions SET finished_at=NULL,finish_requested_at=NULL,updated_at=now()
WHERE manual_label='' AND started_at>now()-interval '6 days' AND current_classification IN ('HUMAN','SUSPICIOUS','BOT')
  AND asn=ANY($1::text[])`, asn.Datacenter())
	if e != nil {
		return filled, 0, e
	}
	return filled, tag.RowsAffected(), nil
}
