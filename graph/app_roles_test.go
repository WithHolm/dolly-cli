package graph_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

// The rules of an app role are from the page of Microsoft Learn: appRole resource type, at the commit of graph/cost/raw/SOURCE.json.
// The texts of the errors are dollycli's own: the real Graph's are not known (a GET is all the Explorer proxy allows).

const (
	roleUser  = "11111111-1111-1111-1111-111111111111"
	roleApp   = "22222222-2222-2222-2222-222222222222"
	roleOther = "33333333-3333-3333-3333-333333333333"
)

func role(id, value string, types ...string) map[string]any {
	ts := make([]any, len(types))
	for i, t := range types {
		ts[i] = t
	}
	return map[string]any{"id": id, "value": value, "displayName": "Role " + value, "description": "The role " + value, "allowedMemberTypes": ts, "isEnabled": true}
}

func body(v any) string   { b, _ := json.Marshal(v); return b2s(b) }
func b2s(b []byte) string { return string(b) }

// callWith is graphCall with headers.
func callWith(t *testing.T, ts *httptest.Server, method, path, payload string, h map[string]string) (int, map[string]any, string) {
	t.Helper()
	req, _ := http.NewRequest(method, ts.URL+"/v1.0"+path, strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer x")
	req.Header.Set("Content-Type", "application/json")
	for k, v := range h {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var raw strings.Builder
	buf := make([]byte, 4096)
	for {
		n, e := res.Body.Read(buf)
		raw.Write(buf[:n])
		if e != nil {
			break
		}
	}
	var out map[string]any
	_ = json.Unmarshal([]byte(raw.String()), &out)
	return res.StatusCode, out, raw.String()
}

// newRegisteredApp makes an app registration with roles, and the service principal of it. It gives their object IDs.
func newRegisteredApp(t *testing.T, ts *httptest.Server, roles ...map[string]any) (appID, appObj, spObj string) {
	t.Helper()
	rs := make([]any, len(roles))
	for i, r := range roles {
		rs[i] = r
	}
	s, app := graphCall(t, ts, "POST", "/applications", body(map[string]any{"displayName": "Timesheets", "appRoles": rs}))
	if s != 201 {
		t.Fatalf("create the application: %d %v", s, app)
	}
	s, sp := graphCall(t, ts, "POST", "/servicePrincipals", body(map[string]any{"appId": app["appId"]}))
	if s != 201 {
		t.Fatalf("create the service principal: %d %v", s, sp)
	}
	return app["appId"].(string), app["id"].(string), sp["id"].(string)
}

func TestAppRoles_AnApplicationKeepsEveryField(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	_, appObj, _ := newRegisteredApp(t, ts, role(roleUser, "Timesheet.Approve", "User"), role(roleApp, "Timesheet.Read.All", "User", "Application"))
	_, got := graphCall(t, ts, "GET", "/applications/"+appObj, "")
	roles := got["appRoles"].([]any)
	if len(roles) != 2 {
		t.Fatalf("roles: %v", roles)
	}
	r := roles[1].(map[string]any)
	if r["id"] != roleApp || r["value"] != "Timesheet.Read.All" || r["description"] != "The role Timesheet.Read.All" || r["displayName"] != "Role Timesheet.Read.All" ||
		r["isEnabled"] != true || r["origin"] != "Application" || len(r["allowedMemberTypes"].([]any)) != 2 {
		t.Errorf("the role lost a field, or has no origin: %v", r)
	}
}

func TestAppRoles_Validation(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	_, appObj, _ := newRegisteredApp(t, ts, role(roleUser, "Reader", "User"))
	bad := map[string]struct {
		roles any
		want  string
	}{
		"no id":                     {[]any{map[string]any{"value": "A", "displayName": "A", "description": "A", "allowedMemberTypes": []any{"User"}, "isEnabled": true}}, "id"},
		"id not a guid":             {[]any{role("x", "A", "User")}, "id"},
		"a space in the value":      {[]any{role(roleOther, "Two words", "User")}, "value"},
		"a leading dot":             {[]any{role(roleOther, ".hidden", "User")}, "value"},
		"a value too long":          {[]any{role(roleOther, strings.Repeat("a", 121), "User")}, "value"},
		"a letter outside the list": {[]any{role(roleOther, "Åre", "User")}, "value"},
		"two roles, one value":      {[]any{role(roleUser, "Reader", "User"), role(roleOther, "Reader", "User")}, "value"},
		"two roles, one id":         {[]any{role(roleUser, "A", "User"), role(roleUser, "B", "User")}, "id"},
		"no member types":           {[]any{role(roleOther, "A")}, "allowedMemberTypes"},
		"a wrong member type":       {[]any{role(roleOther, "A", "Group")}, "allowedMemberTypes"},
		"origin is read-only":       {[]any{role(roleUser, "Reader", "User"), map[string]any{"id": roleOther, "value": "A", "displayName": "A", "description": "A", "allowedMemberTypes": []any{"User"}, "isEnabled": true, "origin": "ServicePrincipal"}}, "origin"},
		"not a list":                {"Reader", "appRoles"},
		"a role is not an object":   {[]any{"Reader"}, "appRoles"},
	}
	for name, c := range bad {
		s, out := graphCall(t, ts, "PATCH", "/applications/"+appObj, body(map[string]any{"appRoles": c.roles}))
		if s != 400 || !strings.Contains(errText(out), c.want) {
			t.Errorf("%s: %d %q", name, s, errText(out))
		}
	}
	// a value may be empty (null): roles of the seed have none
	r := role(roleOther, "", "User")
	r["value"] = nil
	if s, out := graphCall(t, ts, "PATCH", "/applications/"+appObj, body(map[string]any{"appRoles": []any{role(roleUser, "Reader", "User"), r}})); s != 204 {
		t.Errorf("a role with no value: %d %v", s, out)
	}
}

// A role is disabled in one call, and removed in a later one (Learn: isEnabled).
func TestAppRoles_DisableThenRemove(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	_, appObj, _ := newRegisteredApp(t, ts, role(roleUser, "Reader", "User"), role(roleOther, "Writer", "User"))

	if s, out := graphCall(t, ts, "PATCH", "/applications/"+appObj, body(map[string]any{"appRoles": []any{role(roleUser, "Reader", "User")}})); s != 400 || !strings.Contains(errText(out), "disabled") {
		t.Errorf("removing an enabled role: %d %q", s, errText(out))
	}
	off := role(roleOther, "Writer", "User")
	off["isEnabled"] = false
	if s, out := graphCall(t, ts, "PATCH", "/applications/"+appObj, body(map[string]any{"appRoles": []any{role(roleUser, "Reader", "User"), off}})); s != 204 {
		t.Fatalf("disable: %d %v", s, out)
	}
	if s, out := graphCall(t, ts, "PATCH", "/applications/"+appObj, body(map[string]any{"appRoles": []any{role(roleUser, "Reader", "User")}})); s != 204 {
		t.Fatalf("remove the disabled role: %d %v", s, out)
	}
	_, got := graphCall(t, ts, "GET", "/applications/"+appObj, "")
	if n := len(got["appRoles"].([]any)); n != 1 {
		t.Errorf("roles after the remove: %d", n)
	}
}

// The roles of the application are on the service principal that has its client ID, with the origin Application.
func TestAppRoles_TheServicePrincipalHasTheRolesOfTheApplication(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	_, appObj, spObj := newRegisteredApp(t, ts, role(roleUser, "Reader", "User"))

	_, sp := graphCall(t, ts, "GET", "/servicePrincipals/"+spObj, "")
	roles := sp["appRoles"].([]any)
	if len(roles) != 1 || roles[0].(map[string]any)["origin"] != "Application" || roles[0].(map[string]any)["value"] != "Reader" {
		t.Fatalf("the service principal: %v", roles)
	}
	// a change of the application reaches the service principal
	if s, out := graphCall(t, ts, "PATCH", "/applications/"+appObj, body(map[string]any{"appRoles": []any{role(roleUser, "Reader", "User"), role(roleApp, "Sync", "Application")}})); s != 204 {
		t.Fatalf("patch: %d %v", s, out)
	}
	_, sp = graphCall(t, ts, "GET", "/servicePrincipals/"+spObj, "")
	if n := len(sp["appRoles"].([]any)); n != 2 {
		t.Errorf("the service principal has %d roles after the change, want 2", n)
	}

	// a role of its own: origin ServicePrincipal, and it is not for an application (Learn: only on application objects)
	own := role(roleOther, "Local", "User")
	if s, out := graphCall(t, ts, "PATCH", "/servicePrincipals/"+spObj, body(map[string]any{"appRoles": append(sp["appRoles"].([]any), own)})); s != 204 {
		t.Fatalf("a role of the service principal: %d %v", s, out)
	}
	_, sp = graphCall(t, ts, "GET", "/servicePrincipals/"+spObj, "")
	byValue := map[string]string{}
	for _, r := range sp["appRoles"].([]any) {
		m := r.(map[string]any)
		byValue[m["value"].(string)] = m["origin"].(string)
	}
	if byValue["Local"] != "ServicePrincipal" || byValue["Reader"] != "Application" {
		t.Errorf("origins: %v", byValue)
	}
	if s, out := graphCall(t, ts, "PATCH", "/servicePrincipals/"+spObj, body(map[string]any{"appRoles": append(sp["appRoles"].([]any), role("44444444-4444-4444-4444-444444444444", "Perm", "Application"))})); s != 400 || !strings.Contains(errText(out), "Application") {
		t.Errorf("Application on a service principal: %d %q", s, errText(out))
	}
	// and the application never gets the role of the service principal
	_, app := graphCall(t, ts, "GET", "/applications/"+appObj, "")
	if n := len(app["appRoles"].([]any)); n != 2 {
		t.Errorf("the application has %d roles, want 2", n)
	}
}

func firstUserID(t *testing.T, ts *httptest.Server) string {
	_, users := graphCall(t, ts, "GET", "/users?$top=1&$select=id", "")
	return listOf(users)[0]["id"].(string)
}

func TestAppRoles_AssignToAUser(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	_, appObj, spObj := newRegisteredApp(t, ts, role(roleUser, "Reader", "User"), role(roleApp, "Sync.All", "Application"), role(roleOther, "Retired", "User"))
	off := role(roleOther, "Retired", "User")
	off["isEnabled"] = false
	graphCall(t, ts, "PATCH", "/applications/"+appObj, body(map[string]any{"appRoles": []any{role(roleUser, "Reader", "User"), role(roleApp, "Sync.All", "Application"), off}}))
	uid := firstUserID(t, ts)
	assign := func(role string) (int, map[string]any) {
		return graphCall(t, ts, "POST", "/servicePrincipals/"+spObj+"/appRoleAssignedTo", body(map[string]any{"principalId": uid, "resourceId": spObj, "appRoleId": role}))
	}

	s, a := assign(roleUser)
	if s != 201 || a["appRoleId"] != roleUser || a["principalType"] != "User" || a["resourceId"] != spObj || a["resourceDisplayName"] != "Timesheets" {
		t.Fatalf("assign: %d %v", s, a)
	}
	if s, out := assign(roleUser); s != 400 || !strings.Contains(errText(out), "already exists") {
		t.Errorf("a second time: %d %q", s, errText(out))
	}
	if s, out := assign(roleApp); s != 400 || !strings.Contains(errText(out), "User") {
		t.Errorf("a role that is for an application only: %d %q", s, errText(out))
	}
	if s, out := assign(roleOther); s != 400 || !strings.Contains(errText(out), "disabled") {
		t.Errorf("a disabled role: %d %q", s, errText(out))
	}
	if s, out := assign("55555555-5555-5555-5555-555555555555"); s != 400 || !strings.Contains(errText(out), "not found") {
		t.Errorf("an unknown role: %d %q", s, errText(out))
	}

	// the same assignment, read in the three ways: the list, one, and the count
	id := a["id"].(string)
	s, one := graphCall(t, ts, "GET", "/servicePrincipals/"+spObj+"/appRoleAssignedTo/"+id, "")
	if s != 200 || one["id"] != id || one["appRoleId"] != roleUser || !strings.HasSuffix(one["@odata.context"].(string), "#appRoleAssignments/$entity") {
		t.Errorf("get one: %d %v", s, one)
	}
	if s, _ := graphCall(t, ts, "GET", "/servicePrincipals/"+spObj+"/appRoleAssignedTo/nope", ""); s != 404 {
		t.Errorf("get an unknown assignment: %d", s)
	}
	s, out, raw := callWith(t, ts, "GET", "/servicePrincipals/"+spObj+"/appRoleAssignedTo/$count", "", nil)
	if s != 400 || !strings.Contains(raw, "Request_UnsupportedQuery") || !strings.Contains(errText(out), "ConsistencyLevel") {
		t.Errorf("a count with no ConsistencyLevel: %d %s", s, raw)
	}
	if s, _, raw := callWith(t, ts, "GET", "/servicePrincipals/"+spObj+"/appRoleAssignedTo/$count", "", map[string]string{"ConsistencyLevel": "eventual"}); s != 200 || strings.TrimSpace(raw) != "1" {
		t.Errorf("the count: %d %q", s, raw)
	}
	// the user sees the same assignment from the user's side
	_, mine := graphCall(t, ts, "GET", "/users/"+uid+"/appRoleAssignments", "")
	found := false
	for _, x := range listOf(mine) {
		found = found || x["id"] == id
	}
	if !found {
		t.Errorf("the user's appRoleAssignments has not %s: %v", id, mine)
	}
}

// A role that is removed takes its assignments with it, so no assignment points at a role that is not there.
func TestAppRoles_RemovingARoleRemovesItsAssignments(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	_, appObj, spObj := newRegisteredApp(t, ts, role(roleUser, "Reader", "User"), role(roleOther, "Writer", "User"))
	uid := firstUserID(t, ts)
	graphCall(t, ts, "POST", "/servicePrincipals/"+spObj+"/appRoleAssignedTo", body(map[string]any{"principalId": uid, "resourceId": spObj, "appRoleId": roleOther}))
	off := role(roleOther, "Writer", "User")
	off["isEnabled"] = false
	graphCall(t, ts, "PATCH", "/applications/"+appObj, body(map[string]any{"appRoles": []any{role(roleUser, "Reader", "User"), off}}))
	graphCall(t, ts, "PATCH", "/applications/"+appObj, body(map[string]any{"appRoles": []any{role(roleUser, "Reader", "User")}}))
	_, list := graphCall(t, ts, "GET", "/servicePrincipals/"+spObj+"/appRoleAssignedTo", "")
	if n := len(listOf(list)); n != 0 {
		t.Errorf("assignments left: %d", n)
	}
}

// Roles that an old data folder has (an id and a name) still work: enabled, for a user or an application, with a value.
func TestAppRoles_TheRolesOfTheSeedHaveTheFullShape(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	_, list := graphCall(t, ts, "GET", "/servicePrincipals?$top=999", "")
	var roles []any
	for _, sp := range listOf(list) {
		if r, _ := sp["appRoles"].([]any); len(r) > 0 {
			roles = r
			break
		}
	}
	if len(roles) == 0 {
		t.Fatal("the seed has service principals with roles (the demo apps)")
	}
	r := roles[0].(map[string]any)
	for _, k := range []string{"id", "displayName", "description", "allowedMemberTypes", "isEnabled", "origin", "value"} {
		if _, has := r[k]; !has {
			t.Errorf("a role of the seed has no %s: %v", k, r)
		}
	}
	if r["isEnabled"] != true {
		t.Errorf("the roles of the seed are enabled: %v", r)
	}
}
