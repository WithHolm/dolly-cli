package graph_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

// A tool asks /organization for the name of its tenant.
func TestOrganization_ListAndGet(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	call := func(path string) (int, map[string]any) { return graphCall(t, ts, "GET", path, "") }

	code, out := call("/organization")
	list, _ := out["value"].([]any)
	if code != 200 || len(list) != 1 {
		t.Fatalf("list: %d %v", code, out)
	}
	org := list[0].(map[string]any)
	if org["id"] == "" || org["displayName"] != "Acme Corporation (fake)" || org["tenantType"] != "AAD" {
		t.Errorf("organization: %v", org)
	}
	defaults := 0
	for _, d := range org["verifiedDomains"].([]any) {
		if d.(map[string]any)["isDefault"] == true {
			defaults++
		}
	}
	if defaults != 1 {
		t.Errorf("exactly one verified domain is the default, got %d", defaults)
	}

	id := org["id"].(string)
	if code, one := call("/organization/" + id + "?$select=id,displayName"); code != 200 || one["id"] != id || one["displayName"] == nil || one["city"] != nil {
		t.Errorf("get with $select: %d %v", code, one)
	}
	if code, _ := call("/organization/11111111-1111-1111-1111-111111111111"); code != 404 {
		t.Errorf("another id: %d", code)
	}
	// $select on the list works as it does for the other lists
	_, sel := call("/organization?$select=id,verifiedDomains")
	if o := sel["value"].([]any)[0].(map[string]any); o["verifiedDomains"] == nil || o["displayName"] != nil {
		t.Errorf("$select on the list: %v", o)
	}
}

// The fake tenant has the all-zero ID, so nobody takes it for a real tenant. A seed from before
// the tenant existed gets it too.
func TestOrganization_OldSeedGetsTheFakeTenant(t *testing.T) {
	dir := t.TempDir()
	data, err := json.Marshal(map[string]any{"seed": 3, "users": []any{}, "groups": []any{}, "servicePrincipals": []any{}, "assignments": []any{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "seed.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	w, err := graph.Open(dir, graph.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if o := w.ListOrganizations(); len(o) != 1 || o[0]["id"] != zeroTenant {
		t.Errorf("an old seed: %v", o)
	}
}

const zeroTenant = "00000000-0000-0000-0000-000000000000"

// A new world has the same fake tenant, whatever the seed.
func TestOrganization_NewWorldHasTheFakeTenant(t *testing.T) {
	for _, seed := range []int64{1, 4, 5} {
		if id := graph.Generate(graph.Options{Users: 5, Seed: seed}).ListOrganizations()[0]["id"]; id != zeroTenant {
			t.Errorf("seed %d: %v", seed, id)
		}
	}
}
