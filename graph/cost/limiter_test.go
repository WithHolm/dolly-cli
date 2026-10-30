package cost_test

import (
	"strings"
	"testing"
	"time"

	"github.com/withholm/dolly-cli/graph/cost"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time      { return c.t }
func (c *clock) add(d time.Duration) { c.t = c.t.Add(d) }
func newClock() *clock               { return &clock{time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)} }
func get(d *cost.Data) cost.Request  { return req(d, "GET", "/users", 10) }
func req(d *cost.Data, method, path string, users int) cost.Request {
	return cost.Request{App: "app1", Tenant: "t1", TenantUsers: users, Price: d.Price(method, path, nil)}
}

func TestLimiter_SmallTenantAllows3500RUIn10Seconds(t *testing.T) {
	d, _ := cost.Load()
	c := newClock()
	l := cost.NewLimiter(d, c.now)

	for i := range 1750 { // GET users costs 2: 1750 of them are 3500
		if dec := l.Charge(get(d), true); dec.Throttled {
			t.Fatalf("request %d was throttled", i+1)
		}
	}
	dec := l.Charge(get(d), true)
	if !dec.Throttled {
		t.Fatal("the request over 3500 RU must be throttled")
	}
	if dec.Info != "ResourceUnitLimitExceeded" || !strings.HasPrefix(dec.Scope, "Tenant_Application/ReadWrite/app1/t1") {
		t.Errorf("scope %q, info %q", dec.Scope, dec.Info)
	}
	if dec.RetryAfter < time.Second || dec.RetryAfter > 10*time.Second {
		t.Errorf("retry after %v", dec.RetryAfter)
	}

	// a throttled request is not counted, and the window moves on
	c.add(dec.RetryAfter)
	if dec := l.Charge(get(d), true); dec.Throttled {
		t.Errorf("after the wait the request must pass: %+v", dec)
	}
}

func TestLimiter_TheLimitDependsOnTheSizeOfTheTenant(t *testing.T) {
	d, _ := cost.Load()
	l := cost.NewLimiter(d, newClock().now)
	r := req(d, "GET", "/users", 1000) // large: 8000 RU, so 4000 requests
	for i := range 4000 {
		if l.Charge(r, true).Throttled {
			t.Fatalf("request %d was throttled in a large tenant", i+1)
		}
	}
	if !l.Charge(r, true).Throttled {
		t.Error("request 4001 must be throttled")
	}
}

func TestLimiter_WithEnforceOffItCountsAndNeverThrottles(t *testing.T) {
	d, _ := cost.Load()
	l := cost.NewLimiter(d, newClock().now)
	var dec cost.Decision
	for range 2000 {
		if dec = l.Charge(get(d), false); dec.Throttled {
			t.Fatal("throttled with enforce off")
		}
	}
	if dec.Percentage < 1.1 || dec.Percentage > 1.2 { // 4000 of 3500
		t.Errorf("percentage %.2f, want about 1.14", dec.Percentage)
	}
	if u := l.Usage(); len(u) != 1 || u[0].RU != 4000 || u[0].Routes["GET users"].Calls != 2000 {
		t.Errorf("usage: %+v", u)
	}
}

func TestLimiter_WritesHaveTheirOwnQuota(t *testing.T) {
	d, _ := cost.Load()
	c := newClock()
	l := cost.NewLimiter(d, c.now)
	post := req(d, "POST", "/groups", 10) // 1 RU, 1 write
	for i := range 3000 {                 // 3000 writes per 2 minutes 30 seconds, for an app in a tenant
		c.add(40 * time.Millisecond) // 2 minutes in all: the RU window of 10 seconds never fills
		if dec := l.Charge(post, true); dec.Throttled {
			t.Fatalf("write %d was throttled: %+v", i+1, dec)
		}
	}
	dec := l.Charge(post, true)
	if !dec.Throttled || dec.Info != "WriteLimitExceeded" || !strings.Contains(dec.Scope, "/Write/") {
		t.Errorf("write 3001: %+v", dec)
	}
	if dec := l.Charge(get(d), true); dec.Throttled {
		t.Errorf("a read must not be throttled by the write quota: %+v", dec)
	}
}

func TestLimiter_AppsAreCountedOnTheirOwn(t *testing.T) {
	d, _ := cost.Load()
	l := cost.NewLimiter(d, newClock().now)
	a, b := get(d), get(d)
	b.App = "app2"
	for range 1750 {
		l.Charge(a, true)
	}
	if !l.Charge(a, true).Throttled {
		t.Error("app1 must be at its limit")
	}
	if l.Charge(b, true).Throttled {
		t.Error("app2 must not be throttled by app1")
	}
}
