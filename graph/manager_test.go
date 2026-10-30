package graph_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/withholm/dolly-cli/graph"
)

// managerMap reads every user with $expand=manager, and gives user ID to manager ID.
func managerMap(t *testing.T, call func(method, path, body string) (int, map[string]any)) (map[string]string, []string) {
	t.Helper()
	code, out := call("GET", "/users?$top=999&$select=id,userType&$expand=manager($select=id)", "")
	if code != 200 {
		t.Fatalf("list with expand: %d %v", code, out)
	}
	m := map[string]string{}
	var all []string
	for _, v := range out["value"].([]any) {
		u := v.(map[string]any)
		all = append(all, u["id"].(string))
		if mgr, ok := u["manager"].(map[string]any); ok {
			m[u["id"].(string)] = mgr["id"].(string)
		}
	}
	return m, all
}

func TestManager_SeedIsATree(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	call := func(method, path, body string) (int, map[string]any) { return graphCall(t, ts, method, path, body) }
	m, all := managerMap(t, call)

	if len(m) < len(all)/2 {
		t.Fatalf("only %d of %d users have a manager", len(m), len(all))
	}
	roots := 0
	for _, id := range all {
		seen := map[string]bool{}
		for cur := id; ; {
			if seen[cur] {
				t.Fatalf("a loop in the managers at %s", cur)
			}
			seen[cur] = true
			next, ok := m[cur]
			if !ok {
				break
			}
			cur = next
		}
		if _, ok := m[id]; !ok {
			roots++
		}
	}
	// the head of the company, and the guests (a guest has no manager)
	if roots < 1 {
		t.Error("nobody is at the top")
	}
}

func TestManager_Calls(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	call := func(method, path, body string) (int, map[string]any) { return graphCall(t, ts, method, path, body) }
	_, a := call("POST", "/users", newUser)
	_, b := call("POST", "/users", `{"accountEnabled":true,"displayName":"Bo Test","mailNickname":"bo.test","userPrincipalName":"bo.test@acme.example","passwordProfile":{"password":"x"}}`)
	aid, bid := a["id"].(string), b["id"].(string)
	ref := func(id string) string { return `{"@odata.id":"https://graph.microsoft.com/v1.0/users/` + id + `"}` }

	if code, _ := call("GET", "/users/"+aid+"/manager", ""); code != 404 {
		t.Errorf("a new user has no manager: %d", code)
	}
	if code, _ := call("PUT", "/users/"+aid+"/manager/$ref", ref(bid)); code != 204 {
		t.Fatalf("set manager: %d", code)
	}
	code, mgr := call("GET", "/users/"+aid+"/manager", "")
	if code != 200 || mgr["id"] != bid || mgr["@odata.type"] != "#microsoft.graph.user" {
		t.Errorf("manager: %d %v", code, mgr)
	}
	_, reports := call("GET", "/users/"+bid+"/directReports", "")
	if r := ids(reports); len(r) != 1 || r[0] != aid {
		t.Errorf("direct reports: %v", r)
	}
	_, expanded := call("GET", "/users/"+aid+"?$expand=manager($select=displayName)", "")
	if m, _ := expanded["manager"].(map[string]any); m["displayName"] != "Bo Test" {
		t.Errorf("$expand: %v", expanded["manager"])
	}

	// the rules
	if code, out := call("PUT", "/users/"+bid+"/manager/$ref", ref(aid)); code != 400 {
		t.Errorf("a loop: %d %v", code, out)
	}
	if code, _ := call("PUT", "/users/"+aid+"/manager/$ref", ref(aid)); code != 400 {
		t.Errorf("a user as their own manager: %d", code)
	}
	if code, _ := call("PUT", "/users/"+aid+"/manager/$ref", ref("00000000-0000-0000-0000-000000000000")); code != 404 {
		t.Errorf("an unknown manager: %d", code)
	}
	if code, _ := call("GET", "/users/"+aid+"?$expand=memberOf", ""); code != 400 {
		t.Errorf("a relation that is not expanded must give 400, got %d", code)
	}

	// a deleted manager leaves the reports with none
	if code, _ := call("DELETE", "/users/"+bid, ""); code != 204 {
		t.Fatalf("delete the manager: %d", code)
	}
	if code, _ := call("GET", "/users/"+aid+"/manager", ""); code != 404 {
		t.Errorf("after the manager is deleted: %d", code)
	}

	if code, _ := call("PUT", "/users/"+aid+"/manager/$ref", ref(bid)); code != 404 {
		t.Errorf("a deleted user as manager: %d", code)
	}
	if code, _ := call("DELETE", "/users/"+aid+"/manager/$ref", ""); code != 404 {
		t.Errorf("remove when there is none: %d", code)
	}
}

// $levels gives the chain of managers, each inside the one below it (the doc
// "List manager", example 2). It needs ConsistencyLevel: eventual, and it works on
// one user, not on the list.
func TestManager_ExpandLevels(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	call := func(method, path, body string) (int, map[string]any) { return graphCall(t, ts, method, path, body) }
	mk := func(n string) string {
		_, u := call("POST", "/users", `{"accountEnabled":true,"displayName":"`+n+`","mailNickname":"`+n+`","userPrincipalName":"`+n+`@acme.example","passwordProfile":{"password":"x"}}`)
		return u["id"].(string)
	}
	ref := func(id string) string { return `{"@odata.id":"https://graph.microsoft.com/v1.0/users/` + id + `"}` }
	a, b, c := mk("lvl.a"), mk("lvl.b"), mk("lvl.c")
	for _, link := range [][2]string{{a, b}, {b, c}} {
		if code, out := call("PUT", "/users/"+link[0]+"/manager/$ref", ref(link[1])); code != 204 {
			t.Fatalf("set manager: %d %v", code, out)
		}
	}

	get := func(path string, eventual bool) (int, map[string]any) {
		req, _ := http.NewRequest("GET", ts.URL+"/v1.0"+path, nil)
		req.Header.Set("Authorization", "Bearer x")
		if eventual {
			req.Header.Set("ConsistencyLevel", "eventual")
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}
	one := "/users/" + a + "?$select=id,displayName&$expand=manager"

	code, out := get(one+"($levels=max;$select=id,displayName)", true)
	m1, _ := out["manager"].(map[string]any)
	m2, _ := m1["manager"].(map[string]any)
	if code != 200 || m1["displayName"] != "lvl.b" || m2["displayName"] != "lvl.c" || m2["manager"] != nil {
		t.Errorf("max: %d %v", code, out)
	}
	_, out = get(one+"($levels=1)", true)
	if m1, _ := out["manager"].(map[string]any); m1["id"] != b || m1["manager"] != nil {
		t.Errorf("levels=1 must give the manager only: %v", out)
	}
	if code, _ = get(one+"($levels=max)", false); code != 400 {
		t.Errorf("$levels without ConsistencyLevel: %d", code)
	}
	if code, _ = get(one+"($levels=0)", true); code != 400 {
		t.Errorf("$levels=0: %d", code)
	}
	if code, _ = get("/users?$top=2&$expand=manager($levels=2)", true); code != 400 {
		t.Errorf("$levels on the list: %d", code)
	}
	if code, _ = get(one+"($foo=1)", true); code != 400 {
		t.Errorf("an unknown option: %d", code)
	}
}

// A manager that was set is in the history, so it comes back after a restart.
func TestManager_KeptAndReplayed(t *testing.T) {
	dir := t.TempDir()
	opt := graph.OpenOptions{Users: 20, Seed: 3, Now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }}
	w, err := graph.Open(dir, opt)
	if err != nil {
		t.Fatal(err)
	}
	// two new users: no manager yet, and no one above them
	var ids []string
	for _, n := range []string{"a", "b"} {
		u, e := w.CreateUser(map[string]any{"accountEnabled": true, "displayName": n, "mailNickname": n, "userPrincipalName": n + "@acme.example",
			"passwordProfile": map[string]any{"password": "x"}}, graph.Actor{})
		if e != nil {
			t.Fatal(e)
		}
		ids = append(ids, u["id"].(string))
	}
	a, b := ids[0], ids[1]
	if e := w.SetManager(a, b, graph.Actor{}); e != nil {
		t.Fatal(e)
	}
	got, _ := w.Manager(a)
	_ = w.Close()

	w2, err := graph.Open(dir, opt)
	if err != nil {
		t.Fatal(err)
	}
	defer w2.Close()
	again, e := w2.Manager(a)
	if (got == nil) != (e != nil) || (got != nil && again["id"] != got["id"]) {
		t.Errorf("before the restart %v, after %v (%v)", got, again, e)
	}
	if h := w2.History(); len(h) == 0 || h[len(h)-1].Text() == "" {
		t.Errorf("the change is not in the history: %v", h)
	}
}
