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
