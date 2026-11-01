package graph_test

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
	"github.com/withholm/dolly-cli/graph/api"
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
	// Graph does not say 404 for another tenant id: it is a bad request (checked with the real Graph)
	if code, bad := call("/organization/11111111-1111-1111-1111-111111111111"); code != 400 || errCode(bad) != "Request_BadRequest" ||
		errText(bad) != "Invalid tenant identifier; it must match that of the requested tenant." {
		t.Errorf("another id: %d %v", code, bad)
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

func errCode(out map[string]any) string {
	e, _ := out["error"].(map[string]any)
	s, _ := e["code"].(string)
	return s
}

// The organization has every property that Graph gives (checked with the sample tenant of Graph Explorer): the same 28 names, with the
// kinds of value that Graph has, and a directory size that counts the objects.
func TestOrganization_HasEveryPropertyOfGraph(t *testing.T) {
	ts, _, srv := startServer(t, graph.ServerOptions{})
	_, out := graphCall(t, ts, "GET", "/organization", "")
	org := out["value"].([]any)[0].(map[string]any)

	if len(org) != len(api.DefaultOrganizationProperties) {
		t.Errorf("%d properties, want %d: %v", len(org), len(api.DefaultOrganizationProperties), org)
	}
	for _, name := range api.DefaultOrganizationProperties {
		if _, has := org[name]; !has {
			t.Errorf("no property %s", name)
		}
	}
	for _, list := range []string{"businessPhones", "marketingNotificationEmails", "securityComplianceNotificationMails", "securityComplianceNotificationPhones", "technicalNotificationMails", "assignedPlans", "provisionedPlans", "onPremisesSyncStatus", "verifiedDomains"} {
		if _, ok := org[list].([]any); !ok {
			t.Errorf("%s must be a list (an empty list, not null): %v", list, org[list])
		}
	}
	for _, null := range []string{"deletedDateTime", "isMultipleDataLocationsForServicesEnabled", "partnerTenantType"} {
		if v, has := org[null]; !has || v != nil {
			t.Errorf("%s must be there, and null: %v %v", null, v, has)
		}
	}
	if org["city"] != "Stockholm" || org["countryLetterCode"] != "SE" || org["defaultUsageLocation"] != "SE" || org["postalCode"] == "" || org["street"] == "" || org["state"] == "" {
		t.Errorf("the address: %v", org)
	}
	plan := org["assignedPlans"].([]any)[0].(map[string]any)
	if plan["service"] == "" || plan["servicePlanId"] == "" || plan["capabilityStatus"] != "Enabled" || plan["assignedDateTime"] == "" {
		t.Errorf("an assigned plan: %v", plan)
	}
	prov := org["provisionedPlans"].([]any)[0].(map[string]any)
	if prov["service"] == "" || prov["capabilityStatus"] != "Enabled" || prov["provisioningStatus"] != "Success" {
		t.Errorf("a provisioned plan: %v", prov)
	}
	if pp := org["privacyProfile"].(map[string]any); pp["contactEmail"] == "" || pp["statementUrl"] == "" {
		t.Errorf("privacyProfile: %v", pp)
	}
	// the directory size counts the users, the groups, and the service principals
	st := srv.World().Snapshot()
	q := org["directorySizeQuota"].(map[string]any)
	if q["total"] != float64(50000) || q["used"] != float64(st.Users+st.Groups+st.ServicePrincipals) {
		t.Errorf("directorySizeQuota %v, want used %d", q, st.Users+st.Groups+st.ServicePrincipals)
	}
	// one organization by id gives the same properties, and the context of one entity
	id := org["id"].(string)
	_, one := graphCall(t, ts, "GET", "/organization/"+id, "")
	if len(one) != len(api.DefaultOrganizationProperties)+1 || one["@odata.context"] != "https://graph.microsoft.com/v1.0/$metadata#organization/$entity" {
		t.Errorf("by id: %d keys, context %v", len(one), one["@odata.context"])
	}
	// $select: only what it names (no id unless it is named), a property of the resource with no value is null, and a name that is not one is dropped
	_, sel := graphCall(t, ts, "GET", "/organization?$select=displayName,partnerTenantType,nope", "")
	got := sel["value"].([]any)[0].(map[string]any)
	if len(got) != 2 || got["displayName"] == nil || got["partnerTenantType"] != nil {
		t.Errorf("$select: %v", got)
	}
	if _, has := got["partnerTenantType"]; !has {
		t.Error("a property with no value is null, not left out")
	}
}

// What Graph refuses on /organization (checked with the real Graph): it takes no $filter, no $orderby, no $skip, and no /$count. $count=true is
// left out of the answer, and $top follows the rules of a directory object.
func TestOrganization_QueriesAsGraphDoes(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	for _, c := range []struct {
		path, code, text string
	}{
		{"/organization?$filter=" + url.QueryEscape("displayName eq 'x'"), "Request_UnsupportedQuery", "Filtered searches against this resource are not supported."},
		{"/organization?$orderby=displayName", "Request_UnsupportedQuery", "Sorting is not supported for tenant queries."},
		{"/organization?$skip=1", "Request_BadRequest", "'$skip' is not supported by the service."},
		{"/organization?$top=0", "Request_UnsupportedQuery", "Invalid page size specified: '0'. Must be between 1 and 999 inclusive."},
		{"/organization/$count", "Request_UnsupportedQuery", "Raw count (/$count) is not supported because it is used with the unsupported entity 'Organization'. Refer to https://aka.ms/graph-docs/advanced-queries for more information"},
	} {
		code, out := graphCall(t, ts, "GET", c.path, "")
		if code != 400 || errCode(out) != c.code || errText(out) != c.text {
			t.Errorf("%s: %d %s %q", c.path, code, errCode(out), errText(out))
		}
	}
	for _, path := range []string{"/organization?$count=true", "/organization?$top=1", "/organization?$search=" + url.QueryEscape(`"displayName:acme"`)} {
		code, out := graphCall(t, ts, "GET", path, "")
		if _, counted := out["@odata.count"]; code != 200 || counted || len(out["value"].([]any)) != 1 {
			t.Errorf("%s: %d %v", path, code, out)
		}
	}
}

// PATCH changes the five properties that Graph lets a tenant change (organization-update): 204, privacyProfile is merged, a list is not
// null, another property is refused, and the change is in the history and is still there after a restart.
func TestOrganization_Update(t *testing.T) {
	dir := t.TempDir()
	world, err := graph.Open(dir, graph.OpenOptions{Users: 10, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(graph.New(world, graph.ServerOptions{}).Handler())
	defer ts.Close()
	call := func(method, path, body string) (int, map[string]any) { return graphCall(t, ts, method, path, body) }
	id := zeroTenant

	code, _ := call("PATCH", "/organization/"+id, `{"marketingNotificationEmails":["marketing@acme.example"],"privacyProfile":{"contactEmail":"dpo@acme.example"},
		"securityComplianceNotificationMails":["sec@acme.example"],"securityComplianceNotificationPhones":["+46 8 555 0199"],"technicalNotificationMails":["tech@acme.example"]}`)
	if code != 204 {
		t.Fatalf("update: %d", code)
	}
	_, out := call("GET", "/organization/"+id, "")
	pp := out["privacyProfile"].(map[string]any)
	if out["marketingNotificationEmails"].([]any)[0] != "marketing@acme.example" || out["technicalNotificationMails"].([]any)[0] != "tech@acme.example" ||
		pp["contactEmail"] != "dpo@acme.example" || pp["statementUrl"] != "https://acme.example/privacy" {
		t.Errorf("after the update (privacyProfile is merged): %v", out)
	}

	for _, c := range []struct{ body, text string }{
		{`{"displayName":"New name"}`, "cannot be updated"},
		{`{"technicalNotificationMails":null}`, "cannot be null"},
		{`{"technicalNotificationMails":[1]}`, "must be a list of texts"},
		{`{"privacyProfile":{"nope":"x"}}`, "does not exist"},
		{`{"privacyProfile":"x"}`, "must be an object"},
	} {
		if code, bad := call("PATCH", "/organization/"+id, c.body); code != 400 || !strings.Contains(errText(bad), c.text) {
			t.Errorf("%s: %d %q", c.body, code, errText(bad))
		}
	}
	if code, bad := call("PATCH", "/organization/11111111-1111-1111-1111-111111111111", `{"technicalNotificationMails":[]}`); code != 400 || errCode(bad) != "Request_BadRequest" {
		t.Errorf("another tenant: %d %v", code, bad)
	}

	found := false
	for _, e := range world.History() {
		if text := e.Text(); strings.HasPrefix(text, "Changed the organization:") && strings.Contains(text, "technicalNotificationMails") {
			found = true
		}
	}
	if !found {
		t.Errorf("the change is not in the history: %v", world.History())
	}
	// a restart gives the same data: the change is replayed
	ts.Close()
	if err := world.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := graph.Open(dir, graph.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if got := again.ListOrganizations()[0]["technicalNotificationMails"].([]any); len(got) != 1 || got[0] != "tech@acme.example" {
		t.Errorf("after a restart: %v", got)
	}
}

// A seed that has an organization with only some of the properties (made before it was complete) gets the rest, and keeps what it has.
func TestOrganization_OldSeedGetsTheMissingProperties(t *testing.T) {
	dir := t.TempDir()
	old := map[string]any{"id": zeroTenant, "displayName": "My own name", "city": "Malmö", "verifiedDomains": []any{map[string]any{"name": "x.example", "type": "Managed", "isDefault": true, "isInitial": true, "capabilities": ""}}}
	data, _ := json.Marshal(map[string]any{"seed": 3, "users": []any{}, "groups": []any{}, "servicePrincipals": []any{}, "assignments": []any{}, "organization": old})
	if err := os.WriteFile(filepath.Join(dir, "seed.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := graph.Open(dir, graph.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	o := w.ListOrganizations()[0]
	if o["displayName"] != "My own name" || o["city"] != "Malmö" || len(o["verifiedDomains"].([]any)) != 1 {
		t.Errorf("what the seed had must stay: %v", o)
	}
	for _, name := range api.DefaultOrganizationProperties {
		if _, has := o[name]; !has {
			t.Errorf("an old organization lacks %s", name)
		}
	}
}
