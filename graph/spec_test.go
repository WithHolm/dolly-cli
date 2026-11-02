package graph_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
	"github.com/withholm/dolly-cli/graph/api"
	"github.com/withholm/dolly-cli/graph/spec"
)

// graphPaths gives the forms of a dolly path that Graph might have. Dolly has
// one route with a {type} segment where Graph has one path for each type.
func graphPaths(path string) []string {
	if !strings.Contains(path, "/{type}") {
		return []string{path}
	}
	var out []string
	for _, t := range []string{"microsoft.graph.user", "microsoft.graph.group", "microsoft.graph.servicePrincipal"} {
		out = append(out, strings.Replace(path, "/{type}", "/"+t, 1))
	}
	return out
}

// Every call that dolly answers is a call that Graph has. A typo in a path, or a
// call that Graph does not have, would make a tool work against dolly and fail in
// Entra.
func TestSpec_EveryRouteIsInGraph(t *testing.T) {
	s := spec.Load()
	srv := graph.New(graph.NewMemory(graph.Options{Users: 5, Seed: 1}), graph.ServerOptions{})
	endpoints, _ := srv.API()
	for _, e := range endpoints {
		if e.Area == "Batch" || e.Area == "Metadata" {
			continue // /$batch and /$metadata are in the root of Graph, not under a resource
		}
		// The description writes the type cast of a collection as
		// /members/{id}/graph.user. The Learn page "List group members" documents
		// /members/microsoft.graph.user, which is what dolly answers.
		if strings.Contains(e.Path, "/{type}") {
			continue
		}
		// The Learn page "Get place" documents GET /places/{id}. The description has the
		// get only for each type (/places/graph.roomList), and not this one.
		if e.Method == "GET" && spec.Normal(e.Path) == "/places/{}" {
			continue
		}
		// The Learn pages write delta query as GET /users/delta, and the description as /users/delta(). The real Graph answers both
		// (checked with the proxy of Graph Explorer), so dolly does too.
		if e.Method == "GET" && strings.HasSuffix(e.Path, "/delta") {
			e.Path += "()"
		}
		found := false
		for _, p := range graphPaths(e.Path) {
			found = found || s.Has(e.Method, p)
		}
		if !found {
			t.Errorf("%s %s is not in the Graph description", e.Method, e.Path)
		}
	}
}

// A call with no $select gets the default properties only, as in Graph. The list is
// from the Learn page "Get user", and the description must agree with it.
func TestSpec_DefaultUserProperties(t *testing.T) {
	props := spec.Load().Schemas["user"]
	for _, name := range api.DefaultUserProperties {
		if p, ok := props[name]; !ok || !p.Default {
			t.Errorf("%q is a default property in dolly, and the description says it needs $select: %+v", name, p)
		}
	}

	ts, _, _ := startServer(t, graph.ServerOptions{})
	_, list := graphCall(t, ts, "GET", "/users?$top=1", "")
	u := list["value"].([]any)[0].(map[string]any)
	if _, has := u["department"]; has {
		t.Errorf("department needs $select, and the list has it: %v", u)
	}
	if u["id"] == nil || u["userPrincipalName"] == nil {
		t.Errorf("id and the default properties must be there: %v", u)
	}
	_, one := graphCall(t, ts, "GET", "/users/"+u["id"].(string)+"?$select=department", "")
	if one["department"] == nil || one["displayName"] != nil {
		t.Errorf("$select gives what it names, and id: %v", one)
	}
}

// Coverage lists what Graph has under /users, /groups, and /servicePrincipals and
// dolly does not. It is not a failure: it is the list of what to build next. Run
// `go test -run Coverage -v ./internal/dolly` to read it.
func TestSpec_Coverage(t *testing.T) {
	s := spec.Load()
	srv := graph.New(graph.NewMemory(graph.Options{Users: 5, Seed: 1}), graph.ServerOptions{})
	endpoints, _ := srv.API()
	have := map[string]bool{}
	for _, e := range endpoints {
		for _, p := range graphPaths(e.Path) {
			have[e.Method+" "+spec.Normal(p)] = true
		}
	}

	var missing []string
	total, covered := 0, 0
	for _, o := range s.Operations {
		// the main operations only: a resource and its direct relationships
		if strings.Count(o.Path, "/") > 3 || strings.Contains(o.Path, "microsoft.graph.") {
			continue
		}
		total++
		if have[o.Method+" "+spec.Normal(o.Path)] {
			covered++
			continue
		}
		missing = append(missing, o.Method+" "+o.Path+"  ("+o.Summary+")")
	}
	sort.Strings(missing)
	t.Logf("dolly has %d of the %d main operations under /users, /groups, and /servicePrincipals", covered, total)
	if testing.Verbose() {
		for _, m := range missing {
			t.Log("  missing:", m)
		}
	}
}
