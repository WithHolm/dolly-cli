package graph_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/withholm/dolly-cli/graph"
)

func TestPlaces_SeedAndLists(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	call := func(method, path, body string) (int, map[string]any) { return graphCall(t, ts, method, path, body) }

	for kind, want := range map[string]int{"building": 2, "floor": 6, "section": 12, "desk": 48, "room": 24, "workspace": 6, "roomList": 2} {
		code, out := call("GET", "/places/microsoft.graph."+kind, "")
		if got := len(ids(out)); code != 200 || got != want {
			t.Errorf("%s: %d, %d places, want %d", kind, code, got, want)
		}
	}

	// $count needs no ConsistencyLevel for places, and $top and $skip page
	_, out := call("GET", "/places/microsoft.graph.room?$count=true&$top=10&$skip=0", "")
	if out["@odata.count"] != float64(24) || len(ids(out)) != 10 || out["@odata.nextLink"] == nil {
		t.Errorf("count and page: %v", out["@odata.count"])
	}
	_, out = call("GET", "/places/microsoft.graph.room?$top=10&$skip=20", "")
	if len(ids(out)) != 4 {
		t.Errorf("$skip=20 gives %d rooms, want 4", len(ids(out)))
	}
	// $filter works for rooms, workspaces, and room lists
	if code, out := call("GET", "/places/microsoft.graph.room?$filter=startswith(displayName,'stockholm')", ""); code != 200 || len(ids(out)) != 12 {
		t.Errorf("filter: %d, %d rooms, want 12", code, len(ids(out)))
	}

	// get by ID, with the type
	list := ids(mustGet(t, call, "/places/microsoft.graph.building"))
	code, b := call("GET", "/places/"+list[0], "")
	if code != 200 || b["@odata.type"] != "#microsoft.graph.building" || b["placeId"] != b["id"] {
		t.Errorf("get: %d %v", code, b)
	}
	if code, _ := call("GET", "/places/00000000-0000-0000-0000-000000000000", ""); code != 404 {
		t.Errorf("unknown place: %d", code)
	}

	// the rooms of a room list are found by the email address, and the type is not case sensitive
	_, rl := call("GET", "/places/microsoft.graph.roomList", "")
	mail := rl["value"].([]any)[0].(map[string]any)["emailAddress"].(string)
	for _, spelling := range []string{"microsoft.graph.roomlist", "microsoft.graph.roomList"} {
		code, rooms := call("GET", "/places/"+mail+"/"+spelling+"/rooms", "")
		if code != 200 || len(ids(rooms)) != 12 {
			t.Errorf("%s rooms: %d, %d", spelling, code, len(ids(rooms)))
		}
	}
	if _, ws := call("GET", "/places/"+mail+"/microsoft.graph.roomlist/workspaces", ""); len(ids(ws)) != 3 {
		t.Errorf("workspaces: %v", ids(ws))
	}
	if code, _ := call("GET", "/places/nobody@acme.example/microsoft.graph.roomlist/rooms", ""); code != 404 {
		t.Errorf("unknown room list: %d", code)
	}
}

func mustGet(t *testing.T, call func(method, path, body string) (int, map[string]any), path string) map[string]any {
	t.Helper()
	code, out := call("GET", path, "")
	if code != 200 {
		t.Fatalf("GET %s: %d", path, code)
	}
	return out
}

func TestPlaces_CreateUpdateDelete(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	call := func(method, path, body string) (int, map[string]any) { return graphCall(t, ts, method, path, body) }

	// the examples of the Learn page "Create place"
	code, b := call("POST", "/places", `{"@odata.type":"microsoft.graph.building","displayName":"B001"}`)
	bid, _ := b["id"].(string)
	if code != 201 || b["@odata.type"] != "#microsoft.graph.building" || b["placeId"] != bid || b["isWheelChairAccessible"] != false {
		t.Fatalf("building: %d %v", code, b)
	}
	_, f := call("POST", "/places", `{"@odata.type":"microsoft.graph.floor","displayName":"F1","parentId":"`+bid+`"}`)
	fid := f["id"].(string)
	_, s := call("POST", "/places", `{"@odata.type":"microsoft.graph.section","displayName":"S1","parentId":"`+fid+`"}`)
	sid := s["id"].(string)
	_, d := call("POST", "/places", `{"@odata.type":"microsoft.graph.desk","displayName":"D1","parentId":"`+sid+`"}`)
	if m, _ := d["mode"].(map[string]any); m["@odata.type"] != "#microsoft.graph.dropInPlaceMode" {
		t.Errorf("a desk has the drop-in mode: %v", d["mode"])
	}
	_, r := call("POST", "/places", `{"@odata.type":"microsoft.graph.room","displayName":"Conf Room 4/3.3G11","parentId":"`+fid+`","bookingType":"standard","capacity":8}`)
	if r["emailAddress"] == nil || r["bookingType"] != "standard" || r["nickname"] != "Conf Room 4/3.3G11" {
		t.Errorf("a room gets an email address and a nickname: %v", r)
	}
	_, w := call("POST", "/places", `{"@odata.type":"microsoft.graph.workspace","parentId":"`+sid+`","displayName":"testSpace001","tags":["test"]}`)
	if w["emailAddress"] == nil || w["nickname"] != "testSpace001" {
		t.Errorf("a workspace gets an email address and a nickname: %v", w)
	}

	// the rules of the parent
	for name, body := range map[string]string{
		"no type":                  `{"displayName":"X"}`,
		"a room list":              `{"@odata.type":"microsoft.graph.roomList","displayName":"X"}`,
		"no name":                  `{"@odata.type":"microsoft.graph.building"}`,
		"a floor with no parent":   `{"@odata.type":"microsoft.graph.floor","displayName":"X"}`,
		"a building with a parent": `{"@odata.type":"microsoft.graph.building","displayName":"X","parentId":"` + bid + `"}`,
		"a floor in a floor":       `{"@odata.type":"microsoft.graph.floor","displayName":"X","parentId":"` + fid + `"}`,
		"a parent that is not":     `{"@odata.type":"microsoft.graph.floor","displayName":"X","parentId":"00000000-0000-0000-0000-000000000000"}`,
		"an unknown property":      `{"@odata.type":"microsoft.graph.building","displayName":"X","colour":"red"}`,
		"the wrong type of value":  `{"@odata.type":"microsoft.graph.room","displayName":"X","parentId":"` + fid + `","capacity":"many"}`,
		"a property of a room":     `{"@odata.type":"microsoft.graph.floor","displayName":"X","parentId":"` + bid + `","capacity":3}`,
	} {
		if code, _ := call("POST", "/places", body); code != 400 {
			t.Errorf("%s: %d, want 400", name, code)
		}
	}

	// update gives 200 with the place. Some properties cannot change.
	code, u := call("PATCH", "/places/"+fid, `{"@odata.type":"microsoft.graph.floor","isWheelChairAccessible":true,"tags":["a"],"sortOrder":2,"label":"Second"}`)
	if code != 200 || u["isWheelChairAccessible"] != true || u["sortOrder"] != float64(2) || u["label"] != "Second" {
		t.Errorf("patch: %d %v", code, u)
	}
	for name, body := range map[string]string{
		"displayName": `{"displayName":"New"}`, "id": `{"id":"x"}`, "placeId": `{"placeId":"x"}`, "emailAddress": `{"emailAddress":"x@y"}`,
		"bookingType": `{"bookingType":"reserved"}`, "another type": `{"@odata.type":"microsoft.graph.room"}`,
		"a parent that is the place": `{"parentId":"` + fid + `"}`, "tags null": `{"tags":null}`,
	} {
		if code, _ := call("PATCH", "/places/"+fid, body); code != 400 {
			t.Errorf("patch %s: %d, want 400", name, code)
		}
	}
	if code, _ := call("PATCH", "/places/00000000-0000-0000-0000-000000000000", `{"label":"x"}`); code != 404 {
		t.Errorf("patch of an unknown place: %d", code)
	}

	// delete: a room, a room list, and a workspace cannot be deleted, and a place with places below it either
	for _, id := range []string{r["id"].(string), w["id"].(string)} {
		if code, _ := call("DELETE", "/places/"+id, ""); code != 400 {
			t.Errorf("delete of a room or a workspace: %d, want 400", code)
		}
	}
	if code, _ := call("DELETE", "/places/"+bid, ""); code != 400 {
		t.Errorf("delete of a building with a floor: %d, want 400", code)
	}
	if code, _ := call("DELETE", "/places/"+d["id"].(string), ""); code != 204 {
		t.Errorf("delete of a desk: %d", code)
	}
	if code, _ := call("DELETE", "/places/"+sid, ""); code != 400 {
		t.Errorf("delete of a section with a workspace: %d, want 400", code)
	}
	if code, _ := call("GET", "/places/"+d["id"].(string), ""); code != 404 {
		t.Errorf("the desk is gone: %d", code)
	}
}

// A place that was made, changed, and deleted is in the history, and it comes back after a restart.
func TestPlaces_KeptAndReplayed(t *testing.T) {
	dir := t.TempDir()
	opt := graph.OpenOptions{Users: 10, Seed: 3, Now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }}
	w, err := graph.Open(dir, opt)
	if err != nil {
		t.Fatal(err)
	}
	b, e := w.CreatePlace(map[string]any{"@odata.type": "microsoft.graph.building", "displayName": "Keep"}, graph.Actor{})
	if e != nil {
		t.Fatal(e)
	}
	f, e := w.CreatePlace(map[string]any{"@odata.type": "microsoft.graph.floor", "displayName": "Gone", "parentId": b["id"]}, graph.Actor{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e := w.UpdatePlace(b["id"].(string), map[string]any{"label": "kept"}, graph.Actor{}); e != nil {
		t.Fatal(e)
	}
	if e := w.DeletePlace(f["id"].(string), graph.Actor{}); e != nil {
		t.Fatal(e)
	}
	_ = w.Close()

	w2, err := graph.Open(dir, opt)
	if err != nil {
		t.Fatal(err)
	}
	defer w2.Close()
	got, ok := w2.GetPlace(b["id"].(string))
	if !ok || got["label"] != "kept" {
		t.Errorf("after the restart: %v %v", ok, got)
	}
	if _, ok := w2.GetPlace(f["id"].(string)); ok {
		t.Error("the deleted floor is back")
	}
	if h := w2.History(); len(h) != 4 || h[3].Text() != "Deleted the place Gone" {
		t.Errorf("history: %d entries, last %q", len(h), h[len(h)-1].Text())
	}
}

var _ = http.StatusOK
