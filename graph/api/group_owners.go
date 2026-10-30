package api

import "net/http"

// GroupOwners is what the owners of a group need from the data.
type GroupOwners interface {
	// Owners gives the owners of a group as objects with an @odata.type. The bool is
	// false when there is no such group.
	Owners(groupID string) ([]Object, bool)
	AddOwner(groupID, ownerID string, a Actor) *Error
	RemoveOwner(groupID, ownerID string, a Actor) *Error
}

// registerOwners adds the operations for /groups/{id}/owners.
func registerOwners(groups *Scope, env Env, b GroupOwners) {
	s := groups.Scope("/{id}/owners", "Owners")
	s.Handle("GET", "", "List the owners of a group: users and apps.", []string{"$select", "$top", "$count", "$orderby"},
		func(w http.ResponseWriter, r *http.Request) {
			items, ok := b.Owners(r.PathValue("id"))
			if !ok {
				WriteError(w, NotFound("group", r.PathValue("id")))
				return
			}
			ServeList(w, r, env, items, ListOptions{Extra: []string{"@odata.type"}})
		})
	s.Handle("POST", "/$ref", "Make a user or an app an owner of a group. A group cannot be an owner. Says 400 when it is an owner already.", nil,
		func(w http.ResponseWriter, r *http.Request) {
			id, e := refID(r)
			if e == nil {
				e = b.AddOwner(r.PathValue("id"), id, env.Actor(r))
			}
			respond(w, e)
		})
	s.Handle("DELETE", "/{owner}/$ref", "Take an owner out of a group. Says 404 when it is not an owner.", nil,
		func(w http.ResponseWriter, r *http.Request) {
			respond(w, b.RemoveOwner(r.PathValue("id"), r.PathValue("owner"), env.Actor(r)))
		})
}
