package api

import "net/http"

// GroupMemberOf is what the groups that hold a group need from the data.
type GroupMemberOf interface {
	// GroupMemberOf lists the groups that have a group as a member: the direct ones, or (transitive) also the groups that hold
	// those groups. The bool is false when there is no such group.
	GroupMemberOf(groupID string, transitive bool) ([]Object, bool)
}

// registerGroupMemberOf adds the operations for /groups/{id}/memberOf and /groups/{id}/transitiveMemberOf.
func registerGroupMemberOf(groups *Scope, env Env, b GroupMemberOf) {
	for _, edge := range []struct {
		name       string
		transitive bool
		what       string
	}{
		{"memberOf", false, "the groups that a group is a direct member of"},
		{"transitiveMemberOf", true, "all the groups that a group is in, with the groups that hold those groups walked through (each one once)"},
	} {
		s := groups.Scope("/{id}/"+edge.name, "Groups of a group")
		items := func(w http.ResponseWriter, r *http.Request) ([]Object, bool) {
			list, ok := b.GroupMemberOf(r.PathValue("id"), edge.transitive)
			if !ok {
				WriteError(w, NotFound("group", r.PathValue("id")))
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
}
