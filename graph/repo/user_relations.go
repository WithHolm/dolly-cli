package repo

import "strings"

// The relations of a user that the Graph API has next to the manager and the direct reports: the groups that the user is in,
// and the app roles that are assigned to the user.

// groupsHolding gives the IDs of the groups that have an object as a direct member. Call it with the lock held.
func (w *World) groupsHolding(id string) []string {
	var out []string
	for _, g := range w.Groups {
		for _, m := range g.Members {
			if isID(m, id) {
				out = append(out, g.ID)
				break
			}
		}
	}
	return out
}

// UserMemberOf gives the groups that a user is a member of, as objects with an @odata.type: the direct ones, or (transitive) also the
// groups that hold those groups, and so on, each once. A user is found by object ID or user principal name. The bool is false when
// there is no such user.
func (w *World) UserMemberOf(ref string, transitive bool) ([]Object, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	u := w.userByRef(ref)
	if u == nil {
		return nil, false
	}
	return w.objects(w.holders(str(u["id"]), transitive)), true
}

// GroupMemberOf gives the groups that hold a group as a member: the direct ones, or (transitive) also the groups that hold those
// groups, and so on, each once. The bool is false when there is no such group.
func (w *World) GroupMemberOf(groupID string, transitive bool) ([]Object, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	g := w.groups[strings.ToLower(groupID)]
	if g == nil {
		return nil, false
	}
	return w.objects(w.holders(g.ID, transitive)), true
}

// holders gives the IDs of the groups that hold an object, and with transitive the groups that hold those. A cycle of groups does not
// loop, and the object itself is not in the answer. Call it with the lock held.
func (w *World) holders(id string, transitive bool) []string {
	ids := w.groupsHolding(id)
	if !transitive {
		return ids
	}
	seen := map[string]bool{strings.ToLower(id): true}
	for _, g := range ids {
		seen[strings.ToLower(g)] = true
	}
	for i := 0; i < len(ids); i++ {
		for _, parent := range w.groupsHolding(ids[i]) {
			if key := strings.ToLower(parent); !seen[key] {
				seen[key] = true
				ids = append(ids, parent)
			}
		}
	}
	return ids
}

// UserAppRoleAssignments gives the app roles that are assigned to a user itself (not through a group), as the app role assignments
// of Graph. The bool is false when there is no such user.
func (w *World) UserAppRoleAssignments(ref string) ([]Object, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	u := w.userByRef(ref)
	if u == nil {
		return nil, false
	}
	id := str(u["id"])
	var items []Object
	for _, a := range w.Assignments {
		if isID(a.PrincipalID, id) {
			items = append(items, w.assignmentObject(a))
		}
	}
	return items, true
}
