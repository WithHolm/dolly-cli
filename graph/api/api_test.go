package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/withholm/dolly-cli/graph/api"
)

// These tests use fake backends. A resource of the api package needs only the
// interface of its own file, not the data of dolly.

type testEnv struct{ page int }

func (e testEnv) PageSize() int                 { return e.page }
func (e testEnv) Actor(*http.Request) api.Actor { return api.Actor{Source: "test", Agent: "api_test"} }

// fakeUsers is a UsersBackend over a slice. It remembers who made the last change.
type fakeUsers struct {
	mu    sync.Mutex
	users []api.Object
	actor api.Actor
}

func (f *fakeUsers) ListUsers() []api.Object {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]api.Object(nil), f.users...)
}

func (f *fakeUsers) find(ref string) int {
	for i, u := range f.users {
		if u["id"] == ref || u["userPrincipalName"] == ref {
			return i
		}
	}
	return -1
}

func (f *fakeUsers) GetUser(ref string) (api.Object, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if i := f.find(ref); i >= 0 {
		return f.users[i], true
	}
	return nil, false
}

func (f *fakeUsers) CreateUser(props api.Object, a api.Actor) (api.Object, *api.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if props["userPrincipalName"] == nil {
		return nil, api.BadRequest("Property '%s' is required.", "userPrincipalName")
	}
	props["id"] = "id-" + props["userPrincipalName"].(string)
	f.users, f.actor = append(f.users, props), a
	return props, nil
}

func (f *fakeUsers) UpdateUser(ref string, props api.Object, a api.Actor) *api.Error {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.find(ref)
	if i < 0 {
		return api.NotFound("user", ref)
	}
	for k, v := range props {
		f.users[i][k] = v
	}
	f.actor = a
	return nil
}

func (f *fakeUsers) DeleteUser(ref string, a api.Actor) *api.Error {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.find(ref)
	if i < 0 {
		return api.NotFound("user", ref)
	}
	f.users = append(f.users[:i], f.users[i+1:]...)
	f.actor = a
	return nil
}

// The managers are tested with the real data (the dolly tests). This fake has none.
func (f *fakeUsers) Manager(ref string) (api.Object, *api.Error) {
	return nil, api.NotFound("manager", ref)
}
func (f *fakeUsers) DirectReports(string) ([]api.Object, bool) { return nil, true }
func (f *fakeUsers) UserMemberOf(ref string, _ bool) ([]api.Object, bool) {
	return nil, f.find(ref) >= 0
}
func (f *fakeUsers) UserAppRoleAssignments(ref string) ([]api.Object, bool) {
	return nil, f.find(ref) >= 0
}
func (f *fakeUsers) SetManager(string, string, api.Actor) *api.Error {
	return api.BadRequest("no managers in this fake")
}
func (f *fakeUsers) RemoveManager(ref string, _ api.Actor) *api.Error {
	return api.NotFound("manager", ref)
}

func mount(env api.Env, resources ...api.Resource) (*httptest.Server, *api.Router) {
	mux := http.NewServeMux()
	router := api.NewRouter(mux, "/v1.0", env)
	router.Mount(resources...)
	return httptest.NewServer(mux), router
}

func do(t *testing.T, ts *httptest.Server, method, path, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, ts.URL+"/v1.0"+path, strings.NewReader(body))
	req.Header.Set("ConsistencyLevel", "eventual")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return res.StatusCode, out
}

func TestUsers_WorkWithAFakeBackend(t *testing.T) {
	f := &fakeUsers{users: []api.Object{
		{"id": "1", "displayName": "Ada Lovelace", "userPrincipalName": "ada@x"},
		{"id": "2", "displayName": "Alan Turing", "userPrincipalName": "alan@x"},
	}}
	ts, _ := mount(testEnv{}, api.Users(f))
	defer ts.Close()

	// list, with $filter and $search and $select, and a page size
	code, out := do(t, ts, "GET", "/users?$filter=startswith(displayName,'al')&$select=displayName", "")
	v, _ := out["value"].([]any)
	if code != 200 || len(v) != 1 || v[0].(map[string]any)["displayName"] != "Alan Turing" {
		t.Errorf("filter: %d %+v", code, out)
	}
	if _, kept := v[0].(map[string]any)["userPrincipalName"]; kept {
		t.Error("$select must drop the other fields")
	}
	if _, out = do(t, ts, "GET", "/users?$search=%22displayName:lovelace%22&$count=true", ""); out["@odata.count"] != float64(1) {
		t.Errorf("search and count: %+v", out)
	}
	if code, _ = do(t, ts, "GET", "/users?$filter=mail%20ne%20null", ""); code != 400 {
		t.Errorf("ne needs an advanced query, so Graph answers 400 without one: %d", code)
	}
	ts2, _ := mount(testEnv{page: 1}, api.Users(f))
	defer ts2.Close()
	if _, out = do(t, ts2, "GET", "/users?$top=100", ""); out["@odata.nextLink"] == nil || len(out["value"].([]any)) != 1 {
		t.Errorf("the page size of the env wins over $top: %+v", out)
	}

	// get by ID or by name, 404
	if code, got := do(t, ts, "GET", "/users/alan@x", ""); code != 200 || got["id"] != "2" {
		t.Errorf("get: %d %+v", code, got)
	}
	if code, got := do(t, ts, "GET", "/users/nobody", ""); code != 404 || got["error"].(map[string]any)["code"] != "Request_ResourceNotFound" {
		t.Errorf("404: %d %+v", code, got)
	}

	// create, update, delete: the backend gets the actor that the env gives
	if code, _ := do(t, ts, "POST", "/users", `{"userPrincipalName":"new@x","displayName":"New"}`); code != 201 {
		t.Errorf("create: %d", code)
	}
	if f.actor.Agent != "api_test" {
		t.Errorf("the backend must get the actor of the env: %+v", f.actor)
	}
	if code, out := do(t, ts, "POST", "/users", `{"displayName":"No name"}`); code != 400 || !strings.Contains(out["error"].(map[string]any)["message"].(string), "userPrincipalName") {
		t.Errorf("create with a problem: %d %+v", code, out)
	}
	if code, _ := do(t, ts, "POST", "/users", `[1]`); code != 400 {
		t.Errorf("a body that is not an object: %d", code)
	}
	if code, _ := do(t, ts, "PATCH", "/users/new@x", `{"department":"Sales"}`); code != 204 {
		t.Errorf("patch: %d", code)
	}
	if got, _ := f.GetUser("new@x"); got["department"] != "Sales" {
		t.Errorf("the change must reach the backend: %+v", got)
	}
	if code, _ := do(t, ts, "DELETE", "/users/new@x", ""); code != 204 {
		t.Errorf("delete: %d", code)
	}
	if code, _ := do(t, ts, "DELETE", "/users/new@x", ""); code != 404 {
		t.Errorf("a second delete: %d", code)
	}
}

// The router keeps the list of operations that the page shows.
func TestRouter_KeepsTheListOfOperations(t *testing.T) {
	ts, router := mount(testEnv{}, api.Users(&fakeUsers{}))
	defer ts.Close()

	got := map[string]api.Endpoint{}
	for _, e := range router.Endpoints() {
		got[e.Method+" "+e.Path] = e
	}
	for key, writes := range map[string]bool{
		"GET /v1.0/users": false, "GET /v1.0/users/{id}": false,
		"POST /v1.0/users": true, "PATCH /v1.0/users/{id}": true, "DELETE /v1.0/users/{id}": true,
	} {
		e, ok := got[key]
		if !ok {
			t.Errorf("%s must be listed", key)
			continue
		}
		if e.Area != "Users" || e.Summary == "" || e.Writes != writes {
			t.Errorf("%s: %+v", key, e)
		}
	}
	// the manager has its own area
	for _, key := range []string{
		"GET /v1.0/users/{id}/manager", "PUT /v1.0/users/{id}/manager/$ref",
		"DELETE /v1.0/users/{id}/manager/$ref", "GET /v1.0/users/{id}/directReports",
	} {
		if e, ok := got[key]; !ok || e.Area != "Manager" {
			t.Errorf("%s must be listed in the area Manager: %+v", key, e)
		}
	}
	// the 10 of users and manager, 1 for directReports/$count, and 7 for memberOf, transitiveMemberOf (3 each), and appRoleAssignments
	if len(got) != 18 {
		t.Errorf("%d operations, want 18", len(got))
	}
}

// A scope below another has its own area, and joins the path.
func TestScope_JoinsPathsAndKeepsItsOwnArea(t *testing.T) {
	mux := http.NewServeMux()
	router := api.NewRouter(mux, "/v1.0", testEnv{})
	groups := router.Scope("/groups", "Groups")
	members := groups.Scope("/{id}/members", "Members")
	members.Handle("GET", "", "List.", nil, func(w http.ResponseWriter, r *http.Request) {
		api.WriteJSON(w, 200, map[string]string{"id": r.PathValue("id")})
	})
	members.Handle("DELETE", "/{member}/$ref", "Remove.", nil, func(w http.ResponseWriter, _ *http.Request) { api.NoContent(w) })

	eps := router.Endpoints()
	if len(eps) != 2 || eps[0].Path != "/v1.0/groups/{id}/members" || eps[0].Area != "Members" || eps[1].Path != "/v1.0/groups/{id}/members/{member}/$ref" || !eps[1].Writes {
		t.Fatalf("endpoints: %+v", eps)
	}
	ts := httptest.NewServer(mux)
	defer ts.Close()
	if code, out := do(t, ts, "GET", "/groups/g1/members", ""); code != 200 || out["id"] != "g1" {
		t.Errorf("the path value reaches the handler: %d %+v", code, out)
	}
}

// What Graph does with a directory object when the header ConsistencyLevel: eventual is missing:
// $search is an error, and $count=true is left out without an error.
func TestListQuery_WithoutTheConsistencyHeader(t *testing.T) {
	ts, _ := mount(testEnv{}, api.Users(&fakeUsers{users: []api.Object{{"id": "1", "displayName": "A"}}}))
	defer ts.Close()
	get := func(path string, eventual bool) (int, string) {
		req, _ := http.NewRequest("GET", ts.URL+"/v1.0"+path, nil)
		if eventual {
			req.Header.Set("ConsistencyLevel", "eventual")
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}

	code, body := get(`/users?$search="displayName:a"`, false)
	if code != 400 || !strings.Contains(body, "Request_UnsupportedQuery") || !strings.Contains(body, "Request with $search query parameter only works through MSGraph with a special request header: 'ConsistencyLevel: eventual'") {
		t.Errorf("$search without the header: %d %s", code, body)
	}
	if code, body = get("/users?$count=true", false); code != 200 || strings.Contains(body, "@odata.count") {
		t.Errorf("$count=true without the header is left out, with no error: %d %s", code, body)
	}
	if code, body = get("/users?$count=true", true); code != 200 || !strings.Contains(body, `"@odata.count":1`) {
		t.Errorf("$count=true with the header: %d %s", code, body)
	}
}
