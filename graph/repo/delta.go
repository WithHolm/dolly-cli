package repo

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strconv"
	"strings"

	"github.com/withholm/dolly-cli/graph/api"
)

// The log of changes that delta query reads (https://learn.microsoft.com/graph/delta-query-overview).
//
// Every change of a user, a group, or a service principal adds an entry: the number of the change, the object, and the properties that
// changed. The log is made from the events that the history has (changesOf), when a change is recorded and when the history is
// replayed, so it is the same after a restart. It is kept in memory, also in a world that has no folder (which keeps no history).
//
// A token of delta query holds the epoch and a number. The epoch names the data: it changes at a reset and at a rollback, and in a world
// with no folder it is new at each start. A token of another epoch is refused, because the changes that it refers to are not there.

// The kinds of object that the log has.
const (
	KindUser             = "user"
	KindGroup            = "group"
	KindServicePrincipal = "servicePrincipal"
)

// change is one entry of the log.
type change struct {
	seq     int
	kind    string
	id      string   // the object ID, in lowercase
	props   []string // the properties that changed. nil says that the object was made, so every property is new.
	created bool
	removed bool
	// member is set for a change of the members of a group (id is the group): the object that went in, or out (memberRemoved).
	member        string
	memberRemoved bool
}

type changeLog struct {
	base    string // names the seed: the start of every epoch
	resets  int    // how many times the data went back (a reset or a rollback)
	seq     int    // the number of the last change
	changes []change
	// pending are changes that a change of the data made without an event of their own (a dynamic group that took a member, a user that
	// was deleted and went out of its groups). The next noteChange gives them the number of its event.
	pending []change
	quiet   bool // the data is being loaded: its dynamic groups work out their members, and that is not a change
}

func (l *changeLog) epoch() string {
	if l.base == "" {
		b := make([]byte, 4)
		_, _ = rand.Read(b)
		l.base = hex.EncodeToString(b) // a world with no folder: new at each start
	}
	return l.base + "-" + strconv.Itoa(l.resets)
}

// restart empties the log, because the data was replaced.
func (l *changeLog) restart(base string, resets, seq int) {
	*l = changeLog{base: base, resets: resets, seq: seq}
}

// seedBase names a seed file. A world with the same seed file starts with the same epoch, so a token is good after a restart.
func seedBase(baseline []byte) string {
	sum := sha256.Sum256(baseline)
	return hex.EncodeToString(sum[:4])
}

// noteChange adds what an event changed to the log. seq is the number of the event. Call it with the lock of the world held.
func (w *World) noteChange(seq int, e Event) {
	l := &w.delta
	l.seq = seq
	switch e.Type {
	case evReset, evRollback:
		l.epoch() // a world with no folder makes its name now, so the new epoch has the same start
		l.restart(l.base, l.resets+1, seq)
		return
	}
	for _, c := range append(changesOf(e), l.pending...) {
		c.seq = seq
		l.changes = append(l.changes, c)
		w.announce(c)
	}
	l.pending = nil
}

// noteMember keeps a change of the members of a group that has no event of its own, for the next noteChange. Call it with the lock held.
func (w *World) noteMember(group, member string, removed bool) {
	if w.delta.quiet {
		return
	}
	w.delta.pending = append(w.delta.pending, change{kind: KindGroup, id: strings.ToLower(group), member: strings.ToLower(member), memberRemoved: removed})
}

// changesOf says which objects an event changed, and which properties. It is the one place that knows it, for a change that is made
// now and for a change that is replayed.
func changesOf(e Event) []change {
	lower := strings.ToLower
	switch e.Type {
	case evUserCreate:
		if id, _ := e.Object["id"].(string); id != "" {
			return []change{{kind: KindUser, id: lower(id), created: true}}
		}
	case evUser:
		props := make([]string, 0, len(e.Set))
		for k := range e.Set {
			props = append(props, k)
		}
		slices.Sort(props)
		return []change{{kind: KindUser, id: lower(e.User), props: props}}
	case evUserDelete:
		return []change{{kind: KindUser, id: lower(e.User), removed: true}}
	case evRename:
		out := make([]change, 0, len(e.Users))
		for _, id := range e.Users {
			out = append(out, change{kind: KindUser, id: lower(id), props: []string{e.Field}})
		}
		return out
	case evManagerSet, evManagerRemove:
		return []change{{kind: KindUser, id: lower(e.User), props: []string{"manager"}}}
	case evGroupCreate:
		id := e.Group
		if e.GroupObject != nil && e.GroupObject.ID != "" {
			id = e.GroupObject.ID
		}
		return []change{{kind: KindGroup, id: lower(id), created: true}}
	case evGroupUpdate:
		props := make([]string, 0, len(e.Set))
		for k := range e.Set {
			props = append(props, k)
		}
		slices.Sort(props)
		return []change{{kind: KindGroup, id: lower(e.Group), props: props}}
	case evGroupDelete:
		return []change{{kind: KindGroup, id: lower(e.Group), removed: true}}
	case evSPCreate:
		// The other changes of a service principal (the permission mode, the flags, the credential state) are settings of dollycli, and
		// not properties of Graph. A change of a property or a delete (service_principals.go) has its own event.
		if e.SP != nil && e.SP.ID != "" {
			return []change{{kind: KindServicePrincipal, id: lower(e.SP.ID), created: true}}
		}
	case evSPUpdate:
		if e.SP != nil && e.SP.ID != "" { // the event names the service principal by its ID (service_principals.go)
			props := make([]string, 0, len(e.Set))
			for k := range e.Set {
				props = append(props, k)
			}
			slices.Sort(props)
			return []change{{kind: KindServicePrincipal, id: lower(e.SP.ID), props: props}}
		}
	case evSPDelete:
		if e.SP != nil && e.SP.ID != "" {
			return []change{{kind: KindServicePrincipal, id: lower(e.SP.ID), removed: true}}
		}
	case evMemberAdd, evMemberRemove:
		return []change{{kind: KindGroup, id: lower(e.Group), member: lower(e.Member), memberRemoved: e.Type == evMemberRemove}}
	}
	return nil
}

// DeltaChange is what changed for one object since a point of the log.
type DeltaChange = api.DeltaChange

// DeltaGroupMembers gives the object IDs of the members of a group, in lowercase.
func (w *World) DeltaGroupMembers(id string) []string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	g := w.groups[strings.ToLower(id)]
	if g == nil {
		return nil
	}
	out := make([]string, len(g.Members))
	for i, m := range g.Members {
		out[i] = strings.ToLower(m)
	}
	return out
}

// DeltaTypeOf gives the @odata.type of an object by its ID: a user, a group, or a service principal. It is empty when the object is not there.
func (w *World) DeltaTypeOf(id string) string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	key := strings.ToLower(id)
	switch {
	case w.users[key] != nil:
		return typeUser
	case w.groups[key] != nil:
		return typeGroup
	case w.sps[key] != nil:
		return typeSP
	}
	return ""
}

// DeltaNow says where the log is: the epoch, and the number of the last change.
func (w *World) DeltaNow() (epoch string, seq int) {
	w.mu.Lock() // epoch can make the name of the world
	defer w.mu.Unlock()
	return w.delta.epoch(), w.delta.seq
}

// DeltaSince gives what changed for the objects of one kind after the change with this number. Every object is once in the answer, in the
// order that it first changed. It reports false when the epoch is not the one of this data, or the number is later than the last change:
// the token is no good, and the client has to read the objects again.
func (w *World) DeltaSince(kind, epoch string, since int) ([]DeltaChange, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	l := &w.delta
	if epoch != l.epoch() || since < 0 || since > l.seq {
		return nil, false
	}
	var out []DeltaChange
	at := map[string]int{}
	for _, c := range l.changes {
		if c.seq <= since || c.kind != kind {
			continue
		}
		i, seen := at[c.id]
		if !seen {
			at[c.id] = len(out)
			out = append(out, DeltaChange{ID: c.id})
			i = len(out) - 1
		}
		d := &out[i]
		switch {
		case c.member != "":
			if j := slices.IndexFunc(d.Members, func(m api.DeltaMember) bool { return m.ID == c.member }); j >= 0 {
				d.Members[j].Removed = c.memberRemoved // the last change of a member wins
			} else {
				d.Members = append(d.Members, api.DeltaMember{ID: c.member, Removed: c.memberRemoved})
			}
		case c.removed:
			d.Removed, d.Props, d.Members = true, nil, nil
		case c.created:
			d.Removed, d.Created, d.Props = false, true, nil
		case !d.Created:
			for _, p := range c.props {
				if !slices.Contains(d.Props, p) {
					d.Props = append(d.Props, p)
				}
			}
		}
	}
	return out, true
}
