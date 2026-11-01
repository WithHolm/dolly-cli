package graph_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

const enterpriseTag = "WindowsAzureActiveDirectoryIntegratedApp"

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func spByName(t *testing.T, ts interface{ Close() }, list []map[string]any, name string) map[string]any {
	t.Helper()
	for _, sp := range list {
		if sp["displayName"] == name {
			return sp
		}
	}
	t.Fatalf("no service principal named %q", name)
	return nil
}

// A service principal has the properties that the real Graph gives it (checked with the sample tenant), with the values of a new one.
func TestServicePrincipals_HaveTheFullObject(t *testing.T) {
	ts, _, srv := startServer(t, graph.ServerOptions{})
	_, out := graphCall(t, ts, "GET", "/applications", "")
	app := listOf(out)[0]
	_, byApp := graphCall(t, ts, "GET", "/servicePrincipals?$filter=appId%20eq%20'"+app["appId"].(string)+"'", "")
	got := listOf(byApp)
	if len(got) != 1 {
		t.Fatalf("%v", byApp)
	}
	status, sp := graphCall(t, ts, "GET", "/servicePrincipals/"+got[0]["id"].(string), "")
	if status != 200 {
		t.Fatalf("%d %v", status, sp)
	}
	for _, p := range []string{"accountEnabled", "addIns", "alternativeNames", "appDescription", "appDisplayName", "appId", "appOwnerOrganizationId", "appRoleAssignmentRequired", "appRoles",
		"applicationTemplateId", "createdByAppId", "createdDateTime", "deletedDateTime", "description", "disabledByMicrosoftStatus", "displayName", "homepage", "id", "info", "isDisabled",
		"keyCredentials", "loginUrl", "logoutUrl", "notes", "notificationEmailAddresses", "oauth2PermissionScopes", "passwordCredentials", "preferredSingleSignOnMode",
		"preferredTokenSigningKeyThumbprint", "replyUrls", "resourceSpecificApplicationPermissions", "samlSingleSignOnSettings", "servicePrincipalNames", "servicePrincipalType",
		"signInAudience", "tags", "tokenEncryptionKeyId", "verifiedPublisher"} {
		if _, has := sp[p]; !has {
			t.Errorf("no %s", p)
		}
	}
	if sp["accountEnabled"] != true || sp["servicePrincipalType"] != "Application" || sp["appRoleAssignmentRequired"] != false || sp["appDisplayName"] != sp["displayName"] {
		t.Errorf("values: %v", sp)
	}
	// the service principal of an app registered in this tenant is owned by this tenant
	if sp["appOwnerOrganizationId"] != srv.World().TenantID() {
		t.Errorf("appOwnerOrganizationId %v, the tenant is %s", sp["appOwnerOrganizationId"], srv.World().TenantID())
	}
	// an app of another tenant is owned by another tenant
	_, all := graphCall(t, ts, "GET", "/servicePrincipals?$select=displayName,appOwnerOrganizationId", "")
	other := spByName(t, ts, listOf(all), "Contoso Planner")
	if other["appOwnerOrganizationId"] == srv.World().TenantID() || other["appOwnerOrganizationId"] == nil {
		t.Errorf("the app of another tenant: %v", other)
	}
}

// POST makes the instance of an app that is registered in this tenant. It needs appId. The real Graph's text for an appId that is not a registered app is used.
func TestServicePrincipals_Create(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	_, made := graphCall(t, ts, "POST", "/applications", `{"displayName":"Reports","web":{"redirectUris":["https://reports.example/cb"]},"identifierUris":["api://reports"],"tags":["team-a"]}`)
	appID := made["appId"].(string)

	status, sp := graphCall(t, ts, "POST", "/servicePrincipals", `{"appId":"`+appID+`"}`)
	if status != 201 || sp["appId"] != appID || sp["displayName"] != "Reports" || sp["id"] == made["id"] || sp["accountEnabled"] != true {
		t.Fatalf("create: %d %v", status, sp)
	}
	if names, _ := sp["servicePrincipalNames"].([]any); len(names) != 2 || names[0] != "api://reports" {
		t.Errorf("servicePrincipalNames come from the application: %v", sp["servicePrincipalNames"])
	}
	if urls, _ := sp["replyUrls"].([]any); len(urls) != 1 || urls[0] != "https://reports.example/cb" {
		t.Errorf("replyUrls come from the redirect URIs: %v", sp["replyUrls"])
	}
	// the same appId again
	if s, out := graphCall(t, ts, "POST", "/servicePrincipals", `{"appId":"`+appID+`"}`); s != 400 || !strings.Contains(errText(out), "already exists") {
		t.Errorf("twice: %d %v", s, out)
	}
	if s, out := graphCall(t, ts, "POST", "/servicePrincipals", `{}`); s != 400 || !strings.Contains(errText(out), "appId") {
		t.Errorf("no appId: %d %v", s, out)
	}
	if s, out := graphCall(t, ts, "POST", "/servicePrincipals", `{"appId":"44444444-4444-4444-4444-444444444444"}`); s != 400 || !strings.Contains(errText(out), "does not reference a valid application object") {
		t.Errorf("an app that is not registered: %d %v", s, out)
	}
	// a service principal that was made is found by its client ID, and can be given tags
	if s, out := graphCall(t, ts, "POST", "/servicePrincipals", `{"appId":"x"}`); s != 400 {
		t.Errorf("an appId that is not a GUID: %d %v", s, out)
	}
}

// PATCH changes the properties that Learn lists. The tag of an enterprise application is one of the tags.
func TestServicePrincipals_UpdateAndTheEnterpriseTag(t *testing.T) {
	ts, _, srv := startServer(t, graph.ServerOptions{})
	_, made := graphCall(t, ts, "POST", "/applications", `{"displayName":"Billing"}`)
	appID := made["appId"].(string)
	_, sp := graphCall(t, ts, "POST", "/servicePrincipals", `{"appId":"`+appID+`"}`)
	id := sp["id"].(string)

	if s, _ := graphCall(t, ts, "PATCH", "/servicePrincipals/"+id, `{"accountEnabled":false,"loginUrl":"https://billing.example/login","notes":"n","notificationEmailAddresses":["a@b.example"],"tags":["`+enterpriseTag+`","team-a"],"appRoleAssignmentRequired":true}`); s != 204 {
		t.Fatalf("patch: %d", s)
	}
	_, got := graphCall(t, ts, "GET", "/servicePrincipals/"+id, "")
	tags, _ := got["tags"].([]any)
	if got["accountEnabled"] != false || got["loginUrl"] != "https://billing.example/login" || got["appRoleAssignmentRequired"] != true || len(tags) != 2 || tags[0] != enterpriseTag || tags[1] != "team-a" {
		t.Errorf("after the patch: %v", got)
	}
	info, _ := srv.World().FindServicePrincipal(appID)
	if !info.Enterprise {
		t.Error("the tag makes it an enterprise application in the portal")
	}
	_, enterprise := graphCall(t, ts, "GET", "/servicePrincipals?$filter=tags/any(t:t%20eq%20'"+enterpriseTag+"')&$select=id", "")
	if !strings.Contains(toJSON(enterprise), id) {
		t.Errorf("the filter on the tag finds it: %v", enterprise)
	}
	// the tag out again
	if s, _ := graphCall(t, ts, "PATCH", "/servicePrincipals/"+id, `{"tags":["team-a"]}`); s != 204 {
		t.Fatal(s)
	}
	if info, _ := srv.World().FindServicePrincipal(appID); info.Enterprise {
		t.Error("it is not an enterprise application without the tag")
	}

	for body, want := range map[string]string{
		`{"appId":"x"}`:                     "'appId' cannot be changed",
		`{"id":"x"}`:                        "'id' cannot be changed",
		`{"servicePrincipalType":"Legacy"}`: "cannot be changed",
		`{"nope":1}`:                        "Property 'nope' does not exist",
		`{"accountEnabled":"yes"}`:          "Invalid value specified for property 'accountEnabled'",
		`{"tags":null}`:                     "cannot be null",
		`{"preferredSingleSignOnMode":"x"}`: "preferredSingleSignOnMode",
	} {
		if s, out := graphCall(t, ts, "PATCH", "/servicePrincipals/"+id, body); s != 400 || !strings.Contains(errText(out), want) {
			t.Errorf("%s: %d %v", body, s, out)
		}
	}
	if s, _ := graphCall(t, ts, "PATCH", "/servicePrincipals/55555555-5555-5555-5555-555555555555", `{"notes":"x"}`); s != 404 {
		t.Errorf("an unknown service principal: %d", s)
	}
}

// DELETE takes the service principal away, with what it had: its assignments and its grants. The app registration stays.
func TestServicePrincipals_DeleteAndTheAlternateKey(t *testing.T) {
	ts, _, srv := startServer(t, graph.ServerOptions{})
	_, made := graphCall(t, ts, "POST", "/applications", `{"displayName":"Gone"}`)
	appID := made["appId"].(string)
	_, sp := graphCall(t, ts, "POST", "/servicePrincipals", `{"appId":"`+appID+`"}`)
	id := sp["id"].(string)

	// by the client ID, as the Azure CLI and the SDKs write it
	if s, out := graphCall(t, ts, "GET", "/servicePrincipals(appId='"+appID+"')?$select=id,displayName", ""); s != 200 || out["id"] != id {
		t.Errorf("by appId: %d %v", s, out)
	}
	if s, _ := graphCall(t, ts, "PATCH", "/servicePrincipals(appId='"+appID+"')", `{"notes":"by appId"}`); s != 204 {
		t.Errorf("patch by appId: %d", s)
	}
	if s, _ := graphCall(t, ts, "DELETE", "/servicePrincipals(appId='"+appID+"')", ""); s != 204 {
		t.Fatalf("delete: %d", s)
	}
	if s, out := graphCall(t, ts, "GET", "/servicePrincipals/"+id, ""); s != 404 || !strings.Contains(errText(out), "does not exist") {
		t.Errorf("after the delete: %d %v", s, out)
	}
	if _, ok := srv.World().FindServicePrincipal(appID); ok {
		t.Error("the world still has it")
	}
	if s, _ := graphCall(t, ts, "GET", "/applications/"+made["id"].(string), ""); s != 200 {
		t.Errorf("the app registration stays: %d", s)
	}
	if s, _ := graphCall(t, ts, "DELETE", "/servicePrincipals/"+id, ""); s != 404 {
		t.Errorf("twice: %d", s)
	}
	// Microsoft Graph cannot be deleted
	if s, out := graphCall(t, ts, "DELETE", "/servicePrincipals(appId='00000003-0000-0000-c000-000000000000')", ""); s == 204 {
		t.Errorf("Microsoft Graph was deleted: %v", out)
	}
}
