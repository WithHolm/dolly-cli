package permissions

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sync"
)

// operations.json says which permissions each call of Graph needs. It is made from the pages of Microsoft Learn by
// 'make update-operation-permissions' (see operations/README.md). A call is known by the page that tells about it, the part of the address
// after /graph/api/: group-list-members.

//go:embed operations/operations.json
var operationsJSON []byte

// OperationsSource says where operations.json comes from.
type OperationsSource struct {
	Repo       string `json:"repo"`
	Commit     string `json:"commit"`
	CommitDate string `json:"commitDate"`
	License    string `json:"license"`
}

// Operations is the content of operations.json.
type Operations struct {
	Source OperationsSource           `json:"source"`
	Pages  map[string]CallPermissions `json:"pages"`
	// Routes maps a route that dollycli serves, as the server writes it ("GET /v1.0/groups/{id}"), to the page of Learn that is about the call.
	// The page is matched by the calls that the pages list themselves (see callmap.go), and the link of the description of Graph is only used to
	// choose between pages.
	Routes map[string]string `json:"routes"`
	// Inherited are routes that have no page of their own and use the page of another route: a $count route, a route with a type
	// (.../microsoft.graph.group), or a /me route. The value is the route that it takes the page from. It is in Routes too.
	Inherited map[string]string `json:"inherited,omitempty"`
	// NoPage are the routes that no page of Learn is about. The portal says "no permission data" for them.
	NoPage []string `json:"noPage"`
}

// ForRoute gives the permissions of a route that dollycli serves, and the key of the page. The bool is false for a route with no page, and for a
// route that is not in the data.
func (o *Operations) ForRoute(route string) (CallPermissions, string, bool) {
	if o == nil {
		return CallPermissions{}, "", false
	}
	page, ok := o.Routes[route]
	if !ok || page == "" {
		return CallPermissions{}, "", false
	}
	p, ok := o.Pages[page]
	return p, page, ok
}

// LearnURL is the address of the page of Learn for a page key: user-list.
func LearnURL(page string) string {
	return "https://learn.microsoft.com/graph/api/" + page + "?view=graph-rest-1.0"
}

var (
	opsOnce sync.Once
	ops     *Operations
	opsErr  error
)

// LoadOperations reads the data that is built in. It refuses a field that it does not know, so a change in the file is a change in the types.
func LoadOperations() (*Operations, error) {
	opsOnce.Do(func() {
		var o Operations
		dec := json.NewDecoder(bytes.NewReader(operationsJSON))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&o); err != nil {
			opsErr = fmt.Errorf("permissions/operations.json: %w", err)
			return
		}
		ops = &o
	})
	return ops, opsErr
}

var pageRe = regexp.MustCompile(`/graph/api/([A-Za-z0-9_.\-]+)`)

// PageOf gives the key of a page of Learn from its address: https://learn.microsoft.com/graph/api/user-list?view=graph-rest-1.0 is user-list.
func PageOf(doc string) (string, bool) {
	m := pageRe.FindStringSubmatch(doc)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// ForDoc gives the permissions of the call that a page of Learn is about. The bool is false when the data has no such page, and the
// result has Table false when the page has no table of permissions.
func (o *Operations) ForDoc(doc string) (CallPermissions, bool) {
	page, ok := PageOf(doc)
	if !ok || o == nil {
		return CallPermissions{}, false
	}
	p, ok := o.Pages[page]
	return p, ok
}
