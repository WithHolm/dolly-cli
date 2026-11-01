package repo

import (
	"slices"
	"strings"
	"testing"
)

func historyTexts(w *World) []string {
	var out []string
	for _, e := range w.History() {
		out = append(out, e.Text())
	}
	return out
}

// The changes of an app registration are in the history, a restart replays them, and a rollback takes them back.
func TestApplications_HistoryRestartAndRollback(t *testing.T) {
	dir := t.TempDir()
	w := openTemp(t, dir)
	before := len(w.History())
	seeded := len(w.ListApplications())
	if seeded != 2 {
		t.Fatalf("the seed has %d registrations", seeded)
	}

	made, e := w.CreateApplication(map[string]any{"displayName": "Reports", "web": map[string]any{"redirectUris": []any{"https://r.example/cb"}}}, login)
	if e != nil {
		t.Fatal(e)
	}
	afterCreate := len(w.History())
	id := made["id"].(string)
	if e := w.UpdateApplication(id, map[string]any{"description": "Monthly", "web": map[string]any{"logoutUrl": "https://r.example/out"}}, login); e != nil {
		t.Fatal(e)
	}
	if e := w.UpdateApplication(made["appId"].(string), map[string]any{"notes": "by appId"}, login); e != nil {
		t.Fatal(e)
	}
	other := w.ListApplications()[0]["id"].(string)
	if e := w.DeleteApplication(other, login); e != nil {
		t.Fatal(e)
	}

	texts := historyTexts(w)
	for _, want := range []string{"Registered the app Reports", "Changed the app registration Reports: ", "Deleted the app registration "} {
		if !slices.ContainsFunc(texts, func(s string) bool { return strings.HasPrefix(s, want) }) {
			t.Errorf("%q is not in the history: %v", want, texts)
		}
	}

	// a restart: the folder is read again, and the changes are replayed
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	w = openTemp(t, dir)
	app, ok := w.GetApplication(id)
	web := app["web"].(map[string]any)
	if !ok || app["description"] != "Monthly" || app["notes"] != "by appId" || web["logoutUrl"] != "https://r.example/out" || len(web["redirectUris"].([]any)) != 1 {
		t.Errorf("after a restart: %v %v", app, ok)
	}
	if _, ok := w.GetApplication(other); ok || len(w.ListApplications()) != seeded {
		t.Errorf("the deleted one is back, or the count is wrong: %d", len(w.ListApplications()))
	}

	// a rollback to before the first change: the seed again
	if err := w.Rollback(before, login); err != nil {
		t.Fatal(err)
	}
	if _, ok := w.GetApplication(id); ok {
		t.Error("the registration is still there after the rollback")
	}
	if _, ok := w.GetApplication(other); !ok || len(w.ListApplications()) != seeded {
		t.Errorf("the deleted registration is not back: %d", len(w.ListApplications()))
	}
	// a rollback to just after the creation: it is there, with no later change
	if err := w.Rollback(afterCreate, login); err != nil {
		t.Fatal(err)
	}
	app, ok = w.GetApplication(id)
	if !ok || app["description"] != nil || app["notes"] != nil {
		t.Errorf("the state after the creation: %v %v", app, ok)
	}
}
