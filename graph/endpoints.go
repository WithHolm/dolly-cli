package graph

import (
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/withholm/dolly-cli/graph/query"
	"github.com/withholm/dolly-cli/graph/spec"
	"github.com/withholm/dolly-cli/shared/docs"
)

// The API tab of the page: the operations that dolly supports (the table is made by
// mountAPI from the resources in the api package), and how often each was called.

// hit counts the calls of one operation.
type hit struct {
	Calls      int
	Errors     int // answers with a status of 400 or more
	Last       time.Time
	LastStatus int
}

// tracker counts the calls that dolly gets: for each operation, and for the
// calls that no operation answers (a tool asks for something that dolly does not
// know). The second list shows where a tool needs more than dolly gives.
type tracker struct {
	mu          sync.Mutex
	routes      map[string]*hit // by the pattern of the route: "GET /v1.0/groups/{id}"
	unsupported map[string]*hit // by "METHOD /path", with IDs as {id}
}

// maxUnsupported keeps the memory small when a tool asks for many different paths.
const maxUnsupported = 200

var guidSegment = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// normalize writes the IDs in a path as {id}, so the calls to many groups are one line.
func normalize(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if guidSegment.MatchString(p) {
			parts[i] = "{id}"
		}
	}
	return strings.Join(parts, "/")
}

func (t *tracker) add(m map[string]*hit, key string, status int) {
	h := m[key]
	if h == nil {
		h = &hit{}
		m[key] = h
	}
	h.Calls++
	if status >= 400 {
		h.Errors++
	}
	h.Last, h.LastStatus = time.Now(), status
}

// track counts a call that went through the routes. The mux has put the pattern
// of the route it chose in the request. An empty pattern means that no operation
// answers it.
func (s *Server) track(r *http.Request, status int) {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()
	if s.stats.routes == nil {
		s.stats.routes, s.stats.unsupported = map[string]*hit{}, map[string]*hit{}
	}
	if r.Pattern != "" {
		pattern := r.Pattern
		if r.Context().Value(meKey{}) != nil { // a call of /me ran as /users/{id}
			pattern = strings.Replace(pattern, BasePath+"/users/{id}", BasePath+"/me", 1)
		}
		s.stats.add(s.stats.routes, pattern, status)
		s.noteNeed(r, pattern, status) // the permissions that the call needs (needs.go): its own lock, and the stats lock is never taken inside it
		return
	}
	key := r.Method + " " + normalize(r.URL.Path)
	if _, known := s.stats.unsupported[key]; !known && len(s.stats.unsupported) >= maxUnsupported {
		return
	}
	s.stats.add(s.stats.unsupported, key, status)
}

func (s *Server) resetStats() {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()
	s.stats.routes, s.stats.unsupported = nil, nil
}

// EndpointInfo is an Endpoint with how often it was called, and what the description of Graph says
// about the same call.
type EndpointInfo struct {
	Endpoint
	// Doc is the page of Microsoft Learn about the call.
	Doc string `json:"doc,omitempty"`
	// Schema is the schema of the objects that a call with $filter gives: user, group, and so on.
	// filterProperties in the answer of the admin API has what Graph says about its properties.
	Schema string `json:"schema,omitempty"`
	// GraphQuery and GraphHeaders are the query options and the headers that the description of
	// Graph lists for the call. What dolly does not answer is the difference with Query.
	GraphQuery   []string `json:"graphQuery,omitempty"`
	GraphHeaders []string `json:"graphHeaders,omitempty"`
	// Resource is the page of the reference that the call is on (the key of its page: users, groups). Section
	// is the section when the page only covers the call, such as /me on the page of the users. Notes is
	// the text of the call from the markdown files.
	Resource string   `json:"resource"`
	Section  string   `json:"section,omitempty"`
	Notes    *docs.Op `json:"notes,omitempty"`

	Calls      int        `json:"calls"`
	Errors     int        `json:"errors"`
	Last       *time.Time `json:"last,omitempty"`
	LastStatus int        `json:"lastStatus,omitempty"`
}

// schemaOf gives the schema of the objects of a resource, by the first part of its path.
var schemaOf = map[string]string{
	"users": "user", "groups": "group", "servicePrincipals": "servicePrincipal", "applications": "application",
	"organization": "organization", "oauth2PermissionGrants": "oAuth2PermissionGrant", "places": "place",
}

// filterProperties gives, for each schema that a call with $filter gives, what Graph says about
// $filter on its properties.
func filterProperties(endpoints []EndpointInfo) map[string][]spec.FilterProperty {
	out := map[string][]spec.FilterProperty{}
	for _, e := range endpoints {
		if _, done := out[e.Schema]; e.Schema != "" && !done {
			out[e.Schema] = spec.Load().FilterProperties(e.Schema)
		}
	}
	return out
}

// filterRules gives, for each schema that has the rules of Graph for a directory object (user, group,
// servicePrincipal) and that a call with $filter gives, each property with its operators and its sorting.
func filterRules(endpoints []EndpointInfo) map[string][]query.Row {
	out := map[string][]query.Row{}
	m := query.Load()
	for _, e := range endpoints {
		if _, done := out[e.Schema]; e.Schema != "" && !done && m.HasResource(e.Schema) {
			out[e.Schema] = m.Rows(e.Schema)
		}
	}
	return out
}

// UnsupportedCall is a call that no operation answers.
type UnsupportedCall struct {
	Method string    `json:"method"`
	Path   string    `json:"path"`
	Calls  int       `json:"calls"`
	Status int       `json:"status"` // what dolly answered: 404, or 405 when the path is known and the method is not
	Last   time.Time `json:"last"`
}

// API gives the operations that dolly supports, with their call counts, and the
// calls that nothing answers.
func (s *Server) API() ([]EndpointInfo, []UnsupportedCall) {
	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()
	out := make([]EndpointInfo, 0, len(s.endpoints))
	desc := spec.Load()
	for _, e := range s.endpoints {
		info := EndpointInfo{Endpoint: e}
		if op := desc.ForDolly(e.Method, e.Path); op != nil {
			info.Doc, info.GraphQuery, info.GraphHeaders = op.Doc, op.Query, op.Headers
		}
		if slices.Contains(e.Query, "$filter") {
			info.Schema = schemaOf[strings.SplitN(strings.TrimPrefix(e.Path, BasePath+"/"), "/", 2)[0]]
		}
		if h := s.stats.routes[e.Method+" "+e.Path]; h != nil {
			last := h.Last
			info.Calls, info.Errors, info.Last, info.LastStatus = h.Calls, h.Errors, &last, h.LastStatus
		}
		out = append(out, info)
	}
	attachDocs(out, s.DocPages())
	var un []UnsupportedCall
	for key, h := range s.stats.unsupported {
		method, path, _ := strings.Cut(key, " ")
		un = append(un, UnsupportedCall{Method: method, Path: path, Calls: h.Calls, Status: h.LastStatus, Last: h.Last})
	}
	sort.Slice(un, func(i, j int) bool { return un[i].Last.After(un[j].Last) })
	return out, un
}
