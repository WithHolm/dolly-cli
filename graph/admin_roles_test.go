package graph_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

// The portal edits the app roles of an app registration, and assigns them to users, groups, and service principals, through the admin API. The admin routes
// make the same changes as the Graph API, with the same rules, and the history names the actor "admin".

func adminRaw(t *testing.T, base, method, path, payload string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, base+"/_dolly"+path, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

type rolesAnswer struct {
	Roles []map[string]any `json:"roles"`
}

func TestAdminAppRoles_AddEditDisableRemove(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	appID, appObj, _ := newRegisteredApp(t, ts, role(roleUser, "Reader", "User"))

	var got rolesAnswer
	if s := admin(t, ts, "GET", "/applications/"+appObj+"/app-roles", "", &got); s != 200 || len(got.Roles) != 1 || got.Roles[0]["value"] != "Reader" {
		t.Fatalf("list: %d %+v", s, got)
	}
	// the app can be addressed by its client ID too, and a new role gets an id
	var made map[string]any
	if s := admin(t, ts, "POST", "/applications/"+appID+"/app-roles", `{"displayName":"Editor","value":"Editor","description":"Edits","allowedMemberTypes":["User"]}`, &made); s != 201 {
		t.Fatalf("add: %d", s)
	}
	id, _ := made["id"].(string)
	if !graphIsGUID(id) || made["isEnabled"] != true || made["origin"] != "Application" {
		t.Fatalf("the new role: %v", made)
	}

	// the same rules as the Graph API: a value with a space, and a second role with the same value
	for payload, want := range map[string]string{
		`{"displayName":"X","value":"Two words","allowedMemberTypes":["User"]}`: "value",
		`{"displayName":"X","value":"Editor","allowedMemberTypes":["User"]}`:    "same value",
		`{"displayName":"X","allowedMemberTypes":[]}`:                           "allowedMemberTypes",
		`{"value":"NoName","allowedMemberTypes":["User"]}`:                      "displayName",
	} {
		if s, body := adminRaw(t, ts.URL, "POST", "/applications/"+appObj+"/app-roles", payload); s != 400 || !strings.Contains(body, want) {
			t.Errorf("%s: %d %s", payload, s, body)
		}
	}

	// edit: only the fields that are sent change
	if s, body := adminRaw(t, ts.URL, "PATCH", "/applications/"+appObj+"/app-roles/"+id, `{"description":"Edits things","allowedMemberTypes":["User","Application"]}`); s != 204 {
		t.Fatalf("edit: %d %s", s, body)
	}
	admin(t, ts, "GET", "/applications/"+appObj+"/app-roles", "", &got)
	var editor map[string]any
	for _, r := range got.Roles {
		if r["id"] == id {
			editor = r
		}
	}
	if editor["description"] != "Edits things" || editor["value"] != "Editor" || len(editor["allowedMemberTypes"].([]any)) != 2 {
		t.Errorf("after the edit: %v", editor)
	}

	// remove: only a role that is disabled
	if s, body := adminRaw(t, ts.URL, "DELETE", "/applications/"+appObj+"/app-roles/"+id, ""); s != 400 || !strings.Contains(body, "disabled") {
		t.Errorf("remove an enabled role: %d %s", s, body)
	}
	if s, body := adminRaw(t, ts.URL, "PATCH", "/applications/"+appObj+"/app-roles/"+id, `{"isEnabled":false}`); s != 204 {
		t.Fatalf("disable: %d %s", s, body)
	}
	if s, body := adminRaw(t, ts.URL, "DELETE", "/applications/"+appObj+"/app-roles/"+id, ""); s != 204 {
		t.Fatalf("remove: %d %s", s, body)
	}
	if s, _ := adminRaw(t, ts.URL, "DELETE", "/applications/"+appObj+"/app-roles/"+id, ""); s != 404 {
		t.Errorf("remove a role that is gone: %d", s)
	}
	if s, _ := adminRaw(t, ts.URL, "GET", "/applications/22222222-2222-2222-2222-222222222222/app-roles", ""); s != 404 {
		t.Errorf("an app that is not there: %d", s)
	}
}

// The changes of the portal are in the history, by the admin, and they come back after a restart.
func TestAdminAppRoles_AreInTheHistoryAndSurviveARestart(t *testing.T) {
	dir := t.TempDir()
	w, err := graph.Open(dir, graph.OpenOptions{Users: 30, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(graph.New(w, graph.ServerOptions{}).Handler())
	appID, appObj, spObj := newRegisteredApp(t, ts, role(roleUser, "Reader", "User"))
	uid := firstUserID(t, ts)
	if s, body := adminRaw(t, ts.URL, "POST", "/applications/"+appObj+"/app-roles", `{"displayName":"Editor","value":"Editor","allowedMemberTypes":["User"]}`); s != 201 {
		t.Fatalf("add: %d %s", s, body)
	}
	if s, body := adminRaw(t, ts.URL, "POST", "/service-principals/"+appID+"/role-assignments", `{"principalId":"`+uid+`","appRoleId":"`+roleUser+`"}`); s != 204 {
		t.Fatalf("assign: %d %s", s, body)
	}
	var admins int
	for _, e := range w.History() {
		if e.Actor.Source == "admin" {
			admins++
		}
	}
	if admins != 2 {
		t.Errorf("changes by the admin in the history: %d, want 2", admins)
	}
	ts.Close()
	_ = w.Close()

	again, err := graph.Open(dir, graph.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	ts2 := httptest.NewServer(graph.New(again, graph.ServerOptions{}).Handler())
	defer ts2.Close()
	_, sp := graphCall(t, ts2, "GET", "/servicePrincipals/"+spObj, "")
	if n := len(sp["appRoles"].([]any)); n != 2 {
		t.Errorf("roles of the service principal after a restart: %d, want 2 (the role of the portal comes from the application)", n)
	}
	_, list := graphCall(t, ts2, "GET", "/servicePrincipals/"+spObj+"/appRoleAssignedTo", "")
	if n := len(listOf(list)); n != 1 {
		t.Errorf("assignments after a restart: %d", n)
	}
}

func graphIsGUID(s string) bool { return len(s) == 36 && strings.Count(s, "-") == 4 }

type assignmentsAnswer struct {
	Roles       []map[string]any `json:"roles"`
	Assignments []map[string]any `json:"assignments"`
}

func TestAdminRoleAssignments_ListAssignRemove(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	appID, _, spObj := newRegisteredApp(t, ts, role(roleUser, "Reader", "User"), role(roleApp, "Sync.All", "Application"))
	uid := firstUserID(t, ts)
	base := "/service-principals/" + appID + "/role-assignments"

	var got assignmentsAnswer
	if s := admin(t, ts, "GET", base, "", &got); s != 200 || len(got.Roles) != 2 || len(got.Assignments) != 0 {
		t.Fatalf("empty: %d %+v", s, got)
	}
	if s, body := adminRaw(t, ts.URL, "POST", base, `{"principalId":"`+uid+`","appRoleId":"`+roleUser+`"}`); s != 204 {
		t.Fatalf("assign: %d %s", s, body)
	}
	admin(t, ts, "GET", base, "", &got)
	if len(got.Assignments) != 1 || got.Assignments[0]["appRoleId"] != roleUser || got.Assignments[0]["principalType"] != "User" || got.Assignments[0]["principalDisplayName"] == "" {
		t.Fatalf("assignments: %+v", got.Assignments)
	}
	// the Graph API shows the same assignment
	_, list := graphCall(t, ts, "GET", "/servicePrincipals/"+spObj+"/appRoleAssignedTo", "")
	if n := len(listOf(list)); n != 1 {
		t.Errorf("the Graph API has %d assignments", n)
	}
	// the rules of Graph: a role for applications only is not for a user, and the same twice
	if s, body := adminRaw(t, ts.URL, "POST", base, `{"principalId":"`+uid+`","appRoleId":"`+roleApp+`"}`); s != 400 || !strings.Contains(body, "User") {
		t.Errorf("a role for applications to a user: %d %s", s, body)
	}
	if s, body := adminRaw(t, ts.URL, "POST", base, `{"principalId":"`+uid+`","appRoleId":"`+roleUser+`"}`); s != 400 || !strings.Contains(body, "already exists") {
		t.Errorf("twice: %d %s", s, body)
	}
	if s, _ := adminRaw(t, ts.URL, "POST", base, `{}`); s != 400 {
		t.Errorf("no body: %d", s)
	}

	if s, body := adminRaw(t, ts.URL, "DELETE", base+"/"+got.Assignments[0]["id"].(string), ""); s != 204 {
		t.Fatalf("remove: %d %s", s, body)
	}
	if s, _ := adminRaw(t, ts.URL, "DELETE", base+"/"+got.Assignments[0]["id"].(string), ""); s != 404 {
		t.Errorf("remove again: %d", s)
	}
	if s, _ := adminRaw(t, ts.URL, "GET", "/service-principals/22222222-2222-2222-2222-222222222222/role-assignments", ""); s != 404 {
		t.Errorf("a service principal that is not there: %d", s)
	}
}
