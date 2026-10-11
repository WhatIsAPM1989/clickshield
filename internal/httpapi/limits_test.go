package httpapi

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lamooof/clickshield/internal/domain"
)

func TestLimitKeyGroupsIPv6Networks(t *testing.T) {
	for ip, want := range map[string]string{
		"203.0.113.7":             "203.0.113.7",
		"::ffff:203.0.113.7":      "203.0.113.7",
		"2001:db8:1:2:aaaa::1":    "2001:db8:1:2::/64",
		"2001:db8:1:2:ffff::9999": "2001:db8:1:2::/64",
		"2001:db8:1:3::1":         "2001:db8:1:3::/64",
	} {
		if got := limitKey(net.ParseIP(ip)); got != want {
			t.Errorf("%s: got %s, want %s", ip, got, want)
		}
	}
}

func TestLimiterCountsUnits(t *testing.T) {
	l := newLimiter(300, time.Minute)
	if !l.AllowN("a", 200) || l.AllowN("a", 200) {
		t.Fatal("event budget is not spent per unit")
	}
	if !l.AllowN("b", 300) {
		t.Fatal("keys must not share a budget")
	}
}

func TestLimiterMemoryIsBounded(t *testing.T) {
	l := newLimiter(1, time.Minute)
	for i := 0; i < maxLimiterEntries+10; i++ {
		l.Allow(net.IPv4(10, byte(i>>16), byte(i>>8), byte(i)).String())
	}
	if len(l.entries) > maxLimiterEntries {
		t.Fatalf("limiter holds %d entries", len(l.entries))
	}
}

func TestProjectCacheKeepsUnknownKeysAwayFromDatabase(t *testing.T) {
	calls := 0
	c := &projectCache{ttl: time.Minute, entries: map[string]projectEntry{}, load: func(_ context.Context, key string) (domain.Project, error) {
		calls++
		switch key {
		case "pk_known":
			return domain.Project{ID: "p1"}, nil
		case "pk_broken":
			return domain.Project{}, errors.New("connection refused")
		}
		return domain.Project{}, pgx.ErrNoRows
	}}
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if p, found, err := c.Get(ctx, "pk_known"); err != nil || !found || p.ID != "p1" {
			t.Fatalf("known key: %+v %v %v", p, found, err)
		}
		if _, found, err := c.Get(ctx, "pk_random"); err != nil || found {
			t.Fatalf("unknown key: %v %v", found, err)
		}
	}
	if calls != 2 {
		t.Fatalf("database queried %d times, want 2", calls)
	}
	for i := 0; i < 2; i++ {
		if _, _, err := c.Get(ctx, "pk_broken"); err == nil {
			t.Fatal("database error must be returned")
		}
	}
	if calls != 4 {
		t.Fatalf("database errors must not be cached, %d calls", calls)
	}
	c.Reset()
	c.Get(ctx, "pk_known")
	if calls != 5 {
		t.Fatal("reset must drop cached keys")
	}
}
