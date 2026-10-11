package analytics

import (
	"context"
	"fmt"
	"time"
)

var timelineUnits = map[string]string{"hour": "hour", "day": "day", "week": "week", "month": "month"}

const maxTimelinePoints = 1500

type TimelineSeries struct {
	Key    string          `json:"key"`
	Points []ReportMetrics `json:"points"`
}
type Timeline struct {
	Interval string           `json:"interval"`
	Buckets  []string         `json:"buckets"`
	Total    []ReportMetrics  `json:"total"`
	Series   []TimelineSeries `json:"series"`
}

func ValidInterval(v string) bool { _, ok := timelineUnits[v]; return ok }

// TimelineBuckets lists every bucket start between from and to in the zone, formatted like SQL.
func TimelineBuckets(interval string, from, to time.Time, loc *time.Location) ([]string, error) {
	start := from.In(loc)
	switch interval {
	case "hour":
		start = time.Date(start.Year(), start.Month(), start.Day(), start.Hour(), 0, 0, 0, loc)
	case "day":
		start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)
	case "week":
		start = time.Date(start.Year(), start.Month(), start.Day()-(int(start.Weekday())+6)%7, 0, 0, 0, 0, loc)
	case "month":
		start = time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, loc)
	}
	out := []string{}
	seen := map[string]bool{}
	for t := start; t.Before(to); {
		label := t.In(loc).Format("2006-01-02 15:00")
		if !seen[label] {
			seen[label] = true
			out = append(out, label)
		}
		if len(out) > maxTimelinePoints {
			return nil, fmt.Errorf("too many points")
		}
		switch interval {
		case "hour":
			t = t.Add(time.Hour)
		case "day":
			t = t.AddDate(0, 0, 1)
		case "week":
			t = t.AddDate(0, 0, 7)
		default:
			t = t.AddDate(0, 1, 0)
		}
	}
	return out, nil
}

// ReportTimeline splits the report's first dimension into series over time. Without
// explicit series the five largest values are used.
func (s *Repository) ReportTimeline(ctx context.Context, kind, interval, timezone string, series []string, totalOnly bool, filter SessionFilter) (Timeline, error) {
	out := Timeline{Interval: interval, Total: []ReportMetrics{}, Series: []TimelineSeries{}}
	if !ValidReport(kind) || !ValidInterval(interval) || filter.From == nil || filter.To == nil {
		return out, fmt.Errorf("invalid timeline")
	}
	loc, e := time.LoadLocation(timezone)
	if e != nil {
		return out, e
	}
	if out.Buckets, e = TimelineBuckets(interval, *filter.From, *filter.To, loc); e != nil {
		return out, e
	}
	index := map[string]int{}
	for i, b := range out.Buckets {
		index[b] = i
	}
	out.Total = make([]ReportMetrics, len(out.Buckets))
	where, args := sessionWhere(filter)
	expressions := dimensionExpressions()
	key := expressions[reportDimensions[kind][0]]
	switch reportDimensions[kind][0] {
	case "day", "hour", "weekday":
		totalOnly = true
		key = "s.traffic_channel" // grouped but filtered out: only totals are returned
	}
	if !totalOnly && len(series) == 0 {
		rows, e := s.Pool.Query(ctx, `SELECT `+key+` FROM analytics.sessions s WHERE `+where+` GROUP BY 1 ORDER BY count(*) DESC,1 LIMIT 5`, args...)
		if e != nil {
			return out, e
		}
		for rows.Next() {
			var v string
			if e = rows.Scan(&v); e != nil {
				rows.Close()
				return out, e
			}
			series = append(series, v)
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return out, e
		}
	}
	if totalOnly {
		series = nil
	}
	args = append(args, timezone)
	bucket := fmt.Sprintf("to_char(date_trunc('%s',s.started_at AT TIME ZONE $%d),'YYYY-MM-DD HH24:00')", timelineUnits[interval], len(args))
	seriesIndex := map[string]int{}
	for _, v := range series {
		if _, ok := seriesIndex[v]; ok {
			continue
		}
		seriesIndex[v] = len(out.Series)
		out.Series = append(out.Series, TimelineSeries{Key: v, Points: make([]ReportMetrics, len(out.Buckets))})
	}
	args = append(args, series)
	query := `SELECT ` + bucket + `,` + key + `,GROUPING(` + key + `),` + reportMetricsSQL + reportFromSQL + where + ` GROUP BY GROUPING SETS((` + bucket + `,` + key + `),(` + bucket + `)) HAVING GROUPING(` + key + `)=1 OR ` + key + fmt.Sprintf(`=ANY($%d::text[])`, len(args))
	rows, e := s.Pool.Query(ctx, query, args...)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var b string
		var k *string
		var grouping int32
		var m ReportMetrics
		if e = rows.Scan(append([]any{&b, &k, &grouping}, m.scanTargets()...)...); e != nil {
			return out, e
		}
		i, ok := index[b]
		if !ok {
			continue
		}
		if grouping == 1 {
			out.Total[i] = m
		} else if k != nil {
			if j, ok := seriesIndex[*k]; ok {
				out.Series[j].Points[i] = m
			}
		}
	}
	return out, rows.Err()
}
