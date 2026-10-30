package cost

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ParseDuration reads the spans of cost.json: 10s, 2m30s, 5m, 3600s, 24h, and 1d.
func ParseDuration(s string) (time.Duration, error) {
	if n, ok := strings.CutSuffix(s, "d"); ok {
		days, err := strconv.Atoi(n)
		if err != nil {
			return 0, fmt.Errorf("cannot read the span %q", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("cannot read the span %q", s)
	}
	return d, nil
}

// window adds up amounts over the last span. It keeps one sum for each second.
type window struct {
	span    time.Duration
	buckets map[int64]int
}

func (w *window) used(now time.Time) int {
	cutoff := now.Add(-w.span).Unix()
	sum := 0
	for k, v := range w.buckets {
		if k <= cutoff {
			delete(w.buckets, k)
		} else {
			sum += v
		}
	}
	return sum
}

func (w *window) add(now time.Time, n int) { w.buckets[now.Unix()] += n }

// wait is how long until n more fit under limit. At least one second.
func (w *window) wait(now time.Time, n, limit int) time.Duration {
	used := w.used(now)
	keys := make([]int64, 0, len(w.buckets))
	for k := range w.buckets {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for _, k := range keys {
		if used+n <= limit {
			break
		}
		used -= w.buckets[k]
		if used+n <= limit {
			return max(time.Duration(k+int64(w.span/time.Second)-now.Unix())*time.Second, time.Second)
		}
	}
	return w.span
}

// Request is one call to charge.
type Request struct {
	App         string // the application (client) ID
	Tenant      string
	TenantUsers int // the number of users in the tenant, for its size
	Price       Price
}

// Decision is what the limiter says about a request.
type Decision struct {
	Throttled  bool
	Scope      string // x-ms-throttle-scope
	Info       string // x-ms-throttle-information
	RetryAfter time.Duration
	// Percentage is the use of the limit of the client in the tenant (resource units, the last
	// window): 0.8 is 80%, 1.0 is the limit. It is for x-ms-throttle-limit-percentage.
	Percentage float64
}

// Usage is what one client has asked for. A client is known by its application (client) ID.
type Usage struct {
	App       string                `json:"appId"`
	Calls     int                   `json:"calls"`
	RU        int                   `json:"ru"`
	Throttled int                   `json:"throttled"`
	Routes    map[string]RouteUsage `json:"routes"`
}

// RouteUsage is the use of one row of the cost table.
type RouteUsage struct {
	Calls int `json:"calls"`
	RU    int `json:"ru"`
}

// Limiter counts the resource units of the requests against the limits of cost.json. It is
// safe for many goroutines.
type Limiter struct {
	d   *Data
	now func() time.Time

	mu      sync.Mutex
	windows map[string]*window
	usage   map[string]*Usage
}

// NewLimiter makes a limiter. now is the clock (nil is time.Now); a test moves it by hand.
func NewLimiter(d *Data, now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	return &Limiter{d: d, now: now, windows: map[string]*window{}, usage: map[string]*Usage{}}
}

// check is one limit that a request must pass.
type check struct {
	key    string
	span   time.Duration
	limit  int
	amount int
	scope  string // Tenant_Application, Tenant, or Application
	kind   string // Read, Write, or ReadWrite
	info   string
}

func (l *Limiter) checks(r Request) ([]check, error) {
	var out []check
	size := TenantSize(r.TenantUsers)
	g := l.d.Global
	if span, err := ParseDuration(g.Per); err == nil && g.Requests > 0 {
		out = append(out, check{"global|" + r.App, span, g.Requests, 1, "Application", "ReadWrite", "RequestLimitExceeded"})
	}
	for _, lim := range l.d.Identity.Limits {
		scope, id := "", ""
		switch lim.Scope {
		case "appTenant":
			scope, id = "Tenant_Application", r.App+"|"+r.Tenant
		case "app":
			scope, id = "Application", r.App
		case "tenant":
			scope, id = "Tenant", r.Tenant
		default:
			continue
		}
		if lim.RU != nil {
			span, err := ParseDuration(lim.Per)
			if err != nil {
				return nil, err
			}
			out = append(out, check{lim.Scope + "|ru|" + id, span, lim.RU.Size(size), r.Price.RU, scope, "ReadWrite", "ResourceUnitLimitExceeded"})
		}
		if lim.Writes > 0 && r.Price.Write > 0 {
			span, err := ParseDuration(lim.WritesPer)
			if err != nil {
				return nil, err
			}
			out = append(out, check{lim.Scope + "|w|" + id, span, lim.Writes, r.Price.Write, scope, "Write", "WriteLimitExceeded"})
		}
	}
	return out, nil
}

// Charge counts a request. When enforce is true and a limit is passed, the request is not
// counted and the decision is Throttled; otherwise it is counted. A request that is
// throttled is not added to the use, so a tool that waits for Retry-After gets through.
func (l *Limiter) Charge(r Request, enforce bool) Decision {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	checks, err := l.checks(r)
	var dec Decision
	if err != nil {
		return dec // the data was checked at Load: this does not happen
	}

	win := func(c check) *window {
		w := l.windows[c.key]
		if w == nil {
			w = &window{span: c.span, buckets: map[int64]int{}}
			l.windows[c.key] = w
		}
		return w
	}
	for _, c := range checks {
		w := win(c)
		used := w.used(now)
		if strings.Contains(c.key, "appTenant|ru|") {
			dec.Percentage = min(float64(used+c.amount)/float64(c.limit), 1.8)
		}
		if enforce && used+c.amount > c.limit && !dec.Throttled {
			dec.Throttled = true
			dec.Scope = c.scope + "/" + c.kind + "/" + r.App + "/" + r.Tenant
			dec.Info = c.info
			dec.RetryAfter = w.wait(now, c.amount, c.limit)
		}
	}

	u := l.usage[r.App]
	if u == nil {
		u = &Usage{App: r.App, Routes: map[string]RouteUsage{}}
		l.usage[r.App] = u
	}
	u.Calls++
	if dec.Throttled {
		u.Throttled++
		return dec
	}
	for _, c := range checks {
		win(c).add(now, c.amount)
	}
	u.RU += r.Price.RU
	ru := u.Routes[r.Price.Route]
	u.Routes[r.Price.Route] = RouteUsage{ru.Calls + 1, ru.RU + r.Price.RU}
	return dec
}

// Usage gives what each client has asked for, the one with most resource units first.
func (l *Limiter) Usage() []Usage {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Usage, 0, len(l.usage))
	for _, u := range l.usage {
		c := *u
		c.Routes = make(map[string]RouteUsage, len(u.Routes))
		for k, v := range u.Routes {
			c.Routes[k] = v
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RU > out[j].RU })
	return out
}

// Reset forgets the use and the windows.
func (l *Limiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.windows, l.usage = map[string]*window{}, map[string]*Usage{}
}
