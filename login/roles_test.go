package login_test

import (
	"net/url"
	"slices"
	"testing"

	graph "github.com/withholm/dolly-cli/graph"
)

// A user who is assigned app roles of an app gets them in the claim roles when the user signs in to the app: in the ID token, whose audience is the app
// (Learn: ID token claims, "roles"), and in the access token for the app as a resource. The value of each role is in the claim. A role of a group counts for
// the direct members of the group. These are not in the claim: a role with no value, a disabled role, Default access, and a role through a group in a group.

func claimStrings(t *testing.T, c map[string]any, name string) []string {
	t.Helper()
	list, _ := c[name].([]any)
	out := make([]string, len(list))
	for i, v := range list {
		out[i], _ = v.(string)
	}
	slices.Sort(out)
	return out
}

func TestRolesClaim_OfTheAssignedUser(t *testing.T) {
	e := setup(t)
	admin := graph.Actor{Source: "admin"}
	role := func(id, value string, on bool, types ...any) map[string]any {
		return map[string]any{"id": id, "value": value, "displayName": "Role " + id[:1], "description": "d", "allowedMemberTypes": types, "isEnabled": on}
	}
	noValue := role("55555555-5555-5555-5555-555555555555", "", true, "User")
	noValue["value"] = nil
	const (
		reader = "11111111-1111-1111-1111-111111111111"
		editor = "22222222-2222-2222-2222-222222222222"
		old    = "33333333-3333-3333-3333-333333333333"
	)
	app, perr := e.world.CreateApplication(map[string]any{"displayName": "Timesheets", "appRoles": []any{
		role(reader, "Timesheet.Reader", true, "User"),
		role(editor, "Timesheet.Editor", true, "User"),
		role(old, "Timesheet.Old", true, "User"),
		noValue,
	}}, admin)
	if perr != nil {
		t.Fatal(perr)
	}
	appID := app["appId"].(string)
	sp, perr := e.world.CreateServicePrincipal(map[string]any{"appId": appID}, admin)
	if perr != nil {
		t.Fatal(perr)
	}
	spID := sp["id"].(string)

	users := e.world.ListUsers()
	direct, viaGroup, viaNested, nobody := users[0], users[1], users[2], users[3]
	group, perr := e.world.CreateGroup(map[string]any{"displayName": "Editors", "mailEnabled": false, "mailNickname": "editors", "securityEnabled": true}, admin)
	if perr != nil {
		t.Fatal(perr)
	}
	outer, perr := e.world.CreateGroup(map[string]any{"displayName": "Everyone", "mailEnabled": false, "mailNickname": "everyone", "securityEnabled": true}, admin)
	if perr != nil {
		t.Fatal(perr)
	}
	for _, m := range [][2]string{{group["id"].(string), viaGroup["id"].(string)}, {outer["id"].(string), group["id"].(string)}, {outer["id"].(string), viaNested["id"].(string)}} {
		if err := e.world.AddMember(m[0], m[1], admin); err != nil {
			t.Fatal(err)
		}
	}
	assign := func(principal, roleID string) {
		t.Helper()
		if _, err := e.world.AssignAppRole(spID, principal, roleID, admin); err != nil {
			t.Fatalf("assign %s: %v", roleID, err)
		}
	}
	assign(direct["id"].(string), reader)
	assign(direct["id"].(string), "00000000-0000-0000-0000-000000000000") // Default access: no role
	assign(direct["id"].(string), "55555555-5555-5555-5555-555555555555") // a role with no value
	assign(group["id"].(string), editor)
	assign(outer["id"].(string), reader) // the direct members of Everyone have it. The group Editors is in Everyone, and its members do not get it.
	assign(direct["id"].(string), old)

	// the role Old is disabled afterwards: the claim leaves it out
	roles := []any{
		role(reader, "Timesheet.Reader", true, "User"), role(editor, "Timesheet.Editor", true, "User"),
		role(old, "Timesheet.Old", false, "User"), noValue,
	}
	if perr := e.world.UpdateApplication(app["id"].(string), map[string]any{"appRoles": roles}, admin); perr != nil {
		t.Fatal(perr)
	}

	path := "/" + e.tid + "/oauth2/v2.0/token"
	signIn := func(u map[string]any) (access, id map[string]any) {
		t.Helper()
		code, resp := e.post(path, url.Values{
			"grant_type": {"password"}, "client_id": {appID}, "username": {u["userPrincipalName"].(string)}, "password": {"x"},
			"scope": {"openid api://" + appID + "/.default"},
		})
		if code != 200 {
			t.Fatalf("sign in: %d %v", code, resp)
		}
		return claimsOf(t, resp["access_token"].(string)), claimsOf(t, resp["id_token"].(string))
	}
	want := func(who string, u map[string]any, roles ...string) {
		t.Helper()
		access, id := signIn(u)
		slices.Sort(roles)
		if got := claimStrings(t, id, "roles"); !slices.Equal(got, roles) {
			t.Errorf("%s: roles in the ID token %v, want %v", who, got, roles)
		}
		if got := claimStrings(t, access, "roles"); !slices.Equal(got, roles) {
			t.Errorf("%s: roles in the access token %v, want %v", who, got, roles)
		}
		if len(roles) == 0 {
			if _, has := id["roles"]; has {
				t.Errorf("%s: a user with no role has no claim roles: %v", who, id["roles"])
			}
		}
	}
	want("a user with a role of its own (Default access, no value, and the disabled role are left out)", direct, "Timesheet.Reader")
	want("a member of Editors, which is a group in Everyone: only the role of Editors", viaGroup, "Timesheet.Editor")
	want("a direct member of Everyone", viaNested, "Timesheet.Reader")
	want("a user with nothing", nobody)
}
