package login_test

import (
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"

func challenge(v string) string {
	sum := sha256.Sum256([]byte(v))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

var noRedirect = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

// authParams are what a client sends to /authorize.
func authParams(extra url.Values) url.Values {
	v := url.Values{
		"client_id": {client1}, "response_type": {"code"}, "redirect_uri": {"http://localhost:3000/cb"},
		"scope": {"https://graph.microsoft.com/User.Read openid offline_access"}, "state": {"xyz"}, "nonce": {"n-0S6_WzA2Mj"},
		"code_challenge": {challenge(verifier)}, "code_challenge_method": {"S256"},
	}
	for k, vs := range extra {
		if len(vs) == 0 || vs[0] == "-" {
			v.Del(k)
		} else {
			v[k] = vs
		}
	}
	return v
}

// pick is the person who clicks a user on the page. It gives the answer, not followed.
func (e *env) pick(upn string, params url.Values) *http.Response {
	e.t.Helper()
	params.Set("username", upn)
	res, err := noRedirect.PostForm(e.url+"/"+e.tid+"/oauth2/v2.0/authorize", params)
	if err != nil {
		e.t.Fatal(err)
	}
	return res
}

func (e *env) body(res *http.Response) string {
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	return string(b)
}

// codeOf gives the code from the redirect, and checks that the state came back.
func (e *env) codeOf(res *http.Response) string {
	e.t.Helper()
	if res.StatusCode != http.StatusFound {
		e.t.Fatalf("redirect: %d %s", res.StatusCode, e.body(res))
	}
	u, err := url.Parse(res.Header.Get("Location"))
	if err != nil || u.Host != "localhost:3000" || u.Path != "/cb" || u.Query().Get("state") != "xyz" || u.Query().Get("code") == "" {
		e.t.Fatalf("location: %s", res.Header.Get("Location"))
	}
	return u.Query().Get("code")
}

func (e *env) exchange(code string, extra url.Values) (int, map[string]any) {
	f := url.Values{"grant_type": {"authorization_code"}, "client_id": {client1}, "code": {code}, "redirect_uri": {"http://localhost:3000/cb"}, "code_verifier": {verifier}}
	for k, v := range extra {
		f[k] = v
	}
	return e.post("/"+e.tid+"/oauth2/v2.0/token", f)
}

func TestAuthorizationCodeFlow(t *testing.T) {
	e := setup(t)
	user := e.world.ListUsers()[0]
	upn, id := user["userPrincipalName"].(string), user["id"].(string)
	authURL := e.url + "/" + e.tid + "/oauth2/v2.0/authorize"

	// the page: the client, what it asks for, and the users
	res, err := http.Get(authURL + "?" + authParams(nil).Encode())
	if err != nil {
		t.Fatal(err)
	}
	page := e.body(res)
	if res.StatusCode != 200 || !strings.Contains(page, upn) || !strings.Contains(page, client1) || !strings.Contains(page, "User.Read") || !strings.Contains(page, `name="code_challenge"`) {
		t.Fatalf("page: %d %s", res.StatusCode, page)
	}
	if sp, ok := e.world.FindServicePrincipal(client1); !ok || !sp.Auto {
		t.Errorf("the page makes the service principal: %+v", sp)
	}
	// the search keeps what the request had, and filters
	res, _ = http.Get(authURL + "?" + authParams(url.Values{"login_q": {upn}}).Encode())
	if p := e.body(res); !strings.Contains(p, upn) || strings.Count(p, `name="username"`) != 1 {
		t.Errorf("search: %d users in the page", strings.Count(p, `name="username"`))
	}

	// the person picks a user, and the browser goes back with a code
	code := e.codeOf(e.pick(upn, authParams(nil)))
	status, tok := e.exchange(code, nil)
	if status != 200 {
		t.Fatalf("exchange: %d %v", status, tok)
	}
	c := claimsOf(t, tok["access_token"].(string))
	if c["scp"] != "User.Read" || c["oid"] != id || c["upn"] != upn || c["appid"] != client1 || c["appidacr"] != "0" {
		t.Errorf("claims: %v", c)
	}
	idc := claimsOf(t, tok["id_token"].(string))
	if idc["nonce"] != "n-0S6_WzA2Mj" || idc["aud"] != client1 || idc["oid"] != id {
		t.Errorf("id token: %v", idc)
	}
	if tok["refresh_token"] == nil || tok["scope"] != "User.Read" {
		t.Errorf("answer: %v", tok)
	}

	// /me is the user of the token
	access := tok["access_token"].(string)
	code2, body := e.graphGet("/v1.0/me", access)
	if code2 != 200 || !strings.Contains(body, upn) {
		t.Errorf("/me: %d %s", code2, body)
	}
	if code2, body = e.graphGet("/v1.0/me?$select=displayName", access); code2 != 200 || !strings.Contains(body, user["displayName"].(string)) {
		t.Errorf("/me with select: %d %s", code2, body)
	}
	if code2, body = e.graphGet("/v1.0/me/directReports", access); code2 != 200 {
		t.Errorf("/me/directReports: %d %s", code2, body)
	}
	req, _ := http.NewRequest("POST", e.url+"/v1.0/$batch", strings.NewReader(`{"requests":[{"id":"1","method":"GET","url":"/me"},{"id":"2","method":"GET","url":"/users/`+id+`"}]}`))
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Content-Type", "application/json")
	res, _ = http.DefaultClient.Do(req)
	if b := e.body(res); strings.Count(b, `"status":200`) != 2 || strings.Count(b, `"userPrincipalName":"`+upn+`"`) != 2 {
		t.Errorf("$batch with /me: %s", b)
	}
	if code2, body = e.graphGet("/v1.0/meeting", access); code2 == 200 {
		t.Errorf("/meeting is not /me: %d %s", code2, body)
	}

	// /me is in the reference, with what Graph says about it, and it counts its own calls (the /users/{id} that
	// it runs as is not counted: the batch did GET /users/{id} once)
	calls := map[string]map[string]any{}
	for _, ep := range e.admin("GET", "/_dolly/graph/api")["endpoints"].([]any) {
		m := ep.(map[string]any)
		calls[m["method"].(string)+" "+m["path"].(string)] = m
	}
	if me := calls["GET /v1.0/me"]; me == nil || me["calls"].(float64) != 3 || me["doc"] == nil || !strings.Contains(me["summary"].(string), "delegated token") {
		t.Errorf("GET /v1.0/me: %v", me)
	}
	if dr := calls["GET /v1.0/me/directReports"]; dr == nil || dr["calls"].(float64) != 1 {
		t.Errorf("GET /v1.0/me/directReports: %v", dr)
	}
	if u := calls["GET /v1.0/users/{id}"]; u["calls"].(float64) != 1 {
		t.Errorf("GET /v1.0/users/{id}: %v", u["calls"])
	}
	if _, has := calls["DELETE /v1.0/me"]; has {
		t.Error("Graph has no DELETE /me")
	}

	// the code works once
	if status, m := e.exchange(code, nil); status != 400 || errorCode(m) != "AADSTS70000" {
		t.Errorf("a code used twice: %d %v", status, m)
	}
	// PKCE, the redirect address, and the client are checked
	for _, c := range []struct {
		name  string
		extra url.Values
		want  string
	}{
		{"a wrong verifier", url.Values{"code_verifier": {"nope-nope-nope"}}, "AADSTS501481"},
		{"no verifier", url.Values{"code_verifier": {}}, "AADSTS501481"},
		{"another redirect address", url.Values{"redirect_uri": {"http://localhost:3000/other"}}, "AADSTS50011"},
		{"another client", url.Values{"client_id": {"99999999-2222-3333-4444-555555555555"}}, "AADSTS70000"},
	} {
		if status, m := e.exchange(e.codeOf(e.pick(upn, authParams(nil))), c.extra); status != 400 || errorCode(m) != c.want {
			t.Errorf("%s: %d %v", c.name, status, m)
		}
	}
	if status, m := e.exchange("0.garbage", nil); status != 400 || errorCode(m) != "AADSTS70000" {
		t.Errorf("a code that Dolly never gave: %d %v", status, m)
	}
	// without PKCE, no verifier is needed
	plain := authParams(url.Values{"code_challenge": {"-"}, "code_challenge_method": {"-"}})
	if status, m := e.exchange(e.codeOf(e.pick(upn, plain)), url.Values{"code_verifier": {}}); status != 200 {
		t.Errorf("no PKCE: %d %v", status, m)
	}
	// a client that is captured must still show its secret
	e.admin("POST", "/_dolly/graph/service-principals/"+client1+"/credential/arm")
	e.appToken(client1, "s3cret")
	if status, m := e.exchange(e.codeOf(e.pick(upn, authParams(nil))), nil); status != 401 || errorCode(m) != "AADSTS7000218" {
		t.Errorf("a captured client with no secret: %d %v", status, m)
	}
	if status, _ := e.exchange(e.codeOf(e.pick(upn, authParams(nil))), url.Values{"client_secret": {"s3cret"}}); status != 200 {
		t.Errorf("a captured client with its secret: %d", status)
	}
}

func TestRefreshToken(t *testing.T) {
	e := setup(t)
	user := e.world.ListUsers()[0]
	upn := user["userPrincipalName"].(string)
	_, tok := e.exchange(e.codeOf(e.pick(upn, authParams(nil))), nil)
	rt := tok["refresh_token"].(string)
	path := "/" + e.tid + "/oauth2/v2.0/token"
	refresh := func(token, client string, extra url.Values) (int, map[string]any) {
		f := url.Values{"grant_type": {"refresh_token"}, "client_id": {client}, "refresh_token": {token}}
		for k, v := range extra {
			f[k] = v
		}
		return e.post(path, f)
	}

	e.clock.Add(600)
	status, m := refresh(rt, client1, nil)
	if status != 200 || m["refresh_token"] == nil || claimsOf(t, m["access_token"].(string))["scp"] != "User.Read" || m["scope"] != "User.Read" {
		t.Fatalf("refresh: %d %v", status, m)
	}
	if code, _ := e.graphGet("/v1.0/me", m["access_token"].(string)); code != 200 {
		t.Errorf("a refreshed token works on /me: %d", code)
	}
	// a smaller scope can be asked for
	if _, m = refresh(rt, client1, url.Values{"scope": {"https://graph.microsoft.com/User.Read"}}); m["refresh_token"] != nil {
		t.Errorf("no offline_access in the scope, no new refresh token: %v", m)
	}
	// the token is the client's own
	if status, m = refresh(rt, "99999999-2222-3333-4444-555555555555", nil); status != 400 || errorCode(m) != "AADSTS70000" {
		t.Errorf("another client: %d %v", status, m)
	}
	if status, m = refresh("garbage", client1, nil); status != 400 || errorCode(m) != "AADSTS70000" {
		t.Errorf("garbage: %d %v", status, m)
	}
	// an access token is not a refresh token
	if status, m = refresh(tok["access_token"].(string), client1, nil); status != 400 {
		t.Errorf("an access token as a refresh token: %d %v", status, m)
	}
	// the user goes away
	if !e.world.SetUser(upn, map[string]any{"accountEnabled": false}, graph.Actor{Source: "admin"}) {
		t.Fatal("cannot disable")
	}
	if status, m = refresh(rt, client1, nil); status != 400 || errorCode(m) != "AADSTS50057" {
		t.Errorf("a disabled user: %d %v", status, m)
	}
	e.world.SetUser(upn, map[string]any{"accountEnabled": true}, graph.Actor{Source: "admin"})
	// and the token ends after 90 days
	e.clock.Add(91 * 24 * 3600)
	if status, m = refresh(rt, client1, nil); status != 400 || errorCode(m) != "AADSTS70000" {
		t.Errorf("an old refresh token: %d %v", status, m)
	}
}

func TestAuthorize_Errors(t *testing.T) {
	e := setup(t)
	upn := e.world.ListUsers()[0]["userPrincipalName"].(string)
	authURL := e.url + "/" + e.tid + "/oauth2/v2.0/authorize?"

	// what is wrong with the client or the redirect address stays on the page: nobody can be told by redirect
	for _, c := range []struct {
		name  string
		extra url.Values
		want  string
	}{
		{"no redirect address", url.Values{"redirect_uri": {"-"}}, "AADSTS50011"},
		{"a script as redirect address", url.Values{"redirect_uri": {"javascript:alert(1)"}}, "AADSTS50011"},
		{"a client ID that is not a GUID", url.Values{"client_id": {"my-app"}}, "AADSTS700016"},
	} {
		res, _ := noRedirect.Get(authURL + authParams(c.extra).Encode())
		if b := e.body(res); res.StatusCode != 400 || !strings.Contains(b, c.want) {
			t.Errorf("%s: %d %s", c.name, res.StatusCode, b)
		}
	}
	// the rest goes back to the client as an error, with the state
	for _, c := range []struct {
		name  string
		extra url.Values
		want  string
	}{
		{"response_type token", url.Values{"response_type": {"token"}}, "unsupported_response_type"},
		{"no scope", url.Values{"scope": {"-"}}, "invalid_request"},
		{"a scope that is not a permission", url.Values{"scope": {"https://graph.microsoft.com/Bogus.Read"}}, "invalid_scope"},
		{"prompt none", url.Values{"prompt": {"none"}}, "login_required"},
		{"a method of PKCE that is not one", url.Values{"code_challenge_method": {"md5"}}, "invalid_request"},
	} {
		res, _ := noRedirect.Get(authURL + authParams(c.extra).Encode())
		loc, _ := url.Parse(res.Header.Get("Location"))
		if res.StatusCode != 302 || loc == nil || loc.Query().Get("error") != c.want || loc.Query().Get("state") != "xyz" {
			t.Errorf("%s: %d %s", c.name, res.StatusCode, res.Header.Get("Location"))
		}
	}
	// the choice of the person
	if res := e.pick("nobody@acme.example", authParams(nil)); res.StatusCode != 400 || !strings.Contains(e.body(res), "AADSTS50034") {
		t.Errorf("a user that is not there: %d", res.StatusCode)
	}
	e.world.SetUser(upn, map[string]any{"accountEnabled": false}, graph.Actor{Source: "admin"})
	if res := e.pick(upn, authParams(nil)); res.StatusCode != 400 || !strings.Contains(e.body(res), "AADSTS50057") {
		t.Errorf("a disabled user: %d", res.StatusCode)
	}
	if page, _ := http.Get(authURL + authParams(url.Values{"login_q": {upn}}).Encode()); !strings.Contains(e.body(page), "disabled") {
		t.Error("the page shows a disabled user as disabled")
	}
}

func TestAuthorize_ResponseModes(t *testing.T) {
	e := setup(t)
	upn := e.world.ListUsers()[0]["userPrincipalName"].(string)

	res := e.pick(upn, authParams(url.Values{"response_mode": {"fragment"}}))
	loc, _ := url.Parse(res.Header.Get("Location"))
	if res.StatusCode != 302 || loc.Query().Get("code") != "" || !strings.Contains(loc.Fragment, "code=") || !strings.Contains(loc.Fragment, "state=xyz") {
		t.Errorf("fragment: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	res = e.pick(upn, authParams(url.Values{"response_mode": {"form_post"}}))
	if b := e.body(res); res.StatusCode != 200 || !strings.Contains(b, `action="http://localhost:3000/cb"`) || !strings.Contains(b, `name="code"`) || !strings.Contains(b, `name="state" value="xyz"`) {
		t.Errorf("form_post: %d %s", res.StatusCode, b)
	}
}

func TestMe_NeedsAUserToken(t *testing.T) {
	e := setup(t)
	_, resp := e.appToken(client1, "x")
	code, body := e.graphGet("/v1.0/me", resp["access_token"].(string))
	if code != 400 || !strings.Contains(body, "only valid with delegated authentication flow") {
		t.Errorf("an application token on /me: %d %s", code, body)
	}
	if code, _ = e.graphGet("/v1.0/me", "x"); code != 400 {
		t.Errorf("a token that Dolly did not make on /me: %d", code)
	}
}
