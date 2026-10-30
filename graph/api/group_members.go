package api

import (
	"io"
	"net/http"
	"strconv"
	"strings"
)

// GroupMembers is what the members of a group need from the data.
type GroupMembers interface {
	// Members gives the members of a group as objects with an @odata.type: the direct
	// members, or (transitive) all of them with the nested groups walked through, each
	// once. The bool is false when there is no such group.
	Members(groupID string, transitive bool) ([]Object, bool)
	AddMember(groupID, memberID string, a Actor) *Error
	RemoveMember(groupID, memberID string, a Actor) *Error
}

// registerMembers adds the operations for /groups/{id}/members and
// /groups/{id}/transitiveMembers.
func registerMembers(groups *Scope, env Env, b GroupMembers) {
	page := []string{"$select", "$top", "$count", "$orderby"}
	for _, edge := range []struct {
		name       string
		transitive bool
		what       string
	}{
		{"members", false, "the direct members of a group"},
		{"transitiveMembers", true, "all the members of a group, with the nested groups walked through (each one once)"},
	} {
		s := groups.Scope("/{id}/"+edge.name, "Members")
		list := membersList(env, b, edge.transitive, false)
		count := membersList(env, b, edge.transitive, true)
		s.Handle("GET", "", "List "+edge.what+": users, groups, and apps.", page, list)
		s.Handle("GET", "/{type}", "List only one type of "+edge.what+": microsoft.graph.user, .group, or .servicePrincipal.", page, list)
		s.Handle("GET", "/$count", "Count "+edge.what+". Needs ConsistencyLevel: eventual.", nil, count)
		s.Handle("GET", "/{type}/$count", "Count only one type of "+edge.what+". Needs ConsistencyLevel: eventual.", nil, count)
	}

	members := groups.Scope("/{id}/members", "Members")
	members.Handle("POST", "/$ref", "Put a user, a group, or an app in a group. Says 400 when it is a member already, or when the group is dynamic.", nil,
		func(w http.ResponseWriter, r *http.Request) {
			id, e := refID(r)
			if e == nil {
				e = b.AddMember(r.PathValue("id"), id, env.Actor(r))
			}
			respond(w, e)
		})
	members.Handle("DELETE", "/{member}/$ref", "Take a member out of a group. Says 404 when it is not a member.", nil,
		func(w http.ResponseWriter, r *http.Request) {
			respond(w, b.RemoveMember(r.PathValue("id"), r.PathValue("member"), env.Actor(r)))
		})
}

// membersList answers the members of a group, or the number of them. A path
// segment like microsoft.graph.user keeps one type only.
func membersList(env Env, b GroupMembers, transitive, count bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, ok := b.Members(r.PathValue("id"), transitive)
		if !ok {
			WriteError(w, NotFound("group", r.PathValue("id")))
			return
		}
		if t := r.PathValue("type"); t != "" {
			want := strings.ToLower("#" + t)
			if want != strings.ToLower(TypeUser) && want != strings.ToLower(TypeGroup) && want != strings.ToLower(TypeServicePrincipal) {
				WriteError(w, BadRequest("Invalid type segment: %s", t))
				return
			}
			var kept []Object
			for _, o := range items {
				if strings.ToLower(Str(o["@odata.type"])) == want {
					kept = append(kept, o)
				}
			}
			items = kept
		}
		if count {
			// Graph counts directory objects only with the header, and says this without it
			if !eventual(r) {
				WriteError(w, noRawCount())
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, strconv.Itoa(len(items)))
			return
		}
		ServeList(w, r, env, items, ListOptions{Extra: []string{"@odata.type"}})
	}
}
