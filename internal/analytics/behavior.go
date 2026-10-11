package analytics

import (
	"context"
	"fmt"
)

type JourneyEdge struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Transitions int64  `json:"transitions"`
	Visits      int64  `json:"visits"`
}

func (s *Repository) Journey(ctx context.Context, f SessionFilter) ([]JourneyEdge, error) {
	where, args := sessionWhere(f)
	rows, e := s.Pool.Query(ctx, `WITH pages AS (
 SELECT p.session_id,split_part(p.page_url,'?',1) AS page,
 lag(split_part(p.page_url,'?',1)) OVER(PARTITION BY p.session_id ORDER BY p.page_index) AS previous,
 lead(split_part(p.page_url,'?',1)) OVER(PARTITION BY p.session_id ORDER BY p.page_index) AS next
 FROM analytics.session_pages p JOIN analytics.sessions s ON s.id=p.session_id WHERE `+where+`
 ), edges AS (SELECT session_id,COALESCE(previous,'[entry]') AS a,page AS b FROM pages
 UNION ALL SELECT session_id,page,'[exit]' FROM pages WHERE next IS NULL)
 SELECT a,b,count(*),count(DISTINCT session_id) FROM edges GROUP BY a,b ORDER BY count(*) DESC,a,b LIMIT 200`, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []JourneyEdge{}
	for rows.Next() {
		var r JourneyEdge
		if e = rows.Scan(&r.From, &r.To, &r.Transitions, &r.Visits); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type RetentionCell struct {
	Cohort   string `json:"cohort"`
	Day      int    `json:"day"`
	Visitors int64  `json:"visitors"`
	Returned int64  `json:"returned"`
	Observed bool   `json:"observed"`
}

func (s *Repository) Retention(ctx context.Context, f SessionFilter, zone string) ([]RetentionCell, error) {
	where, args := sessionWhere(f)
	args = append(args, zone)
	z := fmt.Sprintf("$%d", len(args))
	// First visit is taken across all retained sessions, before applying cohort filters.
	rows, e := s.Pool.Query(ctx, `WITH first_visits AS (
 SELECT DISTINCT ON(project_id,visitor_id) * FROM analytics.sessions ORDER BY project_id,visitor_id,started_at,id
 ), members AS (
 SELECT s.project_id,s.visitor_id,(s.started_at AT TIME ZONE `+z+`)::date AS cohort FROM first_visits s WHERE `+where+`
 ), sizes AS (SELECT cohort,count(*) AS visitors FROM members GROUP BY cohort), returns AS (
 SELECT m.cohort,(s.started_at AT TIME ZONE `+z+`)::date-m.cohort AS day,count(DISTINCT (m.project_id,m.visitor_id)) AS returned
 FROM members m JOIN analytics.sessions s ON s.project_id=m.project_id AND s.visitor_id=m.visitor_id
 WHERE (s.started_at AT TIME ZONE `+z+`)::date BETWEEN m.cohort AND m.cohort+30 GROUP BY m.cohort,day
 ) SELECT to_char(c.cohort,'YYYY-MM-DD'),d.day,c.visitors,COALESCE(r.returned,0),c.cohort+d.day<=(now() AT TIME ZONE `+z+`)::date
 FROM sizes c CROSS JOIN generate_series(0,30) d(day) LEFT JOIN returns r ON r.cohort=c.cohort AND r.day=d.day ORDER BY c.cohort,d.day`, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []RetentionCell{}
	for rows.Next() {
		var r RetentionCell
		if e = rows.Scan(&r.Cohort, &r.Day, &r.Visitors, &r.Returned, &r.Observed); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type HeatmapPage struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Visits int64  `json:"visits"`
}
type HeatmapCell struct {
	X     int   `json:"x"`
	Y     int   `json:"y"`
	Count int64 `json:"count"`
}
type ScrollBand struct {
	Depth  int   `json:"depth"`
	Visits int64 `json:"visits"`
}
type HeatmapReport struct {
	Pages   []HeatmapPage `json:"pages"`
	Clicks  []HeatmapCell `json:"clicks"`
	Scroll  []ScrollBand  `json:"scroll"`
	Visits  int64         `json:"visits"`
	PageURL string        `json:"page_url"`
	Width   int           `json:"width"`
}

func (s *Repository) Heatmap(ctx context.Context, f SessionFilter, page string, width int) (HeatmapReport, error) {
	out := HeatmapReport{Pages: []HeatmapPage{}, Clicks: []HeatmapCell{}, Scroll: []ScrollBand{}, PageURL: page, Width: width}
	where, args := sessionWhere(f)
	rows, e := s.Pool.Query(ctx, `SELECT h.page_url,h.viewport_width,count(DISTINCT h.session_id) FROM analytics.heatmap_events h JOIN analytics.sessions s ON s.id=h.session_id WHERE `+where+` GROUP BY h.page_url,h.viewport_width ORDER BY count(DISTINCT h.session_id) DESC,h.page_url,h.viewport_width`, args...)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var r HeatmapPage
		if e = rows.Scan(&r.URL, &r.Width, &r.Visits); e != nil {
			rows.Close()
			return out, e
		}
		out.Pages = append(out.Pages, r)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if page == "" && len(out.Pages) > 0 {
		page = out.Pages[0].URL
		out.PageURL = page
	}
	if width == 0 {
		for _, p := range out.Pages {
			if p.URL == page {
				width = p.Width
				out.Width = width
				break
			}
		}
	}
	args = append(args, page, width)
	selected := where + fmt.Sprintf(" AND h.page_url=$%d AND h.viewport_width=$%d", len(args)-1, len(args))
	if e = s.Pool.QueryRow(ctx, `SELECT count(DISTINCT h.session_id) FROM analytics.heatmap_events h JOIN analytics.sessions s ON s.id=h.session_id WHERE `+selected, args...).Scan(&out.Visits); e != nil {
		return out, e
	}
	rows, e = s.Pool.Query(ctx, `SELECT LEAST(99,GREATEST(0,floor(100*h.x/h.viewport_width)))::int,LEAST(99,GREATEST(0,floor(100*h.y/GREATEST(h.document_height,h.viewport_height))))::int,count(*) FROM analytics.heatmap_events h JOIN analytics.sessions s ON s.id=h.session_id WHERE `+selected+` AND h.event_type='click' GROUP BY 1,2 ORDER BY 2,1`, args...)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var r HeatmapCell
		if e = rows.Scan(&r.X, &r.Y, &r.Count); e != nil {
			rows.Close()
			return out, e
		}
		out.Clicks = append(out.Clicks, r)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	rows, e = s.Pool.Query(ctx, `WITH depths AS (SELECT h.session_id,max(h.depth) AS depth FROM analytics.heatmap_events h JOIN analytics.sessions s ON s.id=h.session_id WHERE `+selected+` AND h.event_type='scroll' GROUP BY h.session_id) SELECT d.depth,count(p.session_id) FROM generate_series(0,100,10) d(depth) LEFT JOIN depths p ON p.depth>=d.depth/100.0 GROUP BY d.depth ORDER BY d.depth`, args...)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var r ScrollBand
		if e = rows.Scan(&r.Depth, &r.Visits); e != nil {
			return out, e
		}
		out.Scroll = append(out.Scroll, r)
	}
	return out, rows.Err()
}
