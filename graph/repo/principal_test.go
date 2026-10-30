package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testApp = "11111111-2222-3333-4444-555555555555"

var login = Actor{Source: "login"}

func openTemp(t *testing.T, dir string) *World {
	t.Helper()
	w, err := Open(dir, OpenOptions{Users: 10, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestServicePrincipal_RegisterIsOnceAndOpen(t *testing.T) {
	w := Generate(Options{Users: 10, Seed: 1})
	a, err := w.EnsureServicePrincipal(strings.ToUpper(testApp), login)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Auto || a.Mode != ModeOpen || a.Credential.State != CredAny || a.AppID != testApp || !strings.HasPrefix(a.Name, "Auto-created service principal") {
		t.Errorf("new app: %+v", a)
	}
	b, _ := w.EnsureServicePrincipal(testApp, login)
	if b.ID != a.ID {
		t.Error("a service principal that is known must not be made again")
	}
	if _, err := w.EnsureServicePrincipal("not-a-guid", login); err == nil {
		t.Error("a client ID that is not a GUID must be refused")
	}
	// Graph shows it with the tag, and with nothing about the credential
	obj, ok := w.GetServicePrincipal(a.ID)
	if !ok || obj["appId"] != testApp {
		t.Fatalf("object: %v %v", obj, ok)
	}
	if tags, _ := obj["tags"].([]string); len(tags) != 1 || tags[0] != "dolly:auto" {
		t.Errorf("tags: %v", obj["tags"])
	}
	if _, has := obj["credential"]; has {
		t.Error("the Graph object must not have the credential state")
	}
}

func TestServicePrincipal_CredentialStates(t *testing.T) {
	w := Generate(Options{Users: 10, Seed: 1})
	w.EnsureServicePrincipal(testApp, login)

	// any: anything goes
	for _, c := range [][2]string{{KindSecret, "x"}, {KindSecret, ""}, {KindCertificate, "abc"}} {
		if !w.CheckCredential(testApp, c[0], c[1], login) {
			t.Errorf("state any must accept %v", c)
		}
	}

	// armed: no credential keeps it armed, the next one is kept
	if err := w.ArmCredential(testApp, login); err != nil {
		t.Fatal(err)
	}
	if !w.CheckCredential(testApp, KindSecret, "", login) {
		t.Error("an armed app accepts a public client")
	}
	if a, _ := w.FindServicePrincipal(testApp); a.Credential.State != CredArmed {
		t.Errorf("a public client must leave the app armed: %+v", a.Credential)
	}
	if !w.CheckCredential(testApp, KindSecret, "hunter2-the-real-one", login) {
		t.Error("the first credential after arming is accepted")
	}
	a, _ := w.FindServicePrincipal(testApp)
	if a.Credential.State != CredCaptured || a.Credential.Kind != KindSecret || a.Credential.Hint != "hunt" || a.Credential.At.IsZero() {
		t.Fatalf("kept: %+v", a.Credential)
	}
	if strings.Contains(a.Credential.Hash, "hunter2") || len(a.Credential.Hash) != 64 {
		t.Errorf("the hash: %q", a.Credential.Hash)
	}

	// captured: only that one
	for _, c := range []struct {
		kind, val string
		want      bool
	}{
		{KindSecret, "hunter2-the-real-one", true},
		{KindSecret, "hunter2-the-real-one ", false},
		{KindSecret, "other", false},
		{KindSecret, "", false},
		{KindCertificate, "hunter2-the-real-one", false},
	} {
		if got := w.CheckCredential(testApp, c.kind, c.val, login); got != c.want {
			t.Errorf("%s %q: %v, want %v", c.kind, c.val, got, c.want)
		}
	}

	// clear: any again
	w.ClearCredential(testApp, login)
	if !w.CheckCredential(testApp, KindSecret, "other", login) {
		t.Error("after clear, any credential is accepted")
	}
	if w.CheckCredential("99999999-2222-3333-4444-555555555555", KindSecret, "x", login) {
		t.Error("a client ID that is not known cannot sign in here: the sign-in API makes the service principal first")
	}
}

func TestServicePrincipal_AreKeptAfterARestart_WithoutTheSecret(t *testing.T) {
	dir := t.TempDir()
	w := openTemp(t, dir)
	w.EnsureServicePrincipal(testApp, login)
	w.SetServicePrincipalMode(testApp, ModeEnforce, login)
	w.ArmCredential(testApp, login)
	w.CheckCredential(testApp, KindSecret, "hunter2-the-real-one", login)
	newTenant := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	if err := w.SetTenantID(strings.ToUpper(newTenant), login); err != nil {
		t.Fatal(err)
	}
	w.Close()

	state, err := os.ReadFile(filepath.Join(dir, stateFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(state), "hunter2-the-real-one") {
		t.Fatal("the secret is in the state file")
	}
	if !strings.Contains(string(state), `"hint":"hunt"`) {
		t.Errorf("the hint is not in the state file: %s", state)
	}

	w = openTemp(t, dir)
	defer w.Close()
	a, ok := w.FindServicePrincipal(testApp)
	if !ok || !a.Auto || a.Mode != ModeEnforce || a.Credential.State != CredCaptured {
		t.Fatalf("service principal after a restart: %+v %v", a, ok)
	}
	if !w.CheckCredential(testApp, KindSecret, "hunter2-the-real-one", login) || w.CheckCredential(testApp, KindSecret, "x", login) {
		t.Error("the kept credential must still decide after a restart")
	}
	if got := w.TenantID(); got != newTenant {
		t.Errorf("tenant ID after a restart: %s", got)
	}
	if o := w.ListOrganizations()[0]; o["id"] != newTenant {
		t.Errorf("/organization has the ID %v", o["id"])
	}
	text := ""
	for _, e := range w.History() {
		text += e.Text() + "\n"
	}
	for _, want := range []string{"Created the service principal", "Set the permission mode of the service principal Auto-created service principal", "kept a secret (hunt…)", "Changed the tenant ID"} {
		if !strings.Contains(text, want) {
			t.Errorf("history has no %q:\n%s", want, text)
		}
	}
}

func TestServicePrincipal_RollbackUndoesThem(t *testing.T) {
	w := openTemp(t, t.TempDir())
	defer w.Close()
	before := len(w.History())
	w.EnsureServicePrincipal(testApp, login)
	w.ArmCredential(testApp, login)
	if err := w.Rollback(before, login); err != nil {
		t.Fatal(err)
	}
	if _, ok := w.FindServicePrincipal(testApp); ok {
		t.Error("after the rollback the service principal must be gone")
	}
}

func TestTenantID_MustBeAGUID(t *testing.T) {
	w := Generate(Options{Users: 10, Seed: 1})
	if w.TenantID() != FakeTenantID {
		t.Errorf("default tenant ID: %s", w.TenantID())
	}
	for _, bad := range []string{"", "acme.com", "1234"} {
		if err := w.SetTenantID(bad, login); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
	if w.TenantID() != FakeTenantID {
		t.Error("a refused change must change nothing")
	}
}

func TestServicePrincipal_FlagsAreInTheHistoryAndKeptAfterARestart(t *testing.T) {
	dir := t.TempDir()
	w := openTemp(t, dir)
	w.EnsureServicePrincipal(testApp, login)
	if e := w.SetServicePrincipalFlag(testApp, FlagHideCalls, true, login); e != nil {
		t.Fatal(e)
	}
	if e := w.SetServicePrincipalFlag(testApp, FlagExemptFromThrottle, true, login); e != nil {
		t.Fatal(e)
	}
	if e := w.SetServicePrincipalFlag(testApp, "nonsense", true, login); e == nil {
		t.Error("a flag that is not known must be refused")
	}
	if e := w.SetServicePrincipalFlag("00000000-0000-0000-0000-000000000001", FlagHideCalls, true, login); e == nil {
		t.Error("a client that is not there must be refused")
	}
	n := len(w.History())
	w.SetServicePrincipalFlag(testApp, FlagHideCalls, true, login) // no change: no event
	if len(w.History()) != n {
		t.Error("a flag that is set already must not make an event")
	}
	w.Close()

	w = openTemp(t, dir)
	defer w.Close()
	if exempt, hide := w.ClientFlags(strings.ToUpper(testApp)); !exempt || !hide {
		t.Errorf("after a restart: exempt %v, hide %v", exempt, hide)
	}
	if exempt, hide := w.ClientFlags("anonymous"); exempt || hide {
		t.Error("a client with no service principal has no flag")
	}
	var said []string
	for _, e := range w.History() {
		if e.Type == evSPFlag {
			text, _ := principalEventText(e)
			said = append(said, text)
		}
	}
	if len(said) != 2 || !strings.Contains(said[0], "not in the list of calls") || !strings.Contains(said[1], "do not throttle") {
		t.Errorf("history: %q", said)
	}
}
