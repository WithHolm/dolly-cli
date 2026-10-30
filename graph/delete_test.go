package graph_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

// ids reads the IDs of the objects in the "value" of an answer.
func ids(out map[string]any) []string {
	var list []string
	v, _ := out["value"].([]any)
	for _, o := range v {
		list = append(list, o.(map[string]any)["id"].(string))
	}
	return list
}

func has(list []string, id string) bool {
	for _, x := range list {
		if x == id {
			return true
		}
	}
	return false
}

func TestDelete_UserGoesOutOfEveryGroup(t *testing.T) {
	ts, _, srv := startServer(t, graph.ServerOptions{})
	_, u := graphCall(t, ts, "POST", "/users", newUser)
	uid := u["id"].(string)
	_, g := graphCall(t, ts, "POST", "/groups", `{"displayName":"grp.del","mailEnabled":false,"mailNickname":"del","securityEnabled":true,
	  "members@odata.bind":["https://graph.microsoft.com/v1.0/users/`+uid+`"],"owners@odata.bind":["https://graph.microsoft.com/v1.0/users/`+uid+`"]}`)
	gid := g["id"].(string)
	users := srv.World().Snapshot().Users

	if code, _ := graphCall(t, ts, "DELETE", "/users/"+uid, ""); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if c, _ := graphCall(t, ts, "GET", "/users/"+uid, ""); c != 404 {
		t.Errorf("the user must be gone: %d", c)
	}
	if srv.World().Snapshot().Users != users-1 {
		t.Error("one user fewer")
	}
	_, members := graphCall(t, ts, "GET", "/groups/"+gid+"/members", "")
	_, owners := graphCall(t, ts, "GET", "/groups/"+gid+"/owners", "")
	if len(ids(members)) != 0 || len(ids(owners)) != 0 {
		t.Errorf("the user must be out of the group, as a member and as an owner: %v %v", ids(members), ids(owners))
	}
	if code, _ := graphCall(t, ts, "DELETE", "/users/"+uid, ""); code != 404 {
		t.Errorf("a second delete: %d", code)
	}
	if code, _ := graphCall(t, ts, "DELETE", "/users/nobody@acme.example", ""); code != 404 {
		t.Errorf("an unknown user: %d", code)
	}
	// by user principal name
	_, u2 := graphCall(t, ts, "POST", "/users", strings.Replace(strings.Replace(newUser, "ada.test", "bob.test", 2), "Ada Test", "Bob Test", 1))
	if code, _ := graphCall(t, ts, "DELETE", "/users/"+u2["userPrincipalName"].(string), ""); code != 204 {
		t.Errorf("delete by user principal name: %d", code)
	}
}

func TestDelete_GroupGoesOutOfTheGroupsThatHeldIt_AndTheAppAssignments(t *testing.T) {
	ts, c, _ := startServer(t, graph.ServerOptions{})
	_ = c
	_, main := graphCall(t, ts, "GET", "/groups?$top=999&$select=id,displayName", "")
	var mainID, contribID string
	for _, o := range main["value"].([]any) {
		m := o.(map[string]any)
		switch m["displayName"] {
		case "grp.app.acme.main":
			mainID = m["id"].(string)
		case "grp.app.acme.contributor":
			contribID = m["id"].(string)
		}
	}
	_, before := graphCall(t, ts, "GET", "/groups/"+mainID+"/members?$top=999", "")
	if !has(ids(before), contribID) {
		t.Fatal("the target holds the contributor group")
	}
	_, sps := graphCall(t, ts, "GET", "/servicePrincipals?$top=999&$select=id,displayName", "")
	var spID string
	for _, o := range sps["value"].([]any) {
		if m := o.(map[string]any); m["displayName"] == "Acme Portal" {
			spID = m["id"].(string)
		}
	}
	_, assigned := graphCall(t, ts, "GET", "/servicePrincipals/"+spID+"/appRoleAssignedTo", "")
	assignedBefore := len(ids(assigned))

	if code, _ := graphCall(t, ts, "DELETE", "/groups/"+contribID, ""); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if c, _ := graphCall(t, ts, "GET", "/groups/"+contribID, ""); c != 404 {
		t.Errorf("the group must be gone: %d", c)
	}
	_, after := graphCall(t, ts, "GET", "/groups/"+mainID+"/members?$top=999", "")
	if has(ids(after), contribID) || len(ids(after)) != len(ids(before))-1 {
		t.Errorf("the group must be out of the group that held it: %d -> %d", len(ids(before)), len(ids(after)))
	}
	_, assigned = graphCall(t, ts, "GET", "/servicePrincipals/"+spID+"/appRoleAssignedTo", "")
	if len(ids(assigned)) != assignedBefore-1 {
		t.Errorf("the role assignment of the group must go: %d -> %d", assignedBefore, len(ids(assigned)))
	}
	if code, _ := graphCall(t, ts, "DELETE", "/groups/"+contribID, ""); code != 404 {
		t.Errorf("a second delete: %d", code)
	}
}

func TestOwners_AddListRemove_AndTheRules(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	_, u := graphCall(t, ts, "POST", "/users", newUser)
	uid := u["id"].(string)
	_, g := graphCall(t, ts, "POST", "/groups", `{"displayName":"grp.own","mailEnabled":false,"mailNickname":"own","securityEnabled":true}`)
	gid := g["id"].(string)
	ref := `{"@odata.id":"https://graph.microsoft.com/v1.0/users/` + uid + `"}`

	if code, _ := graphCall(t, ts, "POST", "/groups/"+gid+"/owners/$ref", ref); code != 204 {
		t.Fatalf("add: %d", code)
	}
	_, owners := graphCall(t, ts, "GET", "/groups/"+gid+"/owners", "")
	list, _ := owners["value"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["@odata.type"] != "#microsoft.graph.user" || list[0].(map[string]any)["id"] != uid {
		t.Errorf("owners: %+v", owners)
	}
	if code, out := graphCall(t, ts, "POST", "/groups/"+gid+"/owners/$ref", ref); code != 400 || !strings.Contains(errText(out), "'owners'") {
		t.Errorf("an owner twice: %d %q", code, errText(out))
	}
	if code, out := graphCall(t, ts, "POST", "/groups/"+gid+"/owners/$ref", `{"@odata.id":"x/`+gid+`"}`); code != 400 || !strings.Contains(errText(out), "group cannot be an owner") {
		t.Errorf("a group as owner: %d %q", code, errText(out))
	}
	if code, _ := graphCall(t, ts, "POST", "/groups/"+gid+"/owners/$ref", `{"@odata.id":"x/00000000-0000-0000-0000-00000000dead"}`); code != 404 {
		t.Errorf("an owner that does not exist: %d", code)
	}
	if code, _ := graphCall(t, ts, "POST", "/groups/"+gid+"/owners/$ref", `{}`); code != 400 {
		t.Errorf("no @odata.id: %d", code)
	}
	if code, _ := graphCall(t, ts, "DELETE", "/groups/"+gid+"/owners/"+uid+"/$ref", ""); code != 204 {
		t.Errorf("remove: %d", code)
	}
	if code, _ := graphCall(t, ts, "DELETE", "/groups/"+gid+"/owners/"+uid+"/$ref", ""); code != 404 {
		t.Errorf("remove again: %d", code)
	}
	// owners are for the owners calls: not for a group update
	if code, out := graphCall(t, ts, "PATCH", "/groups/"+gid, `{"owners@odata.bind":["x/`+uid+`"]}`); code != 400 || !strings.Contains(errText(out), "owners calls") {
		t.Errorf("owners on update: %d %q", code, errText(out))
	}
	if code, out := graphCall(t, ts, "POST", "/groups", `{"displayName":"a","mailEnabled":false,"mailNickname":"a","securityEnabled":true,"owners@odata.bind":["https://graph.microsoft.com/v1.0/groups/`+gid+`"]}`); code != 400 || !strings.Contains(errText(out), "group cannot be an owner") {
		t.Errorf("a group as an owner at create: %d %q", code, errText(out))
	}
}

// Deletes and owners are in the history, and a restart and a rollback agree with them.
func TestDeleteAndOwners_AreInTheHistory_SurviveARestart_AndRollBack(t *testing.T) {
	dir := t.TempDir()
	w := openWorld(t, dir, nil)
	ts := httptest.NewServer(graph.New(w, graph.ServerOptions{}).Handler())

	_, u := graphCall(t, ts, "POST", "/users", newUser)
	uid := u["id"].(string)
	_, g := graphCall(t, ts, "POST", "/groups", `{"displayName":"grp.h","mailEnabled":false,"mailNickname":"h","securityEnabled":true,"members@odata.bind":["https://graph.microsoft.com/v1.0/users/`+uid+`"]}`)
	gid := g["id"].(string)
	graphCall(t, ts, "POST", "/groups/"+gid+"/owners/$ref", `{"@odata.id":"x/`+uid+`"}`)
	graphCall(t, ts, "DELETE", "/users/"+uid, "")
	graphCall(t, ts, "DELETE", "/groups/"+gid, "")
	ts.Close()

	var texts []string
	var deleteUser int
	for _, e := range w.History() {
		texts = append(texts, e.Text())
		if e.Type == "user.delete" {
			deleteUser = e.Seq
		}
	}
	all := strings.Join(texts, "\n")
	for _, want := range []string{"Made %s an owner of grp.h", "Deleted the user ada.test@acme.example (it was in 1 group)", "Deleted the group grp.h (it was in 0 groups)"} {
		if !strings.Contains(all, strings.Replace(want, "%s", "ada.test@acme.example", 1)) {
			t.Errorf("history must say %q:\n%s", want, all)
		}
	}

	// a restart: both are gone
	_ = w.Close()
	again := openWorld(t, dir, nil)
	srv2 := httptest.NewServer(graph.New(again, graph.ServerOptions{}).Handler())
	defer srv2.Close()
	if c, _ := graphCall(t, srv2, "GET", "/users/"+uid, ""); c != 404 {
		t.Errorf("the user must stay deleted after a restart: %d", c)
	}
	if c, _ := graphCall(t, srv2, "GET", "/groups/"+gid, ""); c != 404 {
		t.Errorf("the group must stay deleted after a restart: %d", c)
	}

	// a rollback to before the delete of the user: the user and the group are back, with the member and the owner
	if err := again.Rollback(deleteUser-1, person); err != nil {
		t.Fatal(err)
	}
	if c, _ := graphCall(t, srv2, "GET", "/users/"+uid, ""); c != 200 {
		t.Errorf("the user must come back with its ID: %d", c)
	}
	_, members := graphCall(t, srv2, "GET", "/groups/"+gid+"/members", "")
	_, owners := graphCall(t, srv2, "GET", "/groups/"+gid+"/owners", "")
	if !has(ids(members), uid) || !has(ids(owners), uid) {
		t.Errorf("the group must have its member and its owner again: %v %v", ids(members), ids(owners))
	}
}
