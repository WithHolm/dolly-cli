package graph_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
	"github.com/withholm/dolly-cli/graph/permissions"
)

func listOf(out map[string]any) []map[string]any {
	var items []map[string]any
	list, _ := out["value"].([]any)
	for _, x := range list {
		items = append(items, x.(map[string]any))
	}
	return items
}

// The seed has app registrations, and a tenant has apps that are not registered in it.
func TestApplications_ListAndGetByIDAndByAppID(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	status, out := graphCall(t, ts, "GET", "/applications", "")
	apps := listOf(out)
	if status != 200 || len(apps) != 2 || !strings.HasSuffix(out["@odata.context"].(string), "#applications") {
		t.Fatalf("%d %v", status, out)
	}
	// the properties of an application as real Graph gives them (checked with the sample tenant)
	for _, p := range []string{"id", "appId", "displayName", "signInAudience", "requiredResourceAccess", "identifierUris", "web", "spa", "publicClient", "passwordCredentials", "keyCredentials", "publisherDomain", "createdDateTime", "tags", "appRoles", "api", "info", "verifiedPublisher", "notes", "isFallbackPublicClient"} {
		if _, has := apps[0][p]; !has {
			t.Errorf("no %s: %v", p, apps[0])
		}
	}
	id, appID := apps[0]["id"].(string), apps[0]["appId"].(string)
	if id == appID {
		t.Error("the object ID and the appId are two values")
	}

	status, byID := graphCall(t, ts, "GET", "/applications/"+id+"?$select=id,appId,displayName", "")
	if status != 200 || byID["appId"] != appID || len(byID) != 4 { // + @odata.context
		t.Errorf("by ID: %d %v", status, byID)
	}
	// the alternate key, as the Azure CLI and the SDKs write it
	status, byKey := graphCall(t, ts, "GET", "/applications(appId='"+appID+"')?$select=id,displayName", "")
	if status != 200 || byKey["id"] != id {
		t.Errorf("by appId: %d %v", status, byKey)
	}
	status, filtered := graphCall(t, ts, "GET", "/applications?$filter=appId%20eq%20'"+appID+"'&$select=id", "")
	if got := listOf(filtered); status != 200 || len(got) != 1 || got[0]["id"] != id {
		t.Errorf("filter: %d %v", status, filtered)
	}
	// real Graph: 404 Request_ResourceNotFound
	status, missing := graphCall(t, ts, "GET", "/applications/11111111-1111-1111-1111-111111111111", "")
	if status != 404 || !strings.Contains(errText(missing), "Resource '11111111-1111-1111-1111-111111111111' does not exist") {
		t.Errorf("unknown: %d %v", status, missing)
	}
}

// An application and its service principal have the same appId and different IDs. A service principal with no application is an app of another tenant.
func TestApplications_LinkedToServicePrincipalsByAppID(t *testing.T) {
	ts, _, srv := startServer(t, graph.ServerOptions{})
	_, out := graphCall(t, ts, "GET", "/applications", "")
	apps := listOf(out)
	registered := map[string]string{}
	for _, a := range apps {
		registered[a["appId"].(string)] = a["id"].(string)
	}
	var withApp, without, tagged, untaggedWithApp int
	for _, sp := range srv.World().ListServicePrincipalInfo() {
		if app, ok := registered[sp.AppID]; ok {
			withApp++
			if sp.Application != app || sp.ID == app {
				t.Errorf("%s: application %q, service principal ID %s", sp.Name, sp.Application, sp.ID)
			}
			if !sp.Enterprise {
				untaggedWithApp++
			}
		} else {
			without++
			if sp.Application != "" {
				t.Errorf("%s has no registration: %q", sp.Name, sp.Application)
			}
		}
		if sp.Enterprise {
			tagged++
		}
	}
	if withApp != 2 || without < 3 || tagged == 0 || untaggedWithApp != 1 {
		t.Errorf("with %d, without %d, tagged %d, registered but not tagged %d", withApp, without, tagged, untaggedWithApp)
	}
}

func TestApplications_CreateUpdateDelete(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	status, made := graphCall(t, ts, "POST", "/applications", `{"displayName":"Reports","signInAudience":"AzureADMultipleOrgs","web":{"redirectUris":["https://reports.example/cb"]}}`)
	if status != 201 || made["id"] == "" || made["appId"] == "" || made["displayName"] != "Reports" || made["createdDateTime"] == nil {
		t.Fatalf("create: %d %v", status, made)
	}
	id := made["id"].(string)
	for body, want := range map[string]string{
		`{}`:                              "Property 'displayName' is required.",
		`{"displayName":"x","appId":"a"}`: "Property 'appId' is read-only.",
		`{"displayName":"x","nope":1}`:    "Property 'nope' does not exist",
		`{"displayName":"x","signInAudience":"Everyone"}`: "signInAudience",
		`{"displayName":"x","identifierUris":"a"}`:        "Invalid value specified for property 'identifierUris'",
	} {
		if s, out := graphCall(t, ts, "POST", "/applications", body); s != 400 || !strings.Contains(errText(out), want) {
			t.Errorf("%s: %d %v", body, s, out)
		}
	}

	// an object property changes only what the call names
	if s, _ := graphCall(t, ts, "PATCH", "/applications/"+id, `{"description":"Monthly reports","web":{"logoutUrl":"https://reports.example/out"}}`); s != 204 {
		t.Fatalf("patch: %d", s)
	}
	_, got := graphCall(t, ts, "GET", "/applications/"+id, "")
	web := got["web"].(map[string]any)
	if got["description"] != "Monthly reports" || web["logoutUrl"] != "https://reports.example/out" || len(web["redirectUris"].([]any)) != 1 {
		t.Errorf("after the patch: %v", got)
	}
	if s, out := graphCall(t, ts, "PATCH", "/applications/"+id, `{"id":"x"}`); s != 400 || !strings.Contains(errText(out), "cannot be changed") {
		t.Errorf("id: %d %v", s, out)
	}
	if s, _ := graphCall(t, ts, "PATCH", "/applications/22222222-2222-2222-2222-222222222222", `{"notes":"x"}`); s != 404 {
		t.Errorf("patch of an unknown application: %d", s)
	}

	if s, _ := graphCall(t, ts, "DELETE", "/applications(appId='"+made["appId"].(string)+"')", ""); s != 204 {
		t.Fatalf("delete by appId: %d", s)
	}
	if s, _ := graphCall(t, ts, "GET", "/applications/"+id, ""); s != 404 {
		t.Errorf("after the delete: %d", s)
	}
}

// The Azure CLI sets requiredResourceAccess with permission IDs. For Microsoft Graph, an ID that dollycli does not know is a 404 that says what to do.
func TestApplications_RequiredResourceAccessChecksTheIDsOfGraph(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	cat, err := permissions.Load()
	if err != nil {
		t.Fatal(err)
	}
	userReadAll, _ := cat.Find(permissions.Application, "User.Read.All")
	access := func(resource, id, kind string) string {
		return fmt.Sprintf(`{"requiredResourceAccess":[{"resourceAppId":%q,"resourceAccess":[{"id":%q,"type":%q}]}]}`, resource, id, kind)
	}
	_, made := graphCall(t, ts, "POST", "/applications", `{"displayName":"Asks"}`)
	id := made["id"].(string)

	if s, out := graphCall(t, ts, "PATCH", "/applications/"+id, access(permissions.GraphAppID, userReadAll.ID, "Role")); s != 204 {
		t.Fatalf("a known ID: %d %v", s, out)
	}
	_, got := graphCall(t, ts, "GET", "/applications/"+id+"?$select=requiredResourceAccess", "")
	if list := got["requiredResourceAccess"].([]any); len(list) != 1 {
		t.Errorf("it is kept: %v", got)
	}
	// the ID of a delegated permission is not the ID of a role
	s, out := graphCall(t, ts, "PATCH", "/applications/"+id, access(permissions.GraphAppID, userReadAll.ID, "Scope"))
	if s != 404 {
		t.Errorf("the ID of a role as a scope: %d %v", s, out)
	}
	s, out = graphCall(t, ts, "PATCH", "/applications/"+id, access(permissions.GraphAppID, "11111111-1111-1111-1111-111111111111", "Role"))
	msg := errText(out)
	if s != 404 || out["error"].(map[string]any)["code"] != "Request_ResourceNotFound" || !strings.Contains(msg, "does not know a permission of Microsoft Graph with the ID '11111111-1111-1111-1111-111111111111'") || !strings.Contains(msg, "use the permission names") {
		t.Errorf("an unknown ID: %d %v", s, out)
	}
	if s, _ := graphCall(t, ts, "POST", "/applications", `{"displayName":"x","requiredResourceAccess":[{"resourceAppId":"`+permissions.GraphAppID+`","resourceAccess":[{"id":"11111111-1111-1111-1111-111111111111","type":"Scope"}]}]}`); s != 404 {
		t.Errorf("the same check when the application is made: %d", s)
	}
	// another resource: not checked
	if s, _ := graphCall(t, ts, "PATCH", "/applications/"+id, access("99999999-9999-9999-9999-999999999999", "11111111-1111-1111-1111-111111111111", "Role")); s != 204 {
		t.Errorf("a resource that is not Graph: %d", s)
	}
	for body, want := range map[string]string{
		`{"requiredResourceAccess":[{"resourceAccess":[]}]}`:                                          "each item needs a resourceAppId",
		`{"requiredResourceAccess":[{"resourceAppId":"a","resourceAccess":[{"id":"b","type":"X"}]}]}`: "type",
	} {
		if s, out := graphCall(t, ts, "PATCH", "/applications/"+id, body); s != 400 || !strings.Contains(errText(out), want) {
			t.Errorf("%s: %d %v", body, s, out)
		}
	}
}

// A client that signs in with the appId of an application gets a service principal with the name of the application.
func TestApplications_TheServicePrincipalOfASignInHasTheName(t *testing.T) {
	_, _, srv := startServer(t, graph.ServerOptions{})
	w := srv.World()
	app, e := w.CreateApplication(map[string]any{"displayName": "Billing"}, graph.Actor{})
	if e != nil {
		t.Fatal(e)
	}
	info, e := w.EnsureServicePrincipal(app["appId"].(string), graph.Actor{})
	if e != nil || info.Name != "Billing" || info.Application != app["id"] || info.Enterprise {
		t.Errorf("%+v %v", info, e)
	}
	other, _ := w.EnsureServicePrincipal("33333333-3333-3333-3333-333333333333", graph.Actor{})
	if other.Application != "" || !strings.HasPrefix(other.Name, "Auto-created") {
		t.Errorf("an app of another tenant: %+v", other)
	}
}
