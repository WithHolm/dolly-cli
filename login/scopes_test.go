package login_test

import (
	"net/url"
	"strings"
	"testing"

	graph "github.com/withholm/dolly-cli/graph"
)

// The scopes of an API that an app registration exposes are what a client asks for, as api://files-api/Files.Read. The token has them in scp. A disabled scope
// cannot be asked for. A client that the registration pre-authorizes gets its scopes for .default with no consent.
func TestScopes_OfAnExposedAPI(t *testing.T) {
	e := setup(t)
	admin := graph.Actor{Source: "admin"}
	const (
		read  = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		write = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		old   = "dddddddd-dddd-dddd-dddd-dddddddddddd"
	)
	scope := func(id, value, typ string, on bool) map[string]any {
		return map[string]any{
			"id": id, "value": value, "type": typ, "isEnabled": on,
			"adminConsentDisplayName": "n", "adminConsentDescription": "d", "userConsentDisplayName": "n", "userConsentDescription": "d",
		}
	}
	app, perr := e.world.CreateApplication(map[string]any{"displayName": "Files API", "identifierUris": []any{"api://files-api"},
		"api": map[string]any{"oauth2PermissionScopes": []any{scope(read, "Files.Read", "User", true), scope(write, "Files.Write", "Admin", true), scope(old, "Files.Old", "User", false)}}}, admin)
	if perr != nil {
		t.Fatal(perr)
	}
	apiID := app["appId"].(string)
	if _, perr := e.world.CreateServicePrincipal(map[string]any{"appId": apiID}, admin); perr != nil {
		t.Fatal(perr)
	}
	user := e.world.ListUsers()[0]
	path := "/" + e.tid + "/oauth2/v2.0/token"
	ask := func(scope string) (int, map[string]any) {
		return e.post(path, url.Values{"grant_type": {"password"}, "client_id": {client1}, "username": {user["userPrincipalName"].(string)}, "password": {"x"}, "scope": {scope}})
	}

	code, resp := ask("api://files-api/Files.Read")
	if code != 200 {
		t.Fatalf("ask for a scope: %d %v", code, resp)
	}
	// the API has no requestedAccessTokenVersion, so it gets version 1 tokens: the audience is its Application ID URI (token_version_test.go has the rest)
	if c := claimsOf(t, resp["access_token"].(string)); c["scp"] != "Files.Read" || c["aud"] != "api://files-api" {
		t.Errorf("claims: scp=%v aud=%v (want Files.Read, api://files-api)", c["scp"], c["aud"])
	}

	code, resp = ask("api://files-api/Files.Old")
	if code != 400 || !strings.Contains(resp["error_description"].(string), "AADSTS70011") {
		t.Errorf("a disabled scope: %d %v", code, resp)
	}

	// .default is what is consented. A client that is pre-authorized needs no consent for the scopes it is pre-authorized for.
	if _, r := ask("api://files-api/.default"); claimsOf(t, r["access_token"].(string))["scp"] != "" {
		t.Errorf("no consent, no scopes: %v", claimsOf(t, r["access_token"].(string))["scp"])
	}
	if perr := e.world.SetPreAuthorized(apiID, client1, []string{read, old}, admin); perr != nil {
		t.Fatal(perr)
	}
	if _, r := ask("api://files-api/.default"); claimsOf(t, r["access_token"].(string))["scp"] != "Files.Read" {
		t.Errorf("pre-authorized: scp %v, want Files.Read (the disabled scope is left out, and the scope it is not pre-authorized for)", claimsOf(t, r["access_token"].(string))["scp"])
	}
}
