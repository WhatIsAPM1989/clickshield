package store

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lamooof/clickshield/internal/bots"
	"github.com/lamooof/clickshield/internal/domain"
	"github.com/lamooof/clickshield/internal/traffic"
)

type IngestResult struct {
	InsertedEvents int    `json:"inserted_events"`
	Decision       string `json:"decision"`
}

func (s *Store) Ingest(ctx context.Context, project domain.Project, b domain.EventBatch, remote domain.RemoteMeta) (IngestResult, error) {
	tx, e := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if e != nil {
		return IngestResult{}, e
	}
	defer func() { _ = tx.Rollback(ctx) }()
	visitorID, e := UUID()
	if e != nil {
		return IngestResult{}, e
	}
	e = tx.QueryRow(ctx, `INSERT INTO visitors(id,project_id,visitor_key,fingerprint) VALUES($1,$2,$3,$4) ON CONFLICT(project_id,visitor_key) DO UPDATE SET fingerprint=CASE WHEN EXCLUDED.fingerprint<>'' THEN EXCLUDED.fingerprint ELSE visitors.fingerprint END,last_seen_at=now() RETURNING id::text`, visitorID, project.ID, b.VisitorID, b.Fingerprint).Scan(&visitorID)
	if e != nil {
		return IngestResult{}, fmt.Errorf("visitor: %w", e)
	}
	device, _ := json.Marshal(b.Device)
	automation, _ := json.Marshal(b.Automation)
	var ip any
	ipText := ""
	if remote.IP != nil {
		ipText = remote.IP.String()
		ip = ipText
	}
	page := b.PageIndex
	if page < 1 {
		page = 1
	}
	bot, _ := bots.Identify(remote.UserAgent, ipText)
	var sessionID string
	e = tx.QueryRow(ctx, `INSERT INTO sessions(id,project_id,visitor_id,visitor_key,fingerprint,yandex_client_id,google_client_id,yclid,gclid,utm_source,utm_medium,utm_campaign,utm_term,utm_content,referrer,landing_url,last_page_url,ip,asn,as_org,country,user_agent,device_type,device,automation,page_count,bot_name,bot_category,rb_clickid) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24::jsonb,$25::jsonb,$26,$27,$28,$29)
	ON CONFLICT(id) DO UPDATE SET visitor_id=EXCLUDED.visitor_id,visitor_key=EXCLUDED.visitor_key,fingerprint=CASE WHEN EXCLUDED.fingerprint<>'' THEN EXCLUDED.fingerprint ELSE sessions.fingerprint END,yandex_client_id=CASE WHEN EXCLUDED.yandex_client_id<>'' THEN EXCLUDED.yandex_client_id ELSE sessions.yandex_client_id END,google_client_id=CASE WHEN EXCLUDED.google_client_id<>'' THEN EXCLUDED.google_client_id ELSE sessions.google_client_id END,yclid=CASE WHEN EXCLUDED.yclid<>'' THEN EXCLUDED.yclid ELSE sessions.yclid END,rb_clickid=CASE WHEN EXCLUDED.rb_clickid<>'' THEN EXCLUDED.rb_clickid ELSE sessions.rb_clickid END,gclid=CASE WHEN EXCLUDED.gclid<>'' THEN EXCLUDED.gclid ELSE sessions.gclid END,utm_source=sessions.utm_source,utm_medium=sessions.utm_medium,utm_campaign=sessions.utm_campaign,utm_term=sessions.utm_term,utm_content=sessions.utm_content,referrer=sessions.referrer,landing_url=CASE WHEN sessions.landing_url='' THEN EXCLUDED.landing_url ELSE sessions.landing_url END,last_page_url=EXCLUDED.last_page_url,ip=COALESCE(EXCLUDED.ip,sessions.ip),asn=CASE WHEN EXCLUDED.asn<>'' THEN EXCLUDED.asn ELSE sessions.asn END,as_org=CASE WHEN EXCLUDED.as_org<>'' THEN EXCLUDED.as_org ELSE sessions.as_org END,country=CASE WHEN EXCLUDED.country<>'' THEN EXCLUDED.country ELSE sessions.country END,user_agent=CASE WHEN EXCLUDED.user_agent<>'' THEN EXCLUDED.user_agent ELSE sessions.user_agent END,bot_name=CASE WHEN EXCLUDED.user_agent<>'' THEN EXCLUDED.bot_name ELSE sessions.bot_name END,bot_category=CASE WHEN EXCLUDED.user_agent<>'' THEN EXCLUDED.bot_category ELSE sessions.bot_category END,device_type=EXCLUDED.device_type,device=sessions.device||EXCLUDED.device,automation=sessions.automation||EXCLUDED.automation,page_count=GREATEST(sessions.page_count,EXCLUDED.page_count),last_seen_at=now(),updated_at=now() WHERE sessions.project_id=EXCLUDED.project_id AND sessions.visitor_key=EXCLUDED.visitor_key RETURNING id::text`, b.SessionID, project.ID, visitorID, b.VisitorID, b.Fingerprint, b.YandexClientID, b.GoogleClientID, b.YCLID, b.GCLID, b.UTM.Source, b.UTM.Medium, b.UTM.Campaign, b.UTM.Term, b.UTM.Content, b.Referrer, b.LandingURL, b.PageURL, ip, remote.ASN, remote.ASOrg, remote.Country, remote.UserAgent, deviceType(b.Device.Type), device, automation, page, bot.Name, bot.Category, b.RBClickID).Scan(&sessionID)
	if e != nil {
		return IngestResult{}, fmt.Errorf("session: %w", e)
	}
	browser, osName := ParseUserAgent(remote.UserAgent)
	if _, e = tx.Exec(ctx, `UPDATE sessions SET browser=$2,os=$3,region=CASE WHEN $4<>'' THEN $4 ELSE region END,city=CASE WHEN $5<>'' THEN $5 ELSE city END WHERE id=$1`, b.SessionID, browser, osName, remote.Region, remote.City); e != nil {
		return IngestResult{}, e
	}
	a := traffic.Classify(b, project.AllowedOrigins)
	if _, e = tx.Exec(ctx, `UPDATE sessions SET traffic_source=$2,traffic_channel=$3,referrer_host=$4,attribution_basis=$5 WHERE id=$1 AND traffic_channel=''`, b.SessionID, a.Source, a.Channel, a.ReferrerHost, a.Basis); e != nil {
		return IngestResult{}, e
	}
	inserted := 0
	now := time.Now().UTC()
	for i, event := range b.Events {
		payload, err := json.Marshal(event)
		if err != nil {
			return IngestResult{}, err
		}
		at := time.UnixMilli(event.TS).UTC()
		if event.TS <= 0 || at.Before(now.Add(-7*24*time.Hour)) || at.After(now.Add(10*time.Minute)) {
			at = now
		}
		tag, err := tx.Exec(ctx, `INSERT INTO raw_events(project_id,session_id,batch_id,event_index,event_time,event_type,payload) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb) ON CONFLICT(session_id,batch_id,event_index) DO NOTHING`, project.ID, b.SessionID, b.BatchID, i, at, event.Type, payload)
		if err != nil {
			return IngestResult{}, err
		}
		inserted += int(tag.RowsAffected())
		if tag.RowsAffected() > 0 && (event.Type == "business" || event.Type == "click" || event.Type == "scroll") {
			index := event.PageIndex
			if index < 1 {
				index = b.PageIndex
			}
			pageURL := b.PageURL
			// Page index resolves an older page in a batch sent after an SPA transition.
			if err = tx.QueryRow(ctx, `SELECT page_url FROM session_pages WHERE session_id=$1 AND page_index=$2`, b.SessionID, index).Scan(&pageURL); err != nil && !IsNotFound(err) {
				return IngestResult{}, err
			}
			if event.PageURL != "" {
				pageURL = event.PageURL
			}
			pageURL = strings.SplitN(pageURL, "?", 2)[0]
			if event.Type == "business" {
				props, err := json.Marshal(event.Properties)
				if err != nil {
					return IngestResult{}, err
				}
				if event.Properties == nil {
					props = []byte("{}")
				}
				if _, err = tx.Exec(ctx, `INSERT INTO business_events(project_id,session_id,batch_id,event_index,event_time,page_url,name,properties) VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb) ON CONFLICT DO NOTHING`, project.ID, b.SessionID, b.BatchID, i, at, pageURL, event.Name, props); err != nil {
					return IngestResult{}, err
				}
			} else if event.VW > 0 && event.VH > 0 {
				if _, err = tx.Exec(ctx, `INSERT INTO heatmap_events(session_id,batch_id,event_index,event_time,page_url,event_type,viewport_width,viewport_height,document_height,x,y,depth) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT DO NOTHING`, b.SessionID, b.BatchID, i, at, pageURL, event.Type, int(event.VW), int(event.VH), event.DocumentHeight, event.X, event.Y+event.ScrollY, event.Depth); err != nil {
					return IngestResult{}, err
				}
			}
		}
		if event.Type == "pageview" && tag.RowsAffected() > 0 {
			pageURL, _ := event.Extra["page_url"].(string)
			if pageURL == "" {
				pageURL = b.PageURL
			}
			index := event.PageIndex
			if index < 1 {
				index = b.PageIndex
			}
			if _, err = tx.Exec(ctx, `INSERT INTO session_pages(session_id,page_index,page_url,viewed_at) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, b.SessionID, index, pageURL, at); err != nil {
				return IngestResult{}, err
			}
		}

	}
	// Reopen a previously scored session only when this batch actually added new
	// events. An idempotent retry with the same batch_id must not do that.
	if inserted > 0 {
		if _, e = tx.Exec(ctx, `UPDATE sessions SET finished_at=NULL,updated_at=now() WHERE id=$1 AND project_id=$2`, b.SessionID, project.ID); e != nil {
			return IngestResult{}, e
		}
	}
	// A final beacon carries the finish marker together with its events. This
	// keeps short first-page visits from waiting for the stale-session timeout
	// if the separate /session/finish beacon reaches the server first or fails.
	if b.Finish {
		if _, e = tx.Exec(ctx, `UPDATE sessions SET finish_requested_at=now(),updated_at=now() WHERE id=$1 AND project_id=$2`, b.SessionID, project.ID); e != nil {
			return IngestResult{}, e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return IngestResult{}, e
	}
	decision, _ := s.Decision(ctx, project, b.VisitorID)
	return IngestResult{inserted, decision}, nil
}

func (s *Store) RequestFinish(ctx context.Context, projectID string, r domain.FinishRequest) error {
	tag, e := s.Pool.Exec(ctx, `UPDATE sessions SET finish_requested_at=now(),last_seen_at=GREATEST(last_seen_at,now()),updated_at=now() WHERE id=$1 AND project_id=$2 AND visitor_key=$3`, r.SessionID, projectID, r.VisitorID)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// Decision blocks only the visitor whose own sessions were classified as bots.
// The fingerprint is not used: it is computed by the client from the user
// agent and screen, is shared by every identical phone in one time zone and can
// be forged, so a few bot sessions with a popular fingerprint would block real
// visitors.
func (s *Store) Decision(ctx context.Context, p domain.Project, visitor string) (string, error) {
	var blocked []string
	switch p.Mode {
	case domain.ModeBlocking:
		blocked = []string{domain.Bot, domain.KnownBot}
	case domain.ModeBlockingBots:
		blocked = []string{domain.Bot}
	default:
		return "MONITOR", nil
	}
	var b bool
	e := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sessions WHERE project_id=$1 AND last_seen_at>now()-interval '30 days' AND current_classification=ANY($3) AND visitor_key=$2)`, p.ID, visitor, blocked).Scan(&b)
	if e != nil {
		return "MONITOR", e
	}
	if b {
		return "BLOCK", nil
	}
	return "ALLOW", nil
}
func deviceType(v string) string {
	switch strings.ToLower(v) {
	case "desktop", "mobile", "tablet":
		return strings.ToLower(v)
	default:
		return "unknown"
	}
}
func IPFromRequest(remoteAddr string) net.IP {
	host, _, e := net.SplitHostPort(remoteAddr)
	if e != nil {
		host = remoteAddr
	}
	return net.ParseIP(host)
}
