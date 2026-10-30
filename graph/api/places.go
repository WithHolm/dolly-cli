package api

import "net/http"

// PlacesBackend is what the places resource needs from the data: buildings, floors,
// sections, desks, rooms, workspaces, and room lists. A place is an object with its
// type in @odata.type. The lists are copies.
type PlacesBackend interface {
	// ListPlaces gives the places of one type: building, floor, section, desk, room,
	// workspace, or roomList.
	ListPlaces(kind string) []Object
	// GetPlace finds a place by ID.
	GetPlace(id string) (Object, bool)
	// CreatePlace makes a place. @odata.type in the props says the type.
	CreatePlace(props Object, a Actor) (Object, *Error)
	// UpdatePlace changes properties of a place and gives the place.
	UpdatePlace(id string, props Object, a Actor) (Object, *Error)
	// DeletePlace deletes a place.
	DeletePlace(id string, a Actor) *Error
	// RoomListMembers gives the rooms or the workspaces ("room" or "workspace") of a
	// room list, found by email address or by ID. The bool is false when there is no
	// such room list.
	RoomListMembers(ref, kind string) ([]Object, bool)
}

// Places is the resource for /places.
func Places(b PlacesBackend) Resource { return places{b} }

type places struct{ b PlacesBackend }

// placeTypes are the types that /places/microsoft.graph.<type> lists. $filter and
// $count are only for the three that a tenant can have many of (the Learn page
// "List place objects"), and those have pages of 100. The others have pages of 1000.
var placeTypes = []struct {
	name     string
	filtered bool
}{
	{"building", false}, {"floor", false}, {"section", false}, {"desk", false},
	{"room", true}, {"workspace", true}, {"roomList", true},
}

func (p places) Register(r *Router) {
	env := r.Env()
	s := r.Scope("/places", "Places")

	for _, t := range placeTypes {
		query := []string{"$select", "$top", "$skip", "$orderby"}
		size := 1000
		if t.filtered {
			query = append(query, "$filter", "$count")
			size = 100
		}
		list := func(w http.ResponseWriter, req *http.Request) {
			ServeList(w, req, env, p.b.ListPlaces(t.name), ListOptions{Filterable: t.filtered, NoConsistency: true, PageSize: size})
		}
		s.Handle("GET", "/microsoft.graph."+t.name, "List the places of the type "+t.name+".", query, list)
	}
	s.Handle("GET", "/{id}", "Get one place (a building, floor, section, desk, room, workspace, or room list), by ID.", []string{"$select"},
		func(w http.ResponseWriter, req *http.Request) {
			o, ok := p.b.GetPlace(req.PathValue("id"))
			if !ok {
				WriteError(w, NotFound("place", req.PathValue("id")))
				return
			}
			WriteJSON(w, http.StatusOK, SelectFields(o, SplitList(req.URL.Query().Get("$select")), []string{"@odata.type"}))
		})
	s.Handle("POST", "", "Make a place. @odata.type says the type (microsoft.graph.building, floor, section, desk, room, or workspace), and displayName is required. A floor needs a building as parentId, a section a floor, and a desk, a room, or a workspace a floor or a section. Gives 201 with the place.", nil,
		func(w http.ResponseWriter, req *http.Request) {
			props, e := ReadJSON(req)
			if e == nil {
				var o Object
				if o, e = p.b.CreatePlace(props, env.Actor(req)); e == nil {
					WriteJSON(w, http.StatusCreated, o)
					return
				}
			}
			WriteError(w, e)
		})
	s.Handle("PATCH", "/{id}", "Change address, geoCoordinates, isWheelChairAccessible, label, parentId, phone, tags, and the properties of the type. id, placeId, emailAddress, displayName, and bookingType cannot change. @odata.type, if given, must be the type of the place. Gives 200 with the place.", nil,
		func(w http.ResponseWriter, req *http.Request) {
			props, e := ReadJSON(req)
			if e == nil {
				var o Object
				if o, e = p.b.UpdatePlace(req.PathValue("id"), props, env.Actor(req)); e == nil {
					WriteJSON(w, http.StatusOK, o)
					return
				}
			}
			WriteError(w, e)
		})
	s.Handle("DELETE", "/{id}", "Delete a building, floor, section, or desk. A room, a room list, or a workspace cannot be deleted. A place with places below it cannot be deleted. Gives 204.", nil,
		func(w http.ResponseWriter, req *http.Request) {
			respond(w, p.b.DeletePlace(req.PathValue("id"), env.Actor(req)))
		})

	// The room list is found by its email address, not by its ID (the Learn page
	// writes the type as roomlist, and the other pages as roomList).
	for _, kind := range []struct{ path, name string }{{"rooms", "room"}, {"workspaces", "workspace"}} {
		for _, spelling := range []string{"microsoft.graph.roomlist", "microsoft.graph.roomList"} {
			s.Handle("GET", "/{list}/"+spelling+"/"+kind.path, "List the "+kind.path+" of a room list. The room list is found by its email address.",
				[]string{"$select", "$top", "$skip", "$filter", "$count", "$orderby"},
				func(w http.ResponseWriter, req *http.Request) {
					items, ok := p.b.RoomListMembers(req.PathValue("list"), kind.name)
					if !ok {
						WriteError(w, NotFound("room list", req.PathValue("list")))
						return
					}
					ServeList(w, req, env, items, ListOptions{Filterable: true, NoConsistency: true, PageSize: 100})
				})
		}
	}
}
