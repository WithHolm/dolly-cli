package graph_test

import (
	"os"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

// The admin API shows what a profile would make, and makes the data from it. A profile that is not valid is a 400 that names the field.
func TestAdmin_ProfilePreviewAndGenerate(t *testing.T) {
	ts, _, srv := startServer(t, graph.ServerOptions{})
	norway, err := os.ReadFile("../docs/profiles/norway.json")
	if err != nil {
		t.Fatal(err)
	}
	body := `{"profile":` + string(norway) + `,"users":80,"seed":4}`

	var def graph.Profile
	if admin(t, ts, "GET", "/profile/default", "", &def) != 200 || def.Organization.Domain != "acme.example" {
		t.Errorf("the default profile: %+v", def.Organization)
	}

	var pv graph.ProfilePreview
	if admin(t, ts, "POST", "/profile/preview", body, &pv) != 200 || pv.Users != 80 || pv.Admin != "adm.philip@fjordvik.example" {
		t.Errorf("preview: %+v", pv)
	}
	if srv.World().Snapshot().Users != 200 {
		t.Error("a preview changes nothing")
	}

	if code := admin(t, ts, "POST", "/profile/preview", `{"profile":{"version":1}}`, nil); code != 400 {
		t.Errorf("a profile with no fields: %d", code)
	}
	if code := admin(t, ts, "POST", "/profile/preview", `{"profile":{"version":1,"surprise":1}}`, nil); code != 400 {
		t.Errorf("an unknown field: %d", code)
	}
	if code := admin(t, ts, "POST", "/generate", `{"users":1000000}`, nil); code != 400 {
		t.Errorf("too many users: %d", code)
	}

	var st graph.State
	if admin(t, ts, "POST", "/generate", body, &st) != 200 || st.Users != 80 {
		t.Fatalf("generate: %+v", st)
	}
	if srv.World().AdminAccount() != "adm.philip@fjordvik.example" {
		t.Errorf("admin account: %s", srv.World().AdminAccount())
	}
	if _, ok := srv.World().GetUser("adm.philip@fjordvik.example"); !ok {
		t.Error("the new data has the admin account")
	}
	if code := admin(t, ts, "POST", "/generate", strings.Replace(body, `"version": 1`, `"version": 9`, 1), nil); code != 400 {
		t.Errorf("a profile that is not valid: %d", code)
	}
	if srv.World().Snapshot().Users != 80 {
		t.Error("a profile that is not valid changes nothing")
	}
}
