package repo

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// The places of the tenant: buildings, floors, sections, desks, rooms, workspaces,
// and room lists. The data side of api/places.go. It satisfies api.PlacesBackend.
// A place is an object, as a user is, and its type is in @odata.type
// ("#microsoft.graph.room"). The rules are from the Learn pages "Create place",
// "Update place", "Delete place", "Get place", and "List place objects" (see
// .plans/dolly-graph-notes.md).

// Events that the places make.
const (
	evPlaceCreate = "place.create" // a new place (the whole object is in the event)
	evPlaceUpdate = "place.update" // fields of one place changed
	evPlaceDelete = "place.delete" // a place is gone
)

// PlaceTypes are the types of place, with the properties that a call can set on each.
// The tests check them against the description of Graph.
var PlaceTypes = map[string]map[string]propKind{
	"building": withCommon(map[string]propKind{"wifiState": kString, "resourceLinks": kList}),
	"floor":    withCommon(map[string]propKind{"sortOrder": kNumber}),
	"section":  withCommon(nil),
	"desk": withCommon(map[string]propKind{"displayDeviceName": kString, "heightAdjustableState": kString,
		"mailboxDetails": kObject, "mode": kObject, "servicePlans": kList}),
	"room": withCommon(map[string]propKind{"audioDeviceName": kString, "bookingType": kString, "building": kString,
		"capacity": kNumber, "displayDeviceName": kString, "emailAddress": kString, "floorLabel": kString,
		"floorNumber": kNumber, "nickname": kString, "teamsEnabledState": kString, "videoDeviceName": kString}),
	"workspace": withCommon(map[string]propKind{"capacity": kNumber, "displayDeviceName": kString,
		"emailAddress": kString, "mode": kObject, "nickname": kString}),
	"roomList": withCommon(map[string]propKind{"emailAddress": kString}),
}

func withCommon(own map[string]propKind) map[string]propKind {
	props := map[string]propKind{
		"displayName": kString, "address": kObject, "geoCoordinates": kObject, "isWheelChairAccessible": kBool,
		"label": kString, "parentId": kString, "phone": kString, "tags": kList,
	}
	maps.Copy(props, own)
	return props
}

// A call cannot change these (the Learn page "Update place").
var placeFixed = []string{"id", "placeId", "emailAddress", "displayName", "bookingType"}

// parentTypes says what can be the parent of a type. A building has none.
var parentTypes = map[string][]string{
	"floor": {"building"}, "section": {"floor"},
	"desk": {"floor", "section"}, "room": {"floor", "section"}, "workspace": {"floor", "section"},
}

const placeOdata = "#microsoft.graph."

// placeKind gives "room" for "#microsoft.graph.room", and for the form without the #.
func placeKind(odataType string) string {
	return strings.TrimPrefix(strings.TrimPrefix(odataType, "#"), "microsoft.graph.")
}

func (w *World) addPlace(p Object) {
	w.Places = append(w.Places, p)
	w.places[strings.ToLower(str(p["id"]))] = p
}

// ListPlaces gives the places of one type ("room"), as copies, in the order of the data.
func (w *World) ListPlaces(kind string) []Object {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var items []Object
	for _, p := range w.Places {
		if placeKind(str(p["@odata.type"])) == kind {
			items = append(items, deepCopy(p))
		}
	}
	return items
}

// GetPlace gives a copy of a place, found by ID.
func (w *World) GetPlace(id string) (Object, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.places[strings.ToLower(id)]
	if p == nil {
		return nil, false
	}
	return deepCopy(p), true
}

// RoomListMembers gives the rooms ("room") or the workspaces ("workspace") of a room
// list. The room list is found by its email address, or by ID. The bool is false
// when there is no such room list.
func (w *World) RoomListMembers(ref, kind string) ([]Object, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var list Object
	for _, p := range w.Places {
		if placeKind(str(p["@odata.type"])) == "roomList" &&
			(strings.EqualFold(str(p["emailAddress"]), ref) || strings.EqualFold(str(p["id"]), ref)) {
			list = p
		}
	}
	if list == nil {
		return nil, false
	}
	var items []Object
	for _, id := range w.RoomListMembersOf[strings.ToLower(str(list["id"]))] {
		if p := w.places[id]; p != nil && placeKind(str(p["@odata.type"])) == kind {
			items = append(items, deepCopy(p))
		}
	}
	return items, true
}

// CreatePlace makes a place. @odata.type says the type (microsoft.graph.building,
// floor, section, desk, room, or workspace). displayName is required. A floor needs
// a building as parent, a section a floor, and a desk, a room, or a workspace a floor
// or a section. It gives a copy of the new place.
func (w *World) CreatePlace(props map[string]any, a Actor) (Object, *apiError) {
	kind := placeKind(str(props["@odata.type"]))
	allowed, known := PlaceTypes[kind]
	switch {
	case str(props["@odata.type"]) == "":
		return nil, badRequest("The property '@odata.type' is required: it says the type of place.")
	case !known || kind == "roomList":
		return nil, badRequest("A place of the type '%s' cannot be made. Use building, floor, section, desk, room, or workspace.", str(props["@odata.type"]))
	}
	body := maps.Clone(props)
	delete(body, "@odata.type")
	for _, k := range []string{"id", "placeId"} {
		if _, has := body[k]; has {
			return nil, badRequest("Property '%s' is read-only.", k)
		}
	}
	if e := checkProps(body, allowed, "Place"); e != nil {
		return nil, e
	}
	if name, _ := body["displayName"].(string); strings.TrimSpace(name) == "" {
		return nil, missing("displayName")
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if e := w.parentProblem(kind, str(body["parentId"])); e != nil {
		return nil, e
	}
	id := newGUID()
	p := Object{"@odata.type": placeOdata + kind, "id": id, "placeId": id, "tags": []any{}, "isWheelChairAccessible": false}
	maps.Copy(p, body)
	w.placeDefaults(p, kind)
	if mail := str(p["emailAddress"]); mail != "" && w.emailTaken(mail) {
		return nil, badRequest("Another place with the email address '%s' exists already.", mail)
	}
	p = deepCopy(p)
	w.addPlace(p)
	w.record(Event{Type: evPlaceCreate, Actor: a, Place: id, PlaceName: str(p["displayName"]), Object: deepCopy(p)})
	return deepCopy(p), nil
}

// placeDefaults gives a new place the values that Graph gives it: an email address
// and a nickname to a room and a workspace, a booking type to a room, and a mode to
// a desk and a workspace.
func (w *World) placeDefaults(p Object, kind string) {
	name := str(p["displayName"])
	switch kind {
	case "room", "workspace":
		if p["emailAddress"] == nil {
			p["emailAddress"] = fmt.Sprintf("%s%s@acme.example", mailSlug(name), strings.ReplaceAll(str(p["id"]), "-", "")[:8])
		}
		if p["nickname"] == nil {
			p["nickname"] = name
		}
	}
	switch kind {
	case "room":
		if p["bookingType"] == nil {
			p["bookingType"] = "standard"
		}
	case "desk":
		if p["mode"] == nil {
			p["mode"] = Object{"@odata.type": "#microsoft.graph.dropInPlaceMode"}
		}
	case "workspace":
		if p["mode"] == nil {
			p["mode"] = Object{}
		}
	}
}

// mailSlug makes the start of an email address from a name: letters and digits.
func mailSlug(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (w *World) emailTaken(mail string) bool {
	for _, p := range w.Places {
		if strings.EqualFold(str(p["emailAddress"]), mail) {
			return true
		}
	}
	return false
}

// parentProblem says why a place of this type cannot have this parent, or nil. Call it
// with the lock held.
func (w *World) parentProblem(kind, parentID string) *apiError {
	want := parentTypes[kind]
	if parentID == "" {
		if len(want) > 0 {
			return badRequest("A place of the type '%s' needs a parentId (a %s).", kind, strings.Join(want, " or "))
		}
		return nil
	}
	parent := w.places[strings.ToLower(parentID)]
	switch {
	case len(want) == 0:
		return badRequest("A building has no parent.")
	case parent == nil:
		return badRequest("The parent place '%s' does not exist.", parentID)
	case !slices.Contains(want, placeKind(str(parent["@odata.type"]))):
		return badRequest("A place of the type '%s' needs a %s as parent, and '%s' is a %s.",
			kind, strings.Join(want, " or "), parentID, placeKind(str(parent["@odata.type"])))
	}
	return nil
}

// UpdatePlace changes properties of a place and gives the place. @odata.type, if it
// is in the body, must be the type of the place. A call cannot change id, placeId,
// emailAddress, displayName, or bookingType. null clears a property that can be empty.
func (w *World) UpdatePlace(id string, props map[string]any, a Actor) (Object, *apiError) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.places[strings.ToLower(id)]
	if p == nil {
		return nil, notFound("place", id)
	}
	kind := placeKind(str(p["@odata.type"]))
	body := maps.Clone(props)
	if t, has := body["@odata.type"]; has {
		if placeKind(str(t)) != kind {
			return nil, badRequest("The type of a place cannot change. This place is a %s.", kind)
		}
		delete(body, "@odata.type")
	}
	for _, k := range placeFixed {
		if _, has := body[k]; has {
			return nil, badRequest("Property '%s' cannot be changed.", k)
		}
	}
	if e := checkProps(body, PlaceTypes[kind], "Place"); e != nil {
		return nil, e
	}
	if v, has := body["parentId"]; has {
		if e := w.reparentProblem(id, kind, str(v)); e != nil {
			return nil, e
		}
	}
	for _, k := range []string{"tags", "isWheelChairAccessible"} {
		if v, has := body[k]; has && v == nil {
			return nil, badRequest("Property '%s' cannot be null.", k)
		}
	}
	if len(body) > 0 {
		maps.Copy(p, deepCopy(body))
		w.record(Event{Type: evPlaceUpdate, Actor: a, Place: str(p["id"]), PlaceName: str(p["displayName"]), Set: deepCopy(body)})
	}
	return deepCopy(p), nil
}

// reparentProblem is parentProblem for a place that moves: the new parent must have
// the right type, and cannot be the place itself.
func (w *World) reparentProblem(id, kind, parentID string) *apiError {
	if strings.EqualFold(id, parentID) {
		return badRequest("A place cannot be its own parent.")
	}
	return w.parentProblem(kind, parentID)
}

// DeletePlace deletes a building, a floor, a section, or a desk. A room, a room list,
// and a workspace cannot be deleted. A place that has places below it cannot be
// deleted until they are (the Learn page does not say what Graph does, so dolly
// refuses).
func (w *World) DeletePlace(id string, a Actor) *apiError {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.places[strings.ToLower(id)]
	if p == nil {
		return notFound("place", id)
	}
	kind := placeKind(str(p["@odata.type"]))
	if kind == "room" || kind == "roomList" || kind == "workspace" {
		return badRequest("A %s cannot be deleted.", kind)
	}
	for _, c := range w.Places {
		if strings.EqualFold(str(c["parentId"]), str(p["id"])) {
			return badRequest("The place has places below it. Delete them first.")
		}
	}
	name := str(p["displayName"])
	w.removePlace(str(p["id"]))
	w.record(Event{Type: evPlaceDelete, Actor: a, Place: str(p["id"]), PlaceName: name})
	return nil
}

func (w *World) removePlace(id string) {
	delete(w.places, strings.ToLower(id))
	w.Places = slices.DeleteFunc(w.Places, func(p Object) bool { return isID(str(p["id"]), id) })
}

// ---- the history of places ----

func placeEventText(e Event) (string, bool) {
	switch e.Type {
	case evPlaceCreate:
		return fmt.Sprintf("Made the %s %s", placeKind(str(e.Object["@odata.type"])), firstNonEmpty(e.PlaceName, e.Place)), true
	case evPlaceUpdate:
		return fmt.Sprintf("Changed the place %s: %s", firstNonEmpty(e.PlaceName, e.Place), setText(e.Set)), true
	case evPlaceDelete:
		return fmt.Sprintf("Deleted the place %s", firstNonEmpty(e.PlaceName, e.Place)), true
	}
	return "", false
}

func (w *World) applyPlaceEvent(e Event) bool {
	switch e.Type {
	case evPlaceCreate:
		w.addPlace(deepCopy(e.Object))
	case evPlaceUpdate:
		if p := w.places[strings.ToLower(e.Place)]; p != nil {
			maps.Copy(p, deepCopy(e.Set))
		}
	case evPlaceDelete:
		w.removePlace(e.Place)
	default:
		return false
	}
	return true
}
