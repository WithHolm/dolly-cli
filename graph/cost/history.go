package cost

import (
	"fmt"
	"strings"
	"time"
)

// The use of each client over time. The limiter keeps the resource units, the calls, and the throttled calls of each client in buckets of
// BucketSpan, for a time that can be set (SetRetention). It is for a graph of the load: a bucket of 10 seconds is the span of the RU limit of
// a client in a tenant (cost.json, identity, appTenant), so a bar can be compared with that limit.

// BucketSpan is the length of a bucket. It is the same for every client and every retention.
const BucketSpan = 10 * time.Second

const (
	// DefaultRetention is how long the buckets are kept when nothing else is set.
	DefaultRetention = time.Hour
	MinRetention     = time.Minute
	MaxRetention     = 24 * time.Hour

	// maxSeries is how many clients have a series of their own. A client ID comes from the token, so anyone can make one up: the clients
	// after these are counted together as OtherApp.
	maxSeries = 200
	// OtherApp is the series of the clients that came after maxSeries.
	OtherApp = "other"
)

// Point is the use in one bucket. T is the first second of the bucket, as Unix time.
type Point struct {
	T         int64 `json:"t"`
	RU        int   `json:"ru"`
	Calls     int   `json:"calls"` // every call, with the throttled ones
	Throttled int   `json:"throttled"`
}

// History is the use of a client (or of all of them) over the retention, a point for every bucket, with the empty ones as zeros.
type History struct {
	BucketSeconds    int     `json:"bucketSeconds"`
	RetentionSeconds int     `json:"retentionSeconds"`
	Now              int64   `json:"now"`
	Points           []Point `json:"points"`
}

// LimitLine is a limit that a graph can draw: the resource units that a client may use in a span.
type LimitLine struct {
	Scope   string `json:"scope"`
	RU      int    `json:"ru"`
	Seconds int    `json:"seconds"`
}

type series struct{ buckets map[int64]*Point } // by bucket number: Unix seconds divided by the span

type history struct {
	retention time.Duration
	series    map[string]*series
	pruned    int64 // the bucket where the last prune was
}

func newHistory() history { return history{retention: DefaultRetention, series: map[string]*series{}} }

// SetRetention sets how long the buckets are kept: from a minute to a day. A bucket is always BucketSpan.
func (l *Limiter) SetRetention(d time.Duration) error {
	if d < MinRetention || d > MaxRetention {
		return fmt.Errorf("the history must be from %v to %v, it is %v", MinRetention, MaxRetention, d)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hist.retention = d
	return nil
}

func bucketOf(t time.Time) int64 { return t.Unix() / int64(BucketSpan/time.Second) }

// record adds a call to the bucket of the client. The caller holds the lock.
func (l *Limiter) record(app string, now time.Time, ru int, throttled bool) {
	app = strings.ToLower(app) // a client ID is written in more than one way (a GUID in upper or lower case)
	h := &l.hist
	s := h.series[app]
	if s == nil {
		if len(h.series) >= maxSeries {
			app = OtherApp
			s = h.series[app]
		}
		if s == nil {
			s = &series{buckets: map[int64]*Point{}}
			h.series[app] = s
		}
	}
	b := bucketOf(now)
	p := s.buckets[b]
	if p == nil {
		p = &Point{T: b * int64(BucketSpan/time.Second)}
		s.buckets[b] = p
	}
	p.Calls++
	p.RU += ru
	if throttled {
		p.Throttled++
	}
	if b != h.pruned { // once for each bucket: the old ones go
		h.pruned = b
		oldest := b - int64(h.retention/BucketSpan)
		for _, other := range h.series {
			for k := range other.buckets {
				if k <= oldest {
					delete(other.buckets, k)
				}
			}
		}
	}
}

// History gives the use of one client over the retention. With no client it gives the sum of all of them.
func (l *Limiter) History(app string) History {
	app = strings.ToLower(app)
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	n := int(l.hist.retention / BucketSpan)
	last := bucketOf(now)
	out := History{BucketSeconds: int(BucketSpan / time.Second), RetentionSeconds: int(l.hist.retention / time.Second), Now: now.Unix(), Points: make([]Point, n)}
	for i := range out.Points {
		out.Points[i].T = (last - int64(n-1-i)) * int64(BucketSpan/time.Second)
	}
	add := func(s *series) {
		for k, p := range s.buckets {
			if i := n - 1 - int(last-k); i >= 0 && i < n {
				out.Points[i].RU += p.RU
				out.Points[i].Calls += p.Calls
				out.Points[i].Throttled += p.Throttled
			}
		}
	}
	if app == "" {
		for _, s := range l.hist.series {
			add(s)
		}
	} else if s := l.hist.series[app]; s != nil {
		add(s)
	}
	return out
}

// RULimit is the limit of resource units that a client has in a tenant of this many users (cost.json, identity, appTenant). It is false when
// the data has none.
func (l *Limiter) RULimit(users int) (LimitLine, bool) {
	for _, lim := range l.d.Identity.Limits {
		if lim.Scope != "appTenant" || lim.RU == nil {
			continue
		}
		span, err := ParseDuration(lim.Per)
		if err != nil {
			return LimitLine{}, false
		}
		return LimitLine{Scope: "Tenant_Application", RU: lim.RU.Size(TenantSize(users)), Seconds: int(span / time.Second)}, true
	}
	return LimitLine{}, false
}
