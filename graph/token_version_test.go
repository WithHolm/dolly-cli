package graph_test

import (
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

// requestedAccessTokenVersion is null, 1, or 2, and it must be 2 when signInAudience lets personal Microsoft accounts in (Learn: apiApplication). A call that makes an
// app personal and says nothing about the version sets it to 2: a guess, Learn gives the rule only.

func TestAccessTokenVersion_PersonalAccountsNeedVersion2(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	version := func(id string) any {
		_, got := graphCall(t, ts, "GET", "/applications/"+id, "")
		return got["api"].(map[string]any)["requestedAccessTokenVersion"]
	}

	// a new app for personal accounts gets 2, and one for the tenant only has null
	s, personal := graphCall(t, ts, "POST", "/applications", `{"displayName":"Personal","signInAudience":"AzureADandPersonalMicrosoftAccount"}`)
	if s != 201 || version(personal["id"].(string)) != float64(2) {
		t.Fatalf("a new personal app: %d, version %v", s, version(personal["id"].(string)))
	}
	s, tenant := graphCall(t, ts, "POST", "/applications", `{"displayName":"Tenant"}`)
	if s != 201 || version(tenant["id"].(string)) != nil {
		t.Fatalf("a new app: %d, version %v", s, version(tenant["id"].(string)))
	}

	for _, c := range []struct{ id, body, want string }{
		{personal["id"].(string), `{"api":{"requestedAccessTokenVersion":1}}`, "requestedAccessTokenVersion"},
		{personal["id"].(string), `{"api":{"requestedAccessTokenVersion":null}}`, "requestedAccessTokenVersion"},
		{tenant["id"].(string), `{"signInAudience":"PersonalMicrosoftAccount","api":{"requestedAccessTokenVersion":1}}`, "requestedAccessTokenVersion"},
	} {
		if st, out := graphCall(t, ts, "PATCH", "/applications/"+c.id, c.body); st != 400 || !strings.Contains(errText(out), c.want) {
			t.Errorf("%s: %d %q", c.body, st, errText(out))
		}
	}
	if version(tenant["id"].(string)) != nil {
		t.Error("a call that is refused changes nothing")
	}

	// a call that makes the app personal sets 2, and 2 is fine
	if st, out := graphCall(t, ts, "PATCH", "/applications/"+tenant["id"].(string), `{"signInAudience":"AzureADandPersonalMicrosoftAccount"}`); st != 204 {
		t.Fatalf("make it personal: %d %v", st, out)
	}
	if version(tenant["id"].(string)) != float64(2) {
		t.Errorf("after it is personal: %v", version(tenant["id"].(string)))
	}
	if st, out := graphCall(t, ts, "PATCH", "/applications/"+personal["id"].(string), `{"api":{"requestedAccessTokenVersion":2}}`); st != 204 {
		t.Errorf("2 for a personal app: %d %v", st, out)
	}
}

func TestAdminAPI_AccessTokenVersion(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	s, app := graphCall(t, ts, "POST", "/applications", `{"displayName":"Files API"}`)
	if s != 201 {
		t.Fatal(s, app)
	}
	base := "/applications/" + app["id"].(string)
	var got exposedAnswer
	for payload, want := range map[string]any{`{"version":2}`: float64(2), `{"version":1}`: float64(1), `{"version":null}`: nil} {
		if st, body := adminRaw(t, ts.URL, "PUT", base+"/access-token-version", payload); st != 204 {
			t.Fatalf("%s: %d %s", payload, st, body)
		}
		admin(t, ts, "GET", base+"/api", "", &got)
		if got.Version != want {
			t.Errorf("%s: the app has %v, want %v", payload, got.Version, want)
		}
	}
	for _, payload := range []string{`{"version":3}`, `{"version":"2"}`, `{}`} {
		if st, body := adminRaw(t, ts.URL, "PUT", base+"/access-token-version", payload); st != 400 || !strings.Contains(body, "requestedAccessTokenVersion") {
			t.Errorf("%s: %d %s", payload, st, body)
		}
	}
	if st, _ := adminRaw(t, ts.URL, "PUT", "/applications/22222222-2222-2222-2222-222222222222/access-token-version", `{"version":2}`); st != 404 {
		t.Errorf("an app that is not there: %d", st)
	}
	// the rule of personal accounts is the one of the Graph API
	s, personal := graphCall(t, ts, "POST", "/applications", `{"displayName":"Personal","signInAudience":"PersonalMicrosoftAccount"}`)
	if s != 201 {
		t.Fatal(s, personal)
	}
	if st, body := adminRaw(t, ts.URL, "PUT", "/applications/"+personal["id"].(string)+"/access-token-version", `{"version":1}`); st != 400 {
		t.Errorf("1 for a personal app: %d %s", st, body)
	}
}
