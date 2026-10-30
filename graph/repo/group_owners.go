package repo

import (
	"fmt"
	"slices"
	"strings"
)

// The owners of a group. The data side of api/group_owners.go. It satisfies
// api.GroupOwners.

// Events that the owners make.
const (
	evOwnerAdd    = "owner.add"    // a user or an app became an owner of a group
	evOwnerRemove = "owner.remove" // an owner was taken out
)

func (g *Group) isOwner(id string) bool {
	return slices.ContainsFunc(g.Owners, func(o string) bool { return isID(o, id) })
}

// Owners gives the owners of a group as objects with an @odata.type. The bool is
// false when there is no such group.
func (w *World) Owners(groupID string) ([]Object, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	g := w.groups[strings.ToLower(groupID)]
	if g == nil {
		return nil, false
	}
	return w.objects(g.Owners), true
}

// ownerProblem says why an object cannot become an owner of a group, or nil when
// it can. Only users and apps can own a group. Call it with the lock held.
func (w *World) ownerProblem(g *Group, ownerID string) *apiError {
	kind, ok := w.kindOf(ownerID)
	switch {
	case !ok:
		return notFound("object", ownerID)
	case kind == typeGroup:
		return badRequest("A group cannot be an owner of a group. Only users and apps can.")
	case g.isOwner(ownerID):
		return badRequest("One or more added object references already exist for the following modified properties: 'owners'.")
	}
	return nil
}

// bindOwners makes the owners of owners@odata.bind in a new group. Call it with the
// lock held.
func (w *World) bindOwners(g *Group, props map[string]any) *apiError {
	for _, id := range bindIDs(props["owners@odata.bind"]) {
		if e := w.ownerProblem(g, id); e != nil {
			return e
		}
		g.Owners = append(g.Owners, strings.ToLower(id))
	}
	return nil
}

// AddOwner makes a user or an app an owner of a group.
func (w *World) AddOwner(groupID, ownerID string, a Actor) *apiError {
	w.mu.Lock()
	defer w.mu.Unlock()
	g := w.groups[strings.ToLower(groupID)]
	if g == nil {
		return notFound("group", groupID)
	}
	if e := w.ownerProblem(g, ownerID); e != nil {
		return e
	}
	g.Owners = append(g.Owners, strings.ToLower(ownerID))
	w.record(Event{Type: evOwnerAdd, Actor: a, Group: g.ID, GroupName: g.Name, Member: strings.ToLower(ownerID), MemberName: w.nameOf(ownerID)})
	return nil
}

// RemoveOwner takes an owner out of a group. It gives a 404 when the object is not an owner.
func (w *World) RemoveOwner(groupID, ownerID string, a Actor) *apiError {
	w.mu.Lock()
	defer w.mu.Unlock()
	g := w.groups[strings.ToLower(groupID)]
	if g == nil {
		return notFound("group", groupID)
	}
	if !g.isOwner(ownerID) {
		return notFound("owner", ownerID)
	}
	name := w.nameOf(ownerID)
	g.Owners = slices.DeleteFunc(g.Owners, func(o string) bool { return isID(o, ownerID) })
	w.record(Event{Type: evOwnerRemove, Actor: a, Group: g.ID, GroupName: g.Name, Member: strings.ToLower(ownerID), MemberName: name})
	return nil
}

// ---- the history of owners ----

func ownerEventText(e Event) (string, bool) {
	switch e.Type {
	case evOwnerAdd:
		return fmt.Sprintf("Made %s an owner of %s", firstNonEmpty(e.MemberName, e.Member), firstNonEmpty(e.GroupName, e.Group)), true
	case evOwnerRemove:
		return fmt.Sprintf("Took %s out as an owner of %s", firstNonEmpty(e.MemberName, e.Member), firstNonEmpty(e.GroupName, e.Group)), true
	}
	return "", false
}

func (w *World) applyOwnerEvent(e Event) bool {
	g := w.groups[strings.ToLower(e.Group)]
	switch e.Type {
	case evOwnerAdd:
		if g != nil && !g.isOwner(e.Member) {
			g.Owners = append(g.Owners, strings.ToLower(e.Member))
		}
	case evOwnerRemove:
		if g != nil {
			g.Owners = slices.DeleteFunc(g.Owners, func(o string) bool { return isID(o, e.Member) })
		}
	default:
		return false
	}
	return true
}
