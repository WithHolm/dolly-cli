package api

import "net/http"

// GroupApps is what the app role assignments of a group need from the data.
type GroupApps interface {
	// GroupAppRoleAssignments lists the app role assignments of a group. The bool is
	// false when there is no such group.
	GroupAppRoleAssignments(groupID string) ([]Object, bool)
}

// registerGroupApps adds the operation for /groups/{id}/appRoleAssignments.
func registerGroupApps(groups *Scope, env Env, b GroupApps) {
	s := groups.Scope("/{id}/appRoleAssignments", "Service principals")
	s.Handle("GET", "", "List the app role assignments of a group: the service principals that it is assigned to, with the role.", []string{"$select", "$top", "$count", "$orderby"},
		func(w http.ResponseWriter, r *http.Request) {
			items, ok := b.GroupAppRoleAssignments(r.PathValue("id"))
			if !ok {
				WriteError(w, NotFound("group", r.PathValue("id")))
				return
			}
			ServeList(w, r, env, items, ListOptions{})
		})
}
