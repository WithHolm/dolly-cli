package graph_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

// The portal sets the Application ID URI, edits the scopes, and pre-authorizes clients through the admin API. These make the same changes as PATCH /applications,
// with the same rules, and the history names the actor "admin".

type exposedAnswer struct {
	IdentifierURIs []string         `json:"identifierUris"`
	Scopes         []map[string]any `json:"scopes"`
	PreAuthorized  []map[string]any `json:"preAuthorizedApplications"`
	Version        any              `json:"requestedAccessTokenVersion"`
}

func TestAdminAPI_URIScopesAndClients(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	s, app := graphCall(t, ts, "POST", "/applications", `{"displayName":"Files API"}`)
	if s != 201 {
		t.Fatal(s, app)
	}
	appID, id := app["appId"].(string), app["id"].(string)
	base := "/applications/" + id

	var got exposedAnswer
	if st := admin(t, ts, "GET", base+"/api", "", &got); st != 200 || len(got.Scopes) != 0 || len(got.IdentifierURIs) != 0 {
		t.Fatalf("empty: %d %+v", st, got)
	}
	// the Application ID URI: the usual one, and a rule of Graph
	if st, body := adminRaw(t, ts.URL, "PUT", base+"/identifier-uris", `{"identifierUris":["api://`+appID+`"]}`); st != 204 {
		t.Fatalf("set the URI: %d %s", st, body)
	}
	if st, body := adminRaw(t, ts.URL, "PUT", base+"/identifier-uris", `{"identifierUris":["myapi"]}`); st != 400 || !strings.Contains(body, "identifierUris") {
		t.Errorf("a URI that is not one: %d %s", st, body)
	}

	// add a scope: it gets an id, it is enabled, and it is for administrators until the call says otherwise
	var made map[string]any
	if st := admin(t, ts, "POST", base+"/api-scopes", `{"value":"Files.Read","adminConsentDisplayName":"Read files","adminConsentDescription":"Read the files of the app."}`, &made); st != 201 {
		t.Fatalf("add: %d", st)
	}
	sid, _ := made["id"].(string)
	if !graphIsGUID(sid) || made["isEnabled"] != true || made["type"] != "Admin" {
		t.Fatalf("the new scope: %v", made)
	}
	for payload, want := range map[string]string{
		`{"value":"Two words","adminConsentDisplayName":"n","adminConsentDescription":"d"}`:             "value",
		`{"value":"Files.Read","adminConsentDisplayName":"n","adminConsentDescription":"d"}`:            "same value",
		`{"value":"X.Y","adminConsentDescription":"d"}`:                                                 "adminConsentDisplayName",
		`{"value":"X.Y","type":"Everyone","adminConsentDisplayName":"n","adminConsentDescription":"d"}`: "type",
		`{"value":"X.Y","origin":"x"}`:                                                                  "origin",
	} {
		if st, body := adminRaw(t, ts.URL, "POST", base+"/api-scopes", payload); st != 400 || !strings.Contains(body, want) {
			t.Errorf("%s: %d %s", payload, st, body)
		}
	}
	if st, body := adminRaw(t, ts.URL, "PATCH", base+"/api-scopes/"+sid, `{"type":"User","userConsentDisplayName":"Read your files","userConsentDescription":"Read your files."}`); st != 204 {
		t.Fatalf("edit: %d %s", st, body)
	}
	admin(t, ts, "GET", base+"/api", "", &got)
	if got.Scopes[0]["type"] != "User" || got.Scopes[0]["userConsentDisplayName"] != "Read your files" || got.Scopes[0]["value"] != "Files.Read" || len(got.IdentifierURIs) != 1 {
		t.Errorf("after the edit: %+v", got)
	}

	// a client that is pre-authorized: for a scope of the app only
	if st, body := adminRaw(t, ts.URL, "PUT", base+"/pre-authorized", `{"appId":"`+clientApp+`","delegatedPermissionIds":["`+sid+`"]}`); st != 204 {
		t.Fatalf("pre-authorize: %d %s", st, body)
	}
	if st, body := adminRaw(t, ts.URL, "PUT", base+"/pre-authorized", `{"appId":"`+clientApp+`","delegatedPermissionIds":["`+scopeWrite+`"]}`); st != 400 || !strings.Contains(body, "delegatedPermissionIds") {
		t.Errorf("a scope that is not of the app: %d %s", st, body)
	}
	if st, body := adminRaw(t, ts.URL, "PUT", base+"/pre-authorized", `{"appId":"`+clientApp+`","delegatedPermissionIds":[]}`); st != 400 {
		t.Errorf("a client with no scope: %d %s", st, body)
	}
	admin(t, ts, "GET", base+"/api", "", &got)
	if len(got.PreAuthorized) != 1 || got.PreAuthorized[0]["appId"] != clientApp {
		t.Errorf("clients: %+v", got.PreAuthorized)
	}

	// remove the scope: disabled first, and the client loses it
	if st, body := adminRaw(t, ts.URL, "DELETE", base+"/api-scopes/"+sid, ""); st != 400 || !strings.Contains(body, "disabled") {
		t.Errorf("remove an enabled scope: %d %s", st, body)
	}
	if st, body := adminRaw(t, ts.URL, "PATCH", base+"/api-scopes/"+sid, `{"isEnabled":false}`); st != 204 {
		t.Fatalf("disable: %d %s", st, body)
	}
	if st, body := adminRaw(t, ts.URL, "DELETE", base+"/api-scopes/"+sid, ""); st != 204 {
		t.Fatalf("remove: %d %s", st, body)
	}
	if st, _ := adminRaw(t, ts.URL, "DELETE", base+"/api-scopes/"+sid, ""); st != 404 {
		t.Errorf("remove a scope that is gone: %d", st)
	}
	admin(t, ts, "GET", base+"/api", "", &got)
	if len(got.Scopes) != 0 || len(got.PreAuthorized) != 1 || len(got.PreAuthorized[0]["delegatedPermissionIds"].([]any)) != 0 {
		t.Errorf("after the remove: %+v", got)
	}
	if st, _ := adminRaw(t, ts.URL, "DELETE", base+"/pre-authorized/"+clientApp, ""); st != 204 {
		t.Errorf("remove the client: %d", st)
	}
	if st, _ := adminRaw(t, ts.URL, "DELETE", base+"/pre-authorized/"+clientApp, ""); st != 404 {
		t.Errorf("remove the client again: %d", st)
	}
	if st, _ := adminRaw(t, ts.URL, "GET", "/applications/22222222-2222-2222-2222-222222222222/api", ""); st != 404 {
		t.Errorf("an app that is not there: %d", st)
	}
}

func TestAdminAPI_AreInTheHistoryAndSurviveARestart(t *testing.T) {
	dir := t.TempDir()
	w, err := graph.Open(dir, graph.OpenOptions{Users: 30, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(graph.New(w, graph.ServerOptions{}).Handler())
	s, app := graphCall(t, ts, "POST", "/applications", `{"displayName":"Files API","identifierUris":["api://files-api"]}`)
	if s != 201 {
		t.Fatal(s, app)
	}
	s, sp := graphCall(t, ts, "POST", "/servicePrincipals", body(map[string]any{"appId": app["appId"]}))
	if s != 201 {
		t.Fatal(s, sp)
	}
	base := "/applications/" + app["id"].(string)
	var made map[string]any
	if st := admin(t, ts, "POST", base+"/api-scopes", `{"value":"Files.Read","type":"User","adminConsentDisplayName":"Read","adminConsentDescription":"Read."}`, &made); st != 201 {
		t.Fatalf("add: %d", st)
	}
	if st, body := adminRaw(t, ts.URL, "PUT", base+"/pre-authorized", `{"appId":"`+clientApp+`","delegatedPermissionIds":["`+made["id"].(string)+`"]}`); st != 204 {
		t.Fatalf("pre-authorize: %d %s", st, body)
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
	_, got := graphCall(t, ts2, "GET", "/servicePrincipals/"+sp["id"].(string), "")
	scopes := got["oauth2PermissionScopes"].([]any)
	if len(scopes) != 1 || scopes[0].(map[string]any)["value"] != "Files.Read" || scopes[0].(map[string]any)["type"] != "User" {
		t.Errorf("scopes of the service principal after a restart: %v", scopes)
	}
	var settings exposedAnswer
	admin(t, ts2, "GET", base+"/api", "", &settings)
	if len(settings.PreAuthorized) != 1 {
		t.Errorf("pre-authorized clients after a restart: %+v", settings.PreAuthorized)
	}
}
