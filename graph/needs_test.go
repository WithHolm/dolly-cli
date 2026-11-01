package graph

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph/permissions"
)

// jwt makes a token that is not signed. dollycli takes the client and the kind from its claims.
func jwt(claims map[string]any) string {
	b, _ := json.Marshal(claims)
	return "h." + base64.RawURLEncoding.EncodeToString(b) + ".s"
}

func callAs(h http.Handler, token, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://127.0.0.1"+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// send sends a request to the admin API (or any route) with a body, the way the admin API wants a write.
func send(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://127.0.0.1"+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer any")
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func needsOf(t *testing.T, h http.Handler, app string) Needs {
	t.Helper()
	var n Needs
	rec := send(h, "GET", "/_dolly/service-principals/"+app+"/needs", "")
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &n) != nil {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	return n
}

func kindOf(n Needs, kind string) *KindNeeds {
	for i := range n.Kinds {
		if n.Kinds[i].Kind == kind {
			return &n.Kinds[i]
		}
	}
	return nil
}

func rowOf(k *KindNeeds, method, path string) *NeedRow {
	if k == nil {
		return nil
	}
	for i := range k.Rows {
		if k.Rows[i].Method == method && k.Rows[i].Path == path {
			return &k.Rows[i]
		}
	}
	return nil
}

// The calls of a client are noted by kind of token, and each one has the permissions that its page of Learn says.
func TestNeeds_AClientAndWhatItsCallsNeed(t *testing.T) {
	w := Generate(Options{Users: 10, Seed: 1})
	h := New(w, ServerOptions{}).Handler()
	app := w.ListServicePrincipals()[0]["appId"].(string)
	delegated := jwt(map[string]any{"appid": app, "scp": "User.Read"})
	application := jwt(map[string]any{"appid": app, "roles": []string{"User.Read.All"}})
	for range 3 {
		callAs(h, application, "GET", "/v1.0/users")
	}
	callAs(h, application, "GET", "/v1.0/groups")
	callAs(h, delegated, "GET", "/v1.0/organization")
	callAs(h, delegated, "GET", "/v1.0/users/00000000-0000-0000-0000-00000000dead") // a 404 still needs the permission
	callAs(h, jwt(map[string]any{"appid": app}), "GET", "/v1.0/organization")       // a token that says neither

	n := needsOf(t, h, app)
	if n.UnknownCalls != 1 || n.Source.Commit == "" || len(n.Kinds) != 2 || n.Kinds[0].Kind != "delegated" || n.Kinds[1].Kind != "application" {
		t.Fatalf("%+v", n)
	}
	apps := kindOf(n, "application")
	users := rowOf(apps, "GET", "/v1.0/users")
	if users == nil || users.Calls != 3 || users.Status != "missing" || !slices.Equal(users.Least, []string{"User.Read.All"}) || users.Doc == "" {
		t.Fatalf("GET /users as an application: %+v", users)
	}
	if apps.Calls != 4 || apps.Rows[0].Path != "/v1.0/users" {
		t.Errorf("the rows are by calls: %+v", apps.Rows)
	}
	// the least privileged permission of GET /groups is newer than the list of permissions that dollycli has: it cannot be granted
	if !slices.Contains(apps.Minimum, "User.Read.All") || len(apps.Missing)+len(apps.Unavailable) != len(apps.Minimum) || !slices.Contains(apps.Missing, "User.Read.All") {
		t.Errorf("minimum %v, missing %v, unavailable %v", apps.Minimum, apps.Missing, apps.Unavailable)
	}
	if cat, err := permissions.Load(); err == nil {
		for _, p := range apps.Missing {
			if _, ok := cat.Find(permissions.Application, p); !ok {
				t.Errorf("%s is in missing, and dollycli does not have it", p)
			}
		}
		for _, p := range apps.Unavailable {
			if _, ok := cat.Find(permissions.Application, p); ok {
				t.Errorf("%s is in unavailable, and dollycli has it", p)
			}
		}
	}
	del := kindOf(n, "delegated")
	org, bad := rowOf(del, "GET", "/v1.0/organization"), rowOf(del, "GET", "/v1.0/users/{id}")
	if org == nil || !slices.Equal(org.Least, []string{"User.Read"}) || org.Status != "missing" {
		t.Fatalf("GET /organization as a user: %+v", org)
	}
	if bad == nil || bad.Errors != 1 || bad.Calls != 1 {
		t.Errorf("the 404: %+v", bad)
	}
	// a token that is another client has its own list
	if other := needsOf(t, h, "ffffffff-0000-0000-0000-000000000000"); len(other.Kinds) != 0 {
		t.Errorf("another client: %+v", other)
	}
}

// A page with one table for the signed-in user and one for another user: the route decides which one a call is judged by.
func TestNeeds_ThePathPicksTheScenarioOfAPage(t *testing.T) {
	w := Generate(Options{Users: 10, Seed: 1})
	h := New(w, ServerOptions{}).Handler()
	app := w.ListServicePrincipals()[0]["appId"].(string)
	application := jwt(map[string]any{"appid": app, "roles": []string{"x"}})
	user := w.ListUsers()[0]["id"].(string)
	callAs(h, application, "GET", "/v1.0/users/"+user+"/memberOf")
	apps := kindOf(needsOf(t, h, app), "application")
	r := rowOf(apps, "GET", "/v1.0/users/{id}/memberOf")
	if r == nil || r.Status != "missing" || !slices.Equal(r.Least, []string{"Directory.Read.All"}) || r.Page != "user-list-memberof" || !strings.HasSuffix(r.Doc, "/graph/api/user-list-memberof?view=graph-rest-1.0") {
		t.Fatalf("another user, as an application: %+v", r)
	}
	if len(r.Variants) != 0 {
		t.Errorf("the path decided, so nothing varies: %+v", r.Variants)
	}
}

// What the client has been granted decides covered or missing: a higher privileged permission covers a call too, and a permission for the other kind does not.
func TestNeeds_GrantsDecideWhatIsMissing(t *testing.T) {
	w := Generate(Options{Users: 10, Seed: 1})
	h := New(w, ServerOptions{}).Handler()
	app := w.ListServicePrincipals()[0]["appId"].(string)
	application := jwt(map[string]any{"appid": app, "roles": []string{"x"}})
	callAs(h, application, "GET", "/v1.0/users")
	callAs(h, application, "GET", "/v1.0/groups")

	grant := func(kind, value string) {
		t.Helper()
		body := fmt.Sprintf(`{"resourceAppId":%q,"kind":%q,"value":%q}`, permissions.GraphAppID, kind, value)
		if rec := send(h, "POST", "/_dolly/service-principals/"+app+"/grants", body); rec.Code != 200 && rec.Code != 201 && rec.Code != 204 {
			t.Fatalf("grant %s: %d %s", value, rec.Code, rec.Body)
		}
	}
	grant("delegated", "User.Read.All") // the other kind: it changes nothing here
	k := kindOf(needsOf(t, h, app), "application")
	if r := rowOf(k, "GET", "/v1.0/users"); r.Status != "missing" {
		t.Fatalf("a delegated grant does not cover an application call: %+v", r)
	}

	grant("application", "Directory.Read.All") // higher privileged, and it covers both calls
	k = kindOf(needsOf(t, h, app), "application")
	for _, p := range []string{"/v1.0/users", "/v1.0/groups"} {
		if r := rowOf(k, "GET", p); r.Status != "covered" || r.CoveredBy != "Directory.Read.All" {
			t.Errorf("%s: %+v", p, r)
		}
	}
	if len(k.Missing) != 0 || len(k.Minimum) == 0 {
		t.Errorf("nothing is missing, and the least privileged set is still given: minimum %v, missing %v", k.Minimum, k.Missing)
	}
}

// A permission that is assumed to include another one (implies.json): the minimum has it, and not both.
func TestNeeds_AnAssumedPermissionIsNotCountedTwice(t *testing.T) {
	w := Generate(Options{Users: 10, Seed: 1})
	h := New(w, ServerOptions{}).Handler()
	app := w.ListServicePrincipals()[0]["appId"].(string)
	application := jwt(map[string]any{"appid": app, "roles": []string{"x"}})
	user := w.ListUsers()[0]["id"].(string)
	callAs(h, application, "GET", "/v1.0/users")
	callAs(h, application, "PATCH", "/v1.0/users/"+user)

	k := kindOf(needsOf(t, h, app), "application")
	if !slices.Equal(k.Minimum, []string{"User.ReadUpdate.All"}) {
		t.Fatalf("one permission covers the read and the update: %v", k.Minimum)
	}
	if len(k.Assumed) != 1 || k.Assumed[0].Permission != "User.ReadUpdate.All" || !slices.Contains(k.Assumed[0].Includes, "User.Read.All") {
		t.Errorf("the minimum says that it rests on a guess: %+v", k.Assumed)
	}
}

// The compare with what the client has: unused permissions, and a higher one that a lower one could replace.
func TestNeeds_UnusedAndLower(t *testing.T) {
	w := Generate(Options{Users: 10, Seed: 1})
	h := New(w, ServerOptions{}).Handler()
	app := w.ListServicePrincipals()[0]["appId"].(string)
	application := jwt(map[string]any{"appid": app, "roles": []string{"x"}})
	callAs(h, application, "GET", "/v1.0/users")

	k := kindOf(needsOf(t, h, app), "application")
	if k.Granted != 0 || len(k.Unused) != 0 || len(k.Lower) != 0 {
		t.Fatalf("nothing granted, nothing to compare: %+v", k)
	}
	for _, p := range []string{"Directory.Read.All", "Mail.Read"} {
		body := fmt.Sprintf(`{"resourceAppId":%q,"kind":"application","value":%q}`, permissions.GraphAppID, p)
		if rec := send(h, "POST", "/_dolly/service-principals/"+app+"/grants", body); rec.Code > 299 {
			t.Fatalf("grant %s: %d %s", p, rec.Code, rec.Body)
		}
	}
	k = kindOf(needsOf(t, h, app), "application")
	if k.Granted != 2 || !slices.Equal(k.Unused, []string{"Mail.Read"}) {
		t.Errorf("Mail.Read is named by no call: granted %d, unused %v", k.Granted, k.Unused)
	}
	if len(k.Lower) != 1 || k.Lower[0].From != "Directory.Read.All" || len(k.Lower[0].To) == 0 || k.Lower[0].Calls != 1 {
		t.Errorf("Directory.Read.All covers GET /users, and a lower one would do: %+v", k.Lower)
	}
}

// Another kind of call: the forget calls, and the list of clients is limited.
func TestNeeds_ForgetAndTheLimit(t *testing.T) {
	h := New(Generate(Options{Users: 10, Seed: 1}), ServerOptions{}).Handler()
	app := "992c0dfd-a9fe-4265-b4f5-17a0f7fc2f03"
	callAs(h, jwt(map[string]any{"appid": app, "roles": []string{"x"}}), "GET", "/v1.0/users")
	if len(needsOf(t, h, app).Kinds) != 1 {
		t.Fatal("noted")
	}
	if rec := send(h, "DELETE", "/_dolly/service-principals/"+strings.ToUpper(app)+"/needs", ""); rec.Code != 204 { // the case of the ID does not matter
		t.Fatal(rec.Code)
	}
	if len(needsOf(t, h, app).Kinds) != 0 {
		t.Error("forgotten")
	}
	callAs(h, jwt(map[string]any{"appid": app, "roles": []string{"x"}}), "GET", "/v1.0/users")
	send(h, "DELETE", "/_dolly/usage", "") // the reset of the use forgets it too
	if len(needsOf(t, h, app).Kinds) != 0 {
		t.Error("forgotten with the use")
	}
	// a client ID comes from the token: only 200 clients are noted
	for i := range 230 {
		callAs(h, jwt(map[string]any{"appid": fmt.Sprintf("00000000-0000-0000-0000-%012d", i), "roles": []string{"x"}}), "GET", "/v1.0/users")
	}
	noted := 0
	for i := range 230 {
		noted += len(needsOf(t, h, fmt.Sprintf("00000000-0000-0000-0000-%012d", i)).Kinds)
	}
	if noted != maxNeedApps {
		t.Errorf("%d clients noted, want %d", noted, maxNeedApps)
	}
}

func TestMinimumSet(t *testing.T) {
	for name, c := range map[string]struct {
		calls [][]string
		have  []string
		want  []string
	}{
		"nothing":                        {nil, nil, []string{}},
		"one permission serves two":      {[][]string{{"A", "B"}, {"B", "C"}}, nil, []string{"B"}},
		"the one that serves more":       {[][]string{{"A", "X"}, {"B", "X"}, {"C", "X"}, {"A"}}, nil, []string{"A", "X"}},
		"a call with one way goes first": {[][]string{{"A", "B"}, {"B"}}, nil, []string{"B"}},
		"both are needed":                {[][]string{{"A and B"}, {"C"}}, nil, []string{"A", "B", "C"}},
		"an alternative with fewer":      {[][]string{{"A and B", "C"}}, nil, []string{"C"}},
		"what is had costs nothing":      {[][]string{{"A", "B"}, {"C"}}, []string{"b"}, []string{"C"}},
		"all had":                        {[][]string{{"A"}}, []string{"a"}, []string{}},
		"a part of an and is had":        {[][]string{{"A and B", "C and D"}}, []string{"a"}, []string{"B"}},
	} {
		if got := minimumSet(c.calls, setOf(c.have)); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
	// every call has a covered alternative in the result (checked on a larger set)
	calls := [][]string{{"A", "B", "C"}, {"B", "D"}, {"C", "D", "E"}, {"E"}, {"A and F", "G"}, {"G", "H"}}
	got := minimumSet(calls, nil)
	for _, alts := range calls {
		ok := false
		for _, alt := range alts {
			ok = ok || allIn(permissions.Alternatives(alt), setOf(got))
		}
		if !ok {
			t.Errorf("%v does not cover %v", got, alts)
		}
	}
}

func setOf(list []string) map[string]bool {
	m := map[string]bool{}
	for _, p := range list {
		m[strings.ToLower(p)] = true
	}
	return m
}
