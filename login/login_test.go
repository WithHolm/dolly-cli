package login_test

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/withholm/dolly-cli/graph"
	"github.com/withholm/dolly-cli/graph/permissions"
	"github.com/withholm/dolly-cli/graph/repo"
	"github.com/withholm/dolly-cli/login"
	"github.com/withholm/dolly-cli/shared"
)

const (
	graphScope = "https://graph.microsoft.com/.default"
	client1    = "11111111-2222-3333-4444-555555555555"
)

type env struct {
	t     *testing.T
	url   string
	world *graph.World
	api   *login.API
	clock *atomic.Int64 // unix seconds
	tid   string
}

func setup(t *testing.T) *env {
	t.Helper()
	w := graph.Generate(graph.Options{Users: 30, Seed: 1})
	e := &env{t: t, world: w, clock: &atomic.Int64{}, tid: w.TenantID()}
	e.clock.Store(time.Now().Unix())
	gs := graph.New(w, graph.ServerOptions{})
	api, err := login.New(w, login.Options{Now: func() time.Time { return time.Unix(e.clock.Load(), 0) }})
	if err != nil {
		t.Fatal(err)
	}
	gs.SetTokenVerifier(api.Verify)
	e.api = api
	ts := httptest.NewServer(shared.Router(http.NotFoundHandler(), gs, api))
	t.Cleanup(ts.Close)
	e.url = ts.URL
	return e
}

// post sends a form to the token endpoint, and gives the status and the JSON answer.
func (e *env) post(path string, form url.Values) (int, map[string]any) {
	e.t.Helper()
	res, err := http.PostForm(e.url+path, form)
	if err != nil {
		e.t.Fatal(err)
	}
	return res.StatusCode, readMap(e.t, res)
}

func readMap(t *testing.T, res *http.Response) map[string]any {
	t.Helper()
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("not JSON (%d): %s", res.StatusCode, b)
	}
	return m
}

func (e *env) appToken(client, secret string) (int, map[string]any) {
	return e.post("/"+e.tid+"/oauth2/v2.0/token", url.Values{
		"grant_type": {"client_credentials"}, "client_id": {client}, "client_secret": {secret}, "scope": {graphScope},
	})
}

// graphGet calls Graph with a token.
func (e *env) graphGet(path, token string) (int, string) {
	e.t.Helper()
	req, _ := http.NewRequest("GET", e.url+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func (e *env) admin(method, path string) map[string]any {
	e.t.Helper()
	req, _ := http.NewRequest(method, e.url+path, nil)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	if res.StatusCode == http.StatusNoContent {
		return nil
	}
	return readMap(e.t, res)
}

func claimsOf(t *testing.T, tok string) map[string]any {
	t.Helper()
	parts := strings.Split(tok, ".")
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func errorCode(m map[string]any) string {
	codes, _ := m["error_codes"].([]any)
	if len(codes) == 0 {
		return ""
	}
	return "AADSTS" + string(json.Number(strings.TrimSuffix(strings.TrimSpace(jsonNum(codes[0])), ".0")))
}

func jsonNum(v any) string { b, _ := json.Marshal(v); return string(b) }

func TestDiscoveryAndKeys(t *testing.T) {
	e := setup(t)
	for _, tenant := range []string{e.tid, "common", "acme.example", "ACME.onmicrosoft.com"} {
		res, err := http.Get(e.url + "/" + tenant + "/v2.0/.well-known/openid-configuration")
		if err != nil {
			t.Fatal(err)
		}
		doc := readMap(t, res)
		if res.StatusCode != 200 || doc["issuer"] != e.url+"/"+e.tid+"/v2.0" {
			t.Errorf("%s: %d %v", tenant, res.StatusCode, doc["issuer"])
		}
		if !strings.HasPrefix(doc["token_endpoint"].(string), e.url+"/"+e.tid+"/oauth2/v2.0/token") {
			t.Errorf("%s: token_endpoint %v", tenant, doc["token_endpoint"])
		}
	}
	// the v1 document has the issuer of v1
	res, _ := http.Get(e.url + "/" + e.tid + "/.well-known/openid-configuration")
	if doc := readMap(t, res); doc["issuer"] != "https://sts.windows.net/"+e.tid+"/" {
		t.Errorf("v1 issuer: %v", doc["issuer"])
	}
	res, _ = http.Get(e.url + "/other.example/v2.0/.well-known/openid-configuration")
	if m := readMap(t, res); res.StatusCode != 400 || errorCode(m) != "AADSTS90002" {
		t.Errorf("unknown domain: %d %v", res.StatusCode, m)
	}
	res, _ = http.Get(e.url + "/99999999-0000-0000-0000-000000000000/discovery/v2.0/keys")
	if m := readMap(t, res); res.StatusCode != 400 || errorCode(m) != "AADSTS90002" {
		t.Errorf("unknown tenant: %d %v", res.StatusCode, m)
	}
	// the portal and Graph still answer: the sign-in API takes only its own paths
	if code, _ := e.graphGet("/v1.0/users?$top=1", "x"); code != 200 {
		t.Errorf("Graph: %d", code)
	}
}

// The token has a real signature: it is checked with the key from the JWKS, with the standard library only.
func TestClientCredentials_TokenIsSignedAndRegistersTheClient(t *testing.T) {
	e := setup(t)
	code, resp := e.appToken(client1, "s3cret")
	if code != 200 || resp["token_type"] != "Bearer" || resp["expires_in"].(float64) != 3599 {
		t.Fatalf("token: %d %v", code, resp)
	}
	tok := resp["access_token"].(string)

	res, _ := http.Get(e.url + "/" + e.tid + "/discovery/v2.0/keys")
	keys := readMap(t, res)["keys"].([]any)
	jwk := keys[0].(map[string]any)
	nb, _ := base64.RawURLEncoding.DecodeString(jwk["n"].(string))
	eb, _ := base64.RawURLEncoding.DecodeString(jwk["e"].(string))
	pub := &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: int(new(big.Int).SetBytes(eb).Int64())}
	parts := strings.Split(tok, ".")
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err != nil {
		t.Fatalf("the signature does not match the JWKS: %v", err)
	}
	var header map[string]any
	hb, _ := base64.RawURLEncoding.DecodeString(parts[0])
	_ = json.Unmarshal(hb, &header)
	if header["alg"] != "RS256" || header["kid"] != jwk["kid"] {
		t.Errorf("header: %v", header)
	}

	c := claimsOf(t, tok)
	sp, ok := e.world.FindServicePrincipal(client1)
	if !ok || !sp.Auto || sp.Mode != "open" {
		t.Fatalf("the client has no auto-created service principal: %+v", sp)
	}
	for k, want := range map[string]any{
		"aud": permissions.GraphAppID, "tid": e.tid, "appid": client1, "azp": client1, "idtyp": "app",
		"oid": sp.ID, "sub": sp.ID, "ver": "2.0", "iss": e.url + "/" + e.tid + "/v2.0",
	} {
		if c[k] != want {
			t.Errorf("claim %s = %v, want %v", k, c[k], want)
		}
	}
	if _, has := c["roles"]; has {
		t.Errorf("a client with no permissions has no roles: %v", c["roles"])
	}
	if exp, iat := c["exp"].(float64), c["iat"].(float64); exp-iat != 3599 {
		t.Errorf("lifetime %v", exp-iat)
	}

	// Graph accepts it, and counts the call for the real client ID
	if code, body := e.graphGet("/v1.0/users?$top=1", tok); code != 200 {
		t.Fatalf("Graph: %d %s", code, body)
	}
	usage := e.admin("GET", "/_dolly/graph/usage")["servicePrincipals"].([]any)
	if len(usage) != 1 || usage[0].(map[string]any)["appId"] != client1 {
		t.Errorf("usage: %v", usage)
	}

	// the application permissions that are assigned are the roles
	if err := e.world.GrantPermission(client1, permissions.GraphAppID, permissions.Application, "User.Read.All", graph.Actor{Source: "admin"}); err != nil {
		t.Fatal(err)
	}
	if err := e.world.GrantPermission(client1, permissions.GraphAppID, permissions.Application, "Group.Read.All", graph.Actor{Source: "admin"}); err != nil {
		t.Fatal(err)
	}
	_, resp = e.appToken(client1, "s3cret")
	roles, _ := claimsOf(t, resp["access_token"].(string))["roles"].([]any)
	if len(roles) != 2 || roles[0] != "Group.Read.All" || roles[1] != "User.Read.All" {
		t.Errorf("roles: %v", roles)
	}
}

func TestClientCredentials_Errors(t *testing.T) {
	e := setup(t)
	path := "/" + e.tid + "/oauth2/v2.0/token"
	for _, c := range []struct {
		name string
		form url.Values
		want string
	}{
		{"no client ID", url.Values{"grant_type": {"client_credentials"}, "scope": {graphScope}}, "AADSTS900144"},
		{"a client ID that is not a GUID", url.Values{"grant_type": {"client_credentials"}, "client_id": {"my-app"}, "client_secret": {"x"}, "scope": {graphScope}}, "AADSTS700016"},
		{"a scope that is not .default", url.Values{"grant_type": {"client_credentials"}, "client_id": {client1}, "client_secret": {"x"}, "scope": {"https://graph.microsoft.com/User.Read"}}, "AADSTS1002012"},
		{"a resource that is not there", url.Values{"grant_type": {"client_credentials"}, "client_id": {client1}, "client_secret": {"x"}, "scope": {"https://nothing.example/.default"}}, "AADSTS500011"},
		{"a grant that dolly has no flow for", url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}, "client_id": {client1}}, "AADSTS70003"},
	} {
		code, m := e.post(path, c.form)
		if code != 400 || errorCode(m) != c.want || m["trace_id"] == "" || !strings.Contains(m["error_description"].(string), c.want) {
			t.Errorf("%s: %d %v", c.name, code, m)
		}
	}
	// the client with the bad scope was made a service principal anyway, as Entra would know it
	if code, _ := e.post("/other.example/oauth2/v2.0/token", url.Values{}); code != 400 {
		t.Errorf("a tenant that is not there: %d", code)
	}
	// the v1 endpoint asks for a resource
	code, resp := e.post("/"+e.tid+"/oauth2/token", url.Values{"grant_type": {"client_credentials"}, "client_id": {client1}, "client_secret": {"x"}, "resource": {"https://graph.microsoft.com"}})
	if code != 200 || claimsOf(t, resp["access_token"].(string))["ver"] != "1.0" {
		t.Errorf("v1: %d %v", code, resp)
	}
	// basic authentication
	req, _ := http.NewRequest("POST", e.url+path, strings.NewReader(url.Values{"grant_type": {"client_credentials"}, "scope": {graphScope}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(client1, "x")
	res, _ := http.DefaultClient.Do(req)
	if m := readMap(t, res); res.StatusCode != 200 || claimsOf(t, m["access_token"].(string))["appid"] != client1 {
		t.Errorf("basic: %d %v", res.StatusCode, m)
	}
}

// What a person sets on the page decides what the client may show: "accept the next secret".
func TestCredentialCapture(t *testing.T) {
	e := setup(t)
	if code, _ := e.appToken(client1, ""); code != 200 {
		t.Fatalf("a public client gets in while the state is any: %d", code)
	}
	e.admin("POST", "/_dolly/graph/service-principals/"+client1+"/credential/arm")

	if code, _ := e.appToken(client1, ""); code != 200 {
		t.Errorf("an armed service principal waits for a secret, and accepts a call without one")
	}
	if code, _ := e.appToken(client1, "first"); code != 200 {
		t.Fatalf("the secret that arrives first is kept: %d", code)
	}
	sp, _ := e.world.FindServicePrincipal(client1)
	if sp.Credential.State != "captured" || sp.Credential.Hint != "firs" || sp.Credential.Hash == "first" {
		t.Errorf("credential: %+v", sp.Credential)
	}
	code, m := e.appToken(client1, "second")
	if code != 401 || m["error"] != "invalid_client" || errorCode(m) != "AADSTS7000215" {
		t.Errorf("another secret: %d %v", code, m)
	}
	if code, m = e.appToken(client1, ""); code != 401 || errorCode(m) != "AADSTS7000218" {
		t.Errorf("no secret: %d %v", code, m)
	}
	if code, _ := e.appToken(client1, "first"); code != 200 {
		t.Errorf("the kept secret: %d", code)
	}
	e.admin("DELETE", "/_dolly/graph/service-principals/"+client1+"/credential")
	if code, _ := e.appToken(client1, "second"); code != 200 {
		t.Errorf("after the credential is cleared any secret works: %d", code)
	}
}

func TestCredentialCapture_Certificate(t *testing.T) {
	e := setup(t)
	assertion := func(thumb string) string {
		h, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "x5t#S256": thumb})
		p, _ := json.Marshal(map[string]string{"iss": client1, "sub": client1})
		return base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(p) + ".c2ln"
	}
	send := func(thumb string) (int, map[string]any) {
		return e.post("/"+e.tid+"/oauth2/v2.0/token", url.Values{
			"grant_type": {"client_credentials"}, "scope": {graphScope},
			"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"}, "client_assertion": {assertion(thumb)},
		}) // no client_id: it is in the assertion
	}
	if code, resp := send("AAA"); code != 200 || claimsOf(t, resp["access_token"].(string))["appidacr"] != "2" {
		t.Fatalf("certificate: %d %v", code, resp)
	}
	e.admin("POST", "/_dolly/graph/service-principals/"+client1+"/credential/arm")
	send("AAA")
	if code, m := send("BBB"); code != 401 || errorCode(m) != "AADSTS700027" {
		t.Errorf("another certificate: %d %v", code, m)
	}
	if code, m := e.appToken(client1, "AAA"); code != 401 {
		t.Errorf("a secret is not the kept certificate: %d %v", code, m)
	}
	if code, _ := send("AAA"); code != 200 {
		t.Errorf("the kept certificate: %d", code)
	}
}

func TestPasswordGrant_DelegatedToken(t *testing.T) {
	e := setup(t)
	user := e.world.ListUsers()[0]
	upn, id := user["userPrincipalName"].(string), user["id"].(string)
	path := "/" + e.tid + "/oauth2/v2.0/token"
	form := func(scope string) url.Values {
		return url.Values{"grant_type": {"password"}, "client_id": {client1}, "username": {upn}, "password": {"anything"}, "scope": {scope}}
	}

	code, resp := e.post(path, form("https://graph.microsoft.com/User.Read https://graph.microsoft.com/Mail.Read openid profile"))
	if code != 200 {
		t.Fatalf("password: %d %v", code, resp)
	}
	c := claimsOf(t, resp["access_token"].(string))
	if c["scp"] != "Mail.Read User.Read" || c["oid"] != id || c["upn"] != upn || c["aud"] != permissions.GraphAppID || c["idtyp"] != nil {
		t.Errorf("claims: %v", c)
	}
	idc := claimsOf(t, resp["id_token"].(string))
	if idc["aud"] != client1 || idc["preferred_username"] != upn || idc["oid"] != id {
		t.Errorf("id token: %v", idc)
	}
	if code, body := e.graphGet("/v1.0/me", resp["access_token"].(string)); code != 200 || !strings.Contains(body, upn) {
		t.Errorf("Graph with a delegated token: %d %s", code, body)
	}

	// a scope with no resource is a scope of Graph; no resource scope at all gives Graph too
	if _, r := e.post(path, form("User.Read")); claimsOf(t, r["access_token"].(string))["scp"] != "User.Read" {
		t.Errorf("a bare scope: %v", r)
	}
	if code, r := e.post(path, form("openid")); code != 200 || claimsOf(t, r["access_token"].(string))["aud"] != permissions.GraphAppID {
		t.Errorf("openid only: %d %v", code, r)
	}

	// .default is what is consented
	if _, r := e.post(path, form(graphScope)); claimsOf(t, r["access_token"].(string))["scp"] != "" {
		t.Errorf("no consent, no scopes: %v", r)
	}
	if _, perr := e.world.CreateOAuth2Grant(map[string]any{
		"clientId": mustSP(t, e, client1)["id"], "consentType": "AllPrincipals", "resourceId": mustSP(t, e, permissions.GraphAppID)["id"], "scope": "User.ReadBasic.All",
	}, graph.Actor{Source: "admin"}); perr != nil {
		t.Fatal(perr)
	}
	if _, r := e.post(path, form(graphScope)); claimsOf(t, r["access_token"].(string))["scp"] != "User.ReadBasic.All" {
		t.Errorf("consented scopes: %v", r)
	}

	for _, c := range []struct {
		name string
		form url.Values
		want string
	}{
		{"a scope that is not a permission", form("https://graph.microsoft.com/Bogus.Read"), "AADSTS70011"},
		{"a user that is not there", url.Values{"grant_type": {"password"}, "client_id": {client1}, "username": {"nobody@acme.example"}, "password": {"x"}, "scope": {"User.Read"}}, "AADSTS50034"},
		{"no password", url.Values{"grant_type": {"password"}, "client_id": {client1}, "username": {upn}, "scope": {"User.Read"}}, "AADSTS900144"},
	} {
		if code, m := e.post(path, c.form); code != 400 || errorCode(m) != c.want {
			t.Errorf("%s: %d %v", c.name, code, m)
		}
	}
	if err := e.world.SetUser(upn, map[string]any{"accountEnabled": false}, graph.Actor{Source: "admin"}); !err {
		t.Fatal("cannot disable the user")
	}
	if code, m := e.post(path, form("User.Read")); code != 400 || errorCode(m) != "AADSTS50057" {
		t.Errorf("a disabled user: %d %v", code, m)
	}
}

func mustSP(t *testing.T, e *env, appID string) map[string]any {
	t.Helper()
	for _, o := range e.world.ListServicePrincipals() {
		if strings.EqualFold(o["appId"].(string), appID) {
			return o
		}
	}
	t.Fatalf("no service principal %s", appID)
	return nil
}

// Graph checks the tokens that this Dolly made, and lets all other tokens through.
func TestGraphChecksItsOwnTokens(t *testing.T) {
	e := setup(t)
	_, resp := e.appToken(client1, "x")
	tok := resp["access_token"].(string)
	parts := strings.Split(tok, ".")

	if code, _ := e.graphGet("/v1.0/users?$top=1", tok); code != 200 {
		t.Fatalf("a good token: %d", code)
	}
	// a token that says it is another client, with the same signature
	forged := claimsOf(t, tok)
	forged["appid"] = "99999999-2222-3333-4444-555555555555"
	pb, _ := json.Marshal(forged)
	bad := parts[0] + "." + base64.RawURLEncoding.EncodeToString(pb) + "." + parts[2]
	if code, body := e.graphGet("/v1.0/users?$top=1", bad); code != 401 || !strings.Contains(body, "InvalidAuthenticationToken") || !strings.Contains(body, "signature") {
		t.Errorf("a changed token: %d %s", code, body)
	}
	// any other token is not checked
	if code, _ := e.graphGet("/v1.0/users?$top=1", "x"); code != 200 {
		t.Errorf("a foreign token: %d", code)
	}
	foreign := "eyJhbGciOiJub25lIiwia2lkIjoiYSJ9." + base64.RawURLEncoding.EncodeToString([]byte(`{"appid":"aaaaaaaa-2222-3333-4444-555555555555"}`)) + ".x"
	if code, _ := e.graphGet("/v1.0/users?$top=1", foreign); code != 200 {
		t.Errorf("a foreign JWT: %d", code)
	}
	// the token ends
	e.clock.Add(3600)
	if code, body := e.graphGet("/v1.0/users?$top=1", tok); code != 401 || !strings.Contains(body, "expired") {
		t.Errorf("an old token: %d %s", code, body)
	}
}

func TestKeyStaysAfterRestart(t *testing.T) {
	dir := t.TempDir()
	w := graph.Generate(graph.Options{Users: 5, Seed: 1})
	a1, err := login.New(w, login.Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	a2, err := login.New(w, login.Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	get := func(a *login.API) string {
		rec := httptest.NewRecorder()
		a.AdminHandler().ServeHTTP(rec, httptest.NewRequest("GET", "/info", nil))
		var m map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &m)
		return m["keyId"].(string)
	}
	if get(a1) == "" || get(a1) != get(a2) {
		t.Errorf("the key changed: %q %q", get(a1), get(a2))
	}
}

// The quick connect for PowerShell is plain text with the address of the server in it. A Host header with a quote in it is refused:
// the text is run by Invoke-Expression.
func TestPwsh_QuickConnect(t *testing.T) {
	e := setup(t)
	res, err := http.Get(e.url + "/pwsh")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	host := strings.TrimPrefix(e.url, "http://")
	for _, want := range []string{
		"Add-MgEnvironment -Name Dolly -AzureADEndpoint '" + e.url + "' -GraphEndpoint '" + e.url + "'",
		"$token = (iwr '" + e.url + "/jwtToken?hint=" + repo.AdminUPN + "').Content",
		"Connect-MgGraph -Environment Dolly -AccessToken (ConvertTo-SecureString $token -AsPlainText -Force)",
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("the script lacks %q:\n%s", want, body)
		}
	}
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/plain") || !strings.Contains(string(body), host) {
		t.Errorf("%d %s\n%s", res.StatusCode, res.Header.Get("Content-Type"), body)
	}

	req, _ := http.NewRequest("GET", e.url+"/pwsh", nil)
	req.Host = "x'; Remove-Item -Recurse ~ #"
	bad, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Errorf("a Host header that is not a host: %d, want 400", bad.StatusCode)
	}
	if post, _ := http.Post(e.url+"/pwsh", "text/plain", nil); post != nil {
		defer post.Body.Close()
		if post.StatusCode == 200 {
			t.Error("only GET gives the script")
		}
	}
}

// With ?user= the quick connect signs in as that user: the script has the password grant, and what it asks for gives a token for the
// user (so /me is the user). A user that is not there is a 404, and a name that is not safe in a script never gets into the text.
func TestPwsh_QuickConnectAsAUser(t *testing.T) {
	e := setup(t)
	_, users := e.graphGet("/v1.0/users?$top=1&$select=userPrincipalName", "any")
	var one struct {
		Value []struct{ UserPrincipalName string }
	}
	if err := json.Unmarshal([]byte(users), &one); err != nil || len(one.Value) != 1 {
		t.Fatalf("a user: %v %s", err, users)
	}
	upn := one.Value[0].UserPrincipalName

	res, err := http.Get(e.url + "/pwsh?user=" + url.QueryEscape(upn))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if want := "$token = (iwr '" + e.url + "/jwtToken?hint=" + upn + "').Content"; !strings.Contains(string(body), want) {
		t.Errorf("the script lacks %q:\n%s", want, body)
	}
	if strings.Contains(string(body), "NoWelcome") {
		t.Error("the script must not have -NoWelcome")
	}

	for _, ref := range []string{"nobody@nowhere.example", "x'; Remove-Item ~ #"} {
		bad, err := http.Get(e.url + "/pwsh?user=" + url.QueryEscape(ref))
		if err != nil {
			t.Fatal(err)
		}
		msg, _ := io.ReadAll(bad.Body)
		bad.Body.Close()
		if bad.StatusCode != http.StatusNotFound || strings.ContainsAny(string(msg), ";#~") {
			t.Errorf("%q: %d %s", ref, bad.StatusCode, msg)
		}
	}
}

// A client with the flag hideCalls (the portal has it) signs in, gets its token, and is not in the list of sign-ins.
func TestSignInsList_LeavesOutAClientThatIsHidden(t *testing.T) {
	e := setup(t)
	if code, _ := e.appToken(graph.PortalAppID, "x"); code != 200 {
		t.Fatalf("the portal must get a token: %d", code)
	}
	e.appToken(client1, "x")
	list := e.admin("GET", "/_dolly/login/signins")["signins"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["appId"] != client1 {
		t.Errorf("only the client that is not hidden is in the list: %v", list)
	}
}

func TestSignInsList(t *testing.T) {
	e := setup(t)
	e.appToken(client1, "x")
	e.appToken("not a guid", "x")
	list := e.admin("GET", "/_dolly/login/signins")["signins"].([]any)
	if len(list) != 2 {
		t.Fatalf("signins: %v", list)
	}
	newest, oldest := list[0].(map[string]any), list[1].(map[string]any)
	if newest["result"] != "AADSTS700016" || oldest["result"] != "issued" || oldest["registered"] != true || oldest["resource"] != "Microsoft Graph" || oldest["credential"] != "secret" {
		t.Errorf("newest %v, oldest %v", newest, oldest)
	}
	for _, v := range list {
		if strings.Contains(jsonNum(v), `"x"`) {
			t.Errorf("the list must not keep a secret: %v", v)
		}
	}
	e.admin("DELETE", "/_dolly/login/signins")
	if list = e.admin("GET", "/_dolly/login/signins")["signins"].([]any); len(list) != 0 {
		t.Errorf("not cleared: %v", list)
	}
	info := e.admin("GET", "/_dolly/login/info")
	if info["tenantId"] != e.tid || info["token"] != e.url+"/"+e.tid+"/oauth2/v2.0/token" {
		t.Errorf("info: %v", info)
	}
}

// /jwtToken gives a token for the admin account (or the user of the hint): Graph takes it, /me is that user, it is good for years, and
// it has the messages for the person who reads it. It has no line break at the end, and a user that is not there is a 404.
func TestJwtToken(t *testing.T) {
	e := setup(t)
	get := func(query string) (int, string) {
		res, err := http.Get(e.url + "/jwtToken" + query)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}

	code, tok := get("?hint=" + url.QueryEscape(repo.AdminUPN))
	if code != 200 || strings.ContainsAny(tok, " \r\n") || strings.Count(tok, ".") != 2 {
		t.Fatalf("%d %q", code, tok)
	}
	c := claimsOf(t, tok)
	if c["upn"] != repo.AdminUPN || c["appid"] != repo.GraphPowerShellAppID || !strings.Contains(c["scp"].(string), "User.Read") {
		t.Errorf("claims: %v", c)
	}
	if years := (c["exp"].(float64) - c["iat"].(float64)) / (365 * 24 * 3600); years < 9 {
		t.Errorf("the token lasts %.1f years", years)
	}
	if c["message"] != "Thank you for using this application!" || !strings.Contains(c["hello"].(string), "youtube.com") {
		t.Errorf("the messages: %v", c)
	}
	e.clock.Add(3600 * 24 * 365) // a year later
	if code, body := e.graphGet("/v1.0/me", tok); code != 200 || !strings.Contains(body, repo.AdminUPN) {
		t.Errorf("/me: %d %s", code, body)
	}
	if _, def := get(""); claimsOf(t, def)["upn"] != repo.AdminUPN {
		t.Error("no hint must give the admin account")
	}
	if code, _ := get("?hint=" + url.QueryEscape("nobody@nowhere.example")); code != http.StatusNotFound {
		t.Errorf("a user that is not there: %d", code)
	}
	if list := e.admin("GET", "/_dolly/login/signins")["signins"].([]any); len(list) != 0 {
		t.Errorf("a ready-made token is not a sign-in: %v", list)
	}
}
