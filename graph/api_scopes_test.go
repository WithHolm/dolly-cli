package graph_test

import (
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

// An app registration exposes an API: an Application ID URI (identifierUris), the delegated permissions that a client can ask for (api.oauth2PermissionScopes),
// and the clients that need no consent for some of them (api.preAuthorizedApplications). The rules are from the Learn pages permissionScope and apiApplication. The
// texts of the errors are dollycli's own, as for the app roles. All of it is written with PATCH /applications: Graph has no route for a scope.

const (
	scopeRead  = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	scopeWrite = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	clientApp  = "cccccccc-cccc-cccc-cccc-cccccccccccc"
)

func scope(id, value, typ string) map[string]any {
	return map[string]any{
		"id": id, "value": value, "type": typ, "isEnabled": true,
		"adminConsentDisplayName": "Admin " + value, "adminConsentDescription": "Admin description of " + value,
		"userConsentDisplayName": "User " + value, "userConsentDescription": "User description of " + value,
	}
}

func apiBody(scopes ...map[string]any) string {
	list := make([]any, len(scopes))
	for i, s := range scopes {
		list[i] = s
	}
	return body(map[string]any{"api": map[string]any{"oauth2PermissionScopes": list}})
}

func TestAPIScopes_AnApplicationKeepsEveryField(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	list := []any{scope(scopeRead, "Files.Read", "User"), scope(scopeWrite, "Files.ReadWrite", "Admin")}
	s, app := graphCall(t, ts, "POST", "/applications", body(map[string]any{"displayName": "Files API", "identifierUris": []any{"api://files-api"}, "api": map[string]any{"oauth2PermissionScopes": list}}))
	if s != 201 {
		t.Fatalf("create: %d %v", s, app)
	}
	_, got := graphCall(t, ts, "GET", "/applications/"+app["id"].(string), "")
	scopes := got["api"].(map[string]any)["oauth2PermissionScopes"].([]any)
	if len(scopes) != 2 {
		t.Fatalf("scopes: %v", scopes)
	}
	r := scopes[0].(map[string]any)
	for k, want := range map[string]any{
		"id": scopeRead, "value": "Files.Read", "type": "User", "isEnabled": true, "adminConsentDisplayName": "Admin Files.Read",
		"adminConsentDescription": "Admin description of Files.Read", "userConsentDisplayName": "User Files.Read", "userConsentDescription": "User description of Files.Read",
	} {
		if r[k] != want {
			t.Errorf("%s = %v, want %v", k, r[k], want)
		}
	}
	// a change of one key of api leaves the others (the scopes stay when the token version is set)
	if s, out := graphCall(t, ts, "PATCH", "/applications/"+app["id"].(string), `{"api":{"requestedAccessTokenVersion":2}}`); s != 204 {
		t.Fatalf("patch: %d %v", s, out)
	}
	_, got = graphCall(t, ts, "GET", "/applications/"+app["id"].(string), "")
	api := got["api"].(map[string]any)
	if len(api["oauth2PermissionScopes"].([]any)) != 2 || api["requestedAccessTokenVersion"] != float64(2) {
		t.Errorf("api after the patch: %v", api)
	}
}

func TestAPIScopes_Validation(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	s, app := graphCall(t, ts, "POST", "/applications", body(map[string]any{"displayName": "Files API", "api": map[string]any{"oauth2PermissionScopes": []any{scope(scopeRead, "Files.Read", "User")}}}))
	if s != 201 {
		t.Fatal(s, app)
	}
	id := app["id"].(string)
	noID := scope(scopeWrite, "X.Y", "User")
	delete(noID, "id")
	noAdminText := scope(scopeWrite, "X.Y", "User")
	delete(noAdminText, "adminConsentDisplayName")
	badEnabled := scope(scopeWrite, "X.Y", "User")
	badEnabled["isEnabled"] = "yes"
	extra := scope(scopeWrite, "X.Y", "User")
	extra["origin"] = "Application"
	keep := scope(scopeRead, "Files.Read", "User")

	bad := map[string]struct {
		api  any
		want string
	}{
		"no id":                    {[]any{keep, noID}, "id"},
		"id not a guid":            {[]any{keep, scope("x", "X.Y", "User")}, "id"},
		"a space in the value":     {[]any{keep, scope(scopeWrite, "Two words", "User")}, "value"},
		"a leading dot":            {[]any{keep, scope(scopeWrite, ".hidden", "User")}, "value"},
		"a value too long":         {[]any{keep, scope(scopeWrite, strings.Repeat("a", 121), "User")}, "value"},
		"no value":                 {[]any{keep, scope(scopeWrite, "", "User")}, "value"},
		"two scopes, one id":       {[]any{keep, scope(scopeRead, "Other", "User")}, "id"},
		"two scopes, one value":    {[]any{keep, scope(scopeWrite, "Files.Read", "User")}, "value"},
		"a wrong type":             {[]any{keep, scope(scopeWrite, "X.Y", "Everyone")}, "type"},
		"no admin consent text":    {[]any{keep, noAdminText}, "adminConsentDisplayName"},
		"isEnabled is not a bool":  {[]any{keep, badEnabled}, "isEnabled"},
		"a property it has not":    {[]any{keep, extra}, "origin"},
		"not a list":               {"Files.Read", "oauth2PermissionScopes"},
		"a scope is not an object": {[]any{"Files.Read"}, "oauth2PermissionScopes"},
		"removing an enabled one":  {[]any{scope(scopeWrite, "X.Y", "User")}, "disabled"},
	}
	for name, c := range bad {
		st, out := graphCall(t, ts, "PATCH", "/applications/"+id, body(map[string]any{"api": map[string]any{"oauth2PermissionScopes": c.api}}))
		if st != 400 || !strings.Contains(errText(out), c.want) {
			t.Errorf("%s: %d %q", name, st, errText(out))
		}
	}
	for api, want := range map[string]string{
		`{"requestedAccessTokenVersion":3}`:   "requestedAccessTokenVersion",
		`{"requestedAccessTokenVersion":"2"}`: "requestedAccessTokenVersion",
		`{"knownClientApplications":["x"]}`:   "knownClientApplications",
		`{"preAuthorizedApplications":"x"}`:   "preAuthorizedApplications",
		`{"preAuthorizedApplications":[{}]}`:  "appId",
		`{"nope":1}`:                          "nope",
		`{"preAuthorizedApplications":[{"appId":"` + clientApp + `","delegatedPermissionIds":["` + scopeWrite + `"]}]}`: "delegatedPermissionIds",
	} {
		if st, out := graphCall(t, ts, "PATCH", "/applications/"+id, `{"api":`+api+`}`); st != 400 || !strings.Contains(errText(out), want) {
			t.Errorf("%s: %d %q", api, st, errText(out))
		}
	}
	// the values that are fine: a null version, 1, 2, and an empty list of scopes after the scope is disabled
	for _, v := range []string{"null", "1", "2"} {
		if st, out := graphCall(t, ts, "PATCH", "/applications/"+id, `{"api":{"requestedAccessTokenVersion":`+v+`}}`); st != 204 {
			t.Errorf("version %s: %d %v", v, st, out)
		}
	}
}

func TestAPIScopes_DisableThenRemove_AndTheServicePrincipalFollows(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	s, app := graphCall(t, ts, "POST", "/applications", body(map[string]any{"displayName": "Files API", "identifierUris": []any{"api://files-api"},
		"api": map[string]any{"oauth2PermissionScopes": []any{scope(scopeRead, "Files.Read", "User"), scope(scopeWrite, "Files.Write", "Admin")}}}))
	if s != 201 {
		t.Fatal(s, app)
	}
	appObj, appID := app["id"].(string), app["appId"].(string)
	s, sp := graphCall(t, ts, "POST", "/servicePrincipals", body(map[string]any{"appId": appID}))
	if s != 201 {
		t.Fatal(s, sp)
	}
	spObj := sp["id"].(string)

	// the service principal has the scopes, in full, and the Application ID URI
	scopesOf := func() map[string]map[string]any {
		_, got := graphCall(t, ts, "GET", "/servicePrincipals/"+spObj, "")
		out := map[string]map[string]any{}
		for _, x := range got["oauth2PermissionScopes"].([]any) {
			m := x.(map[string]any)
			out[m["value"].(string)] = m
		}
		return out
	}
	have := scopesOf()
	if len(have) != 2 || have["Files.Read"]["type"] != "User" || have["Files.Read"]["userConsentDisplayName"] != "User Files.Read" || have["Files.Write"]["type"] != "Admin" {
		t.Fatalf("the service principal: %v", have)
	}
	_, got := graphCall(t, ts, "GET", "/servicePrincipals/"+spObj, "")
	if names := toJSON(got["servicePrincipalNames"]); !strings.Contains(names, "api://files-api") {
		t.Errorf("servicePrincipalNames: %s", names)
	}

	// a change of the registration reaches it: a new scope, and a new URI
	third := scope("dddddddd-dddd-dddd-dddd-dddddddddddd", "Files.Delete", "Admin")
	if st, out := graphCall(t, ts, "PATCH", "/applications/"+appObj, `{"identifierUris":["api://files-api","https://files.example/api"],"api":{"oauth2PermissionScopes":[`+toJSON(scope(scopeRead, "Files.Read", "User"))+`,`+toJSON(scope(scopeWrite, "Files.Write", "Admin"))+`,`+toJSON(third)+`]}}`); st != 204 {
		t.Fatalf("patch: %d %v", st, out)
	}
	if len(scopesOf()) != 3 {
		t.Errorf("after the change: %v", scopesOf())
	}
	_, got = graphCall(t, ts, "GET", "/servicePrincipals/"+spObj, "")
	if !strings.Contains(toJSON(got["servicePrincipalNames"]), "https://files.example/api") {
		t.Errorf("servicePrincipalNames: %v", got["servicePrincipalNames"])
	}

	// a consent that has the scope loses it when the scope is removed, and a consent with nothing left is gone
	_, users := graphCall(t, ts, "GET", "/servicePrincipals?$top=999&$select=id,appId", "")
	var client string
	for _, x := range listOf(users) {
		if x["id"] != spObj {
			client = x["id"].(string)
			break
		}
	}
	uid := firstUserID(t, ts)
	mk := func(consentType, principal, sc string) string {
		props := map[string]any{"clientId": client, "consentType": consentType, "resourceId": spObj, "scope": sc}
		if principal != "" {
			props["principalId"] = principal
		}
		st, out := graphCall(t, ts, "POST", "/oauth2PermissionGrants", body(props))
		if st != 201 {
			t.Fatalf("consent: %d %v", st, out)
		}
		return out["id"].(string)
	}
	both, solo := mk("AllPrincipals", "", "Files.Read Files.Write"), mk("Principal", uid, "Files.Read")

	off := scope(scopeRead, "Files.Read", "User")
	off["isEnabled"] = false
	all := []any{off, scope(scopeWrite, "Files.Write", "Admin"), third}
	if st, out := graphCall(t, ts, "PATCH", "/applications/"+appObj, body(map[string]any{"api": map[string]any{"oauth2PermissionScopes": all}})); st != 204 {
		t.Fatalf("disable: %d %v", st, out)
	}
	if st, out := graphCall(t, ts, "PATCH", "/applications/"+appObj, body(map[string]any{"api": map[string]any{"oauth2PermissionScopes": all[1:]}})); st != 204 {
		t.Fatalf("remove: %d %v", st, out)
	}
	if _, ok := scopesOf()["Files.Read"]; ok {
		t.Error("the removed scope is still on the service principal")
	}
	if st, grant := graphCall(t, ts, "GET", "/oauth2PermissionGrants/"+both, ""); st != 200 || grant["scope"] != "Files.Write" {
		t.Errorf("the consent after the remove: %d %v", st, grant)
	}
	if st, _ := graphCall(t, ts, "GET", "/oauth2PermissionGrants/"+solo, ""); st != 404 {
		t.Errorf("a consent with no scope left is gone: %d", st)
	}
}

func TestAPIScopes_IdentifierURIs(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	s, app := graphCall(t, ts, "POST", "/applications", `{"displayName":"One","identifierUris":["api://one"]}`)
	if s != 201 {
		t.Fatal(s, app)
	}
	for uri, want := range map[string]string{
		`"myapi"`:                 "identifierUris",
		`"ftp://x.example"`:       "identifierUris",
		`"api://"`:                "identifierUris",
		`"https://"`:              "identifierUris",
		`1`:                       "identifierUris",
		`"api://one"`:             "already exists", // another app has it
		`"API://ONE"`:             "already exists", // the case does not matter
		`"api://two","api://two"`: "identifierUris", // twice in a list
	} {
		st, out := graphCall(t, ts, "POST", "/applications", `{"displayName":"Two","identifierUris":[`+uri+`]}`)
		if st != 400 || !strings.Contains(errText(out), want) {
			t.Errorf("%s: %d %q", uri, st, errText(out))
		}
	}
	// the app keeps its own URI when it writes the same list again, and an app can have several
	if st, out := graphCall(t, ts, "PATCH", "/applications/"+app["id"].(string), `{"identifierUris":["api://one","https://one.example/api"]}`); st != 204 {
		t.Errorf("the same list again: %d %v", st, out)
	}
}

func TestAPIScopes_PreAuthorizedClients(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	s, app := graphCall(t, ts, "POST", "/applications", body(map[string]any{"displayName": "Files API",
		"api": map[string]any{"oauth2PermissionScopes": []any{scope(scopeRead, "Files.Read", "User"), scope(scopeWrite, "Files.Write", "User")}}}))
	if s != 201 {
		t.Fatal(s, app)
	}
	id := app["id"].(string)
	pre := `{"api":{"preAuthorizedApplications":[{"appId":"` + clientApp + `","delegatedPermissionIds":["` + scopeRead + `","` + scopeWrite + `"]}]}}`
	if st, out := graphCall(t, ts, "PATCH", "/applications/"+id, pre); st != 204 {
		t.Fatalf("pre-authorize: %d %v", st, out)
	}
	_, got := graphCall(t, ts, "GET", "/applications/"+id, "")
	list := got["api"].(map[string]any)["preAuthorizedApplications"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["appId"] != clientApp || len(list[0].(map[string]any)["delegatedPermissionIds"].([]any)) != 2 {
		t.Fatalf("preAuthorizedApplications: %v", list)
	}

	// a scope that goes away goes out of the list of the clients
	off := scope(scopeWrite, "Files.Write", "User")
	off["isEnabled"] = false
	both := body(map[string]any{"api": map[string]any{"oauth2PermissionScopes": []any{scope(scopeRead, "Files.Read", "User"), off}}})
	if st, out := graphCall(t, ts, "PATCH", "/applications/"+id, both); st != 204 {
		t.Fatalf("disable: %d %v", st, out)
	}
	if st, out := graphCall(t, ts, "PATCH", "/applications/"+id, apiBody(scope(scopeRead, "Files.Read", "User"))); st != 204 {
		t.Fatalf("remove: %d %v", st, out)
	}
	_, got = graphCall(t, ts, "GET", "/applications/"+id, "")
	list = got["api"].(map[string]any)["preAuthorizedApplications"].([]any)
	ids := list[0].(map[string]any)["delegatedPermissionIds"].([]any)
	if len(ids) != 1 || ids[0] != scopeRead {
		t.Errorf("the client after the remove: %v", list)
	}
}
