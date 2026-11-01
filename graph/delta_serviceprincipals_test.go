package graph_test

import (
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

// The first round has every service principal with the properties of Graph (roles and scopes too, which the list does not have). A client
// that signs in for the first time makes a service principal, and the next round has it. Nothing else changes a service principal.
func TestDelta_ServicePrincipals(t *testing.T) {
	ts, w := serverOf(t, 20)
	items, pages, link := deltaRound(t, ts, "/servicePrincipals/delta", nil)
	if len(items) != len(w.ListServicePrincipals()) || pages[0].Context != "https://graph.microsoft.com/v1.0/$metadata#servicePrincipals" {
		t.Fatalf("%d service principals, context %s", len(items), pages[0].Context)
	}
	roles := 0
	for _, it := range items {
		if it["appId"] == nil || it["displayName"] == nil {
			t.Fatalf("a service principal has its client ID and its name: %v", it)
		}
		if l, _ := it["appRoles"].([]any); len(l) > 0 {
			roles++
		}
		if it["@odata.type"] != nil || it["tags"] != nil {
			t.Fatalf("only the tracked properties: %v", it)
		}
	}
	if roles == 0 {
		t.Error("Microsoft Graph has app roles, and the delta has them")
	}

	// the settings of dollycli are not changes of Graph
	sp := w.ListServicePrincipals()[0]
	if err := w.SetServicePrincipalFlag(sp["appId"].(string), "hideCalls", true, graph.Actor{Source: "admin"}); err != nil {
		t.Fatal(err)
	}
	if again, _, _ := deltaRound(t, ts, link, nil); len(again) != 0 {
		t.Errorf("a setting of dollycli is not a change: %v", again)
	}

	const client = "11111111-2222-3333-4444-555555555555"
	if _, err := w.EnsureServicePrincipal(client, graph.Actor{Source: "login"}); err != nil {
		t.Fatal(err)
	}
	got, _, next := deltaRound(t, ts, link, nil)
	if len(got) != 1 || got[0]["appId"] != client || got[0]["displayName"] == nil {
		t.Fatalf("the new service principal: %v", got)
	}
	if again, _, _ := deltaRound(t, ts, next, nil); len(again) != 0 {
		t.Errorf("the change comes once: %v", again)
	}
}
