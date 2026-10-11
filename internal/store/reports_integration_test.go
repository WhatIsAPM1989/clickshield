package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/lamooof/clickshield/internal/domain"
	"github.com/lamooof/clickshield/internal/scoring"
)

func TestAggregateReports(t *testing.T) {
	dsn := os.Getenv("CLICKSHIELD_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("dedicated database required")
	}
	ctx := context.Background()
	s, e := New(ctx, dsn, []byte("0123456789abcdef0123456789abcdef"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	cfg, _ := json.Marshal(scoring.DefaultConfig())
	p, e := s.CreateProject(ctx, "Report aggregation test", []string{"https://reports.test"}, "MONITORING", cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Pool.Exec(ctx, `DELETE FROM projects WHERE id=$1`, p.ID)
	visitor, _ := UUID()
	var ids []string
	for i := 0; i < 3; i++ {
		sid, _ := UUID()
		bid, _ := UUID()
		ids = append(ids, sid)
		device := "desktop"
		campaign := "first"
		medium := "cpc"
		url := "https://reports.test/start?yclid=a"
		if i == 1 {
			device = "mobile"
			campaign = "second"
			url = "https://reports.test/start?yclid=b"
		}
		if i == 2 {
			medium = "organic"
			campaign = ""
			url = "https://reports.test/other"
		}
		b := domain.EventBatch{SessionID: sid, VisitorID: visitor, BatchID: bid, PageIndex: 1, PageURL: url, LandingURL: url, Device: domain.Device{Type: device}, UTM: domain.UTM{Source: "yandex", Medium: medium, Campaign: campaign}, Events: []domain.Event{{Type: "pageview", TS: time.Now().UnixMilli(), PageIndex: 1}}}
		if _, e = s.Ingest(ctx, p, b, domain.RemoteMeta{}); e != nil {
			t.Fatal(e)
		}
		if _, e = s.Pool.Exec(ctx, `UPDATE sessions SET started_at='2026-10-10 21:30:00+00',last_seen_at='2026-10-10 21:31:00+00' WHERE id=$1`, sid); e != nil {
			t.Fatal(e)
		}
		// A tab left open for hours must not count as time on site; the third visit is not scored yet.
		if i < 2 {
			if _, e = s.Pool.Exec(ctx, `INSERT INTO session_features(session_id,duration_ms,active_time_ms) VALUES($1,10800000,$2)`, sid, 20000-10000*i); e != nil {
				t.Fatal(e)
			}
		}
	}
	// Two leads in one visit must produce one converted visit, not a 200% conversion.
	if _, e = s.Pool.Exec(ctx, `INSERT INTO lead_outcomes(project_id,session_id,external_id,status,source) VALUES($1,$2,'first','QUALIFIED','SERVER'),($1,$2,'second','SPAM','SERVER')`, p.ID, ids[0]); e != nil {
		t.Fatal(e)
	}
	f := SessionFilter{ProjectID: p.ID}
	for _, kind := range []string{"sources", "campaigns", "landing", "devices", "daily"} {
		r, e := projectedReports(t, s).AggregateReport(ctx, kind, "Europe/Moscow", f)
		if e != nil {
			t.Fatalf("%s: %v", kind, e)
		}
		if r.Totals.Visits != 3 || r.Totals.Visitors != 1 || r.Totals.Leads != 2 || r.Totals.ConvertedVisits != 1 || r.Totals.QualifiedVisits != 1 || r.Totals.Pageviews != 3 || r.Totals.AverageDurationMS != 15000 || r.Totals.Bounces != 1 {
			t.Fatalf("%s totals: %+v", kind, r.Totals)
		}
		want := 2
		if kind == "campaigns" {
			want = 3
		}
		if kind == "daily" {
			want = 1
		}
		leaves := []ReportRow{}
		for _, row := range r.Rows {
			if row.Level == len(r.Dimensions) {
				leaves = append(leaves, row)
			}
		}
		if len(leaves) != want {
			t.Fatalf("%s: wanted %d rows, got %+v", kind, want, r.Rows)
		}
		var visits int64
		for _, row := range leaves {
			visits += row.Visits
			drill := f
			for i, k := range r.Dimensions {
				v := row.Dimensions[i]
				switch k {
				case "source":
					drill.Source = v
				case "channel":
					drill.Channel = v
				case "campaign":
					drill.Campaign = v
					drill.CampaignSet = true
				case "landing_url":
					drill.LandingURL = v
				case "device":
					drill.Device = v
					drill.DeviceSet = true
				case "day":
					if v != "2026-10-11" {
						t.Fatalf("timezone boundary: %s", v)
					}
				}
			}
			list, e := s.ListSessions(ctx, drill, 100, 0)
			if e != nil || int64(len(list)) != row.Visits {
				t.Fatalf("%s drill mismatch: %d vs %d (%v)", kind, len(list), row.Visits, e)
			}
		}
		if visits != 3 {
			t.Fatalf("row visits: %d", visits)
		}
	}
	// Parent rows of a hierarchy count their children exactly, including unique visitors.
	camp, e := projectedReports(t, s).AggregateReport(ctx, "campaigns", "UTC", f)
	if e != nil || camp.Rows[0].Level != 1 || camp.Rows[0].Visitors != 1 {
		t.Fatalf("campaign parents: %+v %v", camp.Rows, e)
	}
	from, to := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	tf := f
	tf.From, tf.To = &from, &to
	tl, e := projectedReports(t, s).ReportTimeline(ctx, "devices", "day", "Europe/Moscow", nil, false, tf)
	if e != nil || len(tl.Buckets) != 3 || tl.Total[1].Visits != 3 || len(tl.Series) != 2 || tl.Series[0].Key != "desktop" || tl.Series[0].Points[1].Visits != 2 {
		t.Fatalf("timeline: %+v %v", tl, e)
	}
	tf.Dims = map[string]string{"utm_medium": "organic"}
	organic, e := projectedReports(t, s).AggregateReport(ctx, "utm", "UTC", tf)
	if e != nil || organic.Totals.Visits != 1 {
		t.Fatalf("dimension filter: %+v %v", organic.Totals, e)
	}
	u, e := projectedReports(t, s).AggregateReport(ctx, "daily", "UTC", f)
	if e != nil || u.Rows[0].Dimensions[0] != "2026-10-10" {
		t.Fatalf("UTC date: %+v %v", u, e)
	}
	f.Source = "nonexistent"
	empty, e := projectedReports(t, s).AggregateReport(ctx, "sources", "UTC", f)
	if e != nil || len(empty.Rows) != 0 || empty.Totals.Visits != 0 {
		t.Fatalf("empty: %+v %v", empty, e)
	}
}
