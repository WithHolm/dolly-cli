package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/withholm/dolly-cli/shared"
)

// Delta query (https://learn.microsoft.com/graph/delta-query-overview): a client reads every object of a collection once, and after that it
// reads only what changed. A round is a chain of pages. A page with objects has @odata.nextLink (with $skiptoken). The last page is empty
// and has @odata.deltaLink (with $deltatoken), which starts the next round. The first page of a round has a nextLink even when it is empty
// (checked with the real Graph: a round with no changes takes two calls).

// DeltaChange is what changed for one object since a point of the log of changes (see repo.World.DeltaSince).
type DeltaChange struct {
	ID      string
	Created bool          // the object was made since then: every property is new
	Removed bool          // the object was deleted since then
	Props   []string      // the properties that changed, when it was not made
	Members []DeltaMember // for a group: the members that went in or out since then (the last change of each one)
}

// DeltaMember is a change of one member of a group.
type DeltaMember struct {
	ID      string
	Removed bool
}

// DeltaFeed is what delta query needs from the data: where the log of changes is, and what changed for one kind of object since a point of it.
type DeltaFeed interface {
	// DeltaNow gives the epoch of the data, and the number of the last change.
	DeltaNow() (epoch string, seq int)
	// DeltaSince gives what changed for the objects of one kind after the change with this number. It reports false when the epoch is not the
	// one of the data (a reset, a rollback, or a restart of a world with no folder), or the number is later than the last change.
	DeltaSince(kind, epoch string, since int) ([]DeltaChange, bool)
}

// DeltaSource says how delta query reads one resource.
type DeltaSource struct {
	Feed    DeltaFeed
	Kind    string   // the kind of object in the log of changes
	Set     string   // the set in @odata.context: users
	Tracked []string // the properties that are tracked and returned when the call has no $select. id is always returned.
	List    func() []Object
	Get     func(id string) (Object, bool)
	// Members and TypeOf are for a group, which has members@delta: the members (object IDs) that it has now, and the @odata.type of an object
	// (empty when the object is not there any more). They are nil for a resource with no members.
	Members func(group string) []string
	TypeOf  func(id string) string
}

const deltaPage = 200 // the size of a page. The real Graph gave 200 service principals in the first page. The size for the others is a guess.

// deltaToken is the state that a link carries. The real Graph gives opaque tokens, and so do we.
type deltaToken struct {
	Epoch  string   `json:"e"`
	Since  int      `json:"s"`             // the changes up to this number are known (a round of changes)
	Round  int      `json:"r,omitempty"`   // the number of the last change when this round began: the deltaLink at its end has it
	Phase  string   `json:"p,omitempty"`   // "" for the objects as they are now, "changes" for what changed after Since
	Cursor int      `json:"c,omitempty"`   // how many items of the round are sent
	Select []string `json:"sel,omitempty"` // $select of the first call of the cycle
	IDs    []string `json:"ids,omitempty"` // $filter=id eq '…' of the first call, in lowercase
}

func (t deltaToken) encode() string {
	b, _ := json.Marshal(t)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeDeltaToken(s string) (deltaToken, *Error) {
	var t deltaToken
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || json.Unmarshal(b, &t) != nil || t.Epoch == "" {
		return t, SyntaxError("Badly formed token.")
	}
	return t, nil
}

var idFilterRe = regexp.MustCompile(`(?i)^id\s+eq\s+'([^']+)'$`)

// parseIDFilter reads the only $filter that delta query takes: id eq '…', and several of them with or. Anything else is the error that
// Graph gives.
func parseIDFilter(filter string) ([]string, *Error) {
	var ids []string
	for _, part := range regexp.MustCompile(`(?i)\s+or\s+`).Split(strings.TrimSpace(filter), -1) {
		m := idFilterRe.FindStringSubmatch(strings.TrimSpace(part))
		if m == nil || len(ids) >= 50 {
			return nil, Unsupported("Unsupported query.")
		}
		ids = append(ids, strings.ToLower(m[1]))
	}
	return ids, nil
}

// ServeDelta answers a call of delta query for one resource.
func ServeDelta(w http.ResponseWriter, r *http.Request, src DeltaSource) {
	q := r.URL.Query()
	epoch, seq := src.Feed.DeltaNow()

	// the state of this page: from the token of the link, or new for the first call of a cycle
	tok := deltaToken{Epoch: epoch, Round: seq}
	first := true
	switch skip, delta := q.Get("$skiptoken"), q.Get("$deltatoken"); {
	case skip != "":
		var e *Error
		if tok, e = decodeDeltaToken(skip); e != nil {
			WriteError(w, e)
			return
		}
		first = false
	case delta == "latest":
		// "sync from now": no objects, and the link for the next round
		tok.Phase = "latest"
	case delta != "":
		var e *Error
		if tok, e = decodeDeltaToken(delta); e != nil {
			WriteError(w, e)
			return
		}
		tok.Phase, tok.Cursor, tok.Round = "changes", 0, seq // this is the first page of a round of changes
	}
	// the options of the first call. A $select on a later call of the cycle changes what is tracked, as the overview says.
	if sel := SplitList(q.Get("$select")); len(sel) > 0 {
		tok.Select = sel
	}
	if f := q.Get("$filter"); f != "" {
		ids, e := parseIDFilter(f)
		if e != nil {
			WriteError(w, e)
			return
		}
		tok.IDs = ids
	}
	gone := func() {
		// A token that cannot be used: the data went back (a reset or a rollback), or it is not this data. Graph says 410 and a Location
		// with an empty $deltatoken, which starts the cycle again. The error code and its text are a guess.
		w.Header().Set("Location", fmt.Sprintf("%s://%s%s?$deltatoken=", shared.Scheme(r), r.Host, r.URL.Path))
		WriteError(w, &Error{http.StatusGone, "syncStateNotFound", "The sync state generation is not found. The delta token is not valid for this data. Read the objects again, without a token."})
	}
	if tok.Epoch != epoch {
		gone()
		return
	}

	tracked := tok.Select
	if len(tracked) == 0 {
		tracked = src.Tracked
	}
	minimal := strings.Contains(strings.ToLower(r.Header.Get("Prefer")), "return=minimal")
	// members@delta is tracked by default, and with $select when it names members (checked with the real Graph)
	trackMembers := src.Members != nil && (len(tok.Select) == 0 || slices.ContainsFunc(tok.Select, func(s string) bool { return strings.EqualFold(s, "members") }))

	var items []Object
	more := false
	switch tok.Phase {
	case "latest":
		// nothing to send
	case "changes":
		changes, ok := src.Feed.DeltaSince(src.Kind, tok.Epoch, tok.Since)
		if !ok {
			gone()
			return
		}
		var all []Object
		for _, c := range changes {
			if len(tok.IDs) > 0 && !slices.Contains(tok.IDs, strings.ToLower(c.ID)) {
				continue
			}
			if o, ok := deltaChangeItem(src, c, tracked, minimal, trackMembers); ok {
				all = append(all, o)
			}
		}
		items, more = deltaPageOf(all, tok.Cursor, first)
	default:
		var all []Object
		for _, o := range src.List() {
			if len(tok.IDs) > 0 && !slices.Contains(tok.IDs, strings.ToLower(Str(o["id"]))) {
				continue
			}
			item := deltaObject(o, tracked, nil, false)
			if trackMembers {
				if list := deltaMembers(src, Str(o["id"]), nil, true); len(list) > 0 {
					item["members@delta"] = list
				}
			}
			all = append(all, item)
		}
		items, more = deltaPageOf(all, tok.Cursor, first)
	}

	body := map[string]any{"@odata.context": contextURL(src.Set, tok.Select, false), "value": items}
	if items == nil {
		body["value"] = []Object{}
	}
	link := func(param string, t deltaToken) string {
		return fmt.Sprintf("%s://%s%s?%s=%s", shared.Scheme(r), r.Host, r.URL.Path, param, t.encode())
	}
	if more {
		next := tok
		next.Cursor += len(items)
		body["@odata.nextLink"] = link("$skiptoken", next)
	} else {
		// the end of the round: the link for the next one starts after the changes that this round saw
		body["@odata.deltaLink"] = link("$deltatoken", deltaToken{Epoch: epoch, Since: tok.Round, Select: tok.Select, IDs: tok.IDs})
	}
	WriteJSON(w, http.StatusOK, body)
}

// deltaPageOf cuts one page from the items of a round. more says that the page has a nextLink: it has objects, or it is the first page.
func deltaPageOf(all []Object, cursor int, first bool) (page []Object, more bool) {
	if cursor >= len(all) {
		return nil, first && cursor == 0 // the empty last page has the deltaLink. The first page of an empty round still has a nextLink.
	}
	end := min(cursor+deltaPage, len(all))
	return all[cursor:end], true
}

// deltaChangeItem makes the item for one change: the object with the tracked properties, or the id with @removed. It reports false when
// the change is not one that the call tracks (a property that is not tracked changed) or the object is not there any more.
func deltaChangeItem(src DeltaSource, c DeltaChange, tracked []string, minimal, trackMembers bool) (Object, bool) {
	if c.Removed {
		return Object{"id": c.ID, "@removed": Object{"reason": "deleted"}}, true
	}
	members := trackMembers && len(c.Members) > 0
	if !c.Created && !members {
		hit := false
		for _, p := range c.Props {
			if slices.ContainsFunc(tracked, func(t string) bool { return strings.EqualFold(t, p) }) {
				hit = true
			}
		}
		if !hit {
			return nil, false // with $select, a change to a property that is not selected does not show the object
		}
	}
	o, ok := src.Get(c.ID)
	if !ok {
		return nil, false
	}
	item := deltaObject(o, tracked, c.Props, minimal && !c.Created)
	if trackMembers {
		if list := deltaMembers(src, c.ID, c.Members, c.Created); len(list) > 0 {
			item["members@delta"] = list
		}
	}
	return item, true
}

// deltaMembers makes members@delta for one group: all its members now (a group that is read for the first time, or was made), or the members
// that went in, and the ones that went out with @removed. A member that is not there any more has no @odata.type (a guess: the real Graph may
// give one).
func deltaMembers(src DeltaSource, group string, changes []DeltaMember, all bool) []Object {
	var list []Object
	typed := func(id string, removed bool) Object {
		m := Object{"id": id}
		if t := src.TypeOf(id); t != "" {
			m["@odata.type"] = t
		}
		if removed {
			m["@removed"] = Object{"reason": "deleted"}
		}
		return m
	}
	if all {
		for _, id := range src.Members(group) {
			list = append(list, typed(id, false))
		}
		return list
	}
	for _, c := range changes {
		list = append(list, typed(c.ID, c.Removed))
	}
	return list
}

// deltaObject makes the object for an answer: id, and the tracked properties that the object has a value for. A property that was never set is
// not there (it is not null), as the Learn page says. A property that changed to null is there. With minimal, only the properties that changed.
func deltaObject(o Object, tracked, changed []string, minimal bool) Object {
	out := Object{"id": o["id"]}
	for _, name := range tracked {
		if strings.EqualFold(name, "id") {
			continue
		}
		touched := slices.ContainsFunc(changed, func(c string) bool { return strings.EqualFold(c, name) })
		if minimal && !touched {
			continue
		}
		if key, v, has := foldLookup(o, name); has && (v != nil || touched) {
			out[key] = v
		}
	}
	return out
}
