package permissions_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph/permissions"
)

// The answer of a tenant for the service principal Microsoft Graph, made up for the test: two roles and two scopes, one of each is new.
const tenantAnswer = `{
 "appRoles": [
  {"id": "df021288-bdef-4463-88db-98f22de89214", "value": "User.Read.All", "displayName": "Read all users (tenant)", "description": "From the tenant.", "isEnabled": true, "allowedMemberTypes": ["Application"], "origin": "Application"},
  {"id": "11111111-2222-3333-4444-555555555555", "value": "Made.Up.All", "displayName": "Made up", "description": "Only in the file.", "isEnabled": true, "allowedMemberTypes": ["Application"]}
 ],
 "oauth2PermissionScopes": [
  {"id": "a154be20-db9c-4678-8ab7-66f6cc099a59", "value": "User.Read.All", "type": "Admin", "adminConsentDisplayName": "Read all users (tenant)", "adminConsentDescription": "From the tenant.", "userConsentDisplayName": "", "userConsentDescription": "", "isEnabled": true},
  {"id": "66666666-7777-8888-9999-000000000000", "value": "Made.Up.User", "type": "User", "adminConsentDisplayName": "Made up", "adminConsentDescription": "Only in the file.", "userConsentDisplayName": "Made up for you", "userConsentDescription": "A user can consent.", "isEnabled": true}
 ]
}`

func TestExtra_ParseTheAnswerOfATenant(t *testing.T) {
	d, a, err := permissions.ParseTenant([]byte(tenantAnswer))
	if err != nil || len(d) != 2 || len(a) != 2 {
		t.Fatalf("%v %d %d", err, len(d), len(a))
	}
	if !d[0].IsAdmin || d[1].IsAdmin || d[1].ConsentDisplayName != "Made up for you" {
		t.Errorf("type Admin or User decides who consents: %+v %+v", d[0], d[1])
	}
	if a[1].AdminConsentDescription != "Only in the file." || a[1].AllowedMemberTypes[0] != "Application" {
		t.Errorf("a role: %+v", a[1])
	}
	// the answer of a $filter is a list of one
	if d, a, err := permissions.ParseTenant([]byte(`{"value":[` + tenantAnswer + `]}`)); err != nil || len(d) != 2 || len(a) != 2 {
		t.Errorf("filter form: %v %d %d", err, len(d), len(a))
	}
	for name, bad := range map[string]string{
		"not json":     `<html>`,
		"no lists":     `{"id": "x"}`,
		"no value":     `{"appRoles":[{"id":"x"}]}`,
		"scope no id":  `{"oauth2PermissionScopes":[{"value":"A.B"}]}`,
		"error answer": `{"error":{"code":"Authorization_RequestDenied"}}`,
	} {
		if _, _, err := permissions.ParseTenant([]byte(bad)); err == nil {
			t.Errorf("%s: an error was expected", name)
		}
	}
}

// The file of the person adds permissions and wins over the built-in ones with the same value. Without it nothing changes.
func TestExtra_TheFileIsAddedToTheCatalog(t *testing.T) {
	defer permissions.SetExtra("")
	before, err := permissions.Load()
	if err != nil || before.Extra != nil {
		t.Fatalf("no file: %v %+v", err, before.Extra)
	}

	path := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(path, []byte(tenantAnswer), 0o600); err != nil {
		t.Fatal(err)
	}
	permissions.SetExtra(path)
	c, err := permissions.Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Extra == nil || c.Extra.Delegated != 2 || c.Extra.Application != 2 || c.Extra.Date.IsZero() {
		t.Errorf("extra: %+v", c.Extra)
	}
	if len(c.Application) != len(before.Application)+1 || len(c.Delegated) != len(before.Delegated)+1 {
		t.Errorf("one new of each kind: %d/%d and before %d/%d", len(c.Delegated), len(c.Application), len(before.Delegated), len(before.Application))
	}
	if p, ok := c.Find(permissions.Application, "made.up.all"); !ok || p.ID != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("new permission: %+v %v", p, ok)
	}
	if p, _ := c.Find(permissions.Application, "User.Read.All"); p.AdminConsentDisplayName != "Read all users (tenant)" {
		t.Errorf("the file wins: %+v", p)
	}
	if p, ok := c.ByID(permissions.Delegated, "66666666-7777-8888-9999-000000000000"); !ok || p.Value != "Made.Up.User" {
		t.Errorf("by ID: %+v %v", p, ok)
	}

	// a file that cannot be read is an error that names it, and not a catalog with a part missing
	permissions.SetExtra(filepath.Join(t.TempDir(), "missing.json"))
	if _, err := permissions.Load(); err == nil || !strings.Contains(err.Error(), "missing.json") {
		t.Errorf("missing file: %v", err)
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	_ = os.WriteFile(bad, []byte(`{"error":{}}`), 0o600)
	permissions.SetExtra(bad)
	if _, err := permissions.Load(); err == nil || !strings.Contains(err.Error(), "bad.json") {
		t.Errorf("bad file: %v", err)
	}
}
