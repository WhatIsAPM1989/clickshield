package analytics

import (
	"context"
	"fmt"
	"math/bits"
	"sort"
	"strings"
)

// Presets use fixed SQL expressions; request values are always bound parameters.
var reportDimensions = map[string][]string{
	"sources":   {"channel", "source"},
	"referrers": {"referrer"},
	"campaigns": {"campaign", "source", "channel"},
	"utm":       {"utm_source", "utm_medium", "utm_campaign", "utm_content"},
	"terms":     {"utm_term"},
	"landing":   {"landing_url"},
	"exit":      {"exit_page"},
	"devices":   {"device", "os"},
	"browsers":  {"browser"},
	"geo":       {"country", "region", "city"},
	"languages": {"language"},
	"networks":  {"network"},
	"quality":   {"classification"},
	"bots":      {"bot_category", "bot"},
	"daily":     {"day"},
	"hours":     {"hour"},
	"activity":  {"weekday", "hour"},
}

// Browser and OS names match ParseUserAgent, so older sessions group like new ones.
const browserExpr = `CASE WHEN s.user_agent LIKE '%Edg/%' OR s.user_agent LIKE '%EdgA/%' OR s.user_agent LIKE '%EdgiOS/%' THEN 'Edge' WHEN s.user_agent LIKE '%OPR/%' THEN 'Opera' WHEN s.user_agent LIKE '%YaBrowser/%' THEN 'Yandex' WHEN s.user_agent LIKE '%SamsungBrowser/%' THEN 'Samsung Internet' WHEN s.user_agent LIKE '%Firefox/%' OR s.user_agent LIKE '%FxiOS/%' THEN 'Firefox' WHEN s.user_agent LIKE '%CriOS/%' OR s.user_agent LIKE '%Chrome/%' THEN 'Chrome' WHEN s.user_agent LIKE '%Version/%' THEN 'Safari' ELSE 'unknown' END`
const osExpr = `CASE WHEN s.user_agent LIKE '%Android%' THEN 'Android' WHEN s.user_agent LIKE '%iPhone%' THEN 'iOS' WHEN s.user_agent LIKE '%iPad%' THEN 'iPadOS' WHEN s.user_agent LIKE '%Windows NT%' THEN 'Windows' WHEN s.user_agent LIKE '%CrOS%' THEN 'ChromeOS' WHEN s.user_agent LIKE '%Macintosh%' THEN 'macOS' WHEN s.user_agent LIKE '%Linux%' THEN 'Linux' ELSE 'unknown' END`

// filterExpressions are dimensions that can also narrow sessions and reports by exact value.
var filterExpressions = map[string]string{
	"referrer":     "s.referrer_host",
	"utm_source":   "s.utm_source",
	"utm_medium":   "s.utm_medium",
	"utm_content":  "s.utm_content",
	"utm_term":     "s.utm_term",
	"exit_page":    "split_part(s.last_page_url,'?',1)",
	"os":           osExpr,
	"browser":      browserExpr,
	"country":      "s.country",
	"region":       "s.region",
	"city":         "s.city",
	"language":     "COALESCE(s.device->>'language','')",
	"network":      "COALESCE(NULLIF(s.as_org,''),s.asn)",
	"utm_campaign": "s.utm_campaign",
}

func dimensionExpressions() map[string]string {
	m := map[string]string{"channel": "s.traffic_channel", "source": "s.traffic_source", "campaign": "s.utm_campaign", "landing_url": "split_part(s.landing_url,'?',1)", "device": "COALESCE(NULLIF(s.device_type,''),'unknown')", "classification": "COALESCE(NULLIF(s.current_classification,''),'UNCLASSIFIED')", "bot": "COALESCE(s.bot_name,'')", "bot_category": "CASE WHEN COALESCE(s.bot_name,'')<>'' THEN s.bot_category ELSE '' END"}
	for k, v := range filterExpressions {
		m[k] = v
	}
	return m
}

func ValidReport(kind string) bool { _, ok := reportDimensions[kind]; return ok }

// FilterableDimension reports whether a query parameter narrows sessions by a report dimension.
func FilterableDimension(key string) bool { _, ok := filterExpressions[key]; return ok }

func dimensionWhere(dims map[string]string, args []any) ([]string, []any) {
	keys := make([]string, 0, len(dims))
	for k := range dims {
		if FilterableDimension(k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	where := []string{}
	for _, k := range keys {
		args = append(args, dims[k])
		where = append(where, fmt.Sprintf("%s=$%d", filterExpressions[k], len(args)))
	}
	return where, args
}

type ReportMetrics struct {
	Visits            int64   `json:"visits"`
	Visitors          int64   `json:"visitors"`
	NewVisitors       int64   `json:"new_visitors"`
	Pageviews         int64   `json:"pageviews"`
	Bounces           int64   `json:"bounces"`
	Humans            int64   `json:"humans"`
	Suspicious        int64   `json:"suspicious"`
	Bots              int64   `json:"bots"`
	KnownBots         int64   `json:"known_bots"`
	Leads             int64   `json:"leads"`
	Qualified         int64   `json:"qualified"`
	ConvertedVisits   int64   `json:"converted_visits"`
	QualifiedVisits   int64   `json:"qualified_visits"`
	Replays           int64   `json:"replays"`
	AverageDurationMS float64 `json:"average_duration_ms"`
}

func (m *ReportMetrics) scanTargets() []any {
	return []any{&m.Visits, &m.Visitors, &m.NewVisitors, &m.Pageviews, &m.Bounces, &m.Humans, &m.Suspicious, &m.Bots, &m.KnownBots, &m.Leads, &m.Qualified, &m.ConvertedVisits, &m.QualifiedVisits, &m.Replays, &m.AverageDurationMS}
}

// Time on site is active time: the tab is visible and the visitor interacted within the last
// five seconds. The first-to-last span would count tabs left open in the background for hours.
// Visits not yet scored have no features and are left out of the average.
// A visit is a bounce, as in Yandex Metrica, when it has one page and under 15 seconds of active time.
const reportMetricsSQL = `count(*) AS visits,count(DISTINCT s.visitor_id),
	count(DISTINCT s.visitor_id) FILTER(WHERE NOT EXISTS(SELECT 1 FROM analytics.sessions p WHERE p.project_id=s.project_id AND p.visitor_id=s.visitor_id AND p.started_at<s.started_at)),
	COALESCE(sum(GREATEST(COALESCE(f.page_count,0),pv.pages,s.page_count,1)),0)::bigint,
	count(*) FILTER(WHERE GREATEST(COALESCE(f.page_count,0),pv.pages,s.page_count,1)<=1 AND f.active_time_ms<15000),
	count(*) FILTER(WHERE s.current_classification='HUMAN'),
	count(*) FILTER(WHERE s.current_classification='SUSPICIOUS'),
	count(*) FILTER(WHERE s.current_classification='BOT'),
	count(*) FILTER(WHERE s.current_classification='KNOWN_BOT'),
	COALESCE(sum(o.leads),0)::bigint,COALESCE(sum(o.qualified),0)::bigint,
	count(*) FILTER(WHERE o.leads>0),count(*) FILTER(WHERE o.qualified>0),
	count(*) FILTER(WHERE EXISTS(SELECT 1 FROM analytics.replay_recordings rr WHERE rr.session_id=s.id AND rr.expires_at>now())),
	COALESCE(avg(f.active_time_ms),0)::double precision`

const reportFromSQL = ` FROM analytics.sessions s LEFT JOIN analytics.session_features f ON f.session_id=s.id
	LEFT JOIN LATERAL(SELECT count(*) AS pages FROM analytics.session_pages WHERE session_id=s.id) pv ON true
	LEFT JOIN LATERAL(SELECT count(*) AS leads,count(*) FILTER(WHERE status='QUALIFIED') AS qualified FROM analytics.lead_outcomes WHERE session_id=s.id) o ON true
	WHERE `

type ReportRow struct {
	Dimensions []string `json:"dimensions"`
	// Level is the number of leading dimensions the row groups by; parents precede deeper rows.
	Level int `json:"level"`
	ReportMetrics
}
type AggregateReport struct {
	Kind       string        `json:"kind"`
	Dimensions []string      `json:"dimensions"`
	Rows       []ReportRow   `json:"rows"`
	Totals     ReportMetrics `json:"totals"`
	Timezone   string        `json:"timezone"`
}

func timeExpressions(m map[string]string, args []any, timezone string) []any {
	args = append(args, timezone)
	local := fmt.Sprintf("(s.started_at AT TIME ZONE $%d)", len(args))
	m["day"] = "to_char(" + local + ",'YYYY-MM-DD')"
	m["hour"] = "to_char(" + local + ",'HH24')"
	m["weekday"] = "EXTRACT(ISODOW FROM " + local + ")::int::text"
	return args
}

func (s *Repository) AggregateReport(ctx context.Context, kind, timezone string, filter SessionFilter) (AggregateReport, error) {
	out := AggregateReport{Kind: kind, Dimensions: reportDimensions[kind], Rows: []ReportRow{}, Timezone: timezone}
	if !ValidReport(kind) {
		return out, fmt.Errorf("unknown report")
	}
	where, args := sessionWhere(filter)
	expressions := dimensionExpressions()
	for _, k := range out.Dimensions {
		if k == "day" || k == "hour" || k == "weekday" {
			args = timeExpressions(expressions, args, timezone)
			break
		}
	}
	dims := make([]string, len(out.Dimensions))
	for i, k := range out.Dimensions {
		dims[i] = expressions[k]
	}
	group := strings.Join(dims, ",")
	query := `SELECT ` + group + `,GROUPING(` + group + `),` + reportMetricsSQL + reportFromSQL + where + ` GROUP BY ROLLUP(` + group + `) ORDER BY GROUPING(` + group + `) DESC,visits DESC`
	rows, e := s.Pool.Query(ctx, query, args...)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		r := ReportRow{Dimensions: make([]string, len(dims))}
		// Rolled-up dimensions are NULL, while real empty values remain valid.
		values := make([]*string, len(dims))
		dest := make([]any, 0, len(dims)+16)
		for i := range values {
			dest = append(dest, &values[i])
		}
		var grouping int32
		dest = append(dest, &grouping)
		dest = append(dest, r.ReportMetrics.scanTargets()...)
		if e = rows.Scan(dest...); e != nil {
			return out, e
		}
		r.Level = len(dims) - bits.OnesCount32(uint32(grouping))
		if r.Level == 0 {
			out.Totals = r.ReportMetrics
			continue
		}
		r.Dimensions = r.Dimensions[:r.Level]
		for i := 0; i < r.Level; i++ {
			if values[i] != nil {
				r.Dimensions[i] = *values[i]
			}
		}
		out.Rows = append(out.Rows, r)
	}
	return out, rows.Err()
}
