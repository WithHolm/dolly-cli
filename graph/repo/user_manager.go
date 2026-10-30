package repo

import (
	"fmt"
	"strings"
)

// The manager of a user. The data side of api/user_manager.go. It satisfies
// api.UserManagers. A user has at most one manager. The world keeps the link as
// user ID to manager ID (World.Managers), not in the user object, as Graph does.

// Events that the managers make.
const (
	evManagerSet    = "manager.set"    // a user got a manager (Member is the manager)
	evManagerRemove = "manager.remove" // a user lost the manager
)

// Manager gives the manager of a user.
func (w *World) Manager(ref string) (Object, *apiError) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	u := w.userByRef(ref)
	if u == nil {
		return nil, notFound("user", ref)
	}
	id := w.Managers[strings.ToLower(str(u["id"]))]
	if id == "" || w.users[id] == nil {
		return nil, notFound("manager", ref)
	}
	return w.objectOf(id), nil
}

// DirectReports gives the users that have this user as manager, in the order of
// the data.
func (w *World) DirectReports(ref string) ([]Object, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	u := w.userByRef(ref)
	if u == nil {
		return nil, false
	}
	boss := strings.ToLower(str(u["id"]))
	var ids []string
	for _, r := range w.Users {
		if w.Managers[strings.ToLower(str(r["id"]))] == boss {
			ids = append(ids, str(r["id"]))
		}
	}
	return w.objects(ids), true
}

// SetManager makes a user the manager of another. The manager can be given by
// object ID or by user principal name.
func (w *World) SetManager(ref, managerRef string, a Actor) *apiError {
	w.mu.Lock()
	defer w.mu.Unlock()
	u := w.userByRef(ref)
	if u == nil {
		return notFound("user", ref)
	}
	m := w.userByRef(managerRef)
	if m == nil {
		return notFound("manager", managerRef)
	}
	id, boss := strings.ToLower(str(u["id"])), strings.ToLower(str(m["id"]))
	if e := w.managerProblem(id, boss); e != nil {
		return e
	}
	w.Managers[id] = boss
	w.refreshDynamic() // a direct reports group follows the manager
	w.record(Event{Type: evManagerSet, Actor: a, User: str(u["id"]), UserName: str(u["userPrincipalName"]),
		Member: str(m["id"]), MemberName: str(m["userPrincipalName"])})
	return nil
}

// managerProblem says why a user cannot get this manager, or nil: a user cannot
// manage themself, and the manager cannot be one of the reports of the user, however
// far down the chain. Call it with the lock held.
func (w *World) managerProblem(id, boss string) *apiError {
	if id == boss {
		return badRequest("A user cannot be their own manager.")
	}
	// walk up from the new manager: a loop means that the user is above them
	for cur, steps := boss, 0; cur != "" && steps <= len(w.Users); cur, steps = w.Managers[cur], steps+1 {
		if cur == id {
			return badRequest("A manager cannot report to their own report. That would be a loop.")
		}
	}
	return nil
}

// RemoveManager takes the manager away from a user.
func (w *World) RemoveManager(ref string, a Actor) *apiError {
	w.mu.Lock()
	defer w.mu.Unlock()
	u := w.userByRef(ref)
	if u == nil {
		return notFound("user", ref)
	}
	id := strings.ToLower(str(u["id"]))
	boss := w.Managers[id]
	if boss == "" {
		return notFound("manager", ref)
	}
	name := w.nameOf(boss)
	delete(w.Managers, id)
	w.refreshDynamic()
	w.record(Event{Type: evManagerRemove, Actor: a, User: str(u["id"]), UserName: str(u["userPrincipalName"]), Member: boss, MemberName: name})
	return nil
}

// dropManagers takes a user out of every manager link: as a report, and as a
// manager (their reports have no manager then, as in Graph). Call it with the lock held.
func (w *World) dropManagers(id string) {
	key := strings.ToLower(id)
	delete(w.Managers, key)
	for r, boss := range w.Managers {
		if boss == key {
			delete(w.Managers, r)
		}
	}
}

// ---- the history of managers ----

func managerEventText(e Event) (string, bool) {
	switch e.Type {
	case evManagerSet:
		return fmt.Sprintf("Made %s the manager of %s", firstNonEmpty(e.MemberName, e.Member), firstNonEmpty(e.UserName, e.User)), true
	case evManagerRemove:
		return fmt.Sprintf("Took the manager (%s) away from %s", firstNonEmpty(e.MemberName, e.Member), firstNonEmpty(e.UserName, e.User)), true
	}
	return "", false
}

func (w *World) applyManagerEvent(e Event) bool {
	switch e.Type {
	case evManagerSet:
		if w.users[strings.ToLower(e.User)] != nil && w.users[strings.ToLower(e.Member)] != nil {
			w.Managers[strings.ToLower(e.User)] = strings.ToLower(e.Member)
		}
	case evManagerRemove:
		delete(w.Managers, strings.ToLower(e.User))
	default:
		return false
	}
	return true
}
