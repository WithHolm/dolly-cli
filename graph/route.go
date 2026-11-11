package graph

import (
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/withholm/dolly-cli/graph/cost"
	"github.com/withholm/dolly-cli/graph/permissions"
	"github.com/withholm/dolly-cli/graph/spec"
)

// RouteInfo is what dollycli knows about one call of Graph, joined from the spec, the permission data, and the cost data. It is for a
// person or a tool that asks "will this call work here, and what does it need?".
type RouteInfo struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	// Served is true when dollycli serves the route. Route is the route as the server writes it.
	Served bool   `json:"served"`
	Route  string `json:"route,omitempty"`
	// Summary, Learn, Query, and Headers are from the spec, so they are what Microsoft's description says, not what the real Graph does.
	Summary string   `json:"summary,omitempty"`
	Learn   string   `json:"learn,omitempty"`
	Query   []string `json:"query,omitempty"`
	Headers []string `json:"headers,omitempty"`
	// Permissions is from the Learn page of the call. Table is false when there is no such data.
	Permissions permissions.CallPermissions `json:"permissions"`
	// RU is the cost of the call with no query options, in resource units. CostRow is the row of the table that gave it.
	RU      int    `json:"ru,omitempty"`
	CostRow string `json:"costRow,omitempty"`
	// Nearest are other routes under the same first part of the path, when nothing matches.
	Nearest []string `json:"nearest,omitempty"`
}

var (
	servedOnce sync.Once
	served     map[string]string // spec.Normal form of "METHOD /path" -> the route as the server writes it
)

func servedRoutes() map[string]string {
	servedOnce.Do(func() {
		served = map[string]string{}
		if ops, err := permissions.LoadOperations(); err == nil {
			add := func(route string) {
				m, p, _ := strings.Cut(route, " ")
				served[m+" "+spec.Normal(p)] = route
			}
			for r := range ops.Routes {
				add(r)
			}
			for _, r := range ops.NoPage {
				add(r)
			}
		}
	})
	return served
}

// Describe joins what dollycli has about a call. The path can have any {names}, and a start of /v1.0 or not.
func Describe(method, path string) RouteInfo {
	method = strings.ToUpper(strings.TrimSpace(method))
	path = "/" + strings.TrimPrefix(strings.TrimSpace(path), "/")
	r := RouteInfo{Method: method, Path: path}
	key := method + " " + spec.Normal(path)
	r.Route, r.Served = servedRoutes()[key]

	ops, _ := permissions.LoadOperations()
	if op := spec.Load().ForDolly(method, path); op != nil {
		r.Summary, r.Query, r.Headers = op.Summary, op.Query, op.Headers
		if page, ok := permissions.PageOf(op.Doc); ok {
			r.Learn = permissions.LearnURL(page)
		}
		r.Permissions, _ = ops.ForDoc(op.Doc)
	}
	if r.Served {
		if p, page, ok := ops.ForRoute(r.Route); ok {
			r.Permissions = p
			r.Learn = permissions.LearnURL(page)
		}
	}
	if r.Served || r.Summary != "" {
		if d, err := cost.Load(); err == nil {
			p := d.Price(method, strings.TrimPrefix(path, BasePath), url.Values{})
			r.RU, r.CostRow = p.RU, p.Route
		}
	}
	if !r.Served && r.Summary == "" {
		r.Nearest = nearest(method, path)
	}
	return r
}

// nearest lists up to five routes, served or in the spec, with the same method and the same first part of the path.
func nearest(method, path string) []string {
	first := func(p string) string {
		p = strings.TrimPrefix(spec.Normal(p), "/")
		a, _, _ := strings.Cut(p, "/")
		return a
	}
	want := first(path)
	seen := map[string]string{} // normal form -> path to show
	for _, o := range spec.Load().Operations {
		if o.Method == method && first(o.Path) == want {
			seen[spec.Normal(o.Path)] = o.Path
		}
	}
	for k, route := range servedRoutes() {
		if m, p, _ := strings.Cut(k, " "); m == method && first(p) == want {
			_, shown, _ := strings.Cut(route, " ")
			if _, ok := seen[p]; !ok {
				seen[p] = strings.TrimPrefix(shown, BasePath)
			}
		}
	}
	out := make([]string, 0, len(seen))
	for _, p := range seen {
		out = append(out, p)
	}
	// the shortest paths first: they are the main routes of the resource
	sort.Slice(out, func(i, j int) bool {
		if len(out[i]) != len(out[j]) {
			return len(out[i]) < len(out[j])
		}
		return out[i] < out[j]
	})
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}
