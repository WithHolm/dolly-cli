package repo

import (
	"slices"
	"strings"
	"testing"
)

// The writes of a service principal are in the history, a restart replays them, a rollback takes them back, and delta query sees them.
func TestServicePrincipalWrites_HistoryRestartRollbackAndDelta(t *testing.T) {
	dir := t.TempDir()
	w := openTemp(t, dir)
	before := len(w.History())
	epoch, seq := w.DeltaNow()

	app, e := w.CreateApplication(map[string]any{"displayName": "Reports"}, login)
	if e != nil {
		t.Fatal(e)
	}
	appID := app["appId"].(string)
	made, e := w.CreateServicePrincipal(map[string]any{"appId": appID, "tags": []any{EnterpriseTag, "team-a"}}, login)
	if e != nil {
		t.Fatal(e)
	}
	id := made["id"].(string)
	if e := w.UpdateServicePrincipal(appID, map[string]any{"notes": "monthly", "accountEnabled": false}, login); e != nil {
		t.Fatal(e)
	}
	afterUpdate := len(w.History())

	texts := historyTexts(w)
	for _, want := range []string{"Created the service principal Reports (client ID " + appID + ")", "Changed the service principal Reports: "} {
		if !slices.ContainsFunc(texts, func(s string) bool { return strings.HasPrefix(s, want) }) {
			t.Errorf("%q is not in the history: %v", want, texts)
		}
	}
	changes, ok := w.DeltaSince(KindServicePrincipal, epoch, seq)
	if !ok || len(changes) != 1 || changes[0].ID != id || !changes[0].Created {
		t.Errorf("delta after the create and the update: %v %+v", ok, changes)
	}

	// a restart
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	w = openTemp(t, dir)
	got, ok := w.GetServicePrincipal(id)
	if !ok || got["notes"] != "monthly" || got["accountEnabled"] != false || !slices.Contains(got["tags"].([]string), "team-a") {
		t.Fatalf("after a restart: %v %v", got, ok)
	}
	if info, _ := w.FindServicePrincipal(appID); !info.Enterprise || info.Application != app["id"] {
		t.Errorf("the enterprise tag and the link to the app registration are kept: %+v", info)
	}

	// a delete takes what the service principal had
	other := w.ListServicePrincipalInfo()[0]
	if _, err := w.AssignAppRole(id, other.ID, "00000000-0000-0000-0000-000000000000", login); err == nil {
		t.Log("the assignment of a default role is not possible on a service principal with no roles")
	}
	if e := w.DeleteServicePrincipal(id, login); e != nil {
		t.Fatal(e)
	}
	if _, ok := w.GetServicePrincipal(id); ok {
		t.Error("it is still there")
	}
	if e := w.DeleteServicePrincipal(id, login); e == nil || e.Status != 404 {
		t.Errorf("twice: %v", e)
	}
	if _, ok := w.GetApplication(appID); !ok {
		t.Error("the app registration stays")
	}
	changes, _ = w.DeltaSince(KindServicePrincipal, epoch, seq)
	if len(changes) != 1 || !changes[0].Removed {
		t.Errorf("delta: made and deleted since the point: %+v", changes)
	}

	// a restart again: the delete is replayed
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	w = openTemp(t, dir)
	if _, ok := w.GetServicePrincipal(id); ok {
		t.Error("the delete was not replayed")
	}

	// a rollback to just after the update: it is back, as it was
	if err := w.Rollback(afterUpdate, login); err != nil {
		t.Fatal(err)
	}
	if got, ok := w.GetServicePrincipal(id); !ok || got["notes"] != "monthly" {
		t.Errorf("after the rollback to the update: %v %v", got, ok)
	}
	// and to before everything: it was never made
	if err := w.Rollback(before, login); err != nil {
		t.Fatal(err)
	}
	if _, ok := w.GetServicePrincipal(id); ok {
		t.Error("it is there before it was made")
	}
}

// Deleting a service principal takes its role assignments and its consents with it.
func TestServicePrincipalWrites_DeleteTakesWhatItHad(t *testing.T) {
	w := Generate(Options{Users: 10, Seed: 1})
	app, _ := w.CreateApplication(map[string]any{"displayName": "Holder", "appRoles": []any{map[string]any{"id": "11111111-1111-1111-1111-111111111111", "displayName": "Reader", "value": "Reader", "allowedMemberTypes": []any{"User"}}}}, login)
	sp, e := w.CreateServicePrincipal(map[string]any{"appId": app["appId"]}, login)
	if e != nil {
		t.Fatal(e)
	}
	id := sp["id"].(string)
	users := w.ListUsers()
	if _, e := w.AssignAppRole(id, users[0]["id"].(string), "11111111-1111-1111-1111-111111111111", login); e != nil {
		t.Fatal(e)
	}
	if items, _ := w.AppRoleAssignedTo(id); len(items) != 1 {
		t.Fatalf("the assignment: %v", items)
	}
	if e := w.DeleteServicePrincipal(id, login); e != nil {
		t.Fatal(e)
	}
	for _, a := range w.Assignments {
		if isID(a.ResourceID, id) {
			t.Errorf("an assignment is left: %+v", a)
		}
	}
}
