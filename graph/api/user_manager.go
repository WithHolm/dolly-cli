package api

import (
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/withholm/dolly-cli/graph/spec"
)

// UserManagers is what the manager of a user needs from the data. A user has at
// most one manager, and a manager has direct reports.
type UserManagers interface {
	// Manager gives the manager of a user as an object with an @odata.type. It
	// gives a 404 when there is no such user, and when the user has no manager.
	Manager(ref string) (Object, *Error)
	// DirectReports gives the users that have this user as their manager. The bool
	// is false when there is no such user.
	DirectReports(ref string) ([]Object, bool)
	// SetManager makes a user the manager of another. A user cannot manage
	// themself, and a manager cannot report to their own report (400).
	SetManager(ref, managerRef string, a Actor) *Error
	// RemoveManager takes the manager away. It gives a 404 when there is none.
	RemoveManager(ref string, a Actor) *Error
}

// registerManager adds the operations for /users/{id}/manager and
// /users/{id}/directReports.
func registerManager(users *Scope, env Env, b UserManagers) {
	m := users.Scope("/{id}/manager", "Manager")
	m.Handle("GET", "", "Get the manager of a user. Says 404 when the user has none.", []string{"$select"},
		func(w http.ResponseWriter, r *http.Request) {
			o, e := b.Manager(r.PathValue("id"))
			if e != nil {
				WriteError(w, e)
				return
			}
			WriteJSON(w, http.StatusOK, SelectFields(o, SplitList(r.URL.Query().Get("$select")), []string{"@odata.type"}))
		})
	m.Handle("PUT", "/$ref", "Make a user the manager of this user. Needs @odata.id in the body. Says 400 for a user that would manage themself, or for a loop (the manager reports to this user). Gives 204.", nil,
		func(w http.ResponseWriter, r *http.Request) {
			id, e := refID(r)
			if e == nil {
				e = b.SetManager(r.PathValue("id"), id, env.Actor(r))
			}
			respond(w, e)
		})
	m.Handle("DELETE", "/$ref", "Take the manager away from a user. Says 404 when the user has none. Gives 204.", nil,
		func(w http.ResponseWriter, r *http.Request) {
			respond(w, b.RemoveManager(r.PathValue("id"), env.Actor(r)))
		})

	d := users.Scope("/{id}/directReports", "Manager")
	d.Handle("GET", "", "List the users that have this user as their manager.", []string{"$select", "$top", "$count", "$orderby"},
		func(w http.ResponseWriter, r *http.Request) {
			items, ok := b.DirectReports(r.PathValue("id"))
			if !ok {
				WriteError(w, NotFound("user", r.PathValue("id")))
				return
			}
			ServeList(w, r, env, items, ListOptions{Extra: []string{"@odata.type"}})
		})
	d.Handle("GET", "/$count", "Count the users that have this user as their manager. Needs ConsistencyLevel: eventual.", nil,
		func(w http.ResponseWriter, r *http.Request) {
			items, ok := b.DirectReports(r.PathValue("id"))
			if !ok {
				WriteError(w, NotFound("user", r.PathValue("id")))
				return
			}
			ServeCount(w, r, items, ListOptions{})
		})
}

// expandForm is the form of $expand on a user: manager or directReports, with or without (...)
// with options joined by ";". The options are $select=a,b, and for the manager also $levels=n|max.
var expandForm = regexp.MustCompile(`^(manager|directReports)(?:\(([^()]*)\))?$`)

const (
	maxLevels        = 1000 // the most that Graph takes for $levels
	maxExpandedItems = 20   // Graph gives at most 20 items of a collection that $expand puts inside an object
)

// expandOptions are the options inside manager(...) or directReports(...).
type expandOptions struct {
	relation string // manager or directReports
	sel      []string
	levels   int // 1 is the manager only
}

// rawQueryValue reads one query parameter from the raw query. Graph writes
// $expand=manager($levels=max;$select=id) with a ";" inside the value, and the
// parser of net/url drops every parameter that has a ";".
func rawQueryValue(r *http.Request, name string) string {
	for pair := range strings.SplitSeq(r.URL.RawQuery, "&") {
		k, v, _ := strings.Cut(pair, "=")
		if key, err := url.QueryUnescape(k); err == nil && key == name {
			val, _ := url.QueryUnescape(v)
			return val
		}
	}
	return ""
}

// parseExpand reads $expand. The empty string means no expand (nil, nil). Any
// form that dolly does not know gives 400, so a tool never gets an answer that
// looks right and is not.
func parseExpand(r *http.Request, single bool) (*expandOptions, *Error) {
	expand := rawQueryValue(r, "$expand")
	if expand == "" {
		return nil, nil
	}
	unknown := Unsupported("Unsupported $expand %q on a user. Use manager or directReports, with $select, and for the manager $levels, joined by ';'.", expand)
	m := expandForm.FindStringSubmatch(expand)
	if m == nil {
		// a name that the user does not have at all is a different error from a relation that Dolly does not expand
		name, _, _ := strings.Cut(expand, "(")
		if !spec.Load().Declared("user", name) {
			return nil, SyntaxError("Parsing OData Select and Expand failed: Could not find a property named '%s' on type 'microsoft.graph.user'.", name)
		}
		return nil, unknown
	}
	opt := &expandOptions{relation: m[1], levels: 1}
	for part := range strings.SplitSeq(m[2], ";") {
		switch name, val, _ := strings.Cut(part, "="); {
		case part == "":
		case name == "$select" && val != "":
			opt.sel = SplitList(val)
		case name == "$levels" && opt.relation == "manager":
			if !single {
				return nil, Unsupported("$levels works on one user (/users/{id}), and not on the list of users.")
			}
			if e := NeedsConsistency(r, "$levels in $expand"); e != nil {
				return nil, e
			}
			n := maxLevels
			if val != "max" {
				var err error
				if n, err = strconv.Atoi(val); err != nil || n < 1 || n > maxLevels {
					return nil, Unsupported("$levels must be max, or a number from 1 to %d. It got %q.", maxLevels, val)
				}
			}
			opt.levels = n
		default:
			return nil, unknown
		}
	}
	return opt, nil
}

// expandManager adds the manager (or the direct reports) to each user when the call has $expand.
// With $levels it adds the chain of managers, each inside the one below it, up to
// the number of levels or the top. A user with no manager gets no manager
// property, as in Graph. single is true for /users/{id}.
func expandManager(r *http.Request, b UserManagers, single bool, items ...Object) *Error {
	opt, e := parseExpand(r, single)
	if opt == nil || e != nil {
		return e
	}
	if opt.relation == "directReports" {
		for _, u := range items {
			reports, _ := b.DirectReports(Str(u["id"]))
			out := []Object{}
			for _, r := range reports[:min(len(reports), maxExpandedItems)] {
				out = append(out, SelectFields(r, opt.sel, []string{"@odata.type"}))
			}
			if len(out) > 0 { // a user with no reports has no property, as in Graph
				u["directReports"] = out
			}
		}
		return nil
	}
	for _, u := range items {
		cur := u
		for level := 0; level < opt.levels; level++ {
			mgr, e := b.Manager(Str(cur["id"]))
			if e != nil {
				break
			}
			next := SelectFields(mgr, opt.sel, []string{"@odata.type"})
			cur["manager"] = next
			cur = next
		}
	}
	return nil
}
