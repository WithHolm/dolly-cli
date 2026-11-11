package login_test

import (
	"net/url"
	"slices"
	"testing"

	graph "github.com/withholm/dolly-cli/graph"
	"github.com/withholm/dolly-cli/graph/permissions"
)

// An app role for applications is an application permission. It is listed, granted, and put in the claim roles by its value, as it is written in the app
// registration, and not by its name. Only an enabled role that is for applications is a permission.
func TestRolesClaim_AnApplicationPermissionHasTheValueOfTheRole(t *testing.T) {
	e := setup(t)
	admin := graph.Actor{Source: "admin"}
	const (
		forApps  = "11111111-1111-1111-1111-111111111111"
		forUsers = "22222222-2222-2222-2222-222222222222"
		off      = "33333333-3333-3333-3333-333333333333"
	)
	role := func(id, name, value string, on bool, types ...any) map[string]any {
		return map[string]any{"id": id, "displayName": name, "value": value, "description": "d", "allowedMemberTypes": types, "isEnabled": on}
	}
	app, perr := e.world.CreateApplication(map[string]any{"displayName": "Timesheets API", "identifierUris": []any{"api://timesheets-api"}, "appRoles": []any{
		role(forApps, "Sync timesheets", "Timesheet.Sync", true, "Application"),
		role(forUsers, "Approve timesheets", "Timesheet.Approve", true, "User"),
		role(off, "Old sync", "Timesheet.OldSync", false, "Application"),
	}}, admin)
	if perr != nil {
		t.Fatal(perr)
	}
	apiID := app["appId"].(string)
	if _, perr := e.world.CreateServicePrincipal(map[string]any{"appId": apiID}, admin); perr != nil {
		t.Fatal(perr)
	}

	list, _, ok := e.world.ResourcePermissions(apiID, permissions.Application, "", 0)
	var values []string
	for _, p := range list {
		values = append(values, p.Value)
	}
	if !ok || !slices.Equal(values, []string{"Timesheet.Sync"}) {
		t.Fatalf("the application permissions of the API: %v (want only the enabled role for applications, by its value)", values)
	}

	// a client signs in once (it gets a service principal), is granted the permission by its value, and asks for a token
	if code, r := e.appToken(client1, "anything"); code != 200 {
		t.Fatalf("first sign-in: %d %v", code, r)
	}
	if err := e.world.GrantPermission(client1, apiID, permissions.Application, "Timesheet.Sync", admin); err != nil {
		t.Fatal(err)
	}
	code, resp := e.post("/"+e.tid+"/oauth2/v2.0/token", url.Values{
		"grant_type": {"client_credentials"}, "client_id": {client1}, "client_secret": {"anything"}, "scope": {"api://timesheets-api/.default"},
	})
	if code != 200 {
		t.Fatalf("token: %d %v", code, resp)
	}
	if got := claimStrings(t, claimsOf(t, resp["access_token"].(string)), "roles"); !slices.Equal(got, []string{"Timesheet.Sync"}) {
		t.Errorf("roles in the token of the client: %v, want [Timesheet.Sync]", got)
	}
}

// A client keeps its assignment when the role is disabled, but the token no longer has the role: only an enabled role that is for applications and has a value is in
// the claim roles. A role with no value is never in it, and the claim is not an empty string.
func TestRolesClaim_AClientDoesNotGetADisabledRoleOrOneWithNoValue(t *testing.T) {
	e := setup(t)
	admin := graph.Actor{Source: "admin"}
	const (
		withValue = "11111111-1111-1111-1111-111111111111"
		noValue   = "22222222-2222-2222-2222-222222222222"
	)
	role := func(id, name string, value any, on bool) map[string]any {
		return map[string]any{"id": id, "displayName": name, "value": value, "description": "d", "allowedMemberTypes": []any{"Application"}, "isEnabled": on}
	}
	app, perr := e.world.CreateApplication(map[string]any{"displayName": "Timesheets API", "identifierUris": []any{"api://timesheets-api"}, "appRoles": []any{
		role(withValue, "Sync timesheets", "Timesheet.Sync", true), role(noValue, "No value", nil, true),
	}}, admin)
	if perr != nil {
		t.Fatal(perr)
	}
	apiID := app["appId"].(string)
	api, perr := e.world.CreateServicePrincipal(map[string]any{"appId": apiID}, admin)
	if perr != nil {
		t.Fatal(perr)
	}
	if code, r := e.appToken(client1, "anything"); code != 200 { // the client signs in once, and gets its service principal
		t.Fatalf("first sign-in: %d %v", code, r)
	}
	client, ok := e.world.GetServicePrincipal(client1)
	if !ok {
		t.Fatal("the client has no service principal")
	}
	for _, id := range []string{withValue, noValue} {
		if _, perr := e.world.AssignAppRole(api["id"].(string), client["id"].(string), id, admin); perr != nil {
			t.Fatalf("assign %s: %v", id, perr)
		}
	}
	claim := func() []string {
		t.Helper()
		code, resp := e.post("/"+e.tid+"/oauth2/v2.0/token", url.Values{
			"grant_type": {"client_credentials"}, "client_id": {client1}, "client_secret": {"anything"}, "scope": {"api://timesheets-api/.default"},
		})
		if code != 200 {
			t.Fatalf("token: %d %v", code, resp)
		}
		return claimStrings(t, claimsOf(t, resp["access_token"].(string)), "roles")
	}
	if got := claim(); !slices.Equal(got, []string{"Timesheet.Sync"}) {
		t.Fatalf("roles before the role is disabled: %v, want [Timesheet.Sync] (a role with no value is left out)", got)
	}

	if perr := e.world.UpdateApplication(app["id"].(string), map[string]any{"appRoles": []any{role(withValue, "Sync timesheets", "Timesheet.Sync", false), role(noValue, "No value", nil, true)}}, admin); perr != nil {
		t.Fatal(perr)
	}
	if got := claim(); len(got) != 0 {
		t.Errorf("roles after the role is disabled: %v, want none", got)
	}
	// the assignment is still there: the portal shows it, and an administrator can take it away
	if g, _ := e.world.GrantsOf(client1); len(g.Application) != 2 {
		t.Errorf("the client has %d application permissions, want its 2 assignments", len(g.Application))
	}
}
