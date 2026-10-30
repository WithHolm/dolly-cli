package repo

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// World is everything that dolly knows: the users, the groups, the apps, and
// the role assignments. It is safe for many requests at once: one lock covers all
// of it, and each resource file takes that lock. The history (journal.go) records
// every change.
type World struct {
	mu          sync.RWMutex
	Seed        int64               `json:"seed"`
	Users       []Object            `json:"users"`
	Groups      []*Group            `json:"groups"`
	SPs         []*ServicePrincipal `json:"servicePrincipals"`
	Assignments []Assignment        `json:"assignments"`
	// Grants are the consents to delegated permissions (grants.go).
	Grants []Grant `json:"oauth2PermissionGrants,omitempty"`
	// Managers links a user ID to the ID of the manager, both in lowercase.
	Managers map[string]string `json:"managers,omitempty"`
	// Places are buildings, floors, sections, desks, rooms, workspaces, and room
	// lists (places.go). RoomListMembersOf gives the IDs of the rooms and workspaces
	// of each room list, by the lowercase ID of the room list.
	Places []Object `json:"places,omitempty"`
	// Organization is the tenant (organization.go).
	Organization      Object              `json:"organization,omitempty"`
	RoomListMembersOf map[string][]string `json:"roomListMembers,omitempty"`

	users   map[string]Object
	groups  map[string]*Group
	sps     map[string]*ServicePrincipal
	byAppID map[string]*ServicePrincipal // by the lowercase application (client) ID
	places  map[string]Object

	j *journal // the files of the data. nil: the world is only in memory
}

// index builds the lookups by ID. Call it after a change of the lists.
func (w *World) index() {
	w.users = make(map[string]Object, len(w.Users))
	for _, u := range w.Users {
		w.users[strings.ToLower(str(u["id"]))] = u
	}
	w.groups = make(map[string]*Group, len(w.Groups))
	for _, g := range w.Groups {
		w.groups[strings.ToLower(g.ID)] = g
	}
	if w.Managers == nil {
		w.Managers = map[string]string{}
	}
	w.ensureOrganization()
	defer w.refreshDynamic() // after the lookups below are made
	w.places = make(map[string]Object, len(w.Places))
	for _, p := range w.Places {
		w.places[strings.ToLower(str(p["id"]))] = p
	}
	w.sps = make(map[string]*ServicePrincipal, len(w.SPs))
	w.byAppID = make(map[string]*ServicePrincipal, len(w.SPs))
	for _, s := range w.SPs {
		w.sps[strings.ToLower(s.ID)] = s
		w.byAppID[strings.ToLower(s.AppID)] = s
	}
	w.ensureGraphServicePrincipal()
}

// replaceWith puts the content of another world in this one. The folder and the
// lock stay. Call it with the lock held.
func (w *World) replaceWith(o *World) {
	w.Seed, w.Users, w.Groups, w.SPs, w.Assignments, w.Managers = o.Seed, o.Users, o.Groups, o.SPs, o.Assignments, o.Managers
	w.Grants = o.Grants
	w.Places, w.RoomListMembersOf, w.Organization = o.Places, o.RoomListMembersOf, o.Organization
	w.index()
}

// loadBytes makes a world from the content of a seed file.
func loadBytes(data []byte) (*World, error) {
	w := &World{}
	if err := json.Unmarshal(data, w); err != nil {
		return nil, fmt.Errorf("dolly: the seed is not valid: %w", err)
	}
	w.index()
	return w, nil
}

// NewMemory makes a world that is only in memory. It keeps no history and no
// files. Tests use it.
func NewMemory(o Options) *World { return Generate(o) }

// ---- what the resources share ----

// kindOf says what an object ID is. Call it with the lock held.
func (w *World) kindOf(id string) (string, bool) {
	id = strings.ToLower(id)
	switch {
	case w.users[id] != nil:
		return typeUser, true
	case w.groups[id] != nil:
		return typeGroup, true
	case w.sps[id] != nil:
		return typeSP, true
	}
	return "", false
}

// objectOf gives the Graph object for an ID, with an @odata.type, or nil. Call it
// with the lock held.
func (w *World) objectOf(id string) Object {
	id = strings.ToLower(id)
	switch {
	case w.users[id] != nil:
		o := Object{"@odata.type": typeUser}
		for k, v := range w.users[id] {
			o[k] = v
		}
		return o
	case w.groups[id] != nil:
		return w.groups[id].object()
	case w.sps[id] != nil:
		return w.sps[id].object(false)
	}
	return nil
}

// objects gives the objects for IDs. An ID that has no object is left out. Call it
// with the lock held.
func (w *World) objects(ids []string) []Object {
	var items []Object
	for _, id := range ids {
		if o := w.objectOf(id); o != nil {
			items = append(items, o)
		}
	}
	return items
}

// nameOf is the name that a person knows an object by: the user principal name
// of a user, and the display name of a group or an app. Call it with the lock held.
func (w *World) nameOf(id string) string {
	key := strings.ToLower(id)
	switch {
	case w.users[key] != nil:
		return str(w.users[key]["userPrincipalName"])
	case w.groups[key] != nil:
		return w.groups[key].Name
	case w.sps[key] != nil:
		return w.sps[key].Name
	}
	return id
}

// Reset makes the world new again: the seed, with none of the changes. The
// changes stay in the file of changes, and a rollback can bring them back.
func (w *World) Reset(a Actor) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	var fresh *World
	if w.j != nil {
		var err error
		if fresh, err = loadBytes(w.j.baseline); err != nil {
			return err
		}
	} else {
		fresh = Generate(Options{Users: len(w.Users), Seed: w.Seed})
	}
	w.replaceWith(fresh)
	w.record(Event{Type: evReset, Actor: a})
	return nil
}

// ---- a short account of the data ----

// ValueCount is a value and how many users have it.
type ValueCount struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// State is a short account of the world.
type State struct {
	Seed              int64        `json:"seed"`
	Users             int          `json:"users"`
	Groups            int          `json:"groups"`
	ServicePrincipals int          `json:"servicePrincipals"`
	Departments       []ValueCount `json:"departments"`
	JobTitles         []ValueCount `json:"jobTitles"`
	Offices           []ValueCount `json:"offices"`
}

func distinct(users []Object, field string) []ValueCount {
	n := map[string]int{}
	for _, u := range users {
		if v := str(u[field]); v != "" {
			n[v]++
		}
	}
	out := make([]ValueCount, 0, len(n))
	for v, c := range n {
		out = append(out, ValueCount{v, c})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Value < out[j].Value })
	return out
}

// Snapshot describes the world now.
func (w *World) Snapshot() State {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return State{
		Seed: w.Seed, Users: len(w.Users), Groups: len(w.Groups), ServicePrincipals: len(w.SPs),
		Departments: distinct(w.Users, "department"),
		JobTitles:   distinct(w.Users, "jobTitle"),
		Offices:     distinct(w.Users, "officeLocation"),
	}
}

// UserCount is the number of users. It is the size of the tenant, for the limits of throttling.
func (w *World) UserCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.Users)
}

// ---- helpers for the checks of the resources ----

type propKind int

const (
	kString propKind = iota
	kBool
	kObject
	kList
	kNumber
)

// checkProps says what is wrong with the properties of a call, or nil: a property
// that does not exist, or a value of the wrong type. The order of the checks does
// not depend on the order of the keys, so the answer is the same each time.
func checkProps(props map[string]any, allowed map[string]propKind, resource string) *apiError {
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		kind, ok := allowed[k]
		if !ok {
			return badRequest("Property '%s' does not exist as declared property or extension property.", k)
		}
		v := props[k]
		if v == nil {
			continue
		}
		valid := false
		switch kind {
		case kString:
			_, valid = v.(string)
		case kBool:
			_, valid = v.(bool)
		case kObject:
			_, valid = v.(map[string]any)
		case kList:
			_, valid = v.([]any)
		case kNumber:
			_, valid = v.(float64)
		}
		if !valid {
			return badRequest("Invalid value specified for property '%s' of resource '%s'.", k, resource)
		}
	}
	return nil
}

func missing(prop string) *apiError { return badRequest("Property '%s' is required.", prop) }

// deepCopy copies an object so that a later change of the data cannot change it
// (an event keeps a copy of what was made).
func deepCopy(o map[string]any) map[string]any {
	data, _ := json.Marshal(o)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return out
}

func newGUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// isID reports whether two object IDs are the same (the case does not matter).
func isID(a, b string) bool { return strings.EqualFold(a, b) }
