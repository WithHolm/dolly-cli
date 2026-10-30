package api

import "net/http"

// UserRelations is what the groups and the app roles of a user need from the data.
type UserRelations interface {
	// UserMemberOf lists the groups that a user is a member of: the direct ones, or (transitive) also the groups that hold those
	// groups. The bool is false when there is no such user.
	UserMemberOf(ref string, transitive bool) ([]Object, bool)
	// UserAppRoleAssignments lists the app roles that are assigned to a user. The bool is false when there is no such user.
	UserAppRoleAssignments(ref string) ([]Object, bool)
}

// registerRelations adds the operations for /users/{id}/memberOf, /users/{id}/transitiveMemberOf, and /users/{id}/appRoleAssignments.
func registerRelations(users *Scope, env Env, b UserRelations) {
	for _, edge := range []struct {
		name       string
		transitive bool
		what       string
	}{
		{"memberOf", false, "the groups that a user is a direct member of"},
		{"transitiveMemberOf", true, "all the groups that a user is in, with the groups that hold those groups walked through (each one once)"},
	} {
		s := users.Scope("/{id}/"+edge.name, "Groups of a user")
		items := func(w http.ResponseWriter, r *http.Request) ([]Object, bool) {
			list, ok := b.UserMemberOf(r.PathValue("id"), edge.transitive)
			if !ok {
				WriteError(w, NotFound("user", r.PathValue("id")))
			}
			return list, ok
		}
		list := func(w http.ResponseWriter, r *http.Request) {
			if list, ok := items(w, r); ok {
				ServeList(w, r, env, list, ListOptions{Extra: []string{"@odata.type"}, Context: "directoryObjects"})
			}
		}
		count := func(w http.ResponseWriter, r *http.Request) {
			if list, ok := items(w, r); ok {
				ServeCount(w, r, list, ListOptions{})
			}
		}
		s.Handle("GET", "", "List "+edge.what+".", []string{"$select", "$top", "$count", "$orderby"}, list)
		s.Handle("GET", "/microsoft.graph.group", "List "+edge.what+", as groups.", []string{"$select", "$top", "$count", "$orderby"}, list)
		s.Handle("GET", "/$count", "Count "+edge.what+". Needs ConsistencyLevel: eventual.", nil, count)
	}

	s := users.Scope("/{id}/appRoleAssignments", "App roles of a user")
	s.Handle("GET", "", "List the app roles that are assigned to a user: the apps that the user has a role in. A role that a group has is not here.", []string{"$select", "$top", "$count", "$orderby"},
		func(w http.ResponseWriter, r *http.Request) {
			items, ok := b.UserAppRoleAssignments(r.PathValue("id"))
			if !ok {
				WriteError(w, NotFound("user", r.PathValue("id")))
				return
			}
			ServeList(w, r, env, items, ListOptions{Context: "appRoleAssignments"})
		})
}
