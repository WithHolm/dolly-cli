package graph_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
	"github.com/withholm/dolly-cli/graph/permissions"
)

const permClient = "aaaaaaaa-1111-2222-3333-444444444444"

func permServer(t *testing.T) (http.Handler, *graph.Server, string, string) {
	t.Helper()
	srv := graph.New(graph.Generate(graph.Options{Users: 10, Seed: 1}), graph.ServerOptions{})
	c, err := srv.World().EnsureServicePrincipal(permClient, graph.Actor{Source: "login"})
	if err != nil {
		t.Fatal(err)
	}
	g, _ := srv.World().FindServicePrincipal(permissions.GraphAppID)
	return srv.Handler(), srv, c.ID, g.ID
}

func decode(t *testing.T, body string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(body), v); err != nil {
		t.Fatalf("%v: %s", err, body)
	}
}

// The Graph API: an application permission is an app role assignment, and a delegated permission is a consent.
func TestGraphAPI_PermissionGrants(t *testing.T) {
	h, _, client, gid := permServer(t)

	// app role assignment from the side of the client
	body := `{"principalId":"` + client + `","resourceId":"` + gid + `","appRoleId":"df021288-bdef-4463-88db-98f22de89214"}`
	rec := send(h, "POST", "/v1.0/servicePrincipals/"+client+"/appRoleAssignments", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("assign: %d %s", rec.Code, rec.Body)
	}
	var created struct{ ID, PrincipalType, ResourceDisplayName string }
	decode(t, rec.Body.String(), &created)
	if created.PrincipalType != "ServicePrincipal" || created.ResourceDisplayName != "Microsoft Graph" {
		t.Errorf("assignment: %+v", created)
	}
	if rec = send(h, "POST", "/v1.0/servicePrincipals/"+client+"/appRoleAssignments", body); rec.Code != 400 || !strings.Contains(rec.Body.String(), "already exists") {
		t.Errorf("twice: %d %s", rec.Code, rec.Body)
	}
	if rec = send(h, "POST", "/v1.0/servicePrincipals/"+gid+"/appRoleAssignments", body); rec.Code != 400 {
		t.Errorf("the principalId must be the ID in the path: %d", rec.Code)
	}
	var list struct{ Value []map[string]any }
	decode(t, send(h, "GET", "/v1.0/servicePrincipals/"+client+"/appRoleAssignments", "").Body.String(), &list)
	if len(list.Value) != 1 {
		t.Errorf("client side: %v", list.Value)
	}
	decode(t, send(h, "GET", "/v1.0/servicePrincipals/"+gid+"/appRoleAssignedTo", "").Body.String(), &list)
	if len(list.Value) != 1 || list.Value[0]["principalDisplayName"] == "" {
		t.Errorf("resource side: %v", list.Value)
	}
	if rec = send(h, "DELETE", "/v1.0/servicePrincipals/"+client+"/appRoleAssignments/"+created.ID, ""); rec.Code != 204 {
		t.Errorf("delete: %d %s", rec.Code, rec.Body)
	}
	if rec = send(h, "DELETE", "/v1.0/servicePrincipals/"+client+"/appRoleAssignments/"+created.ID, ""); rec.Code != 404 {
		t.Errorf("delete twice: %d", rec.Code)
	}

	// consent
	rec = send(h, "POST", "/v1.0/oauth2PermissionGrants", `{"clientId":"`+client+`","consentType":"AllPrincipals","resourceId":"`+gid+`","scope":"User.Read Mail.Read"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("consent: %d %s", rec.Code, rec.Body)
	}
	var grant struct{ ID, Scope string }
	decode(t, rec.Body.String(), &grant)
	if grant.Scope != "User.Read Mail.Read" {
		t.Errorf("grant: %+v", grant)
	}
	if rec = send(h, "POST", "/v1.0/oauth2PermissionGrants", `{"clientId":"`+client+`","consentType":"AllPrincipals","resourceId":"`+gid+`","scope":"Nothing.Here"}`); rec.Code != 400 {
		t.Errorf("a scope that is not there: %d", rec.Code)
	}
	decode(t, send(h, "GET", "/v1.0/oauth2PermissionGrants?$filter="+url.QueryEscape("clientId eq '"+client+"'"), "").Body.String(), &list)
	if len(list.Value) != 1 {
		t.Errorf("filter: %v", list.Value)
	}
	decode(t, send(h, "GET", "/v1.0/oauth2PermissionGrants?$filter="+url.QueryEscape("clientId eq '"+gid+"'"), "").Body.String(), &list)
	if len(list.Value) != 0 {
		t.Errorf("filter on another client: %v", list.Value)
	}
	if rec = send(h, "PATCH", "/v1.0/oauth2PermissionGrants/"+grant.ID, `{"scope":"Calendars.Read"}`); rec.Code != 204 {
		t.Errorf("patch: %d %s", rec.Code, rec.Body)
	}
	if rec = send(h, "GET", "/v1.0/oauth2PermissionGrants/"+grant.ID, ""); !strings.Contains(rec.Body.String(), `"scope":"Calendars.Read"`) {
		t.Errorf("get: %s", rec.Body)
	}
	if rec = send(h, "DELETE", "/v1.0/oauth2PermissionGrants/"+grant.ID, ""); rec.Code != 204 {
		t.Errorf("delete: %d", rec.Code)
	}
	if rec = send(h, "GET", "/v1.0/oauth2PermissionGrants/"+grant.ID, ""); rec.Code != 404 {
		t.Errorf("get after delete: %d", rec.Code)
	}

	// the full service principal of Microsoft Graph has the catalog; a list does not
	rec = send(h, "GET", "/v1.0/servicePrincipals/"+gid, "")
	var one struct {
		AppRoles               []any
		Oauth2PermissionScopes []any `json:"oauth2PermissionScopes"`
	}
	decode(t, rec.Body.String(), &one)
	cat, _ := permissions.Load()
	if len(one.AppRoles) != len(cat.Application) || len(one.Oauth2PermissionScopes) != len(cat.Delegated) {
		t.Errorf("%d roles and %d scopes", len(one.AppRoles), len(one.Oauth2PermissionScopes))
	}
}

// The admin API of the page: pick a resource, pick permissions, grant, and read the strings to copy.
func TestAdminAPI_Permissions(t *testing.T) {
	h, _, _, _ := permServer(t)
	cat, _ := permissions.Load()

	var res struct {
		Resources []struct {
			AppID, Namespace, Builtin string
			Application, Delegated    int
		}
	}
	decode(t, send(h, "GET", "/_dolly/resources", "").Body.String(), &res)
	if len(res.Resources) < 2 || res.Resources[0].AppID != permissions.GraphAppID || res.Resources[0].Namespace != "https://graph.microsoft.com" || res.Resources[0].Delegated != len(cat.Delegated) {
		t.Fatalf("resources: %+v", res.Resources)
	}

	var pick struct {
		Permissions []struct{ Value, Scope, Description string }
		Total       int
	}
	decode(t, send(h, "GET", "/_dolly/resources/"+permissions.GraphAppID+"/permissions?kind=delegated&q=user.read&limit=3", "").Body.String(), &pick)
	if pick.Total < 3 || len(pick.Permissions) != 3 || !strings.HasPrefix(pick.Permissions[0].Scope, "https://graph.microsoft.com/") {
		t.Errorf("pick: %+v", pick)
	}
	if rec := send(h, "GET", "/_dolly/resources/"+permissions.GraphAppID+"/permissions?kind=nope", ""); rec.Code != 400 {
		t.Errorf("a kind that is not known: %d", rec.Code)
	}
	if rec := send(h, "GET", "/_dolly/resources/99999999-1111-1111-1111-111111111111/permissions?kind=delegated", ""); rec.Code != 404 {
		t.Errorf("a resource that is not known: %d", rec.Code)
	}

	for _, b := range []string{
		`{"resourceAppId":"` + permissions.GraphAppID + `","kind":"delegated","value":"User.Read"}`,
		`{"resourceAppId":"` + permissions.GraphAppID + `","kind":"application","value":"User.Read.All"}`,
	} {
		if rec := send(h, "POST", "/_dolly/service-principals/"+permClient+"/grants", b); rec.Code != 204 {
			t.Fatalf("grant: %d %s", rec.Code, rec.Body)
		}
	}
	if rec := send(h, "POST", "/_dolly/service-principals/"+permClient+"/grants", `{"resourceAppId":"`+permissions.GraphAppID+`","kind":"delegated","value":"User.Read"}`); rec.Code != 400 {
		t.Errorf("granted twice: %d", rec.Code)
	}

	var gr struct {
		Application []struct{ Value, Scope, AssignmentID string }
		Delegated   []struct{ Scope, ConsentType string }
	}
	decode(t, send(h, "GET", "/_dolly/service-principals/"+permClient+"/grants", "").Body.String(), &gr)
	if len(gr.Application) != 1 || gr.Application[0].Scope != "https://graph.microsoft.com/.default" ||
		len(gr.Delegated) != 1 || gr.Delegated[0].Scope != "https://graph.microsoft.com/User.Read" || gr.Delegated[0].ConsentType != "AllPrincipals" {
		t.Errorf("grants: %+v", gr)
	}

	q := "?resourceAppId=" + permissions.GraphAppID
	if rec := send(h, "DELETE", "/_dolly/service-principals/"+permClient+"/grants"+q+"&kind=delegated&value=user.read", ""); rec.Code != 204 {
		t.Errorf("revoke: %d %s", rec.Code, rec.Body)
	}
	if rec := send(h, "DELETE", "/_dolly/service-principals/"+permClient+"/grants"+q+"&kind=application&value=User.Read.All", ""); rec.Code != 204 {
		t.Errorf("revoke: %d %s", rec.Code, rec.Body)
	}
	decode(t, send(h, "GET", "/_dolly/service-principals/"+permClient+"/grants", "").Body.String(), &gr)
	if len(gr.Application)+len(gr.Delegated) != 0 {
		t.Errorf("after the revokes: %+v", gr)
	}
	if rec := send(h, "GET", "/_dolly/service-principals/99999999-1111-1111-1111-111111111111/grants", ""); rec.Code != 404 {
		t.Errorf("a service principal that is not known: %d", rec.Code)
	}
}
