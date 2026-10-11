package scoring

import (
	"github.com/lamooof/clickshield/internal/domain"
	"testing"
)

func TestWeakSignalCannotMakeBot(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Weights["FAST_FIRST_CLICK"] = 100
	r := Score(domain.SessionContext{}, domain.Features{TimeToFirstClickMS: 1}, cfg)
	if r.Classification == domain.Bot {
		t.Fatalf("single weak signal classified BOT")
	}
}
func TestCombinedSignalsMakeBot(t *testing.T) {
	c := domain.SessionContext{DeviceType: "desktop", Automation: map[string]bool{"webdriver": true}, SessionsPerFingerprint: 20, VisitorsPerFingerprint: 20}
	f := domain.Features{TimeToFirstClickMS: 1000}
	r := Score(c, f, DefaultConfig())
	if r.Classification != domain.Bot {
		t.Fatalf("got %s score=%d", r.Classification, r.RiskScore)
	}
}
func TestKnownBot(t *testing.T) {
	r := Score(domain.SessionContext{KnownBot: true, UserAgent: "Googlebot"}, domain.Features{}, DefaultConfig())
	if r.Classification != domain.KnownBot {
		t.Fatal(r.Classification)
	}
}

func TestRapidUnengagedPageBurst(t *testing.T) {
	c := domain.SessionContext{SessionsPerFingerprint: 10, VisitorsPerFingerprint: 1, NearbyVisitorSessions: 10, NearbyVisitorPages: 10}
	f := domain.Features{DurationMS: 890, TimeToFirstClickMS: -1}
	r := Score(c, f, DefaultConfig())
	if r.Classification != domain.Bot || r.RiskScore != 75 {
		t.Fatalf("burst got %s score=%d", r.Classification, r.RiskScore)
	}
	c.NearbyVisitorSessions = 1 // A shared fingerprint across unrelated visitors is not a page burst.
	c.VisitorsPerFingerprint = 10
	r = Score(c, f, DefaultConfig())
	if r.Classification != domain.Human || r.RiskScore != 32 {
		t.Fatalf("shared fingerprint got %s score=%d", r.Classification, r.RiskScore)
	}
	c.NearbyVisitorSessions = 10
	c.VisitorsPerFingerprint = 1
	f.ClickCount = 1
	r = Score(c, f, DefaultConfig())
	if r.Classification != domain.Human || r.RiskScore != 0 {
		t.Fatalf("engaged burst got %s score=%d", r.Classification, r.RiskScore)
	}
}

func TestSharedFingerprintAndSingleMouseSpikeAreHuman(t *testing.T) {
	c := domain.SessionContext{SessionsPerFingerprint: 9, VisitorsPerFingerprint: 2}
	f := domain.Features{MouseMoves: 222, MouseMaxVelocity: 6000, MouseP95Velocity: 2600, TimeToFirstClickMS: 1000}
	r := Score(c, f, DefaultConfig())
	if r.Classification != domain.Human || r.RiskScore != 0 {
		t.Fatalf("shared fingerprint and isolated spike got %s score=%d", r.Classification, r.RiskScore)
	}
	c.VisitorsPerFingerprint = 9
	r = Score(c, f, DefaultConfig())
	if r.Classification != domain.Human || r.RiskScore != 32 {
		t.Fatalf("distinct visitors got %s score=%d", r.Classification, r.RiskScore)
	}
	f.MouseP95Velocity = 4100 // Fast but common for real desktop users.
	r = Score(c, f, DefaultConfig())
	if r.Classification != domain.Human || r.RiskScore != 32 {
		t.Fatalf("fast human mouse got %s score=%d", r.Classification, r.RiskScore)
	}
	f.MouseP95Velocity = 6500
	r = Score(c, f, DefaultConfig())
	if r.Classification != domain.Suspicious || r.RiskScore != 47 {
		t.Fatalf("sustained high mouse speed got %s score=%d", r.Classification, r.RiskScore)
	}
}

func TestValidateConfig(t *testing.T) {
	valid := []byte(`{"thresholds":{"suspicious":35,"bot":70}}`)
	if err := Validate(valid); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	invalid := []byte(`{"thresholds":{"suspicious":80,"bot":70}}`)
	if err := Validate(invalid); err == nil {
		t.Fatal("invalid thresholds accepted")
	}
}

func TestParseAddsNewRuleToExistingProjectConfig(t *testing.T) {
	cfg := Parse([]byte(`{"weights":{"EXCESSIVE_FINGERPRINT_VELOCITY":32}}`))
	if cfg.Weights["RAPID_UNENGAGED_PAGE_BURST"] != 75 {
		t.Fatalf("new rule missing from existing config: %v", cfg.Weights)
	}
	if cfg.Weights["EXCESSIVE_FINGERPRINT_VELOCITY"] != 32 {
		t.Fatalf("existing weight changed: %v", cfg.Weights)
	}
}

func TestAutomationAloneMakesBot(t *testing.T) {
	r := Score(domain.SessionContext{Automation: map[string]bool{"webdriver": true}}, domain.Features{TimeToFirstClickMS: -1}, DefaultConfig())
	if r.Classification != domain.Bot || r.RiskScore != 70 {
		t.Fatalf("webdriver got %s score=%d", r.Classification, r.RiskScore)
	}
	r = Score(domain.SessionContext{UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/146.0.0.0 Safari/537.36"}, domain.Features{TimeToFirstClickMS: -1}, DefaultConfig())
	if r.Classification != domain.Bot {
		t.Fatalf("headless user agent got %s score=%d", r.Classification, r.RiskScore)
	}
}

func TestLowActiveRatioIgnoresBackgroundTabs(t *testing.T) {
	f := domain.Features{DurationMS: 3 * 60 * 60 * 1000, ActiveRatio: 0.01, ScrollEvents: 50, TimeToFirstClickMS: -1}
	if r := Score(domain.SessionContext{}, f, DefaultConfig()); r.RiskScore != 0 {
		t.Fatalf("background tab got score=%d", r.RiskScore)
	}
	f.DurationMS = 60000
	if r := Score(domain.SessionContext{}, f, DefaultConfig()); r.RiskScore != 15 {
		t.Fatalf("short idle session got score=%d", r.RiskScore)
	}
}

func TestAbandonedAfterFirstFlush(t *testing.T) {
	f := domain.Features{DurationMS: 4995, HeartbeatEvents: 1, TimeToFirstClickMS: -1}
	r := Score(domain.SessionContext{}, f, DefaultConfig())
	if r.Classification != domain.Suspicious || r.RiskScore != 35 {
		t.Fatalf("abandoned page got %s score=%d", r.Classification, r.RiskScore)
	}
	f.VisibilityEvents = 1 // A visitor closing the tab fires visibilitychange.
	if r = Score(domain.SessionContext{}, f, DefaultConfig()); r.RiskScore != 0 {
		t.Fatalf("closed tab got score=%d", r.RiskScore)
	}
	f.VisibilityEvents, f.TouchEvents = 0, 1
	if r = Score(domain.SessionContext{}, f, DefaultConfig()); r.RiskScore != 0 {
		t.Fatalf("touched page got score=%d", r.RiskScore)
	}
}

func TestCloudSessionWithoutInteraction(t *testing.T) {
	idle := domain.Features{DurationMS: 35000, ScrollEvents: 10, ActiveRatio: 0.5, TimeToFirstClickMS: -1}
	for _, c := range []struct {
		asn  string
		f    domain.Features
		want string
		risk int
	}{
		{"45102", idle, domain.Bot, 73}, // Alibaba Cloud scraper
		{"396982", domain.Features{DurationMS: 9000, TimeToFirstClickMS: -1}, domain.Bot, 73}, // Google Cloud
		{"210644", idle, domain.Human, 18}, // VPN exit on a hosting provider
		{"45102", domain.Features{DurationMS: 2 * 60 * 60 * 1000, TimeToFirstClickMS: -1}, domain.Human, 18}, // background tab
		{"45102", domain.Features{DurationMS: 35000, MouseMoves: 40, TimeToFirstClickMS: -1}, domain.Human, 18},
		{"8402", idle, domain.Human, 0},
	} {
		r := Score(domain.SessionContext{ASN: c.asn}, c.f, DefaultConfig())
		if r.Classification != c.want || r.RiskScore != c.risk {
			t.Errorf("AS%s %+v: got %s score=%d, want %s score=%d", c.asn, c.f, r.Classification, r.RiskScore, c.want, c.risk)
		}
	}
}
