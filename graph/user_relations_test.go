package graph_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

// listIDs reads the sorted ids of the items of a Graph list.
func listIDs(t *testing.T, body string) []string {
	t.Helper()
	var l struct{ Value []struct{ ID string } }
	if err := json.Unmarshal([]byte(body), &l); err != nil {
		t.Fatalf("%v: %s", err, body)
	}
	out := []string{}
	for _, v := range l.Value {
		out = append(out, v.ID)
	}
	slices.Sort(out)
	return out
}

// The groups of a user can be read from two sides: the memberOf of the user, and the members of each group. The portal reads the
// first; a tool can read the second. They must agree, for every user and every group (a group is a member of a group too).
func TestMemberOf_AgreesWithTheMembersOfTheGroups(t *testing.T) {
	h := graph.New(graph.Generate(graph.Options{Users: 60, Seed: 1}), graph.ServerOptions{}).Handler()
	get := func(path string) string {
		rec := send(h, "GET", path, "")
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body)
		}
		return rec.Body.String()
	}

	groups := listIDs(t, get("/v1.0/groups?$select=id&$top=999"))
	members := map[string][]string{} // group -> the ids of its direct members
	for _, g := range groups {
		members[g] = listIDs(t, get("/v1.0/groups/"+g+"/members?$select=id&$top=999"))
	}
	heldBy := func(id string) []string {
		out := []string{}
		for _, g := range groups {
			if slices.Contains(members[g], id) {
				out = append(out, g)
			}
		}
		slices.Sort(out)
		return out
	}

	objects := append(listIDs(t, get("/v1.0/users?$select=id&$top=999")), groups...)
	inAGroup, inAGroupThatIsInAGroup := false, false
	for _, id := range objects {
		kind := "users"
		if slices.Contains(groups, id) {
			kind = "groups"
		}
		direct := listIDs(t, get("/v1.0/"+kind+"/"+id+"/memberOf?$select=id&$top=999"))
		if want := heldBy(id); !slices.Equal(direct, want) {
			t.Errorf("%s %s: memberOf is %v, the groups that have it as a member are %v", kind, id, direct, want)
		}
		all := listIDs(t, get("/v1.0/"+kind+"/"+id+"/transitiveMemberOf?$select=id&$top=999"))
		for _, g := range direct {
			if !slices.Contains(all, g) {
				t.Errorf("%s %s: transitiveMemberOf lacks the direct group %s", kind, id, g)
			}
		}
		if slices.Contains(all, id) {
			t.Errorf("%s %s is in its own transitiveMemberOf", kind, id)
		}
		inAGroup = inAGroup || len(direct) > 0
		inAGroupThatIsInAGroup = inAGroupThatIsInAGroup || len(all) > len(direct)
	}
	if !inAGroup || !inAGroupThatIsInAGroup {
		t.Error("the data has no group in a group, so the test checks too little")
	}

	for _, kind := range []string{"users", "groups"} {
		for _, edge := range []string{"memberOf", "transitiveMemberOf"} {
			if rec := send(h, "GET", "/v1.0/"+kind+"/nobody@nowhere/"+edge, ""); rec.Code != 404 {
				t.Errorf("%s of a %s that is not there: %d", edge, kind, rec.Code)
			}
		}
	}
}

// The app roles of a user can be read from the user (appRoleAssignments), and from the service principals (appRoleAssignedTo).
func TestUserAppRoleAssignments_AgreeWithTheServicePrincipals(t *testing.T) {
	h := graph.New(graph.Generate(graph.Options{Users: 60, Seed: 1}), graph.ServerOptions{}).Handler()
	get := func(path string) string {
		rec := send(h, "GET", path, "")
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body)
		}
		return rec.Body.String()
	}

	var assigned struct {
		Value []struct{ PrincipalID, PrincipalType string }
	}
	perUser := map[string]int{}
	for _, sp := range listIDs(t, get("/v1.0/servicePrincipals?$select=id&$top=999")) {
		if err := json.Unmarshal([]byte(get("/v1.0/servicePrincipals/"+sp+"/appRoleAssignedTo?$top=999")), &assigned); err != nil {
			t.Fatal(err)
		}
		for _, a := range assigned.Value {
			if a.PrincipalType == "User" {
				perUser[a.PrincipalID]++
			}
		}
	}
	if len(perUser) == 0 {
		t.Fatal("no user has an app role: the test checks nothing")
	}
	for _, u := range listIDs(t, get("/v1.0/users?$select=id&$top=999")) {
		var mine struct{ Value []any }
		if err := json.Unmarshal([]byte(get("/v1.0/users/"+u+"/appRoleAssignments")), &mine); err != nil {
			t.Fatal(err)
		}
		if len(mine.Value) != perUser[u] {
			t.Errorf("user %s: appRoleAssignments has %d, the service principals have %d", u, len(mine.Value), perUser[u])
		}
	}
	if rec := send(h, "GET", "/v1.0/users/nobody@nowhere/appRoleAssignments", ""); rec.Code != 404 {
		t.Errorf("appRoleAssignments of a user that is not there: %d", rec.Code)
	}
}
