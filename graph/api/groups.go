package api

import "net/http"

// GroupCore is the part of the groups resource that is about the group itself.
type GroupCore interface {
	// ListGroups gives every group as an object, in the order of the data.
	ListGroups() []Object
	GetGroup(id string) (Object, bool)
	// CreateGroup makes a group and gives it as an object.
	CreateGroup(props Object, a Actor) (Object, *Error)
	UpdateGroup(id string, props Object, a Actor) *Error
	// DeleteGroup deletes a group, and every reference to it.
	DeleteGroup(id string, a Actor) *Error
}

// GroupsBackend is everything that /groups needs: the group, its members, its
// owners, and the apps that it is assigned to. Each part is in its own file.
type GroupsBackend interface {
	GroupCore
	GroupMembers
	GroupOwners
	GroupApps
	GroupMemberOf
}

// DefaultGroupDeltaProperties are the properties that /groups/delta tracks and returns when the call has no $select (checked with the real
// Graph). members@delta comes with them. A property that the group does not have is left out.
var DefaultGroupDeltaProperties = []string{
	"createdDateTime", "description", "displayName", "mailEnabled", "mailNickname", "renewedDateTime", "securityEnabled",
	"securityIdentifier", "visibility", "id",
}

// groupDelta is what delta query for groups needs from the data, on top of the log of changes.
type groupDelta interface {
	DeltaFeed
	DeltaGroupMembers(group string) []string
	DeltaTypeOf(id string) string
}

// Groups is the resource for /groups, with everything below it: /groups/{id},
// and the members, the owners, and the apps of a group.
func Groups(b GroupsBackend) Resource { return groups{b} }

type groups struct{ b GroupsBackend }

func (g groups) Register(r *Router) {
	env := r.Env()
	s := r.Scope("/groups", "Groups")
	list := []string{"$select", "$top", "$filter", "$search", "$count", "$orderby"}
	// a client can subscribe to all groups, or to one. (The members and the owners of a group are a later step.)
	r.Subscribable(Subscribable{Pattern: "groups", Kind: "group", MaxMinutes: directoryLifetime}, Subscribable{Pattern: "groups/{id}", Kind: "group", MaxMinutes: directoryLifetime})

	s.Handle("GET", "", "List the groups.", list, func(w http.ResponseWriter, req *http.Request) {
		ServeList(w, req, env, g.b.ListGroups(), ListOptions{Filterable: true, Resource: "group", Context: "groups"})
	})
	if feed, ok := g.b.(groupDelta); ok { // the data has a log of changes (the fake of a test may not)
		src := DeltaSource{Feed: feed, Kind: "group", Set: "groups", Tracked: DefaultGroupDeltaProperties, List: g.b.ListGroups, Get: g.b.GetGroup,
			Members: feed.DeltaGroupMembers, TypeOf: feed.DeltaTypeOf}
		serve := func(w http.ResponseWriter, req *http.Request) { ServeDelta(w, req, src) }
		const summary = "Read the groups, and then only what changed since (delta query). It works as /users/delta. Each group has members@delta: its members in the first round, and later the members that went in, and the ones that went out with @removed. $select without members leaves members@delta out."
		opts := []string{"$select", "$filter", "$deltatoken", "$skiptoken"}
		s.Handle("GET", "/delta", summary, opts, serve)
		s.Handle("GET", "/delta()", summary, opts, serve)
	}
	s.Handle("GET", "/$count", "Count the groups. Needs ConsistencyLevel: eventual.", []string{"$filter", "$search"}, func(w http.ResponseWriter, req *http.Request) {
		ServeCount(w, req, g.b.ListGroups(), ListOptions{Filterable: true, Resource: "group"})
	})
	s.Handle("GET", "/{id}", "Get one group.", []string{"$select"}, func(w http.ResponseWriter, req *http.Request) {
		o, ok := g.b.GetGroup(req.PathValue("id"))
		if !ok {
			WriteError(w, NotFound("group", req.PathValue("id")))
			return
		}
		WriteJSON(w, http.StatusOK, one("groups", o, req, nil, nil, "group"))
	})
	s.Handle("POST", "", "Make a group. Needs displayName, mailEnabled, mailNickname, and securityEnabled. groupTypes can have Unified (Microsoft 365) and DynamicMembership (needs membershipRule). members@odata.bind puts members in. owners@odata.bind makes owners (users and apps). Visibility is accepted and not kept. Gives 201 with the group.", nil,
		func(w http.ResponseWriter, req *http.Request) {
			props, e := ReadJSON(req)
			if e == nil {
				var o Object
				if o, e = g.b.CreateGroup(props, env.Actor(req)); e == nil {
					WriteJSON(w, http.StatusCreated, o)
					return
				}
			}
			WriteError(w, e)
		})
	s.Handle("PATCH", "/{id}", "Change displayName, description, mailNickname, mailEnabled, securityEnabled, or membershipRule (dynamic groups only). groupTypes and isAssignableToRole cannot change. Gives 204.", nil,
		func(w http.ResponseWriter, req *http.Request) {
			props, e := ReadJSON(req)
			if e == nil {
				e = g.b.UpdateGroup(req.PathValue("id"), props, env.Actor(req))
			}
			respond(w, e)
		})
	s.Handle("DELETE", "/{id}", "Delete a group. It also goes out of every group that held it, and out of the app role assignments. Gives 204.", nil,
		func(w http.ResponseWriter, req *http.Request) {
			respond(w, g.b.DeleteGroup(req.PathValue("id"), env.Actor(req)))
		})

	registerMembers(s, env, g.b)
	registerOwners(s, env, g.b)
	registerGroupApps(s, env, g.b)
	registerGroupMemberOf(s, env, g.b)
}
