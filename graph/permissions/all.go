package permissions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// The map of every call of the description of Graph (graph/spec) and every permission that the pages of Microsoft Learn name for them. It is a
// file to look things up in: operations/all.json. It is made by 'make update-permission-map' and is not built into the program (it is a few
// megabytes, and the program needs only the calls that it serves: operations.json).

// AllPage is one page of Learn about a call: the calls it is about, and the permissions that they need.
type AllPage struct {
	// Summary is the summary of the page in the description of Graph, when the description links to it: "List groups".
	Summary string `json:"summary,omitempty"`
	// Calls are the calls of the description that are matched to this page: "GET /groups". More than one call can have the same page.
	Calls []string `json:"calls"`
	// Lists are the calls that the page lists itself, in the form of permissions.CallKey. They can be more than Calls: the page can tell about
	// calls that the description does not have.
	Lists []string `json:"lists,omitempty"`
	CallPermissions
}

// Use is a call that a permission allows. With are the other permissions that the call needs at the same time ("A and B"): the permission
// alone does not allow it.
type Use struct {
	Page string   `json:"page"`
	With []string `json:"with,omitempty"`
	// Resource is the type of object, when the call needs this permission only for it: "user" or "group".
	Resource string `json:"resource,omitempty"`
	// Scenario is the heading of the table, when the page has more than one: "Permissions for another user's direct memberships".
	Scenario string `json:"scenario,omitempty"`
}

// Uses are the calls that a permission allows for one kind of token: as the least privileged way, and as a higher privileged way.
type Uses struct {
	Least  []Use `json:"least,omitempty"`
	Higher []Use `json:"higher,omitempty"`
}

// PermissionUse is what one permission allows. InCatalog says for which kinds the list of the permissions of Graph that dollycli has
// (graph/permissions/raw) has it. A page can name a permission that is newer than that list.
type PermissionUse struct {
	InCatalog   []string `json:"inCatalog"`
	Delegated   Uses     `json:"delegated"`
	Application Uses     `json:"application"`
}

// AllProblems is what could not be read. The file says so, and does not hide it.
type AllProblems struct {
	// NotFound are the pages (or include files) that are not in the repo of Microsoft at the commit.
	NotFound []string `json:"notFound,omitempty"`
	// Unreadable are the parts of tables that the parser could not read, by page.
	Unreadable map[string][]string `json:"unreadable,omitempty"`
	// NoTable are the pages that have no table of permissions in a form that the parser knows.
	NoTable []string `json:"noTable,omitempty"`
	// Counts say how many calls were matched in each way (see CallRef): verified, matched, spec-only, and none.
	Counts map[string]int `json:"counts,omitempty"`
}

// AllFile is the content of operations/all.json.
type AllFile struct {
	Source OperationsSource `json:"source"`
	// Pages by the key of the page of Learn: user-list.
	Pages map[string]AllPage `json:"pages"`
	// Calls maps every call of the description, "GET /users/{user-id}", to its page and how sure that is (see CallRef). The page is "" when no
	// page of Learn is about the call.
	Calls map[string]CallRef `json:"calls"`
	// Permissions maps each permission that the pages name, by its name, to the calls that it allows.
	Permissions map[string]PermissionUse `json:"permissions"`
	Problems    AllProblems              `json:"problems"`
}

// Lookup finds a call by method and path, with any names for the parameters and any capitals: GET /users/{id} finds GET /users/{user-id}. It gives
// the call as the description of Graph writes it, how it was matched to a page, and the page. The bool is false when the description has no such call.
// A call that no page is about has an empty page.
func (f *AllFile) Lookup(method, path string) (call string, ref CallRef, page AllPage, ok bool) {
	want := CallKey(method, path)
	for c, r := range f.Calls {
		m, p, _ := strings.Cut(c, " ")
		if CallKey(m, p) == want {
			return c, r, f.Pages[r.Page], true
		}
	}
	return "", CallRef{}, AllPage{}, false
}

// Allows lists the calls that a permission allows, for a kind of token ("delegated" or "application"): as the least privileged way (least) or a
// higher privileged way. It gives the pages, and each page has its calls.
func (f *AllFile) Allows(permission, kind string) Uses {
	for name, u := range f.Permissions {
		if strings.EqualFold(name, permission) {
			if kind == "application" {
				return u.Application
			}
			return u.Delegated
		}
	}
	return Uses{}
}

// LoadAllFile reads operations/all.json. It refuses a field that it does not know.
func LoadAllFile(path string) (*AllFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f AllFile
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &f, nil
}
