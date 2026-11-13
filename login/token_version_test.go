package login_test

import (
	"net/url"
	"strings"
	"testing"

	graph "github.com/withholm/dolly-cli/graph"
	"github.com/withholm/dolly-cli/graph/permissions"
)

// The version of an access token is the one that the API asks for (requestedAccessTokenVersion of its app registration), whatever endpoint the client used. The
// endpoint, v1.0 or v2.0, only decides the version of an ID token (Learn: apiApplication). Null is 1.
//
//	version 1: ver 1.0, iss https://sts.windows.net/<tenant>/, and aud is the Application ID URI of the API (its client ID if it has none)
//	version 2: ver 2.0, iss <the address of dollycli>/<tenant>/v2.0, and aud is the client ID of the API
//
// Microsoft Graph keeps the tokens that it had. The audience of a v1 token is what the real Entra gives; that is from memory of the docs of Entra, not from Learn.

func TestAccessTokenVersion_IsTheOneThatTheAPIAsksFor(t *testing.T) {
	e := setup(t)
	admin := graph.Actor{Source: "admin"}
	const read = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	scope := map[string]any{
		"id": read, "value": "Files.Read", "type": "User", "isEnabled": true,
		"adminConsentDisplayName": "n", "adminConsentDescription": "d", "userConsentDisplayName": "n", "userConsentDescription": "d",
	}
	app, perr := e.world.CreateApplication(map[string]any{"displayName": "Files API", "identifierUris": []any{"api://files-api"},
		"api": map[string]any{"oauth2PermissionScopes": []any{scope}}}, admin)
	if perr != nil {
		t.Fatal(perr)
	}
	apiID := app["appId"].(string)
	if _, perr := e.world.CreateServicePrincipal(map[string]any{"appId": apiID}, admin); perr != nil {
		t.Fatal(perr)
	}
	user := e.world.ListUsers()[0]
	v2Path, v1Path := "/"+e.tid+"/oauth2/v2.0/token", "/"+e.tid+"/oauth2/token"
	sts := "https://sts.windows.net/" + e.tid + "/"

	// userToken asks as a user does, on the v2.0 endpoint (with a scope) or on the v1.0 endpoint (with a resource). It gives the access token and the ID token.
	userToken := func(path string) (access, id map[string]any) {
		t.Helper()
		form := url.Values{"grant_type": {"password"}, "client_id": {client1}, "username": {user["userPrincipalName"].(string)}, "password": {"x"}}
		if strings.Contains(path, "v2.0") {
			form.Set("scope", "openid api://files-api/Files.Read")
		} else {
			form.Set("resource", "api://files-api")
		}
		code, resp := e.post(path, form)
		if code != 200 {
			t.Fatalf("%s: %d %v", path, code, resp)
		}
		access = claimsOf(t, resp["access_token"].(string))
		if tok, ok := resp["id_token"].(string); ok {
			id = claimsOf(t, tok)
		}
		return access, id
	}
	appToken := func() map[string]any {
		t.Helper()
		code, resp := e.post(v2Path, url.Values{"grant_type": {"client_credentials"}, "client_id": {client1}, "client_secret": {"x"}, "scope": {"api://files-api/.default"}})
		if code != 200 {
			t.Fatalf("client credentials: %d %v", code, resp)
		}
		return claimsOf(t, resp["access_token"].(string))
	}
	check := func(what string, c map[string]any, ver, iss, aud string) {
		t.Helper()
		if c["ver"] != ver || c["aud"] != aud || (iss != "" && c["iss"] != iss) {
			t.Errorf("%s: ver=%v iss=%v aud=%v, want ver=%s iss=%s aud=%s", what, c["ver"], c["iss"], c["aud"], ver, iss, aud)
		}
		if iss == "" && !strings.HasSuffix(c["iss"].(string), "/"+e.tid+"/v2.0") {
			t.Errorf("%s: the issuer of a v2.0 token is %v", what, c["iss"])
		}
	}

	// null: version 1, on both endpoints
	v2access, v2id := userToken(v2Path)
	check("null, v2.0 endpoint", v2access, "1.0", sts, "api://files-api")
	check("null, v1.0 endpoint", func() map[string]any { a, _ := userToken(v1Path); return a }(), "1.0", sts, "api://files-api")
	check("null, client credentials", appToken(), "1.0", sts, "api://files-api")
	// the ID token follows the endpoint, and not the API: this is a v2.0 endpoint, so it is a v2.0 token for the client
	if v2id["ver"] != "2.0" || v2id["aud"] != client1 || !strings.HasSuffix(v2id["iss"].(string), "/"+e.tid+"/v2.0") {
		t.Errorf("the ID token of the v2.0 endpoint: %v", v2id)
	}

	// version 2: the client ID is the audience, on both endpoints
	if perr := e.world.UpdateApplication(app["id"].(string), map[string]any{"api": map[string]any{"requestedAccessTokenVersion": 2}}, admin); perr != nil {
		t.Fatal(perr)
	}
	v2access, v2id = userToken(v2Path)
	check("2, v2.0 endpoint", v2access, "2.0", "", apiID)
	check("2, v1.0 endpoint", func() map[string]any { a, _ := userToken(v1Path); return a }(), "2.0", "", apiID)
	check("2, client credentials", appToken(), "2.0", "", apiID)
	if v2id["ver"] != "2.0" {
		t.Errorf("the ID token: %v", v2id)
	}

	// an API with no Application ID URI: the audience of a v1 token is its client ID
	bare, perr := e.world.CreateApplication(map[string]any{"displayName": "Bare API", "api": map[string]any{"oauth2PermissionScopes": []any{scope}}}, admin) // a scope makes it an API
	if perr != nil {
		t.Fatal(perr)
	}
	bareID := bare["appId"].(string)
	if _, perr := e.world.CreateServicePrincipal(map[string]any{"appId": bareID}, admin); perr != nil {
		t.Fatal(perr)
	}
	code, resp := e.post(v2Path, url.Values{"grant_type": {"client_credentials"}, "client_id": {client1}, "client_secret": {"x"}, "scope": {"api://" + bareID + "/.default"}})
	if code != 200 {
		t.Fatalf("a bare API: %d %v", code, resp)
	}
	check("no URI, null", claimsOf(t, resp["access_token"].(string)), "1.0", sts, bareID)
}

// Microsoft Graph keeps its tokens: the v2.0 endpoint gives a 2.0 token, as it did.
func TestAccessTokenVersion_MicrosoftGraphKeepsItsTokens(t *testing.T) {
	e := setup(t)
	code, resp := e.appToken(client1, "x")
	if code != 200 {
		t.Fatalf("%d %v", code, resp)
	}
	c := claimsOf(t, resp["access_token"].(string))
	if c["ver"] != "2.0" || c["aud"] != permissions.GraphAppID {
		t.Errorf("a token for Graph: ver=%v aud=%v", c["ver"], c["aud"])
	}
}
