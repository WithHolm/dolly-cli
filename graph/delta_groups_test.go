package graph_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

func memberRef(id string) string {
	return `{"@odata.id":"https://graph.microsoft.com/v1.0/directoryObjects/` + id + `"}`
}

// memberDeltas reads members@delta of an item: the ids that went in, and the ids that went out (with @removed).
func memberDeltas(item map[string]any) (in, out map[string]map[string]any) {
	in, out = map[string]map[string]any{}, map[string]map[string]any{}
	list, _ := item["members@delta"].([]any)
	for _, m := range list {
		m := m.(map[string]any)
		if _, removed := m["@removed"]; removed {
			out[m["id"].(string)] = m
		} else {
			in[m["id"].(string)] = m
		}
	}
	return in, out
}

// The first round has every group, with the default properties, and members@delta with the members (each with its @odata.type). $select without
// members leaves members@delta out, and $select with members brings it back (checked with the real Graph).
func TestDelta_Groups_FirstRound(t *testing.T) {
	ts, w := serverOf(t, 60)
	items, pages, _ := deltaRound(t, ts, "/groups/delta", nil)
	if len(items) != len(w.ListGroups()) || pages[0].Context != "https://graph.microsoft.com/v1.0/$metadata#groups" {
		t.Fatalf("%d groups, context %s", len(items), pages[0].Context)
	}
	withMembers := 0
	for _, it := range items {
		for k := range it {
			switch k {
			case "id", "displayName", "description", "mailEnabled", "mailNickname", "securityEnabled", "members@delta", "createdDateTime":
			default:
				t.Fatalf("a property that is not tracked: %s in %v", k, it)
			}
		}
		if in, _ := memberDeltas(it); len(in) > 0 {
			withMembers++
			for _, m := range in {
				if m["@odata.type"] == nil || len(m) != 2 {
					t.Fatalf("a member is its type and id: %v", m)
				}
			}
		}
	}
	if withMembers == 0 {
		t.Error("some groups have members")
	}

	noMembers, _, _ := deltaRound(t, ts, "/groups/delta?$select=displayName", nil)
	for _, it := range noMembers {
		if _, has := it["members@delta"]; has || len(it) != 2 {
			t.Fatalf("$select=displayName: no members@delta: %v", it)
		}
	}
	asked, _, _ := deltaRound(t, ts, "/groups/delta?$select=displayName,members", nil)
	got := 0
	for _, it := range asked {
		if _, has := it["members@delta"]; has {
			got++
		}
	}
	if got != withMembers {
		t.Errorf("$select with members: %d groups have members@delta, want %d", got, withMembers)
	}
}

// Members that go in or out show the group with members@delta: the user that came, and the one that went with @removed. A change to the group
// itself shows its properties. A group that is made has all its members, and a group that is deleted is its id and @removed.
func TestDelta_Groups_Changes(t *testing.T) {
	ts, w := serverOf(t, 60)
	_, _, deltaLink := deltaRound(t, ts, "/groups/delta", nil)

	users := w.ListUsers()
	u1, u2 := users[0]["id"].(string), users[1]["id"].(string)
	_, g := graphCall(t, ts, "POST", "/groups", `{"displayName":"Delta test","mailEnabled":false,"mailNickname":"deltatest","securityEnabled":true}`)
	gid := g["id"].(string)
	items, _, deltaLink := deltaRound(t, ts, deltaLink, nil)
	got := byID(items)
	if got[gid]["displayName"] != "Delta test" || len(items) != 1 {
		t.Fatalf("a made group: %v", items)
	}

	graphCall(t, ts, "POST", "/groups/"+gid+"/members/$ref", memberRef(u1))
	graphCall(t, ts, "POST", "/groups/"+gid+"/members/$ref", memberRef(u2))
	items, _, deltaLink = deltaRound(t, ts, deltaLink, nil)
	in, out := memberDeltas(byID(items)[gid])
	if len(items) != 1 || len(in) != 2 || len(out) != 0 || in[u1]["@odata.type"] != "#microsoft.graph.user" || byID(items)[gid]["displayName"] != "Delta test" {
		t.Fatalf("two members came: %v", items)
	}

	graphCall(t, ts, "DELETE", "/groups/"+gid+"/members/"+u1+"/$ref", "")
	graphCall(t, ts, "PATCH", "/groups/"+gid, `{"description":"New text"}`)
	items, _, deltaLink = deltaRound(t, ts, deltaLink, nil)
	in, out = memberDeltas(byID(items)[gid])
	if len(in) != 0 || out[u1]["@removed"].(map[string]any)["reason"] != "deleted" || byID(items)[gid]["description"] != "New text" {
		t.Fatalf("a member went, and the group changed: %v", items)
	}

	// a user that is deleted leaves its groups: the group shows with the member removed. The object is gone, so it has no @odata.type.
	graphCall(t, ts, "DELETE", "/users/"+u2, "")
	items, _, deltaLink = deltaRound(t, ts, deltaLink, nil)
	if _, out = memberDeltas(byID(items)[gid]); out[u2] == nil || out[u2]["@odata.type"] != nil {
		t.Fatalf("a deleted user went out of its group: %v", items)
	}

	// a change to a property that is not tracked shows nothing, and $select=members tracks only the members
	graphCall(t, ts, "PATCH", "/groups/"+gid, `{"displayName":"Renamed"}`)
	if items, _, _ = deltaRound(t, ts, deltaLink, nil); len(items) != 1 || items[0]["displayName"] != "Renamed" {
		t.Fatalf("a renamed group: %v", items)
	}

	graphCall(t, ts, "DELETE", "/groups/"+gid, "")
	items, _, _ = deltaRound(t, ts, deltaLink, nil)
	if len(items) != 1 || items[0]["id"] != gid || items[0]["@removed"] == nil {
		t.Errorf("a deleted group: %v", items)
	}
}

// A dynamic group takes and loses members when users change. Those changes have no event of their own, and they show in members@delta
// all the same.
func TestDelta_Groups_DynamicMembers(t *testing.T) {
	ts, w := serverOf(t, 60)
	call := func(method, path, body string) (int, map[string]any) { return graphCall(t, ts, method, path, body) }
	_, g := call("POST", "/groups", `{"displayName":"Dyn","mailEnabled":false,"mailNickname":"dyn","securityEnabled":true,
		"groupTypes":["DynamicMembership"],"membershipRule":"(user.department -eq \"Zebra\")","membershipRuleProcessingState":"On"}`)
	gid := g["id"].(string)
	_, _, link := deltaRound(t, ts, "/groups/delta", nil)

	uid := w.ListUsers()[0]["id"].(string)
	call("PATCH", "/users/"+uid, `{"department":"Zebra"}`)
	items, _, link := deltaRound(t, ts, link, nil)
	if in, _ := memberDeltas(byID(items)[gid]); len(items) != 1 || in[uid] == nil {
		t.Fatalf("the user came into the dynamic group: %v", items)
	}
	call("PATCH", "/users/"+uid, `{"department":"Horse"}`)
	items, _, _ = deltaRound(t, ts, link, nil)
	if _, out := memberDeltas(byID(items)[gid]); len(items) != 1 || out[uid] == nil {
		t.Fatalf("the user went out of the dynamic group: %v", items)
	}
}

// The members that a dynamic group takes are worked out again when the history is replayed, and they are in the log of changes then too:
// a deltaLink from before a restart still shows them. Loading the data is not a change.
func TestDelta_Groups_AfterARestart(t *testing.T) {
	dir := t.TempDir()
	w, err := graph.Open(dir, graph.OpenOptions{Users: 40, Seed: 5})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(graph.New(w, graph.ServerOptions{}).Handler())
	_, g := graphCall(t, ts, "POST", "/groups", `{"displayName":"Dyn","mailEnabled":false,"mailNickname":"dyn","securityEnabled":true,
		"groupTypes":["DynamicMembership"],"membershipRule":"(user.department -eq \"Zebra\")"}`)
	gid := g["id"].(string)
	_, _, deltaLink := deltaRound(t, ts, "/groups/delta", nil)
	uid := w.ListUsers()[0]["id"].(string)
	graphCall(t, ts, "PATCH", "/users/"+uid, `{"department":"Zebra"}`)
	host := strings.TrimPrefix(ts.URL, "http://")
	ts.Close()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	again, err := graph.Open(dir, graph.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	ts2 := httptest.NewServer(graph.New(again, graph.ServerOptions{}).Handler())
	defer ts2.Close()
	items, _, next := deltaRound(t, ts2, strings.Replace(deltaLink, host, strings.TrimPrefix(ts2.URL, "http://"), 1), nil)
	if in, _ := memberDeltas(byID(items)[gid]); len(items) != 1 || in[uid] == nil {
		t.Fatalf("after a restart: %v", items)
	}
	if again2, _, _ := deltaRound(t, ts2, next, nil); len(again2) != 0 {
		t.Errorf("loading the data made changes: %v", again2)
	}
}
