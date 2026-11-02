package graph_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

// These are contract tests: a plain Graph client runs against the server. If the
// server answers in a shape that a tool does not expect, a test fails. The tests
// that run the cohort code against the server are in the cohort repo.

func start(t *testing.T, users, pageSize int) (*client, *graph.Server) {
	t.Helper()
	srv := graph.New(graph.Generate(graph.Options{Users: users, Seed: 7, Profile: graph.AcmeProfile()}), graph.ServerOptions{PageSize: pageSize})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return &client{BaseURL: ts.URL + graph.BasePath}, srv
}

// listUsers reads every user, and follows @odata.nextLink.
func listUsers(t *testing.T, c *client) []map[string]any {
	t.Helper()
	var all []map[string]any
	next := "/users?$select=id,department,jobTitle,officeLocation,userPrincipalName,onPremisesExtensionAttributes"
	for next != "" {
		var page struct {
			Value []map[string]any `json:"value"`
			Next  string           `json:"@odata.nextLink"`
		}
		if err := c.Get(context.Background(), next, &page); err != nil {
			t.Fatal(err)
		}
		all = append(all, page.Value...)
		next = ""
		if page.Next != "" {
			next = page.Next[len(c.BaseURL):]
		}
	}
	return all
}

func TestContract_Users(t *testing.T) {
	c, srv := start(t, 120, 50) // 3 pages
	users := listUsers(t, c)
	if len(users) != 120 {
		t.Fatalf("got %d users, want 120: paging must follow nextLink", len(users))
	}
	u := users[0]
	for _, p := range []string{"department", "jobTitle", "officeLocation", "userPrincipalName"} {
		if v, _ := u[p].(string); v == "" {
			t.Errorf("the property %s is empty: %+v", p, u)
		}
	}
	if ext, _ := u["onPremisesExtensionAttributes"].(map[string]any); ext["extensionAttribute1"] == nil {
		t.Errorf("extensionAttribute1 is empty: %+v", u)
	}

	// the world can change between two reads
	if len(srv.World().Snapshot().Departments) == 0 {
		t.Fatal("no departments")
	}
	n := srv.World().Rename("department", "Sales", "Sales Global", graph.Actor{Source: "admin"})
	if n == 0 {
		t.Fatal("nobody is in Sales")
	}
	got := 0
	for _, u := range listUsers(t, c) {
		if u["department"] == "Sales Global" {
			got++
		}
	}
	if got != n {
		t.Errorf("after the rename %d users are in Sales Global, want %d", got, n)
	}
}

func TestRefusesAdvancedQueryWithoutHeader(t *testing.T) {
	c, _ := start(t, 50, 0)
	var out any
	err := c.Get(context.Background(), "/groups?$search="+`"displayName:acme"`, &out) // no ConsistencyLevel
	var ge *graphError
	if err == nil || !errors.As(err, &ge) || ge.Status != 400 {
		t.Errorf("Graph refuses $search without the header, and the server must too: %v", err)
	}
}

func TestRefusesNoToken(t *testing.T) {
	c, _ := start(t, 10, 0)
	c.NoToken = true
	if err := c.Get(context.Background(), "/users", nil); err == nil {
		t.Error("a request with no token must fail")
	}
}

func TestSeedIsRepeatable(t *testing.T) {
	a := graph.Generate(graph.Options{Users: 80, Seed: 3}).Snapshot()
	b := graph.Generate(graph.Options{Users: 80, Seed: 3}).Snapshot()
	if len(a.Departments) != len(b.Departments) || a.Departments[0] != b.Departments[0] || a.Groups != b.Groups {
		t.Error("the same seed must make the same world")
	}
	if len(a.Departments) != len(graph.Departments) {
		t.Errorf("the world uses %d departments, the set has %d", len(a.Departments), len(graph.Departments))
	}
}
