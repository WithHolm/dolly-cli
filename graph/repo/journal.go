package repo

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Dolly keeps its data in two files in one folder:
//
//   - seed.json is the data that dolly made on its first start (or that a
//     person wrote). It never changes.
//   - state.jsonl has every change since, one JSON object on each line, with
//     the time and who made the change. It only grows.
//
// The world now is the seed plus every change. So the file is also an audit log,
// and a rollback is a replay of the changes up to a point in time. A rollback is
// a change too: it is in the file, so it can be undone.

const (
	seedFile  = "seed.json"
	stateFile = "state.jsonl"
	lockFile  = ".lock"
)

// The types of the events that belong to the whole world. The events of a resource
// are in its file (users.go, groups.go, group_members.go, group_owners.go): each
// file has its types, the sentence for each (xxxEventText), and how a replay does
// each (applyXxxEvent). Add a resource, and add its three to the lists in Text and in apply.
const (
	evReset    = "reset"    // the world went back to the seed
	evRollback = "rollback" // the world went back to the state after an earlier event
)

// Event is one change of the world.
type Event struct {
	Seq   int       `json:"seq"` // 1 for the first change
	Time  time.Time `json:"time"`
	Type  string    `json:"type"`
	Actor Actor     `json:"actor"`

	// rename
	Field   string `json:"field,omitempty"`
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
	Changed int    `json:"changed,omitempty"`

	// user
	User     string         `json:"user,omitempty"` // the object ID
	UserName string         `json:"userName,omitempty"`
	Set      map[string]any `json:"set,omitempty"`

	// user.create: the object as it was made, with its ID
	Object map[string]any `json:"object,omitempty"`
	// group.create: the group as it was made, with its ID and members
	GroupObject *Group `json:"groupObject,omitempty"`

	// member.add, member.remove, and group.update (Set has the fields)
	Group      string `json:"group,omitempty"`
	GroupName  string `json:"groupName,omitempty"`
	Member     string `json:"member,omitempty"`
	MemberName string `json:"memberName,omitempty"`

	// place.create (Object has the whole place), place.update (Set), place.delete
	Place     string `json:"place,omitempty"`
	PlaceName string `json:"placeName,omitempty"`

	// rollback: the Seq of the last change that is kept. 0 is the seed.
	Target int `json:"target,omitempty"`

	// serviceprincipal.create (SP has it), serviceprincipal.mode (To), serviceprincipal.credential
	// (Credential; none means any), serviceprincipal.flag (Field is the flag, To is "true" or "false").
	// AppID is the application (client) ID. See principal.go.
	// tenant.set uses From and To.
	AppID      string            `json:"appId,omitempty"`
	SP         *ServicePrincipal `json:"servicePrincipal,omitempty"`
	Credential *CredState        `json:"credential,omitempty"`

	// assignment.create and .delete (Assignment), oauth2grant.create, .update, and .delete (Grant).
	// The names are for the history, which reads only the event. See grants.go.
	Assignment     *Assignment `json:"assignment,omitempty"`
	Grant          *Grant      `json:"grant,omitempty"`
	PrincipalName  string      `json:"principalName,omitempty"` // the principal, or for a grant the client
	ResourceName   string      `json:"resourceName,omitempty"`
	PermissionName string      `json:"permissionName,omitempty"` // the role, or for a grant the scope
}

// Text says in a sentence what the event did.
func (e Event) Text() string {
	switch e.Type {
	case evReset:
		return "Made the data new again, from the seed"
	case evRollback:
		if e.Target == 0 {
			return "Went back to the seed"
		}
		return fmt.Sprintf("Went back to the state after change %d", e.Target)
	}
	// the events of the resources: each file says its own
	for _, text := range []func(Event) (string, bool){userEventText, managerEventText, groupEventText, memberEventText, ownerEventText, placeEventText, principalEventText, grantEventText} {
		if s, ok := text(e); ok {
			return s
		}
	}
	return e.Type
}

// setText writes the fields of a change as "name = value", in the order of the names.
func setText(set map[string]any) string {
	parts := make([]string, 0, len(set))
	for k, v := range set {
		parts = append(parts, fmt.Sprintf("%s = %v", k, v))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// journal is the file of changes, and the changes in memory.
type journal struct {
	mu       sync.Mutex
	dir      string
	f        *os.File
	events   []Event
	baseline []byte // the content of seed.json
	now      func() time.Time
}

// OpenOptions sets how a new world is made, when the folder has no seed file.
type OpenOptions struct {
	Users int
	Seed  int64
	// Now gives the time of a change. nil uses the clock. Tests set it.
	Now func() time.Time
}

// Open opens the folder, makes it when it is not there, and gives the world: the
// seed plus every change in the file. A folder with no seed file gets one, made
// with opt. The folder is locked, so one dolly at a time uses it. Close
// releases it.
func Open(dir string, opt OpenOptions) (*World, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("dolly: creating %q: %w", dir, err)
	}
	if err := lock(dir); err != nil {
		return nil, err
	}
	w, err := open(dir, opt)
	if err != nil {
		unlock(dir)
		return nil, err
	}
	return w, nil
}

// Replace makes a new world in the folder, in place of the one that is there: a new seed (made
// with opt), and an empty history. The old seed and every change are deleted, and a rollback
// cannot bring them back. It fails when a dolly uses the folder. It gives a short account of the
// new world.
func Replace(dir string, opt OpenOptions) (State, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return State{}, fmt.Errorf("dolly: creating %q: %w", dir, err)
	}
	if err := lock(dir); err != nil {
		return State{}, err
	}
	defer unlock(dir)
	for _, name := range []string{seedFile, stateFile} {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return State{}, fmt.Errorf("dolly: removing the old data: %w", err)
		}
	}
	w, err := open(dir, opt)
	if err != nil {
		return State{}, err
	}
	st := w.Snapshot()
	_ = w.j.f.Close() // the folder stays locked until the end of this function
	return st, nil
}

func open(dir string, opt OpenOptions) (*World, error) {
	seedPath := filepath.Join(dir, seedFile)
	baseline, err := os.ReadFile(seedPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		fresh := Generate(Options{Users: opt.Users, Seed: opt.Seed})
		if baseline, err = json.MarshalIndent(fresh, "", "  "); err != nil {
			return nil, err
		}
		if err := os.WriteFile(seedPath, baseline, 0o644); err != nil {
			return nil, fmt.Errorf("dolly: writing %q: %w", seedPath, err)
		}
		slog.Info("Dolly made the seed data.", "file", seedPath, "users", opt.Users)
	case err != nil:
		return nil, fmt.Errorf("dolly: reading %q: %w", seedPath, err)
	}

	statePath := filepath.Join(dir, stateFile)
	events, err := readEvents(statePath)
	if err != nil {
		return nil, err
	}
	w, err := replay(baseline, events, len(events))
	if err != nil {
		return nil, fmt.Errorf("dolly: replaying %q: %w", statePath, err)
	}
	f, err := os.OpenFile(statePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("dolly: opening %q: %w", statePath, err)
	}
	now := opt.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	w.j = &journal{dir: dir, f: f, events: events, baseline: baseline, now: now}
	return w, nil
}

// readEvents reads the file of changes. A last line that is cut (a crash while
// it was written) is left out, with a warning. A bad line elsewhere is an error.
func readEvents(path string) ([]Event, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("dolly: reading %q: %w", path, err)
	}
	defer f.Close()

	var lines [][]byte
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		if line := bytes.TrimSpace(sc.Bytes()); len(line) > 0 {
			lines = append(lines, append([]byte(nil), line...))
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("dolly: reading %q: %w", path, err)
	}
	events := make([]Event, 0, len(lines))
	for i, line := range lines {
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			if i == len(lines)-1 {
				slog.Warn("Dolly left out the last line of the state file. It is cut.", "file", path)
				break
			}
			return nil, fmt.Errorf("dolly: line %d of %q is not valid: %w", i+1, path, err)
		}
		if e.Seq != len(events)+1 {
			return nil, fmt.Errorf("dolly: line %d of %q has the number %d, and %d was expected", i+1, path, e.Seq, len(events)+1)
		}
		events = append(events, e)
	}
	return events, nil
}

// Close closes the state file and releases the folder.
func (w *World) Close() error {
	if w.j == nil {
		return nil
	}
	w.j.mu.Lock()
	defer w.j.mu.Unlock()
	err := w.j.f.Close()
	unlock(w.j.dir)
	w.j = nil
	return err
}

// Dir is the folder of the data, or "" when the world is only in memory.
func (w *World) Dir() string {
	if w.j == nil {
		return ""
	}
	return w.j.dir
}

// now is the time of a change: the clock of the history, or the real clock for a
// world with no folder.
func (w *World) now() time.Time {
	if w.j == nil {
		return time.Now().UTC()
	}
	return w.j.now()
}

// record saves a change that is done. Call it with the lock of the world held,
// so the order of the file is the order of the changes. A world with no folder
// keeps no record.
func (w *World) record(e Event) {
	if w.j == nil {
		return
	}
	w.j.mu.Lock()
	defer w.j.mu.Unlock()
	e.Seq = len(w.j.events) + 1
	e.Time = w.j.now()
	line, err := json.Marshal(e)
	if err == nil {
		_, err = w.j.f.Write(append(line, '\n'))
	}
	if err != nil {
		// The change is in memory already. Say it, and keep working.
		slog.Error("Dolly could not write a change to the state file. The change is not saved.", "err", err)
		return
	}
	w.j.events = append(w.j.events, e)
}

// History gives the changes, oldest first.
func (w *World) History() []Event {
	if w.j == nil {
		return nil
	}
	w.j.mu.Lock()
	defer w.j.mu.Unlock()
	return append([]Event(nil), w.j.events...)
}

// ---- replay ----

// replay makes the world from the seed and the first n changes. A rollback in
// them takes the world back to the state after an earlier change.
func replay(baseline []byte, events []Event, n int) (*World, error) {
	w, err := loadBytes(baseline)
	if err != nil {
		return nil, err
	}
	for _, e := range events[:n] {
		switch e.Type {
		case evReset:
			if w, err = loadBytes(baseline); err != nil {
				return nil, err
			}
		case evRollback:
			if e.Target < 0 || e.Target >= e.Seq {
				return nil, fmt.Errorf("change %d goes back to change %d, which is not before it", e.Seq, e.Target)
			}
			if w, err = replay(baseline, events, e.Target); err != nil {
				return nil, err
			}
		default:
			w.apply(e)
		}
	}
	return w, nil
}

// apply does a change on a world that nobody else uses. The change was right when
// it was made, so a problem now (a group that is gone) is not an error.
func (w *World) apply(e Event) {
	// the events of the resources: each file does its own
	switch {
	case w.applyUserEvent(e), w.applyManagerEvent(e), w.applyGroupEvent(e), w.applyMemberEvent(e), w.applyOwnerEvent(e), w.applyPlaceEvent(e), w.applyPrincipalEvent(e), w.applyGrantEvent(e):
	}
	// The members of a dynamic group are not in the history: they are worked out again, as
	// they were when the change was made (a paused group keeps the members that it had).
	w.refreshFor(e)
}

// ---- go back ----

// RollbackTarget finds the last change that was made at or before t. 0 means that
// no change was, and the seed is the state.
func (w *World) RollbackTarget(t time.Time) int {
	target := 0
	for _, e := range w.History() {
		if e.Time.After(t) {
			break
		}
		target = e.Seq
	}
	return target
}

// Rollback takes the world back to the state after the change with this number
// (0 is the seed). The changes after it stay in the file, and so does the
// rollback itself. It is an error when the number is not a change that exists.
func (w *World) Rollback(target int, a Actor) error {
	if w.j == nil {
		return errors.New("this dolly keeps no history, so it cannot go back")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	events := w.History()
	if target < 0 || target > len(events) {
		return fmt.Errorf("there is no change number %d. The history has %d", target, len(events))
	}
	fresh, err := replay(w.j.baseline, events, target)
	if err != nil {
		return err
	}
	w.replaceWith(fresh)
	w.record(Event{Type: evRollback, Target: target, Actor: a})
	return nil
}
