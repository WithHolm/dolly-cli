package graph_test

import (
	"encoding/json"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/withholm/dolly-cli/graph"
)

type call func(method, path, body string) (int, map[string]any)

// usersWhere gives the IDs of the users with this department.
func usersIn(t *testing.T, c call, department string) []string {
	t.Helper()
	_, out := c("GET", "/users?$top=999&$select=id,department&$filter="+url.QueryEscape("department eq '"+department+"'"), "")
	return ids(out)
}

func membersOf(t *testing.T, c call, group string) []string {
	t.Helper()
	code, out := c("GET", "/groups/"+group+"/members?$top=999", "")
	if code != 200 {
		t.Fatalf("members of %s: %d", group, code)
	}
	return ids(out)
}

func sameSet(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

func newDynamic(t *testing.T, c call, name, rule string) string {
	t.Helper()
	code, g := c("POST", "/groups", `{"displayName":"`+name+`","mailEnabled":false,"mailNickname":"`+name+`","securityEnabled":true,
	  "groupTypes":["DynamicMembership"],"membershipRule":`+quote(rule)+`,"membershipRuleProcessingState":"On"}`)
	if code != 201 {
		t.Fatalf("make %s: %d %v", name, code, g)
	}
	return g["id"].(string)
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// The members of a dynamic group come from its rule, at once.
func TestDynamic_MembersComeFromTheRule(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	c := call(func(m, p, b string) (int, map[string]any) { return graphCall(t, ts, m, p, b) })

	sales := usersIn(t, c, "Sales")
	if len(sales) == 0 {
		t.Fatal("the seed has no Sales users")
	}
	gid := newDynamic(t, c, "tst.dyn.sales", `(user.department -eq "Sales")`)
	if got := membersOf(t, c, gid); !sameSet(got, sales) {
		t.Errorf("the group has %d members, the Sales users are %d", len(got), len(sales))
	}
	_, g := c("GET", "/groups/"+gid, "")
	if g["membershipRuleProcessingState"] != "On" || g["membershipRule"] == nil {
		t.Errorf("the group: %v", g)
	}

	// a rule with more in it: and, or, not, a list, and a start of a word
	gid2 := newDynamic(t, c, "tst.dyn.mix", `(user.department -in ["Sales","Finance"]) -and -not (user.jobTitle -startsWith "zzz") -and (user.accountEnabled -eq true)`)
	if len(membersOf(t, c, gid2)) < len(sales) {
		t.Error("Sales and Finance are more users than Sales")
	}
	// the seed has a dynamic group too, and its members come from its rule now
	_, list := c("GET", "/groups?$filter="+url.QueryEscape("startswith(displayName,'grp.dyn.')"), "")
	for _, id := range ids(list) {
		if len(membersOf(t, c, id)) == 0 {
			t.Errorf("the seed group %s has no members", id)
		}
	}
}

// The members follow the users: a change of the user, a new user, and a deleted user.
func TestDynamic_FollowsTheUsers(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	c := call(func(m, p, b string) (int, map[string]any) { return graphCall(t, ts, m, p, b) })
	gid := newDynamic(t, c, "tst.dyn.sales", `(user.department -eq "Sales")`)
	before := len(membersOf(t, c, gid))

	// a new user in Sales is a member at once
	_, u := c("POST", "/users", `{"accountEnabled":true,"displayName":"New Sales","mailNickname":"new.sales","userPrincipalName":"new.sales@acme.example","department":"Sales","passwordProfile":{"password":"x"}}`)
	uid := u["id"].(string)
	if got := membersOf(t, c, gid); len(got) != before+1 || !slices.Contains(got, uid) {
		t.Fatalf("a new Sales user: %d members, want %d", len(got), before+1)
	}
	// a change of the department takes the user out, and back in
	c("PATCH", "/users/"+uid, `{"department":"Legal"}`)
	if slices.Contains(membersOf(t, c, gid), uid) {
		t.Error("the user moved to Legal and is still a member")
	}
	c("PATCH", "/users/"+uid, `{"department":"sales"}`) // a rule is not case sensitive
	if !slices.Contains(membersOf(t, c, gid), uid) {
		t.Error("the user is in Sales again and is not a member")
	}
	// a deleted user is gone
	c("DELETE", "/users/"+uid, "")
	if got := membersOf(t, c, gid); len(got) != before || slices.Contains(got, uid) {
		t.Errorf("after the delete: %d members, want %d", len(got), before)
	}
}

func TestDynamic_ChangeTheRuleAndPauseIt(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	c := call(func(m, p, b string) (int, map[string]any) { return graphCall(t, ts, m, p, b) })
	gid := newDynamic(t, c, "tst.dyn.x", `(user.department -eq "Sales")`)

	// a new rule gives new members
	if code, _ := c("PATCH", "/groups/"+gid, `{"membershipRule":"(user.department -eq \"Finance\")"}`); code != 204 {
		t.Fatalf("patch the rule: %d", code)
	}
	finance := usersIn(t, c, "Finance")
	if got := membersOf(t, c, gid); !sameSet(got, finance) {
		t.Errorf("after the new rule: %d members, Finance has %d", len(got), len(finance))
	}

	// a paused group keeps its members, whatever the users do
	if code, _ := c("PATCH", "/groups/"+gid, `{"membershipRuleProcessingState":"Paused"}`); code != 204 {
		t.Fatalf("pause: %d", code)
	}
	_, g := c("GET", "/groups/"+gid, "")
	if g["membershipRuleProcessingState"] != "Paused" {
		t.Errorf("state: %v", g["membershipRuleProcessingState"])
	}
	_, u := c("POST", "/users", `{"accountEnabled":true,"displayName":"Late","mailNickname":"late","userPrincipalName":"late@acme.example","department":"Finance","passwordProfile":{"password":"x"}}`)
	if got := membersOf(t, c, gid); !sameSet(got, finance) {
		t.Error("a paused group changed its members")
	}
	// On again: it catches up
	c("PATCH", "/groups/"+gid, `{"membershipRuleProcessingState":"On"}`)
	if got := membersOf(t, c, gid); !slices.Contains(got, u["id"].(string)) || len(got) != len(finance)+1 {
		t.Errorf("after On again: %d members, want %d", len(got), len(finance)+1)
	}
	// a rename, the way cohort does it for a frozen group: the members stay
	if code, _ := c("PATCH", "/groups/"+gid, `{"displayName":"tst.dyn.x_DEPRECATED_1","membershipRuleProcessingState":"Paused"}`); code != 204 {
		t.Errorf("freeze: %d", code)
	}
}

// What Entra refuses when a rule is saved.
func TestDynamic_ABadRuleIsRefused(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	c := call(func(m, p, b string) (int, map[string]any) { return graphCall(t, ts, m, p, b) })
	good := newDynamic(t, c, "tst.dyn.good", `(user.department -eq "Sales")`)

	for name, rule := range map[string]string{
		"an unknown property": `(user.departmnt -eq "Sales")`,
		"no operator":         `(user.department "Sales")`,
		"a missing quote":     `(user.department -eq "Sales)`,
		"a device rule":       `(device.displayName -eq "x")`,
		"an unknown operator": `(user.department -equals "Sales")`,
		"memberOf":            `user.memberOf -any (group.objectId -in ["x"])`,
	} {
		code, out := c("POST", "/groups", `{"displayName":"tst.bad","mailEnabled":false,"mailNickname":"bad","securityEnabled":true,
		  "groupTypes":["DynamicMembership"],"membershipRule":`+quote(rule)+`}`)
		if code != 400 {
			t.Errorf("%s: make a group: %d, want 400 (%v)", name, code, out)
		}
		if code, _ := c("PATCH", "/groups/"+good, `{"membershipRule":`+quote(rule)+`}`); code != 400 {
			t.Errorf("%s: patch: %d, want 400", name, code)
		}
	}
	// the state: only On and Paused, and only for a dynamic group
	if code, _ := c("PATCH", "/groups/"+good, `{"membershipRuleProcessingState":"Sleeping"}`); code != 400 {
		t.Errorf("a bad state: %d", code)
	}
	_, plain := c("POST", "/groups", `{"displayName":"tst.plain","mailEnabled":false,"mailNickname":"plain","securityEnabled":true}`)
	if code, _ := c("PATCH", "/groups/"+plain["id"].(string), `{"membershipRuleProcessingState":"Paused"}`); code != 400 {
		t.Errorf("a state on a group that is not dynamic: %d", code)
	}
	// the rule did not change
	if _, g := c("GET", "/groups/"+good, ""); g["membershipRule"] != `(user.department -eq "Sales")` {
		t.Errorf("a refused patch changed the rule: %v", g["membershipRule"])
	}
}

// The CLI changes more properties of a user (POST /_dolly/graph/user), and a rule follows. The result is read through Graph.
func TestDynamic_AdminUserFields(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	c := call(func(m, p, b string) (int, map[string]any) { return graphCall(t, ts, m, p, b) })
	_, list := c("GET", "/users?$top=1&$select=id", "")
	id := list["value"].([]any)[0].(map[string]any)["id"].(string)

	gid := newDynamic(t, c, "tst.dyn.fields", `(user.city -eq "Faketown") -and (user.extensionAttribute7 -eq "X7") -and (user.accountEnabled -eq false)`)
	body := `{"user":"` + id + `","set":{"city":"Faketown","accountEnabled":false,"onPremisesExtensionAttributes":{"extensionAttribute7":"X7"}}}`
	if s := admin(t, ts, "POST", "/user", body, nil); s != 200 {
		t.Fatalf("set: %d", s)
	}
	if got := membersOf(t, c, gid); len(got) != 1 || got[0] != id {
		t.Errorf("members %v, want the user", got)
	}
	// the other extension attributes stay (the merge)
	admin(t, ts, "POST", "/user", `{"user":"`+id+`","set":{"onPremisesExtensionAttributes":{"extensionAttribute8":"X8"}}}`, nil)
	_, u := c("GET", "/users/"+id+"?$select=onPremisesExtensionAttributes", "")
	ext, _ := u["onPremisesExtensionAttributes"].(map[string]any)
	if ext["extensionAttribute7"] != "X7" || ext["extensionAttribute8"] != "X8" {
		t.Errorf("extension attributes: %v", ext)
	}
}

// The rule "Direct Reports for" follows the manager.
func TestDynamic_DirectReports(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	c := call(func(m, p, b string) (int, map[string]any) { return graphCall(t, ts, m, p, b) })
	mk := func(n string) string {
		_, u := c("POST", "/users", `{"accountEnabled":true,"displayName":"`+n+`","mailNickname":"`+n+`","userPrincipalName":"`+n+`@acme.example","passwordProfile":{"password":"x"}}`)
		return u["id"].(string)
	}
	boss, a, b := mk("dr.boss"), mk("dr.a"), mk("dr.b")
	ref := func(id string) string { return `{"@odata.id":"https://graph.microsoft.com/v1.0/users/` + id + `"}` }
	c("PUT", "/users/"+a+"/manager/$ref", ref(boss))

	gid := newDynamic(t, c, "tst.dyn.reports", `Direct Reports for "`+boss+`"`)
	if got := membersOf(t, c, gid); !sameSet(got, []string{boss, a}) { // the manager is in it too
		t.Errorf("members %v, want the boss and a", got)
	}
	c("PUT", "/users/"+b+"/manager/$ref", ref(boss))
	if got := membersOf(t, c, gid); !sameSet(got, []string{boss, a, b}) {
		t.Errorf("after b got the boss: %v", got)
	}
	c("DELETE", "/users/"+a+"/manager/$ref", "")
	if got := membersOf(t, c, gid); !sameSet(got, []string{boss, b}) {
		t.Errorf("after a lost the boss: %v", got)
	}
	c("DELETE", "/users/"+boss, "")
	if got := membersOf(t, c, gid); len(got) != 0 {
		t.Errorf("after the boss is deleted: %v", got)
	}
}

// The members are not in the history: a replay works them out again, and a paused group keeps the
// members that it had when it was paused.
func TestDynamic_SurvivesARestartAndARollback(t *testing.T) {
	dir := t.TempDir()
	opt := graph.OpenOptions{Users: 60, Seed: 3, Profile: graph.AcmeProfile(), Now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }}
	w, err := graph.Open(dir, opt)
	if err != nil {
		t.Fatal(err)
	}
	props := func(rule string) map[string]any {
		return map[string]any{"displayName": "tst.dyn", "mailEnabled": false, "mailNickname": "d", "securityEnabled": true,
			"groupTypes": []any{"DynamicMembership"}, "membershipRule": rule}
	}
	g, e := w.CreateGroup(props(`(user.department -eq "Sales")`), graph.Actor{})
	if e != nil {
		t.Fatal(e)
	}
	gid := g["id"].(string)
	members := func(w *graph.World) []string {
		items, _ := w.Members(gid, false)
		var out []string
		for _, o := range items {
			out = append(out, o["id"].(string))
		}
		return out
	}
	salesNow := members(w)
	if len(salesNow) == 0 {
		t.Fatal("no Sales users")
	}

	// pause it, then move every Sales user away: a paused group keeps its members
	if e := w.UpdateGroup(gid, map[string]any{"membershipRuleProcessingState": "Paused"}, graph.Actor{}); e != nil {
		t.Fatal(e)
	}
	w.Rename("department", "Sales", "Legal", graph.Actor{})
	if got := members(w); !sameSet(got, salesNow) {
		t.Fatalf("the paused group changed: %d members, had %d", len(got), len(salesNow))
	}
	_ = w.Close()

	// after a restart (a replay): the same
	w2, err := graph.Open(dir, opt)
	if err != nil {
		t.Fatal(err)
	}
	defer w2.Close()
	if got := members(w2); !sameSet(got, salesNow) {
		t.Errorf("after the restart: %d members, want %d", len(got), len(salesNow))
	}
	// On again: nobody is in Sales now
	if e := w2.UpdateGroup(gid, map[string]any{"membershipRuleProcessingState": "On"}, graph.Actor{}); e != nil {
		t.Fatal(e)
	}
	if got := members(w2); len(got) != 0 {
		t.Errorf("On again, with nobody in Sales: %d members", len(got))
	}
	// a rollback to before the rename gives the members back
	if err := w2.Rollback(2, graph.Actor{}); err != nil { // 1 made the group, 2 paused it
		t.Fatal(err)
	}
	if got := members(w2); !sameSet(got, salesNow) {
		t.Errorf("after the rollback: %d members, want %d", len(got), len(salesNow))
	}
}
