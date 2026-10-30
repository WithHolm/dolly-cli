package graph_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

type apiAnswer struct {
	Endpoints []struct {
		graph.Endpoint
		Calls      int
		Errors     int
		LastStatus int
	}
	Unsupported []graph.UnsupportedCall
	Options     []graph.QueryOption
}

func (a apiAnswer) find(method, path string) (calls, errs int, ok bool) {
	for _, e := range a.Endpoints {
		if e.Method == method && e.Path == path {
			return e.Calls, e.Errors, true
		}
	}
	return 0, 0, false
}

func TestAPI_ListsWhatDollySupportsWithDescriptions(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	var a apiAnswer
	getJSON(t, ts.URL+"/_dolly/api", &a)

	if len(a.Endpoints) < 15 {
		t.Fatalf("%d endpoints", len(a.Endpoints))
	}
	seen := map[string]bool{}
	for _, e := range a.Endpoints {
		key := e.Method + " " + e.Path
		if seen[key] {
			t.Errorf("%s is listed twice", key)
		}
		seen[key] = true
		if e.Area == "" || e.Summary == "" || !strings.HasPrefix(e.Path, "/v1.0/") {
			t.Errorf("an endpoint needs an area, a summary, and a path under /v1.0: %+v", e.Endpoint)
		}
	}
	for _, want := range []string{
		"GET /v1.0/users", "GET /v1.0/groups/{id}", "GET /v1.0/groups/{id}/transitiveMembers/{type}/$count",
		"POST /v1.0/groups/{id}/members/$ref", "DELETE /v1.0/groups/{id}/members/{member}/$ref", "POST /v1.0/$batch",
		"GET /v1.0/servicePrincipals/{id}/appRoleAssignedTo",
	} {
		if !seen[want] {
			t.Errorf("%s must be in the list", want)
		}
	}
	// the writes are marked, so a person sees which operations change the data
	for _, e := range a.Endpoints {
		wantWrite := e.Method == "DELETE" || e.Method == "PATCH" || (e.Method == "POST" && !strings.HasSuffix(e.Path, "$batch"))
		if e.Writes != wantWrite {
			t.Errorf("%s %s: writes = %v", e.Method, e.Path, e.Writes)
		}
	}
	names := map[string]bool{}
	for _, o := range a.Options {
		names[o.Name] = true
	}
	for _, n := range []string{"$filter", "$search", "$count", "ConsistencyLevel"} {
		if !names[n] {
			t.Errorf("the option %s must be described", n)
		}
	}
}

// Every route in the table answers, and nothing answers that is not in the table.
func TestAPI_CountsTheCallsOfEachOperation(t *testing.T) {
	ts, c, _ := startServer(t, graph.ServerOptions{})
	ctx := context.Background()
	var out any
	for range 3 {
		if err := c.Get(ctx, "/groups?$top=1", &out); err != nil {
			t.Fatal(err)
		}
	}
	_ = c.Get(ctx, "/groups/00000000-0000-0000-0000-000000000001", &out) // a group that is not there: 404 counts as an error of this operation
	// two calls inside a $batch are counted for their own operations
	if _, err := c.Batch(ctx, []batchRequest{
		{ID: "a", Method: "GET", URL: "/users?$top=1"},
		{ID: "b", Method: "GET", URL: "/users?$top=1"},
	}); err != nil {
		t.Fatal(err)
	}

	var a apiAnswer
	getJSON(t, ts.URL+"/_dolly/api", &a)
	if calls, errs, _ := a.find("GET", "/v1.0/groups"); calls != 3 || errs != 0 {
		t.Errorf("groups: %d calls, %d errors", calls, errs)
	}
	if calls, errs, _ := a.find("GET", "/v1.0/groups/{id}"); calls != 1 || errs != 1 {
		t.Errorf("one group: %d calls, %d errors (a 404 is an error)", calls, errs)
	}
	if calls, _, _ := a.find("GET", "/v1.0/users"); calls != 2 {
		t.Errorf("users: %d calls, want 2 from the batch", calls)
	}
	if calls, _, _ := a.find("POST", "/v1.0/$batch"); calls != 1 {
		t.Errorf("batch: %d calls", calls)
	}
	if len(a.Unsupported) != 0 {
		t.Errorf("nothing that dolly does not know was called: %+v", a.Unsupported)
	}

	// the counts can be reset
	if s := admin(t, ts, "DELETE", "/stats", "", nil); s != 204 {
		t.Fatalf("reset: %d", s)
	}
	getJSON(t, ts.URL+"/_dolly/api", &a)
	if calls, _, _ := a.find("GET", "/v1.0/groups"); calls != 0 {
		t.Errorf("after the reset: %d calls", calls)
	}
}

// A tool that asks for something that dolly does not know is the thing that a
// person needs to see.
func TestAPI_ShowsWhatATooAskedForThatIsNotSupported(t *testing.T) {
	ts, c, _ := startServer(t, graph.ServerOptions{})
	ctx := context.Background()
	var out any
	for _, id := range []string{"11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222"} {
		_ = c.Get(ctx, "/devices/"+id, &out) // no such operation
	}
	_ = c.Get(ctx, "/subscribedSkus", &out)
	// a path that dolly knows, with a method that it does not
	req, _ := http.NewRequest("PUT", ts.URL+"/v1.0/groups", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer x")
	req.Header.Set("Content-Type", "application/json")
	if res, err := http.DefaultClient.Do(req); err == nil {
		res.Body.Close()
	}
	// a call with no token never gets that far, so it is not "not supported"
	noToken, _ := http.NewRequest("GET", ts.URL+"/v1.0/devices", nil)
	if res, err := http.DefaultClient.Do(noToken); err == nil {
		res.Body.Close()
	}

	var a apiAnswer
	getJSON(t, ts.URL+"/_dolly/api", &a)
	got := map[string]graph.UnsupportedCall{}
	for _, u := range a.Unsupported {
		got[u.Method+" "+u.Path] = u
	}
	if u := got["GET /v1.0/devices/{id}"]; u.Calls != 2 || u.Status != 404 {
		t.Errorf("devices: %+v (two calls with different IDs are one line)", u)
	}
	if u := got["GET /v1.0/subscribedSkus"]; u.Calls != 1 {
		t.Errorf("subscribedSkus: %+v", u)
	}
	if u := got["PUT /v1.0/groups"]; u.Calls != 1 || u.Status != 405 {
		t.Errorf("a known path with an unknown method: %+v", u)
	}
	if _, bad := got["GET /v1.0/devices"]; bad {
		t.Error("a call with no token is a 401, not an unsupported call")
	}
}
