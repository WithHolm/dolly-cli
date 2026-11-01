package graph_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
	"github.com/withholm/dolly-cli/graph/permissions"
)

// Every route that dollycli serves has its page of Learn in graph/permissions/operations (or is said to have none). A new route brings a new page:
// run 'make update-operation-permissions', and commit the result. (This is a step of adding a route, see docs/development.md.)
func TestOperationPermissions_EveryServedRouteHasItsData(t *testing.T) {
	ops, err := permissions.LoadOperations()
	if err != nil {
		t.Fatal(err)
	}
	endpoints, _ := graph.New(graph.NewMemory(graph.Options{Users: 5, Seed: 1}), graph.ServerOptions{}).API()
	if len(endpoints) < 60 {
		t.Fatalf("only %d routes: the list of routes is wrong", len(endpoints))
	}
	for _, e := range endpoints {
		route := e.Method + " " + e.Path
		page, has := ops.Routes[route]
		switch {
		case has && ops.Pages[page].Table || has && page != "":
			if _, ok := ops.Pages[page]; !ok {
				t.Errorf("%s has the page %q, and the data has no such page", route, page)
			}
		case slices.Contains(ops.NoPage, route):
		default:
			t.Errorf("no permission data for %s. Run: make update-operation-permissions", route)
		}
	}
	for route := range ops.Routes {
		if !slices.ContainsFunc(endpoints, func(e graph.EndpointInfo) bool { return e.Method+" "+e.Path == route }) {
			t.Errorf("the data has the route %s, and dollycli does not serve it any more. Run: make update-operation-permissions", route)
		}
	}
}

// The full map of the calls of the description of Graph (operations/all.json): it is consistent with itself, and what it says about the calls that
// dollycli serves is what dollycli uses.
func TestOperationPermissions_TheFullMapIsConsistent(t *testing.T) {
	all, err := permissions.LoadAllFile("permissions/operations/all.json")
	if err != nil {
		t.Fatal(err)
	}
	ops, _ := permissions.LoadOperations()
	if all.Source.Commit != ops.Source.Commit {
		t.Errorf("all.json is from commit %s, and operations.json from %s: make them together", all.Source.Commit, ops.Source.Commit)
	}
	if len(all.Calls) < 5000 || len(all.Pages) < 500 || len(all.Permissions) < 200 {
		t.Fatalf("%d calls, %d pages, %d permissions", len(all.Calls), len(all.Pages), len(all.Permissions))
	}
	if all.Problems.Unreadable != nil || len(all.Problems.NotFound) > 0 || len(all.Problems.NoTable) > 0 {
		t.Errorf("problems in the file: %+v", all.Problems)
	}
	// every call points to a page that is in the file, and the page points back
	for call, ref := range all.Calls {
		if ref.Page == "" {
			if ref.Via != "" {
				t.Errorf("%s: no page, and the way is %q", call, ref.Via)
			}
			continue
		}
		p, ok := all.Pages[ref.Page]
		if !ok {
			t.Errorf("%s: the page %q is not in the file", call, ref.Page)
			continue
		}
		if !slices.Contains(p.Calls, call) {
			t.Errorf("%s: the page %q does not have it in its calls", call, ref.Page)
		}
		if !slices.Contains([]string{"verified", "matched", "spec-only"}, ref.Via) {
			t.Errorf("%s: the way is %q", call, ref.Via)
		}
	}
	// every permission of the index is in the page that it says, and every permission of a page is in the index
	uses := 0
	for name, u := range all.Permissions {
		for kind, us := range map[string]permissions.Uses{"delegated": u.Delegated, "application": u.Application} {
			for level, list := range map[string][]permissions.Use{"least": us.Least, "higher": us.Higher} {
				for _, use := range list {
					uses++
					p, ok := all.Pages[use.Page]
					if !ok {
						t.Fatalf("%s: the page %q is not in the file", name, use.Page)
					}
					if !slices.ContainsFunc(needsOf(p, use.Resource, use.Scenario, kind, level), func(alt string) bool { return strings.Contains(" "+alt+" ", " "+name+" ") }) {
						t.Errorf("%s (%s, %s): page %q does not say it", name, kind, level, use.Page)
					}
				}
			}
		}
	}
	if uses < 2000 {
		t.Errorf("only %d uses in the index", uses)
	}
	// the calls that dollycli serves: the page that dollycli uses is the page that the full map has for the same call of the description
	for call, ref := range all.Calls {
		if call == "GET /users" && (ref.Page != "user-list" || ref.Via != "verified") {
			t.Errorf("GET /users: %+v", ref)
		}
		if call == "GET /me/messages" && ref.Page != "user-list-messages" {
			t.Errorf("GET /me/messages must not be the page of the open extensions: %+v", ref)
		}
	}
}

// The lookups of the full map: by call (with any names for the parameters) and by permission.
func TestOperationPermissions_Lookups(t *testing.T) {
	all, err := permissions.LoadAllFile("permissions/operations/all.json")
	if err != nil {
		t.Fatal(err)
	}
	call, ref, page, ok := all.Lookup("get", "/users/{id}/memberOf")
	if !ok || call != "GET /users/{user-id}/memberOf" || ref.Page != "user-list-memberof" || !page.Table || len(page.Variants) != 2 {
		t.Fatalf("%q %+v %+v %v", call, ref, page, ok)
	}
	// the page has a table for the signed-in user and one for another user: the path of the call picks
	if _, app, ok := page.CallPermissions.Resolve("/users/{user-id}/memberOf"); !ok || !slices.Equal(app.Least, []string{"Directory.Read.All"}) {
		t.Errorf("another user, as an application: %+v %v", app, ok)
	}
	if _, app, ok := page.CallPermissions.Resolve("/me/memberOf"); !ok || !app.NotSupported {
		t.Errorf("the signed-in user, as an application: %+v %v", app, ok)
	}
	if _, _, _, ok := all.Lookup("GET", "/not/a/call"); ok {
		t.Error("a call that Graph does not have")
	}
	// a call that no page is about has no page and no permissions
	if _, ref, page, ok := all.Lookup("GET", "/groups/{id}/sites/{id}/termStore/sets/{id}/terms"); !ok || ref.Page != "" || page.Table {
		t.Errorf("%+v %+v %v", ref, page, ok)
	}
	// what a permission allows: Group.Read.All allows the list of groups as a least privileged way for an application? (it is a higher one: see the data)
	least := all.Allows("user.read.all", "application").Least
	if !slices.ContainsFunc(least, func(u permissions.Use) bool { return u.Page == "user-list" }) {
		t.Errorf("User.Read.All is the least privileged permission of an application for user-list: %v", least)
	}
	if got := all.Allows("No.Such.Permission", "delegated"); len(got.Least)+len(got.Higher) != 0 {
		t.Errorf("%+v", got)
	}
}

// needsOf gives the alternatives of a page for a kind and a level. When the call depends on the type of object, it gives those of all the rows of
// that type (a page can have more than one row for a type, for example with different paths).
func needsOf(p permissions.AllPage, resource, scenario, kind, level string) []string {
	pick := func(n permissions.Need) []string {
		if level == "least" {
			return n.Least
		}
		return n.Higher
	}
	if resource == "" && scenario == "" {
		if kind == "application" {
			return pick(p.Application)
		}
		return pick(p.Delegated)
	}
	var out []string
	for _, v := range p.Variants {
		if v.Resource != resource || v.Scenario != scenario {
			continue
		}
		if kind == "application" {
			out = append(out, pick(v.Application)...)
		} else {
			out = append(out, pick(v.Delegated)...)
		}
	}
	return out
}
