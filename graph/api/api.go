// Package api is the Graph API of dolly, split by resource. Each resource has
// its own file with its own router and handlers: users.go, groups.go (with
// group_members.go, group_owners.go, and group_apps.go below it), and apps.go. A
// resource asks the data for what it needs through a small interface that its
// file defines (UsersBackend, GroupsBackend, ServicePrincipalsBackend), so a resource can be
// tested without the data of dolly, and a new resource is a new file and one line
// in the server.
//
// The framework is here too: the router that records each operation (the API tab
// of the page shows the list), the query options ($filter, $search, $select,
// $top, $count, paging), and the errors in the shape of Graph. The server, the
// data, the history, and the admin API are in the package above (dolly).
package api

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Object is one Graph object as JSON, with the camelCase names that Graph writes.
type Object = map[string]any

// The OData types of the objects that can be members of a group.
const (
	TypeUser             = "#microsoft.graph.user"
	TypeGroup            = "#microsoft.graph.group"
	TypeServicePrincipal = "#microsoft.graph.servicePrincipal"
)

// Actor says who made a change. The history keeps it.
type Actor struct {
	// Source is "graph" for a change that a tool made through the Graph API, and
	// "admin" for a change that a person made through the admin API or the page.
	Source string `json:"source"`
	Agent  string `json:"agent,omitempty"` // the User-Agent of a tool
	Call   int64  `json:"call,omitempty"`  // the ID of the call in the list of calls
}

// Env is what a resource needs from the server, apart from the data.
type Env interface {
	// PageSize is a size that forces every page (0 uses $top). The Behavior tab sets it.
	PageSize() int
	// Actor names the tool behind a request.
	Actor(r *http.Request) Actor
}

// Resource is one part of the API: it registers its routes. Add a resource by
// writing a file that has one, and mounting it in the server.
type Resource interface {
	Register(r *Router)
}

// Endpoint is one operation that dolly supports. The page shows the list.
type Endpoint struct {
	Method  string   `json:"method"`
	Path    string   `json:"path"` // with the base path and {names} for the parts that change
	Area    string   `json:"area"`
	Summary string   `json:"summary"`
	Query   []string `json:"query,omitempty"` // the query options that it knows
	Writes  bool     `json:"writes"`          // it changes the data
}

// Router registers the routes of the resources on a mux, and keeps the list of
// operations. The table of operations and the routes of the server are the same
// thing, so the list cannot differ from what is served.
type Router struct {
	mux       *http.ServeMux
	base      string
	env       Env
	endpoints []Endpoint
}

// NewRouter makes a router for the paths under base (for example /v1.0).
func NewRouter(mux *http.ServeMux, base string, env Env) *Router {
	return &Router{mux: mux, base: base, env: env}
}

// Env gives the server's side of the resources.
func (r *Router) Env() Env { return r.env }

// Mount registers resources.
func (r *Router) Mount(resources ...Resource) {
	for _, res := range resources {
		res.Register(r)
	}
}

// Endpoints gives the operations that are registered, in the order they were added.
func (r *Router) Endpoints() []Endpoint { return append([]Endpoint(nil), r.endpoints...) }

// Scope is a part of the paths, with an area (the heading of the API tab). A
// resource makes one for its own path, and one for each sub path that has its
// own area (the members of a group are in the area Members).
type Scope struct {
	r      *Router
	prefix string
	area   string
}

// Scope makes a scope for a path under the base path.
func (r *Router) Scope(prefix, area string) *Scope { return &Scope{r: r, prefix: prefix, area: area} }

// Scope makes a scope below this one, with its own area.
func (s *Scope) Scope(sub, area string) *Scope {
	return &Scope{r: s.r, prefix: s.prefix + sub, area: area}
}

// Handle registers one operation. path is added to the prefix of the scope, and it
// can be empty. query names the query options that the operation knows.
func (s *Scope) Handle(method, path, summary string, query []string, h http.HandlerFunc) {
	full := s.r.base + s.prefix + path
	s.r.mux.HandleFunc(method+" "+full, h)
	s.r.endpoints = append(s.r.endpoints, Endpoint{
		Method: method, Path: full, Area: s.area, Summary: summary, Query: query,
		Writes: method == http.MethodPost || method == http.MethodDelete || method == http.MethodPatch,
	})
}

// ---- errors and answers ----

// Error is an error in the shape that Graph uses.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Stamp puts on a response the headers that every answer of Graph has: request-id, client-request-id (the one that
// the client sent, or the same as request-id), and Cache-Control. An error reads the two ids from there, for its innerError.
func Stamp(h http.Header, r *http.Request) {
	id := newID()
	h.Set("request-id", id)
	if c := r.Header.Get("client-request-id"); c != "" {
		id = c
	}
	h.Set("client-request-id", id)
	h.Set("Cache-Control", "no-cache")
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// WriteError answers with an error, with the innerError that Graph adds: the time, and the ids of the request.
func WriteError(w http.ResponseWriter, e *Error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.Status)
	body := map[string]any{"code": e.Code, "message": e.Message}
	if id := w.Header().Get("request-id"); id != "" {
		body["innerError"] = map[string]any{
			"date": time.Now().UTC().Format("2006-01-02T15:04:05"), "request-id": id, "client-request-id": w.Header().Get("client-request-id"),
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"error": body})
}

// jsonType is the Content-Type that Graph gives to a JSON answer.
const jsonType = "application/json; odata.metadata=minimal; odata.streaming=true; IEEE754Compatible=false; charset=utf-8"

// WriteJSON answers with a JSON value.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", jsonType)
	w.Header().Set("OData-Version", "4.0")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// SyntaxError is the 400 with the plain code BadRequest, that Graph gives for a query that cannot be read.
func SyntaxError(format string, args ...any) *Error {
	return &Error{http.StatusBadRequest, "BadRequest", fmt.Sprintf(format, args...)}
}

// NoContent answers 204.
func NoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

// Unsupported is the error for a query that dolly does not know.
func Unsupported(format string, args ...any) *Error {
	return &Error{http.StatusBadRequest, "Request_UnsupportedQuery", fmt.Sprintf(format, args...)}
}

// BadRequest is a 400 with the code that Graph uses for a bad request.
func BadRequest(format string, args ...any) *Error {
	return &Error{http.StatusBadRequest, "Request_BadRequest", fmt.Sprintf(format, args...)}
}

// NotFound is the 404 of Graph for an object that is not there.
func NotFound(what, id string) *Error {
	return &Error{http.StatusNotFound, "Request_ResourceNotFound",
		fmt.Sprintf("Resource '%s' does not exist or one of its queried reference-property objects are not present.", id)}
}

// NeedsConsistency fails a request that uses an advanced query without the
// header that Graph asks for.
func NeedsConsistency(r *http.Request, what string) *Error {
	if strings.EqualFold(r.Header.Get("ConsistencyLevel"), "eventual") {
		return nil
	}
	return Unsupported("Request with %s only works through MSGraph with a special request header: 'ConsistencyLevel: eventual'.", what)
}

// ReadJSON decodes the JSON object of a request.
func ReadJSON(r *http.Request) (Object, *Error) {
	var props Object
	if err := json.NewDecoder(r.Body).Decode(&props); err != nil || props == nil {
		return nil, BadRequest("The request body must be a JSON object.")
	}
	return props, nil
}

// refID reads the object ID at the end of an @odata.id in the body of a $ref call.
func refID(r *http.Request) (string, *Error) {
	props, e := ReadJSON(r)
	if e != nil {
		return "", e
	}
	ref, _ := props["@odata.id"].(string)
	if ref == "" {
		return "", BadRequest("The request body needs @odata.id.")
	}
	return ref[strings.LastIndex(ref, "/")+1:], nil
}

// Str reads a string out of a JSON value, and gives "" for another type.
func Str(v any) string {
	s, _ := v.(string)
	return s
}

// QueryOption is a query option or a header that dolly knows, with the forms of
// it that work. A form that is not here gives 400 (Request_UnsupportedQuery), so a
// tool does not get an answer that looks right and is not.
type QueryOption struct {
	Name string `json:"name"`
	Text string `json:"text"`
	// Usage is the forms of the option, written like the help of a command: the syntax, a short note, and
	// an example that a person can copy. A test sends every Example to a server, so the help is true.
	Usage []Usage `json:"usage,omitempty"`
}

// Usage is one form of a query option.
type Usage struct {
	Syntax  string `json:"syntax"`  // $expand=manager($levels=n|max)
	Note    string `json:"note"`    // what it does, in a short sentence
	Example string `json:"example"` // the value of the option that works: manager($levels=max)
}

// QueryOptions are the query options and headers of Graph that dolly knows.
var QueryOptions = []QueryOption{
	{Name: "$select", Text: "Keeps the fields that you name, and id."},
	{Name: "$top", Text: "The size of a page, up to 999. When there is more, the answer has @odata.nextLink."},
	{Name: "$filter", Text: filterSummary()},
	{Name: "$search", Text: "On a directory object: \"property:text\" clauses, joined with AND and OR. Needs ConsistencyLevel: eventual."},
	{Name: "$orderby", Text: "property [asc|desc], separated by commas. On a directory object, only some properties sort, and some need an advanced query."},
	{
		Name: "$expand",
		Text: "On a user: the manager, or the direct reports, inside each user. A user with none has no such property. Another relation, or $filter inside $expand, gets 400.",
		Usage: []Usage{
			{"$expand=directReports", "The direct reports come inside the user, at most 20.", "directReports"},
			{"$expand=directReports($select=property,...)", "Keep only these properties of the direct reports.", "directReports($select=id,displayName)"},
			{"$expand=manager", "The manager comes inside the user.", "manager"},
			{"$expand=manager($select=property,...)", "Keep only these properties of the manager.", "manager($select=id,displayName)"},
			{"$expand=manager($levels=n|max)", "Follow the chain up, each manager inside the one below. n is 1 to 1000. One user only (/users/{id}, /me). Needs ConsistencyLevel: eventual.", "manager($levels=max)"},
			{"$expand=manager($levels=n;$select=property,...)", "The options are joined with ;", "manager($levels=2;$select=id,displayName)"},
		},
	},
	{Name: "$count", Text: "$count=true adds @odata.count. The path /$count gives the number alone. On a directory object, both need ConsistencyLevel: eventual: without it, /$count is an error, and $count=true is left out."},
	{Name: "ConsistencyLevel", Text: "ConsistencyLevel: eventual, with $count=true, makes an advanced query on a directory object. $search, $count, and some uses of $filter and $orderby need it."},
	{Name: "Authorization", Text: "Any bearer token works. A call with no token gets 401."},
}
