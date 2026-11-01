package repo

import (
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph/permissions"
)

const (
	userReadAll = "df021288-bdef-4463-88db-98f22de89214" // application permission User.Read.All
	clientApp   = "aaaaaaaa-1111-2222-3333-444444444444"
)

var admin = Actor{Source: "admin"}

func graphSP(t *testing.T, w *World) *ServicePrincipal {
	t.Helper()
	sp := w.byAppID[permissions.GraphAppID]
	if sp == nil {
		t.Fatal("the world has no service principal Microsoft Graph")
	}
	return sp
}

func newClient(t *testing.T, w *World) ServicePrincipalInfo {
	t.Helper()
	c, err := w.EnsureServicePrincipal(clientApp, admin)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestGraphServicePrincipal_IsInEveryWorldWithTheSameID(t *testing.T) {
	a, b := Generate(Options{Users: 10, Seed: 1}), Generate(Options{Users: 10, Seed: 2})
	if graphSP(t, a).ID != graphSP(t, b).ID {
		t.Error("Microsoft Graph must have the same object ID in every dolly")
	}
	// a list leaves out the thousand permissions, and the call for one has them
	for _, o := range a.ListServicePrincipals() {
		if o["appId"] == permissions.GraphAppID {
			if _, has := o["appRoles"]; has {
				t.Error("a list must not carry the roles of Microsoft Graph")
			}
		}
	}
	o, ok := a.GetServicePrincipal(graphSP(t, a).ID)
	if !ok {
		t.Fatal("not found")
	}
	roles, _ := o["appRoles"].([]Object)
	scopes, _ := o["oauth2PermissionScopes"].([]Object)
	if len(roles) != len(catalogOnce().Application) || len(scopes) != len(catalogOnce().Delegated) {
		t.Errorf("%d roles and %d scopes", len(roles), len(scopes))
	}
	if names, _ := o["servicePrincipalNames"].([]string); len(names) != 2 || names[0] != permissions.GraphIdentifierURI {
		t.Errorf("servicePrincipalNames: %v", o["servicePrincipalNames"])
	}
}

func TestAppRoleAssignments_ApplicationPermissionsOfGraph(t *testing.T) {
	w := Generate(Options{Users: 10, Seed: 1})
	client := newClient(t, w)
	gid := graphSP(t, w).ID

	o, err := w.AssignAppRole(gid, client.ID, userReadAll, admin)
	if err != nil {
		t.Fatal(err)
	}
	if o["principalType"] != PrincipalService || o["resourceDisplayName"] != "Microsoft Graph" || o["principalId"] != client.ID {
		t.Errorf("assignment: %v", o)
	}
	if _, err := w.AssignAppRole(gid, client.ID, userReadAll, admin); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("twice: %v", err)
	}
	// an ID that dollycli does not know, for Microsoft Graph: a 404 that says what to do (Graph itself says 400 "Permission being assigned was not found")
	if _, err := w.AssignAppRole(gid, client.ID, "11111111-1111-1111-1111-111111111111", admin); err == nil || err.Status != 404 || err.Code != "Request_ResourceNotFound" ||
		!strings.Contains(err.Message, "does not know a permission of Microsoft Graph with the ID '11111111-1111-1111-1111-111111111111'") || !strings.Contains(err.Message, "use the permission names") {
		t.Errorf("a role that is not there: %v", err)
	}
	users := w.Users[0]["id"].(string)
	if _, err := w.AssignAppRole(gid, users, userReadAll, admin); err == nil {
		t.Error("the application permissions of Graph are for a service principal, not for a user")
	}
	if _, err := w.AssignAppRole(gid, "99999999-1111-1111-1111-111111111111", userReadAll, admin); err == nil {
		t.Error("a principal that does not exist must be refused")
	}

	// both sides of the relation list it
	mine, _ := w.AppRoleAssignments(client.ID)
	to, _ := w.AppRoleAssignedTo(gid)
	if len(mine) != 1 || len(to) != 1 || to[0]["principalDisplayName"] != client.Name {
		t.Errorf("client side %v, resource side %v", mine, to)
	}
	// a wrong side does not delete
	if err := w.DeleteAppRoleAssignment(gid, assignmentID(Assignment{PrincipalID: client.ID, RoleID: userReadAll}), false, admin); err == nil {
		t.Error("the resource is not the principal of the assignment")
	}
	if err := w.DeleteAppRoleAssignment(client.ID, assignmentID(Assignment{PrincipalID: client.ID, RoleID: userReadAll}), false, admin); err != nil {
		t.Errorf("delete: %v", err)
	}
	if mine, _ = w.AppRoleAssignments(client.ID); len(mine) != 0 {
		t.Errorf("after the delete: %v", mine)
	}
}

func TestOAuth2Grants_DelegatedPermissions(t *testing.T) {
	w := Generate(Options{Users: 10, Seed: 1})
	client, gid := newClient(t, w), graphSP(t, w).ID

	g, err := w.CreateOAuth2Grant(map[string]any{"clientId": client.ID, "resourceId": gid, "consentType": "AllPrincipals", "scope": "user.read MAIL.read user.read"}, admin)
	if err != nil {
		t.Fatal(err)
	}
	if g["scope"] != "User.Read Mail.Read" || g["principalId"] != nil {
		t.Errorf("the scopes are written in the case of Graph, once each: %v", g)
	}
	for name, props := range map[string]map[string]any{
		"twice":              {"clientId": client.ID, "resourceId": gid, "consentType": "AllPrincipals", "scope": "User.Read"},
		"a scope not there":  {"clientId": client.ID, "resourceId": gid, "consentType": "Principal", "principalId": w.Users[0]["id"], "scope": "Nothing.Here"},
		"no consent type":    {"clientId": client.ID, "resourceId": gid, "scope": "User.Read"},
		"Principal, no user": {"clientId": client.ID, "resourceId": gid, "consentType": "Principal", "scope": "User.Read"},
		"an unknown client":  {"clientId": "99999999-1111-1111-1111-111111111111", "resourceId": gid, "consentType": "AllPrincipals", "scope": "User.Read"},
		"an unknown field":   {"clientId": client.ID, "resourceId": gid, "consentType": "AllPrincipals", "scope": "User.Read", "color": "red"},
	} {
		if _, err := w.CreateOAuth2Grant(props, admin); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
	// one user can consent for himself or herself
	if _, err := w.CreateOAuth2Grant(map[string]any{"clientId": client.ID, "resourceId": gid, "consentType": "Principal", "principalId": strings.ToUpper(w.Users[0]["id"].(string)), "scope": "User.Read"}, admin); err != nil {
		t.Errorf("consent of one user: %v", err)
	}

	id := g["id"].(string)
	if err := w.UpdateOAuth2Grant(id, map[string]any{"scope": "Calendars.Read"}, admin); err != nil {
		t.Fatal(err)
	}
	if got, _ := w.GetOAuth2Grant(id); got["scope"] != "Calendars.Read" {
		t.Errorf("after the update: %v", got)
	}
	if err := w.UpdateOAuth2Grant(id, map[string]any{"clientId": "x"}, admin); err == nil {
		t.Error("only the scope can change")
	}
	if err := w.DeleteOAuth2Grant(id, admin); err != nil {
		t.Fatal(err)
	}
	if _, ok := w.GetOAuth2Grant(id); ok {
		t.Error("the consent must be gone")
	}
}

func TestGrantPermission_OneCallForTheScreen(t *testing.T) {
	w := Generate(Options{Users: 10, Seed: 1})
	client := newClient(t, w)

	for _, p := range []struct {
		kind  permissions.Kind
		value string
	}{
		{permissions.Delegated, "user.read"}, {permissions.Delegated, "Mail.Read"},
		{permissions.Application, "user.read.all"},
	} {
		if err := w.GrantPermission(clientApp, permissions.GraphAppID, p.kind, p.value, admin); err != nil {
			t.Fatalf("%s %s: %v", p.kind, p.value, err)
		}
	}
	if err := w.GrantPermission(clientApp, permissions.GraphAppID, permissions.Delegated, "USER.READ", admin); err == nil {
		t.Error("a permission that the client has must be refused")
	}
	if err := w.GrantPermission(clientApp, permissions.GraphAppID, permissions.Application, "User.Read", admin); err == nil {
		t.Error("User.Read is a delegated permission, not an application permission")
	}

	gr, _ := w.GrantsOf(clientApp)
	if len(gr.Application) != 1 || len(gr.Delegated) != 1 {
		t.Fatalf("grants: %+v", gr)
	}
	// the strings that a developer copies into the app
	if gr.Application[0].Value != "User.Read.All" || gr.Application[0].Scope != "https://graph.microsoft.com/.default" {
		t.Errorf("application permission: %+v", gr.Application[0])
	}
	d := gr.Delegated[0]
	if d.Scope != "https://graph.microsoft.com/User.Read https://graph.microsoft.com/Mail.Read" || len(d.Permissions) != 2 {
		t.Errorf("delegated: %+v", d)
	}
	if d.Permissions[0].Description == "" || d.Permissions[0].Admin {
		t.Errorf("permission: %+v", d.Permissions[0])
	}

	// revoke: one scope leaves the consent, and the last one takes the consent away
	if err := w.RevokePermission(clientApp, permissions.GraphAppID, permissions.Delegated, "user.read", admin); err != nil {
		t.Fatal(err)
	}
	if gr, _ = w.GrantsOf(clientApp); gr.Delegated[0].Scope != "https://graph.microsoft.com/Mail.Read" {
		t.Errorf("after one revoke: %+v", gr.Delegated)
	}
	w.RevokePermission(clientApp, permissions.GraphAppID, permissions.Delegated, "Mail.Read", admin)
	w.RevokePermission(clientApp, permissions.GraphAppID, permissions.Application, "User.Read.All", admin)
	if gr, _ = w.GrantsOf(clientApp); len(gr.Delegated)+len(gr.Application) != 0 || len(w.Grants) != 0 {
		t.Errorf("after the last revoke: %+v", gr)
	}
	if err := w.RevokePermission(clientApp, permissions.GraphAppID, permissions.Delegated, "Mail.Read", admin); err == nil {
		t.Error("revoke of a permission that is not there must be refused")
	}
	_ = client
}

// An app can use the API of another: the second service principal has roles and scopes, and the first one is granted them.
func TestGrantPermission_AppToApp(t *testing.T) {
	w := Generate(Options{Users: 10, Seed: 1})
	newClient(t, w)
	var api *ServicePrincipal
	for _, sp := range w.SPs {
		if len(sp.Roles) > 0 && sp.Builtin == "" {
			api = sp
			break
		}
	}
	api.IdentifierURIs = []string{"api://acme-portal"}
	api.Scopes = []AppScope{{ID: newGUID(), Value: "Files.Read", Name: "Read files", Description: "Read the files of the portal."}}

	var found bool
	for _, r := range w.Resources() {
		if r.AppID == api.AppID {
			found = true
			if r.Namespace != "api://acme-portal" || r.Application != len(api.Roles) || r.Delegated != 1 {
				t.Errorf("resource: %+v", r)
			}
		}
	}
	if !found || w.Resources()[0].Builtin != BuiltinGraph {
		t.Errorf("the resources: %+v", w.Resources())
	}

	if err := w.GrantPermission(clientApp, api.AppID, permissions.Delegated, "files.read", admin); err != nil {
		t.Fatal(err)
	}
	if err := w.GrantPermission(clientApp, api.AppID, permissions.Application, "admin", admin); err != nil {
		t.Fatal(err)
	}
	gr, _ := w.GrantsOf(clientApp)
	if gr.Delegated[0].Scope != "api://acme-portal/Files.Read" || gr.Application[0].Scope != "api://acme-portal/.default" {
		t.Errorf("scopes to copy: %+v %+v", gr.Delegated[0].Scope, gr.Application[0].Scope)
	}
	if err := w.GrantPermission(clientApp, api.AppID, permissions.Delegated, "Mail.Read", admin); err == nil {
		t.Error("a permission of Microsoft Graph is not a permission of this API")
	}
	// the roles of an API of a person can go to users too: Default access, and its own roles
	if _, err := w.AssignAppRole(api.ID, w.Users[0]["id"].(string), DefaultAccess, admin); err != nil {
		t.Errorf("default access for a user: %v", err)
	}
}

func TestResourcePermissions_ForThePicker(t *testing.T) {
	w := Generate(Options{Users: 10, Seed: 1})
	list, total, ok := w.ResourcePermissions(permissions.GraphAppID, permissions.Delegated, "user.read", 5)
	if !ok || total < 3 || len(list) > 5 {
		t.Fatalf("%d of %d %v", len(list), total, ok)
	}
	var user *PermissionInfo
	for i := range list {
		if list[i].Value == "User.Read" {
			user = &list[i]
		}
	}
	if user == nil || user.Scope != "https://graph.microsoft.com/User.Read" || user.ID != "e1fe6dd8-ba31-4d61-89e7-88639da4683d" {
		t.Errorf("User.Read: %+v", user)
	}
	app, _, _ := w.ResourcePermissions(permissions.GraphAppID, permissions.Application, "user.read.all", 0)
	if len(app) == 0 || app[0].Scope != "https://graph.microsoft.com/.default" {
		t.Errorf("an application permission is asked for with .default: %+v", app)
	}
	if _, _, ok := w.ResourcePermissions("99999999-1111-1111-1111-111111111111", permissions.Delegated, "", 0); ok {
		t.Error("an unknown resource")
	}
	if all, total, _ := w.ResourcePermissions(permissions.GraphAppID, permissions.Delegated, "", 0); len(all) != total || total != len(catalogOnce().Delegated) {
		t.Errorf("without a query and a limit it gives all: %d of %d", len(all), total)
	}
}

func TestGrants_AreKeptAfterARestart_AndRollBack(t *testing.T) {
	dir := t.TempDir()
	w := openTemp(t, dir)
	newClient(t, w)
	before := len(w.History())
	w.GrantPermission(clientApp, permissions.GraphAppID, permissions.Application, "User.Read.All", admin)
	w.GrantPermission(clientApp, permissions.GraphAppID, permissions.Delegated, "User.Read", admin)
	w.GrantPermission(clientApp, permissions.GraphAppID, permissions.Delegated, "Mail.Read", admin)
	w.RevokePermission(clientApp, permissions.GraphAppID, permissions.Delegated, "User.Read", admin)
	w.Close()

	w = openTemp(t, dir)
	gr, _ := w.GrantsOf(clientApp)
	if len(gr.Application) != 1 || len(gr.Delegated) != 1 || gr.Delegated[0].Scope != "https://graph.microsoft.com/Mail.Read" {
		t.Fatalf("after a restart: %+v", gr)
	}
	text := ""
	for _, e := range w.History() {
		text += e.Text() + "\n"
	}
	for _, want := range []string{"Assigned the role User.Read.All of Microsoft Graph to Auto-created service principal", "(service principal)", "Consent for", "Changed the delegated permissions"} {
		if !strings.Contains(text, want) {
			t.Errorf("history has no %q:\n%s", want, text)
		}
	}
	if err := w.Rollback(before, admin); err != nil {
		t.Fatal(err)
	}
	if gr, _ = w.GrantsOf(clientApp); len(gr.Application)+len(gr.Delegated) != 0 {
		t.Errorf("after the rollback: %+v", gr)
	}
	w.Close()
}
