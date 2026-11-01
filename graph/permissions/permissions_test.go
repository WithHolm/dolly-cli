package permissions_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph/permissions"
)

func load(t *testing.T) *permissions.Catalog {
	t.Helper()
	c, err := permissions.Load()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCatalog_MatchesItsSource(t *testing.T) {
	c := load(t)
	// the file of Microsoft, and the permissions of supplement.json that it does not have yet
	if len(c.Delegated)+len(c.Application) != c.Source.Delegated+c.Source.Application+c.Added+c.Named {
		t.Errorf("SOURCE.json says %d delegated and %d application, %d were added, and %d have a made ID: the catalog has %d and %d",
			c.Source.Delegated, c.Source.Application, c.Added, c.Named, len(c.Delegated), len(c.Application))
	}
	if p, ok := c.Find(permissions.Application, "user.readupdate.all"); !ok || p.ID != "5639c449-cfd9-4088-bc48-3e16da108bf8" || p.Kind != permissions.Application {
		t.Errorf("User.ReadUpdate.All (supplement.json): %+v %v", p, ok)
	}
	if len(c.Delegated) < 500 || len(c.Application) < 400 {
		t.Errorf("only %d delegated and %d application permissions: has the file changed?", len(c.Delegated), len(c.Application))
	}
	if c.Source.Commit == "" || !strings.Contains(c.Source.License, "MIT") {
		t.Errorf("source: %+v", c.Source)
	}
}

// A permission that only the list of names has gets an ID that dollycli makes: always the same one, and marked. The permissions that have a real ID keep it.
func TestCatalog_MadeIDs(t *testing.T) {
	c := load(t)
	if c.Named < 100 {
		t.Fatalf("only %d permissions have a made ID: has names.json changed?", c.Named)
	}
	made := 0
	for _, k := range []permissions.Kind{permissions.Delegated, permissions.Application} {
		list := c.Delegated
		if k == permissions.Application {
			list = c.Application
		}
		for _, p := range list {
			if p.IDMade {
				made++
				if p.ID != permissions.MadeID(k, p.Value) { // some of them have no texts: the list of names has none for them
					t.Errorf("%s %s: %+v", k, p.Value, p)
				}
			}
		}
	}
	if made != c.Named {
		t.Errorf("%d are marked, %d were counted", made, c.Named)
	}
	for _, real := range []string{"User.Read.All", "Mail.Read", "User.ReadUpdate.All"} {
		if p, ok := c.Find(permissions.Application, real); !ok || p.IDMade {
			t.Errorf("%s has a real ID: %+v %v", real, p, ok)
		}
	}
	if id := permissions.MadeID(permissions.Application, "Made.Up.All"); id != permissions.MadeID(permissions.Application, "made.up.all") || id == permissions.MadeID(permissions.Delegated, "Made.Up.All") || len(id) != 36 {
		t.Errorf("a made ID is stable, in any case, and different for each kind: %s", id)
	}
}

func TestCatalog_RawFileIsAsFetched(t *testing.T) {
	// raw/ is not edited by hand: its hash is the one that the fetch tool wrote
	c := load(t)
	data, err := os.ReadFile("raw/permissions-descriptions.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != c.Source.File.SHA256 {
		t.Errorf("raw/ has the hash %s, SOURCE.json has %s: do not edit raw/, run 'make update-permissions'", got, c.Source.File.SHA256)
	}
}

func TestCatalog_FindsByValueInAnyCaseAndByID(t *testing.T) {
	c := load(t)
	for _, w := range []struct {
		kind  permissions.Kind
		value string
		id    string
	}{
		{permissions.Delegated, "User.Read", "e1fe6dd8-ba31-4d61-89e7-88639da4683d"},
		{permissions.Delegated, "user.read", "e1fe6dd8-ba31-4d61-89e7-88639da4683d"},
		{permissions.Application, "User.Read.All", "df021288-bdef-4463-88db-98f22de89214"},
		{permissions.Application, "USER.READ.ALL", "df021288-bdef-4463-88db-98f22de89214"},
	} {
		p, ok := c.Find(w.kind, w.value)
		if !ok || p.ID != w.id || p.Kind != w.kind {
			t.Errorf("%s %s: %+v %v", w.kind, w.value, p, ok)
		}
		if q, ok := c.ByID(w.kind, strings.ToUpper(w.id)); !ok || q.ID != w.id {
			t.Errorf("by ID %s: %+v %v", w.id, q, ok)
		}
	}
	// a delegated permission is not an application permission of the same name
	if _, ok := c.Find(permissions.Application, "User.Read"); ok {
		t.Error("User.Read is a delegated permission only")
	}
	if _, ok := c.Find(permissions.Delegated, "Nothing.Here"); ok {
		t.Error("an unknown value must not be found")
	}
	p, _ := c.Find(permissions.Delegated, "User.Read")
	if p.Scope() != "https://graph.microsoft.com/User.Read" {
		t.Errorf("scope: %s", p.Scope())
	}
	if p.AdminConsentDisplayName == "" || p.ConsentDescription == "" {
		t.Errorf("the texts are missing: %+v", p)
	}
}

func TestCatalog_ParseRefusesWhatItDoesNotKnow(t *testing.T) {
	for name, data := range map[string]string{
		"a field the types do not have": `{"delegatedScopesList":[{"id":"1","value":"A","surprise":true}],"applicationScopesList":[]}`,
		"no value":                      `{"delegatedScopesList":[{"id":"1"}],"applicationScopesList":[]}`,
		"a value twice":                 `{"delegatedScopesList":[{"id":"1","value":"A"},{"id":"2","value":"a"}],"applicationScopesList":[]}`,
		"an ID twice":                   `{"delegatedScopesList":[{"id":"1","value":"A"},{"id":"1","value":"B"}],"applicationScopesList":[]}`,
	} {
		if _, err := permissions.Parse([]byte(data)); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
	// the same ID in the two kinds is what Microsoft has (User.Export.All)
	if _, err := permissions.Parse([]byte(`{"delegatedScopesList":[{"id":"1","value":"A"}],"applicationScopesList":[{"id":"1","value":"A"}]}`)); err != nil {
		t.Errorf("the same ID in the two kinds must be accepted: %v", err)
	}
}

func TestParseScope(t *testing.T) {
	for scope, want := range map[string][2]string{
		"https://graph.microsoft.com/User.Read":                 {"https://graph.microsoft.com", "User.Read"},
		"https://graph.microsoft.com/.default":                  {"https://graph.microsoft.com", ".default"},
		"api://11111111-2222-3333-4444-555555555555/Files.Read": {"api://11111111-2222-3333-4444-555555555555", "Files.Read"},
		"User.Read": {"", "User.Read"},
		"openid":    {"", "openid"},
	} {
		if ns, v := permissions.ParseScope(scope); ns != want[0] || v != want[1] {
			t.Errorf("%s: %q %q, want %q %q", scope, ns, v, want[0], want[1])
		}
	}
}
