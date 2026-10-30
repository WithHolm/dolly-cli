package graph_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

// graphCall sends one request to the Graph API of a test server, as a tool would.
func graphCall(t *testing.T, ts *httptest.Server, method, path, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, ts.URL+"/v1.0"+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer x")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "objects-test/1")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func errText(out map[string]any) string {
	e, _ := out["error"].(map[string]any)
	s, _ := e["message"].(string)
	return s
}

const newUser = `{"accountEnabled":true,"displayName":"Ada Test","mailNickname":"ada.test","userPrincipalName":"ada.test@acme.example",
 "department":"Sales","jobTitle":"Tester","passwordProfile":{"password":"x","forceChangePasswordNextSignIn":false},
 "onPremisesExtensionAttributes":{"extensionAttribute1":"CC-1"}}`

func TestUsers_CreateGetUpdate(t *testing.T) {
	ts, _, srv := startServer(t, graph.ServerOptions{})
	before := srv.World().Snapshot().Users

	code, u := graphCall(t, ts, "POST", "/users", newUser)
	if code != 201 || u["id"] == "" || u["displayName"] != "Ada Test" || u["userType"] != "Member" {
		t.Fatalf("create: %d %+v", code, u)
	}
	if _, kept := u["passwordProfile"]; kept {
		t.Error("dolly keeps no passwords")
	}
	id := u["id"].(string)
	if srv.World().Snapshot().Users != before+1 {
		t.Error("the user must be in the data")
	}

	// read it back by ID and by user principal name
	for _, ref := range []string{id, "ADA.TEST@acme.example"} {
		if c, got := graphCall(t, ts, "GET", "/users/"+ref+"?$select=displayName,department", ""); c != 200 || got["department"] != "Sales" {
			t.Errorf("get %s: %d %+v", ref, c, got)
		}
	}
	// and it is in the list that a blueprint reads
	_, list := graphCall(t, ts, "GET", "/users?$filter="+url.QueryEscape(`startswith(displayName,'Ada Test')`), "")
	if v, _ := list["value"].([]any); len(v) != 1 {
		t.Errorf("filter: %+v", list)
	}

	// update by ID: a change, a clear with null, and a merge of the extension attributes
	code, _ = graphCall(t, ts, "PATCH", "/users/"+id, `{"department":"Sales-Global","jobTitle":null,"onPremisesExtensionAttributes":{"extensionAttribute2":"Contractor"}}`)
	if code != 204 {
		t.Fatalf("patch: %d", code)
	}
	_, got := graphCall(t, ts, "GET", "/users/"+id+"?$select=department,jobTitle,onPremisesExtensionAttributes", "")
	ext, _ := got["onPremisesExtensionAttributes"].(map[string]any)
	if got["department"] != "Sales-Global" || got["jobTitle"] != nil || ext["extensionAttribute1"] != "CC-1" || ext["extensionAttribute2"] != "Contractor" {
		t.Errorf("after the patch: %+v", got)
	}
	// update by user principal name, and the name can change
	if code, _ = graphCall(t, ts, "PATCH", "/users/ada.test@acme.example", `{"userPrincipalName":"ada.new@acme.example"}`); code != 204 {
		t.Errorf("patch by UPN: %d", code)
	}
	if c, _ := graphCall(t, ts, "GET", "/users/ada.new@acme.example", ""); c != 200 {
		t.Errorf("the new user principal name must work: %d", c)
	}
}

func TestUsers_ValidationLikeGraph(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	graphCall(t, ts, "POST", "/users", newUser)

	cases := []struct{ name, body, want string }{
		{"no display name", `{"accountEnabled":true,"mailNickname":"a","userPrincipalName":"a@x","passwordProfile":{}}`, "'displayName' is required"},
		{"no password profile", `{"accountEnabled":true,"displayName":"A","mailNickname":"a","userPrincipalName":"a@x"}`, "'passwordProfile' is required"},
		{"no accountEnabled", `{"displayName":"A","mailNickname":"a","userPrincipalName":"a@x","passwordProfile":{}}`, "'accountEnabled' is required"},
		{"a property that does not exist", `{"accountEnabled":true,"displayName":"A","mailNickname":"a","userPrincipalName":"a@x","passwordProfile":{},"favouriteColour":"red"}`, "'favouriteColour' does not exist"},
		{"a value of the wrong type", `{"accountEnabled":"yes","displayName":"A","mailNickname":"a","userPrincipalName":"a@x","passwordProfile":{}}`, "Invalid value specified for property 'accountEnabled'"},
		{"a user principal name that is taken", `{"accountEnabled":true,"displayName":"B","mailNickname":"b","userPrincipalName":"ADA.TEST@acme.example","passwordProfile":{}}`, "same value for property userPrincipalName"},
		{"not an object", `[1]`, "must be a JSON object"},
	}
	for _, c := range cases {
		code, out := graphCall(t, ts, "POST", "/users", c.body)
		if code != 400 || !strings.Contains(errText(out), c.want) {
			t.Errorf("%s: %d %q, want 400 with %q", c.name, code, errText(out), c.want)
		}
	}
	// update
	if code, out := graphCall(t, ts, "PATCH", "/users/ada.test@acme.example", `{"id":"x"}`); code != 400 || !strings.Contains(errText(out), "read-only") {
		t.Errorf("id is read-only: %d %q", code, errText(out))
	}
	if code, _ := graphCall(t, ts, "PATCH", "/users/nobody@acme.example", `{"department":"X"}`); code != 404 {
		t.Errorf("an unknown user: %d", code)
	}
	// a taken name, also on update (another user has it)
	_, other := graphCall(t, ts, "GET", "/users?$top=1&$select=userPrincipalName", "")
	taken := other["value"].([]any)[0].(map[string]any)["userPrincipalName"].(string)
	if code, out := graphCall(t, ts, "PATCH", "/users/ada.test@acme.example", `{"userPrincipalName":"`+taken+`"}`); code != 400 || !strings.Contains(errText(out), "userPrincipalName") {
		t.Errorf("a taken name on update: %d %q", code, errText(out))
	}
	// the same name for the same user is fine
	if code, _ := graphCall(t, ts, "PATCH", "/users/ada.test@acme.example", `{"userPrincipalName":"ada.test@acme.example"}`); code != 204 {
		t.Errorf("the same name for the same user: %d", code)
	}
}

func TestGroups_CreateAndUpdate(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	_, user := graphCall(t, ts, "POST", "/users", newUser)
	uid := user["id"].(string)

	code, g := graphCall(t, ts, "POST", "/groups", `{"displayName":"grp.test.one","mailEnabled":false,"mailNickname":"grp-test-one","securityEnabled":true,
	 "description":"for tests","groupTypes":[],"members@odata.bind":["https://graph.microsoft.com/v1.0/directoryObjects/`+uid+`"]}`)
	if code != 201 || g["displayName"] != "grp.test.one" || g["securityEnabled"] != true || g["description"] != "for tests" {
		t.Fatalf("create: %d %+v", code, g)
	}
	gid := g["id"].(string)
	_, members := graphCall(t, ts, "GET", "/groups/"+gid+"/members", "")
	if v, _ := members["value"].([]any); len(v) != 1 {
		t.Errorf("members@odata.bind must put the user in: %+v", members)
	}
	// two groups can have the same name, as in Entra
	if code, _ = graphCall(t, ts, "POST", "/groups", `{"displayName":"grp.test.one","mailEnabled":false,"mailNickname":"x2","securityEnabled":true}`); code != 201 {
		t.Errorf("the same display name: %d", code)
	}

	if code, _ = graphCall(t, ts, "PATCH", "/groups/"+gid, `{"displayName":"grp.test.renamed","description":"new"}`); code != 204 {
		t.Fatalf("patch: %d", code)
	}
	_, got := graphCall(t, ts, "GET", "/groups/"+gid, "")
	if got["displayName"] != "grp.test.renamed" || got["description"] != "new" {
		t.Errorf("after the patch: %+v", got)
	}
	// it is in the list by its new name
	_, list := graphCall(t, ts, "GET", "/groups?$filter="+url.QueryEscape(`displayName eq 'grp.test.renamed'`), "")
	if v, _ := list["value"].([]any); len(v) != 1 {
		t.Errorf("filter: %+v", list)
	}
}

func TestGroups_KindsAndTheirRules(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	post := func(body string) (int, map[string]any) { return graphCall(t, ts, "POST", "/groups", body) }

	cases := []struct{ name, body, want string }{
		{"no display name", `{"mailEnabled":false,"mailNickname":"a","securityEnabled":true}`, "'displayName' is required"},
		{"no securityEnabled", `{"displayName":"a","mailEnabled":false,"mailNickname":"a"}`, "'securityEnabled' is required"},
		{"dynamic with no rule", `{"displayName":"a","mailEnabled":false,"mailNickname":"a","securityEnabled":true,"groupTypes":["DynamicMembership"]}`, "needs the property 'membershipRule'"},
		{"a rule on a group that is not dynamic", `{"displayName":"a","mailEnabled":false,"mailNickname":"a","securityEnabled":true,"membershipRule":"x"}`, "only for a group with dynamic membership"},
		{"role-assignable and dynamic", `{"displayName":"a","mailEnabled":false,"mailNickname":"a","securityEnabled":true,"isAssignableToRole":true,"groupTypes":["DynamicMembership"],"membershipRule":"x"}`, "cannot have dynamic membership"},
		{"role-assignable, not security", `{"displayName":"a","mailEnabled":true,"mailNickname":"a","securityEnabled":false,"isAssignableToRole":true}`, "must be a security group"},
		{"a group type that does not exist", `{"displayName":"a","mailEnabled":false,"mailNickname":"a","securityEnabled":true,"groupTypes":["Fancy"]}`, "groupTypes"},
		{"a member that does not exist", `{"displayName":"a","mailEnabled":false,"mailNickname":"a","securityEnabled":true,"members@odata.bind":["https://graph.microsoft.com/v1.0/directoryObjects/00000000-0000-0000-0000-00000000dead"]}`, "does not exist"},
		{"an unknown property", `{"displayName":"a","mailEnabled":false,"mailNickname":"a","securityEnabled":true,"colour":"red"}`, "'colour' does not exist"},
	}
	for _, c := range cases {
		if code, out := post(c.body); code < 400 || !strings.Contains(errText(out), c.want) {
			t.Errorf("%s: %d %q, want an error with %q", c.name, code, errText(out), c.want)
		}
	}

	// a Microsoft 365 group cannot hold a group, also when the group is made with members
	_, sec := post(`{"displayName":"sec","mailEnabled":false,"mailNickname":"sec","securityEnabled":true}`)
	if code, out := post(`{"displayName":"m365","mailEnabled":true,"mailNickname":"m365","securityEnabled":false,"groupTypes":["Unified"],"members@odata.bind":["https://graph.microsoft.com/v1.0/groups/` + sec["id"].(string) + `"]}`); code != 400 || !strings.Contains(errText(out), "Microsoft 365 group cannot hold a group") {
		t.Errorf("m365 with a group member: %d %q", code, errText(out))
	}

	// a dynamic group: made, and it refuses member calls, as Entra does
	code, dyn := post(`{"displayName":"dyn","mailEnabled":false,"mailNickname":"dyn","securityEnabled":true,"groupTypes":["DynamicMembership"],"membershipRule":"(user.department -eq \"Sales\")"}`)
	if code != 201 {
		t.Fatalf("dynamic: %d %+v", code, dyn)
	}
	if types, _ := dyn["groupTypes"].([]any); len(types) != 1 || types[0] != "DynamicMembership" || dyn["membershipRule"] == nil {
		t.Errorf("dynamic group: %+v", dyn)
	}
	if code, _ := graphCall(t, ts, "POST", "/groups/"+dyn["id"].(string)+"/members/$ref", `{"@odata.id":"x/`+sec["id"].(string)+`"}`); code != 400 {
		t.Errorf("a dynamic group refuses members: %d", code)
	}
	// the rule can change on a dynamic group only
	if code, _ := graphCall(t, ts, "PATCH", "/groups/"+dyn["id"].(string), `{"membershipRule":"(user.department -eq \"IT\")"}`); code != 204 {
		t.Errorf("a new rule on a dynamic group: %d", code)
	}
	if code, out := graphCall(t, ts, "PATCH", "/groups/"+sec["id"].(string), `{"membershipRule":"x"}`); code != 400 || !strings.Contains(errText(out), "dynamic") {
		t.Errorf("a rule on a group that is not dynamic: %d %q", code, errText(out))
	}
	// what cannot change after the group is made
	for _, body := range []string{`{"groupTypes":["Unified"]}`, `{"isAssignableToRole":true}`} {
		if code, out := graphCall(t, ts, "PATCH", "/groups/"+sec["id"].(string), body); code != 400 || !strings.Contains(errText(out), "cannot be changed") {
			t.Errorf("%s: %d %q", body, code, errText(out))
		}
	}
	if code, _ := graphCall(t, ts, "PATCH", "/groups/00000000-0000-0000-0000-00000000dead", `{"description":"x"}`); code != 404 {
		t.Errorf("an unknown group: %d", code)
	}
}

// What a tool makes is in the history with the tool's name, and a restart and a
// rollback give the same data, with the same IDs.
func TestCreateUpdate_AreInTheHistory_SurviveARestart_AndRollBack(t *testing.T) {
	dir := t.TempDir()
	w := openWorld(t, dir, nil)
	ts := httptest.NewServer(graph.New(w, graph.ServerOptions{}).Handler())

	_, u := graphCall(t, ts, "POST", "/users", newUser)
	uid := u["id"].(string)
	_, g := graphCall(t, ts, "POST", "/groups", `{"displayName":"grp.keep","mailEnabled":false,"mailNickname":"keep","securityEnabled":true,"members@odata.bind":["https://graph.microsoft.com/v1.0/users/`+uid+`"]}`)
	gid := g["id"].(string)
	graphCall(t, ts, "PATCH", "/users/"+uid, `{"department":"Moved","onPremisesExtensionAttributes":{"extensionAttribute3":"x"}}`)
	graphCall(t, ts, "PATCH", "/groups/"+gid, `{"description":"changed"}`)
	ts.Close()

	hist := w.History()
	var texts []string
	for _, e := range hist {
		texts = append(texts, e.Text())
		if e.Actor.Source != "graph" || !strings.HasPrefix(e.Actor.Agent, "objects-test") {
			t.Errorf("a change by a tool names the tool: %+v", e.Actor)
		}
	}
	for i, want := range []string{"Made the user ada.test@acme.example (Ada Test)", "Made the group grp.keep", "Changed ada.test@acme.example: department = Moved", "Changed the group grp.keep: description = changed"} {
		if i >= len(texts) || !strings.Contains(strings.Join(texts[len(texts)-4:], "\n"), want) {
			t.Errorf("history %d: want %q in %q", i, want, texts)
		}
	}

	// restart: the same user, with the same ID, and the group with its member
	_ = w.Close()
	again := openWorld(t, dir, nil)
	srv2 := httptest.NewServer(graph.New(again, graph.ServerOptions{}).Handler())
	defer srv2.Close()
	_, got := graphCall(t, srv2, "GET", "/users/"+uid+"?$select=department,onPremisesExtensionAttributes", "")
	ext, _ := got["onPremisesExtensionAttributes"].(map[string]any)
	if got["department"] != "Moved" || ext["extensionAttribute1"] != "CC-1" || ext["extensionAttribute3"] != "x" {
		t.Errorf("after a restart: %+v", got)
	}
	_, grp := graphCall(t, srv2, "GET", "/groups/"+gid, "")
	_, members := graphCall(t, srv2, "GET", "/groups/"+gid+"/members", "")
	if grp["description"] != "changed" || len(members["value"].([]any)) != 1 {
		t.Errorf("group after a restart: %+v %+v", grp, members)
	}

	// rollback to before the group was made: the user stays, the group is gone
	made := 0
	for _, e := range again.History() {
		if e.Type == "group.create" {
			made = e.Seq
		}
	}
	if err := again.Rollback(made-1, person); err != nil {
		t.Fatal(err)
	}
	if c, _ := graphCall(t, srv2, "GET", "/groups/"+gid, ""); c != 404 {
		t.Errorf("the group must be gone after the rollback: %d", c)
	}
	if c, _ := graphCall(t, srv2, "GET", "/users/"+uid, ""); c != 200 {
		t.Errorf("the user must stay: %d", c)
	}
	// and back again: the same group, with the same ID
	if err := again.Rollback(len(again.History())-1-0, person); err != nil { // before the rollback itself
		t.Fatal(err)
	}
	if c, _ := graphCall(t, srv2, "GET", "/groups/"+gid, ""); c != 200 {
		t.Errorf("going back past the rollback must bring the group back with its ID: %d", c)
	}
}
