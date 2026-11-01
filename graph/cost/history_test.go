package cost_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/withholm/dolly-cli/graph/cost"
)

func sumOf(h cost.History) (ru, calls, throttled int) {
	for _, p := range h.Points {
		ru, calls, throttled = ru+p.RU, calls+p.Calls, throttled+p.Throttled
	}
	return
}

// The use is in buckets of 10 seconds, one point for every bucket of the retention, with the empty ones as zeros.
func TestHistory_BucketsOf10Seconds(t *testing.T) {
	d, _ := cost.Load()
	c := newClock()
	l := cost.NewLimiter(d, c.now)

	for range 3 { // GET /users costs 2 RU
		l.Charge(get(d), false)
	}
	c.add(25 * time.Second)
	l.Charge(get(d), false)

	h := l.History("app1")
	if h.BucketSeconds != 10 || h.RetentionSeconds != 3600 || len(h.Points) != 360 {
		t.Fatalf("%d points, bucket %ds, retention %ds", len(h.Points), h.BucketSeconds, h.RetentionSeconds)
	}
	last, before := h.Points[359], h.Points[357] // the bucket of now, and the one 25 seconds before (aligned to 10 s)
	if last.RU != 2 || last.Calls != 1 || before.RU != 6 || before.Calls != 3 {
		t.Errorf("last %+v, before %+v", last, before)
	}
	for i, p := range h.Points {
		if want := h.Points[359].T - int64(359-i)*10; p.T != want || p.T%10 != 0 {
			t.Fatalf("point %d is at %d, want %d", i, p.T, want)
		}
	}
	if ru, calls, thr := sumOf(h); ru != 8 || calls != 4 || thr != 0 {
		t.Errorf("sums: %d RU, %d calls, %d throttled", ru, calls, thr)
	}
	if ru, _, _ := sumOf(l.History("other-app")); ru != 0 {
		t.Error("a client with no calls has zeros")
	}
}

// A throttled call is a call and a throttled call in its bucket, and it costs nothing (it is not counted in the use).
func TestHistory_ThrottledCallsAreMarked(t *testing.T) {
	d, _ := cost.Load()
	c := newClock()
	l := cost.NewLimiter(d, c.now)
	for range 1750 { // a small tenant: 3500 RU in 10 seconds
		l.Charge(get(d), true)
	}
	for range 5 {
		if !l.Charge(get(d), true).Throttled {
			t.Fatal("must be throttled")
		}
	}
	ru, calls, thr := sumOf(l.History("app1"))
	if ru != 3500 || calls != 1755 || thr != 5 {
		t.Errorf("%d RU, %d calls, %d throttled", ru, calls, thr)
	}
	if u := l.Usage()[0]; u.RU != ru || u.Calls != calls || u.Throttled != thr {
		t.Errorf("the totals and the history agree: %+v", u)
	}
}

// With no client the history is the sum of all of them.
func TestHistory_AllClientsAndTheTotal(t *testing.T) {
	d, _ := cost.Load()
	c := newClock()
	l := cost.NewLimiter(d, c.now)
	for i := range 3 {
		r := get(d)
		r.App = fmt.Sprintf("app%d", i)
		l.Charge(r, false)
	}
	if ru, calls, _ := sumOf(l.History("")); ru != 6 || calls != 3 {
		t.Errorf("the sum of all: %d RU, %d calls", ru, calls)
	}
	if ru, _, _ := sumOf(l.History("app1")); ru != 2 {
		t.Errorf("one client: %d RU", ru)
	}
}

// Buckets older than the retention are gone, and the retention can be set (from a minute to a day).
func TestHistory_RetentionIsKeptAndSet(t *testing.T) {
	d, _ := cost.Load()
	c := newClock()
	l := cost.NewLimiter(d, c.now)
	if err := l.SetRetention(2 * time.Minute); err != nil {
		t.Fatal(err)
	}
	l.Charge(get(d), false)
	c.add(90 * time.Second)
	l.Charge(get(d), false)
	h := l.History("app1")
	if len(h.Points) != 12 || h.RetentionSeconds != 120 {
		t.Fatalf("%d points, retention %ds", len(h.Points), h.RetentionSeconds)
	}
	if ru, _, _ := sumOf(h); ru != 4 {
		t.Errorf("both calls are in the 2 minutes: %d RU", ru)
	}
	c.add(60 * time.Second) // the first call is now 150 seconds old
	l.Charge(get(d), false)
	if ru, calls, _ := sumOf(l.History("app1")); calls != 2 || ru != 4 {
		t.Errorf("the first call is gone: %d calls, %d RU", calls, ru)
	}
	for _, bad := range []time.Duration{0, 30 * time.Second, 25 * time.Hour} {
		if l.SetRetention(bad) == nil {
			t.Errorf("%v must be refused", bad)
		}
	}
	if l.SetRetention(24*time.Hour) != nil || len(l.History("").Points) != 8640 {
		t.Error("a day is 8640 buckets")
	}
}

// A client ID comes from the token, so the number of series is limited: the clients after 200 are counted together.
func TestHistory_ManyClientsAreCountedTogether(t *testing.T) {
	d, _ := cost.Load()
	l := cost.NewLimiter(d, newClock().now)
	for i := range 250 {
		r := get(d)
		r.App = fmt.Sprintf("app%03d", i)
		l.Charge(r, false)
	}
	if ru, calls, _ := sumOf(l.History("")); calls != 250 || ru != 500 {
		t.Errorf("all are in the sum: %d calls, %d RU", calls, ru)
	}
	if _, calls, _ := sumOf(l.History(cost.OtherApp)); calls != 50 {
		t.Errorf("the last 50 are other: %d", calls)
	}
	if _, calls, _ := sumOf(l.History("app249")); calls != 0 {
		t.Error("a client after the limit has no series of its own")
	}
}

func TestHistory_ResetForgetsIt(t *testing.T) {
	d, _ := cost.Load()
	l := cost.NewLimiter(d, newClock().now)
	l.Charge(get(d), false)
	l.Reset()
	if ru, calls, _ := sumOf(l.History("")); ru != 0 || calls != 0 {
		t.Errorf("%d RU, %d calls after a reset", ru, calls)
	}
}

// The limit that the graph draws is the RU limit of a client in a tenant, by the size of the tenant (cost.json).
func TestHistory_TheLimitLine(t *testing.T) {
	d, _ := cost.Load()
	l := cost.NewLimiter(d, newClock().now)
	for users, want := range map[int]int{10: 3500, 200: 5000, 1000: 8000} {
		line, ok := l.RULimit(users)
		if !ok || line.RU != want || line.Seconds != 10 || line.Scope != "Tenant_Application" {
			t.Errorf("%d users: %+v", users, line)
		}
	}
}
