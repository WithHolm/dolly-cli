package repo

// The feed of changes, for what wants to be told when an object changes: the change notifications of Graph (graph/notify). Delta query reads the same changes later, from
// its log. A listener is called at the moment of the change, with the lock of the world held, so it must not block and must not call the world: it puts the change in a queue.

// ChangeEvent says that an object changed.
type ChangeEvent struct {
	Kind    string // user, group, servicePrincipal
	ID      string // the object ID, in lowercase
	Created bool
	Removed bool
	// Props are the properties that changed, when the object was not made or removed.
	Props []string
	// Member is set for a change of the members of a group (ID is the group): the object that went in, or out (MemberRemoved).
	Member        string
	MemberRemoved bool
}

// OnChange calls fn for each change that is made from now on. A change that is replayed (a restart, a reset, a rollback) is not one: nobody is told.
func (w *World) OnChange(fn func(ChangeEvent)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.listeners = append(w.listeners, fn)
}

// announce tells the listeners about a change. Call it with the lock held.
func (w *World) announce(c change) {
	if len(w.listeners) == 0 || w.delta.quiet {
		return
	}
	e := ChangeEvent{Kind: c.kind, ID: c.id, Created: c.created, Removed: c.removed, Props: c.props, Member: c.member, MemberRemoved: c.memberRemoved}
	for _, fn := range w.listeners {
		fn(e)
	}
}

// SeedID names the seed of the data. It is the same after a restart, and another when the data is made again from a new seed (a generation): what belongs to the tenant, such as
// a subscription, belongs to this value. A world with no folder gets a new one at each start.
func (w *World) SeedID() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.delta.epoch() // gives the log a name, when it has none
	return w.delta.base
}
