package store

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/lamooof/clickshield/internal/analytics"
	"github.com/lamooof/clickshield/internal/antifraud"
	"github.com/lamooof/clickshield/internal/domain"
	"github.com/lamooof/clickshield/internal/feature"
	"github.com/lamooof/clickshield/internal/scoring"
)

func architectureFixture(t *testing.T) (*Store, domain.Project, domain.EventBatch) {
	t.Helper()
	dsn := os.Getenv("CLICKSHIELD_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("dedicated database required")
	}
	ctx := context.Background()
	st, err := New(ctx, dsn, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err = st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	cfg, _ := json.Marshal(scoring.DefaultConfig())
	p, err := st.CreateProject(ctx, "Architecture regression", []string{"https://example.test"}, domain.ModeNotify, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Pool.Exec(ctx, `DELETE FROM projects WHERE id=$1`, p.ID); projectedReports(t, st) })
	sid, _ := UUID()
	vid, _ := UUID()
	bid, _ := UUID()
	now := time.Now().UnixMilli()
	b := domain.EventBatch{SessionID: sid, VisitorID: vid, BatchID: bid, PageURL: "https://example.test/", PageIndex: 1, Events: []domain.Event{{Type: "pageview", TS: now, PageIndex: 1}, {Type: "click", TS: now + 50, X: 10, Y: 20}}}
	if _, err = st.Ingest(ctx, p, b, domain.RemoteMeta{}); err != nil {
		t.Fatal(err)
	}
	return st, p, b
}

func TestScoreAfterRawExpiryAndConcurrentChanges(t *testing.T) {
	st, p, b := architectureFixture(t)
	ctx := context.Background()
	scorer := antifraud.NewScorer(st)
	if _, err := st.Pool.Exec(ctx, `UPDATE sessions SET feature_state=NULL WHERE id=$1`, b.SessionID); err != nil {
		t.Fatal(err)
	}
	if err := scorer.Finalize(ctx, b.SessionID); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := st.Pool.QueryRow(ctx, `SELECT max(id) FROM classifications WHERE session_id=$1`, b.SessionID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	history, err := st.ScoreHistory(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	reproduced := scoring.Score(history.Input.Context, history.Features, scoring.Parse(history.Config))
	var original string
	var risk int
	st.Pool.QueryRow(ctx, `SELECT classification,risk_score FROM classifications WHERE id=$1`, id).Scan(&original, &risk)
	if reproduced.Classification != original || reproduced.RiskScore != risk {
		t.Fatal("historical reproduction changed")
	}
	if _, err = st.Pool.Exec(ctx, `DELETE FROM raw_events WHERE session_id=$1`, b.SessionID); err != nil {
		t.Fatal(err)
	}
	if err = scorer.Finalize(ctx, b.SessionID); err != nil {
		t.Fatal(err)
	}
	var newID int64
	st.Pool.QueryRow(ctx, `SELECT max(id) FROM classifications WHERE session_id=$1`, b.SessionID).Scan(&newID)
	after, err := st.ScoreHistory(ctx, newID)
	if err != nil {
		t.Fatal(err)
	}
	beforeFeatures, afterFeatures := history.Features, after.Features
	beforeFeatures.ComputedAt = time.Time{}
	afterFeatures.ComputedAt = time.Time{}
	if !reflect.DeepEqual(beforeFeatures, afterFeatures) {
		t.Fatal("raw expiry changed features")
	}
	// Existing installations may already have lost raw events before upgrade.
	if _, err = st.Pool.Exec(ctx, `UPDATE sessions SET feature_state=NULL WHERE id=$1`, b.SessionID); err != nil {
		t.Fatal(err)
	}
	if err = scorer.Finalize(ctx, b.SessionID); err == nil {
		t.Fatal("expired legacy history silently recomputed from empty events")
	}
	var state feature.State
	state.Add(b.Events)
	stateJSON, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.Pool.Exec(ctx, `UPDATE sessions SET feature_state=$2 WHERE id=$1`, b.SessionID, stateJSON); err != nil {
		t.Fatal(err)
	}

	lease, err := st.ClaimScore(ctx, b.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	input, err := st.LoadScore(ctx, b.SessionID, lease)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = antifraud.Label(ctx, st, b.SessionID, domain.Bot, "reviewed", ""); err != nil {
		t.Fatal(err)
	}
	write := antifraud.ScoreWrite{Input: input, Features: after.Features, Score: reproduced, Config: history.Config, Version: history.Version}
	if _, err = st.SaveScore(ctx, write); !errors.Is(err, antifraud.ErrChanged) {
		t.Fatalf("manual label overwritten: %v", err)
	}
	st.ReleaseScore(ctx, b.SessionID, lease)
	lease, err = st.ClaimScore(ctx, b.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	input, err = st.LoadScore(ctx, b.SessionID, lease)
	if err != nil {
		t.Fatal(err)
	}
	write.Input = input
	if _, err = st.Pool.Exec(ctx, `UPDATE projects SET scoring_config=scoring_config||'{"human_threshold":0}' WHERE id=$1`, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = st.SaveScore(ctx, write); !errors.Is(err, antifraud.ErrChanged) {
		t.Fatalf("config change ignored: %v", err)
	}
	st.ReleaseScore(ctx, b.SessionID, lease)
	// Both contenders start together; exactly one owns the score lease.
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; _, err := st.ClaimScore(ctx, b.SessionID); results <- err }()
	}
	close(start)
	wg.Wait()
	close(results)
	won := 0
	busy := 0
	for err := range results {
		if err == nil {
			won++
		} else if errors.Is(err, antifraud.ErrBusy) {
			busy++
		} else {
			t.Fatal(err)
		}
	}
	if won != 1 || busy != 1 {
		t.Fatalf("claims won=%d busy=%d", won, busy)
	}
}

func TestAnalyticsProjectionRetryDeletionAndIsolation(t *testing.T) {
	st, p, b := architectureFixture(t)
	ctx := context.Background()
	reader := analytics.NewRepository(st.Pool)
	f := analytics.SessionFilter{ProjectID: p.ID}
	stats, err := reader.FilteredStats(ctx, f)
	if err != nil || stats.Total != 0 {
		t.Fatalf("unexpected synchronous projection: %+v %v", stats, err)
	}
	projectedReports(t, st)
	stats, err = reader.FilteredStats(ctx, f)
	if err != nil || stats.Total != 1 {
		t.Fatalf("missing projection: %+v %v", stats, err)
	}
	if _, err = st.Pool.Exec(ctx, `UPDATE sessions SET current_classification='BOT' WHERE id=$1`, b.SessionID); err != nil {
		t.Fatal(err)
	}
	projectedReports(t, st)
	stats, err = reader.FilteredStats(ctx, f)
	if err != nil || stats.Bot != 1 {
		t.Fatal("classification did not project", err)
	}
	// A failing projection rolls back and remains retryable.
	if _, err = st.Pool.Exec(ctx, `ALTER TABLE analytics.sessions ADD CONSTRAINT projection_test CHECK(current_classification<>'BROKEN')`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Pool.Exec(ctx, `ALTER TABLE analytics.sessions DROP CONSTRAINT IF EXISTS projection_test`) })
	st.Pool.Exec(ctx, `UPDATE sessions SET current_classification='BROKEN' WHERE id=$1`, b.SessionID)
	if _, err = analytics.NewProjector(st.Pool).ProjectOne(ctx); err == nil {
		t.Fatal("expected projection failure")
	}
	var attempts int
	st.Pool.QueryRow(ctx, `SELECT max(attempts) FROM analytics_projection_outbox WHERE session_id=$1`, b.SessionID).Scan(&attempts)
	if attempts == 0 {
		t.Fatal("failed projection disappeared")
	}
	stats, err = reader.FilteredStats(ctx, f)
	if err != nil || stats.Bot != 1 {
		t.Fatal("failed projection partially committed", err)
	}
	st.Pool.Exec(ctx, `ALTER TABLE analytics.sessions DROP CONSTRAINT projection_test`)
	st.Pool.Exec(ctx, `UPDATE analytics_projection_outbox SET next_attempt_at=now() WHERE session_id=$1`, b.SessionID)
	projectedReports(t, st)
	// Reports work with a role whose search_path contains only the reporting schema.
	u, err := url.Parse(os.Getenv("CLICKSHIELD_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", "analytics")
	u.RawQuery = q.Encode()
	isolated, err := analytics.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer isolated.Close()
	stats, err = isolated.FilteredStats(ctx, f)
	if err != nil || stats.Total != 1 {
		t.Fatal("report depends on source tables", err)
	}

	_, err = reader.SaveDefinition(ctx, analytics.AnalyticsDefinition{ProjectID: p.ID, Kind: "goal", Name: "Visit", WindowMinutes: 1, Steps: []analytics.AnalyticsStep{{Type: "page", Value: "/"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.Pool.Exec(ctx, `DELETE FROM projects WHERE id=$1`, p.ID); err != nil {
		t.Fatal(err)
	}
	projectedReports(t, st)
	stats, err = reader.FilteredStats(ctx, f)
	if err != nil || stats.Total != 0 {
		t.Fatal("deleted project still reported", err)
	}
	defs, err := reader.Definitions(ctx, p.ID)
	if err != nil || len(defs) != 0 {
		t.Fatal("orphaned definitions", err)
	}
}

func TestOutboxFencingAndBootstrapPreservesSettings(t *testing.T) {
	st, p, b := architectureFixture(t)
	ctx := context.Background()
	if err := st.EnsureIntegration(ctx, p.ID, "GOOGLE_ANALYTICS", map[string]any{"measurement_id": "initial"}, []byte("initial")); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertIntegration(ctx, p.ID, "GOOGLE_ANALYTICS", false, map[string]any{"measurement_id": "edited"}, []byte("edited")); err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureIntegration(ctx, p.ID, "GOOGLE_ANALYTICS", map[string]any{"measurement_id": "replacement"}, []byte("replacement")); err != nil {
		t.Fatal(err)
	}
	items, err := st.Integrations(ctx, p.ID)
	if err != nil || len(items) != 1 || items[0].Enabled || string(items[0].SecretEncrypted) != "edited" {
		t.Fatalf("bootstrap overwrote settings: %+v %v", items, err)
	}
	st.UpsertIntegration(ctx, p.ID, "GOOGLE_ANALYTICS", true, map[string]any{"measurement_id": "edited"}, nil)
	if err = antifraud.NewScorer(st).Finalize(ctx, b.SessionID); err != nil {
		t.Fatal(err)
	}
	// Explicit manual classification creates a deterministic outbound notification.
	if _, err = antifraud.Label(ctx, st, b.SessionID, domain.Bot, "", ""); err != nil {
		t.Fatal(err)
	}
	old, ok, err := st.ClaimOutbox(ctx)
	if err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	st.Pool.Exec(ctx, `UPDATE integration_outbox SET locked_at=now()-interval '6 minutes' WHERE id=$1`, old.ID)
	if err = st.ResetStuckOutbox(ctx); err != nil {
		t.Fatal(err)
	}
	newer, ok, err := st.ClaimOutbox(ctx)
	if err != nil || !ok || newer.ID != old.ID {
		t.Fatalf("reclaim: %+v %v", newer, err)
	}
	if err = st.FinishDelivery(ctx, old, "SENT", "", 0); !errors.Is(err, antifraud.ErrChanged) {
		t.Fatalf("old worker acknowledged: %v", err)
	}
	if err = st.FinishDelivery(ctx, newer, "SENT", "", 0); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentBatchRetryAndProjectionWorkers(t *testing.T) {
	st, p, b := architectureFixture(t)
	ctx := context.Background()
	b.BatchID, _ = UUID()
	b.Events = nil
	for i := 0; i < 40; i++ {
		b.Events = append(b.Events, domain.Event{Type: "business", TS: time.Now().UnixMilli(), Name: "concurrent", PageIndex: 1})
	}
	start := make(chan struct{})
	results := make(chan IngestResult, 12)
	errs := make(chan error, 12)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r, err := st.Ingest(ctx, p, b, domain.RemoteMeta{})
			results <- r
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	accepted := 0
	for r := range results {
		accepted += r.InsertedEvents
	}
	if accepted != 40 {
		t.Fatalf("concurrent retry inserted %d events", accepted)
	}
	projectedReports(t, st)
	for i := 0; i < 8; i++ {
		if _, err := st.Pool.Exec(ctx, `INSERT INTO analytics_projection_outbox(session_id,project_id) VALUES($1,$2)`, b.SessionID, p.ID); err != nil {
			t.Fatal(err)
		}
	}
	failures := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := analytics.NewProjector(st.Pool).ProjectOne(ctx); failures <- err }()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	reader := projectedReports(t, st)
	events, err := reader.BusinessReport(ctx, analytics.SessionFilter{ProjectID: p.ID})
	if err != nil || len(events) != 1 || events[0].Events != 40 {
		t.Fatalf("projection duplicated events: %+v %v", events, err)
	}
	if _, err = st.Pool.Exec(ctx, `DELETE FROM sessions WHERE id=$1`, b.SessionID); err != nil {
		t.Fatal(err)
	}
	projectedReports(t, st)
	stats, err := reader.FilteredStats(ctx, analytics.SessionFilter{ProjectID: p.ID})
	if err != nil || stats.Total != 0 {
		t.Fatal("session deletion not projected", err)
	}
}
