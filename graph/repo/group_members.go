package repo

import (
	"fmt"
	"slices"
	"strings"
)

// The members of a group. The data side of api/group_members.go. It satisfies
// api.GroupMembers.

// Events that the members make.
const (
	evMemberAdd    = "member.add"    // an object went into a group
	evMemberRemove = "member.remove" // an object went out of a group
)

func (g *Group) has(id string) bool {
	return slices.ContainsFunc(g.Members, func(m string) bool { return isID(m, id) })
}

// removeMember takes an ID out of the list of members.
func (g *Group) removeMember(id string) {
	g.Members = slices.DeleteFunc(g.Members, func(m string) bool { return isID(m, id) })
}

// Members gives the members of a group as objects with an @odata.type: the direct
// members, or (transitive) all of them with the nested groups walked through, each
// once. The bool is false when there is no such group.
func (w *World) Members(groupID string, transitive bool) ([]Object, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	g := w.groups[strings.ToLower(groupID)]
	if g == nil {
		return nil, false
	}
	ids := g.Members
	if transitive {
		ids = w.transitive(g)
	}
	return w.objects(ids), true
}

// transitive returns the IDs of every member, nested groups included, once
// each. A cycle of groups does not loop. Call it with the lock held.
func (w *World) transitive(g *Group) []string {
	seen := map[string]bool{strings.ToLower(g.ID): true}
	var out []string
	queue := []*Group{g}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, m := range cur.Members {
			key := strings.ToLower(m)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, m)
			if sub := w.groups[key]; sub != nil {
				queue = append(queue, sub)
			}
		}
	}
	return out
}

// AddMember adds an object to a group. The rules are the ones of Graph: a
// dynamic group cannot change, a Microsoft 365 group holds no group, and an
// object that is a member already gives a 400.
func (w *World) AddMember(groupID, memberID string, a Actor) *apiError {
	w.mu.Lock()
	defer w.mu.Unlock()
	g := w.groups[strings.ToLower(groupID)]
	if g == nil {
		return notFound("group", groupID)
	}
	if e := w.memberProblem(g, memberID); e != nil {
		return e
	}
	g.Members = append(g.Members, strings.ToLower(memberID))
	w.record(Event{Type: evMemberAdd, Actor: a, Group: g.ID, GroupName: g.Name, Member: strings.ToLower(memberID), MemberName: w.nameOf(memberID)})
	return nil
}

// memberProblem says why an object cannot go into a group, or nil when it can.
// Call it with the lock held.
func (w *World) memberProblem(g *Group, memberID string) *apiError {
	kind, ok := w.kindOf(memberID)
	switch {
	case !ok:
		return notFound("object", memberID)
	case g.Dynamic:
		return badRequest("Unable to change the members of a group with dynamic membership.")
	case kind == typeGroup && g.Unified:
		return badRequest("A Microsoft 365 group cannot hold a group.")
	case kind == typeGroup && isID(memberID, g.ID):
		return badRequest("A group cannot be a member of itself.")
	case g.has(memberID):
		return badRequest("One or more added object references already exist for the following modified properties: 'members'.")
	}
	return nil
}

// bindMembers puts the members of members@odata.bind in a new group. Call it with
// the lock held.
func (w *World) bindMembers(g *Group, props map[string]any) *apiError {
	ids := bindIDs(props["members@odata.bind"])
	if len(ids) > 0 && g.Dynamic {
		return badRequest("Unable to change the members of a group with dynamic membership.")
	}
	for _, id := range ids {
		if e := w.memberProblem(g, id); e != nil {
			return e
		}
		g.Members = append(g.Members, strings.ToLower(id))
	}
	return nil
}

// RemoveMember takes an object out of a group. A group with dynamic membership
// cannot change, and an object that is not a member gives a 404.
func (w *World) RemoveMember(groupID, memberID string, a Actor) *apiError {
	w.mu.Lock()
	defer w.mu.Unlock()
	g := w.groups[strings.ToLower(groupID)]
	if g == nil {
		return notFound("group", groupID)
	}
	if g.Dynamic {
		return badRequest("Unable to change the members of a group with dynamic membership.")
	}
	if !g.has(memberID) {
		return notFound("member", memberID)
	}
	name := w.nameOf(memberID)
	g.removeMember(memberID)
	w.record(Event{Type: evMemberRemove, Actor: a, Group: g.ID, GroupName: g.Name, Member: strings.ToLower(memberID), MemberName: name})
	return nil
}

// ---- the history of members ----

func memberEventText(e Event) (string, bool) {
	switch e.Type {
	case evMemberAdd:
		return fmt.Sprintf("Put %s in %s", firstNonEmpty(e.MemberName, e.Member), firstNonEmpty(e.GroupName, e.Group)), true
	case evMemberRemove:
		return fmt.Sprintf("Took %s out of %s", firstNonEmpty(e.MemberName, e.Member), firstNonEmpty(e.GroupName, e.Group)), true
	}
	return "", false
}

func (w *World) applyMemberEvent(e Event) bool {
	g := w.groups[strings.ToLower(e.Group)]
	switch e.Type {
	case evMemberAdd:
		if g != nil && !g.has(e.Member) {
			g.Members = append(g.Members, strings.ToLower(e.Member))
		}
	case evMemberRemove:
		if g != nil {
			g.removeMember(e.Member)
		}
	default:
		return false
	}
	return true
}
