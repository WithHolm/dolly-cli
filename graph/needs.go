package graph

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/withholm/dolly-cli/graph/permissions"
)

// What the calls of a service principal need. The server notes, for each client, which calls it made and with which kind of token (delegated or
// application). The permissions that each call needs are in graph/permissions/operations (made from the pages of Microsoft Learn). From these the
// portal shows the permissions that the client needs, and which of them it does not have yet.

const (
	kindDelegated   = "delegated"   // a user signed in: the token has scopes (scp)
	kindApplication = "application" // no user: the token has roles
	kindUnknown     = "unknown"     // a token that says neither
)

// maxNeedApps is how many clients are noted. A client ID comes from the token, so anyone can make one up. The routes are few, so a client does not
// need a limit of its own.
const maxNeedApps = 200

type callNeed struct {
	method, path string
	calls        int
	errors       int
	last         time.Time
}

type needTracker struct {
	mu   sync.Mutex
	apps map[string]map[string]*callNeed // the client ID in lower case, then the kind and the route
}

// tokenKind says whether a token is delegated (it has a user and scopes) or application (it has roles). A token that the sign-in API made is
// known. For another token the claims say it, and they can say anything: it is a hint for a page and nothing else.
func (s *Server) tokenKind(r *http.Request) string {
	if info, ours := s.token(r); ours {
		if info.UserID != "" {
			return kindDelegated
		}
		return kindApplication
	}
	parts := strings.Split(bearer(r), ".")
	if len(parts) != 3 {
		return kindUnknown
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return kindUnknown
	}
	var c struct {
		Scp   string   `json:"scp"`
		Roles []string `json:"roles"`
		Idtyp string   `json:"idtyp"`
	}
	if json.Unmarshal(payload, &c) != nil {
		return kindUnknown
	}
	switch {
	case c.Scp != "":
		return kindDelegated
	case len(c.Roles) > 0 || c.Idtyp == "app":
		return kindApplication
	}
	return kindUnknown
}

// noteNeed notes a call that a route answered. pattern is the route: "GET /v1.0/groups/{id}".
func (s *Server) noteNeed(r *http.Request, pattern string, status int) {
	method, path, ok := strings.Cut(pattern, " ")
	if !ok {
		return
	}
	app, kind := strings.ToLower(s.appID(r)), s.tokenKind(r)
	t := &s.needs
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.apps == nil {
		t.apps = map[string]map[string]*callNeed{}
	}
	calls := t.apps[app]
	if calls == nil {
		if len(t.apps) >= maxNeedApps {
			return
		}
		calls = map[string]*callNeed{}
		t.apps[app] = calls
	}
	key := kind + " " + pattern
	c := calls[key]
	if c == nil {
		c = &callNeed{method: method, path: path}
		calls[key] = c
	}
	c.calls++
	if status >= 400 {
		c.errors++
	}
	c.last = time.Now()
}

// forgetNeeds forgets what one client did, or what all of them did (with no client).
func (s *Server) forgetNeeds(app string) {
	s.needs.mu.Lock()
	defer s.needs.mu.Unlock()
	if app == "" {
		s.needs.apps = nil
		return
	}
	delete(s.needs.apps, strings.ToLower(app))
}

// ---- the answer ----

// NeedRow is one call that a client made, and what it needs.
type NeedRow struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	// Page is the page of Learn about the call (user-list), and Doc is its address. Both are empty when no page is about the call.
	Page   string    `json:"page,omitempty"`
	Doc    string    `json:"doc,omitempty"`
	Calls  int       `json:"calls"`
	Errors int       `json:"errors"`
	Last   time.Time `json:"last"`
	// Status is covered (the client has a permission that allows the call), missing, noData (no page of Learn has a table of permissions
	// for the call), notSupported (the page says that this kind of token cannot make the call), or varies (the permissions depend on the type of
	// object that the call is about: see Variants).
	Status string `json:"status"`
	// Variants are the rows of a page with a table by type of object.
	Variants []permissions.Variant `json:"variants,omitempty"`
	// Least are the alternatives with the least privilege (one of them is enough; "A and B" needs both), and Higher the ones with more.
	Least  []string `json:"least,omitempty"`
	Higher []string `json:"higher,omitempty"`
	// CoveredBy is the alternative that the client has.
	CoveredBy string `json:"coveredBy,omitempty"`
}

// KindNeeds is what the calls of one kind of token need.
type KindNeeds struct {
	Kind  string    `json:"kind"`
	Calls int       `json:"calls"`
	Rows  []NeedRow `json:"rows"`
	// Minimum is a set of permissions that allows every call that has data, with the least privilege: one alternative for each call, and as few
	// permissions as a simple search finds. Missing is what to add to the permissions that the client has, and that dollycli can grant.
	Minimum []string `json:"minimum"`
	Missing []string `json:"missing"`
	// Unavailable are permissions of the minimum that the client does not have, and that are not in the list of the permissions of Graph that
	// dollycli has (the pages of Learn are newer than that list), so dollycli cannot grant them.
	Unavailable []string `json:"unavailable"`
	// Assumed are permissions of the minimum that rest on a guess from implies.json: the minimum has them because they are assumed to include more.
	Assumed []permissions.AssumedPermission `json:"assumed"`
	// Granted is how many permissions of Microsoft Graph of this kind the client has. With none, there is nothing to compare.
	Granted int `json:"granted"`
	// Unused are permissions that the client has, and that none of the calls with data names. The client may make other calls that dollycli has not seen.
	Unused []string `json:"unused"`
	// Lower are permissions that the client has, that allow its calls, and that are higher than they need to be: a lower one would do.
	Lower []Lower `json:"lower"`
}

// Lower says that a permission could be replaced by lower ones: the calls that it covers need only these.
type Lower struct {
	From  string   `json:"from"`
	To    []string `json:"to"`
	Calls int      `json:"calls"` // the calls that the permission covers
}

// tokenScopes are scopes that shape the token and allow no call of Graph. They are never "unused".
var tokenScopes = map[string]bool{"openid": true, "profile": true, "offline_access": true, "email": true}

// Needs is the answer of the admin API for one client.
type Needs struct {
	AppID string      `json:"appId"`
	Kinds []KindNeeds `json:"kinds"`
	// UnknownCalls are the calls that came with a token that is neither delegated nor application.
	UnknownCalls int `json:"unknownCalls"`
	Source       struct {
		Commit     string `json:"commit"`
		CommitDate string `json:"commitDate"`
	} `json:"source"`
}

// needsOf works out what the calls of a client need, from what it did and what it has been granted.
func (s *Server) needsOf(app string) Needs {
	out := Needs{AppID: app, Kinds: []KindNeeds{}}
	ops, err := permissions.LoadOperations()
	if err != nil {
		return out // the data is built in and tested: this does not happen
	}
	out.Source.Commit, out.Source.CommitDate = ops.Source.Commit, ops.Source.CommitDate

	s.needs.mu.Lock()
	mine := map[string]callNeed{}
	for key, c := range s.needs.apps[strings.ToLower(app)] {
		mine[key] = *c
	}
	s.needs.mu.Unlock()

	catalog, _ := permissions.Load() // the permissions of Graph that dollycli knows. nil if it cannot be read: then nothing is held back.
	have := map[string]map[string]bool{kindDelegated: {}, kindApplication: {}}
	names := map[string][]string{} // the permissions of Graph that the client has, as written, by kind
	grant := func(kind, value string) {
		if l := strings.ToLower(value); !have[kind][l] {
			have[kind][l] = true
			if !tokenScopes[l] {
				names[kind] = append(names[kind], value)
			}
			for _, inc := range permissions.Includes(value) { // assumed to be included: see implies.json
				have[kind][inc] = true
			}
		}
	}
	if g, ok := s.world.GrantsOf(app); ok {
		for _, a := range g.Application {
			if strings.EqualFold(a.ResourceAppID, permissions.GraphAppID) {
				grant(kindApplication, a.Value)
			}
		}
		for _, d := range g.Delegated {
			if strings.EqualFold(d.ResourceAppID, permissions.GraphAppID) {
				for _, p := range d.Permissions {
					grant(kindDelegated, p.Value)
				}
			}
		}
	}

	rows := map[string][]NeedRow{}
	for key, c := range mine {
		kind, _, _ := strings.Cut(key, " ")
		if kind == kindUnknown {
			out.UnknownCalls += c.calls
			continue
		}
		row := NeedRow{Method: c.method, Path: c.path, Calls: c.calls, Errors: c.errors, Last: c.last, Status: "noData"}
		if perms, page, ok := ops.ForRoute(c.method + " " + c.path); ok {
			row.Page, row.Doc = page, permissions.LearnURL(page)
			// a page with a table for the signed-in user and one for another user: the path says which
			delegated, application, decided := perms.Resolve(c.path)
			need := delegated
			if kind == kindApplication {
				need = application
			}
			row.Least, row.Higher = need.Least, need.Higher
			switch {
			case !perms.Table:
				// the page has no table that dollycli can read: nothing to say
			case !decided:
				row.Status, row.Variants = "varies", perms.Variants
			case need.NotSupported && len(need.Least) == 0:
				row.Status = "notSupported"
			default:
				row.Status = "missing"
				for _, alt := range slices.Concat(need.Least, need.Higher) {
					if allIn(permissions.Alternatives(alt), have[kind]) {
						row.Status, row.CoveredBy = "covered", alt
						break
					}
				}
			}
		}
		rows[kind] = append(rows[kind], row)
	}
	for _, kind := range []string{kindDelegated, kindApplication} {
		list := rows[kind]
		if len(list) == 0 {
			continue
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].Calls != list[j].Calls {
				return list[i].Calls > list[j].Calls
			}
			return list[i].Method+list[i].Path < list[j].Method+list[j].Path
		})
		k := KindNeeds{Kind: kind, Rows: list, Minimum: []string{}, Missing: []string{}, Unavailable: []string{}}
		var all, uncovered [][]string
		for _, r := range list {
			k.Calls += r.Calls
			if len(r.Least) > 0 {
				all = append(all, r.Least)
				if r.Status == "missing" {
					uncovered = append(uncovered, r.Least)
				}
			}
		}
		k.Minimum = minimumSet(all, nil)
		catalogKind := permissions.Delegated
		if kind == kindApplication {
			catalogKind = permissions.Application
		}
		for _, p := range minimumSet(uncovered, have[kind]) {
			known := catalog == nil
			if catalog != nil {
				_, known = catalog.Find(catalogKind, p)
			}
			if known {
				k.Missing = append(k.Missing, p)
			} else {
				k.Unavailable = append(k.Unavailable, p)
			}
		}
		k.Assumed = []permissions.AssumedPermission{}
		for _, p := range k.Minimum {
			if a, ok := permissions.Assumed(p); ok {
				k.Assumed = append(k.Assumed, permissions.AssumedPermission{Permission: p, Assumption: a})
			}
		}
		k.Granted, k.Unused, k.Lower = len(names[kind]), []string{}, []Lower{}
		if k.Granted > 0 {
			k.Unused, k.Lower = compare(list, names[kind], have[kind], catalog, catalogKind)
		}
		out.Kinds = append(out.Kinds, k)
	}
	return out
}

// compare sets what the client has against what its calls need. Unused: granted, and named by no call with data. Lower: a granted permission that
// covers calls only as a higher alternative, while the least ones of those calls are not granted.
func compare(rows []NeedRow, granted []string, have map[string]bool, catalog *permissions.Catalog, kind permissions.Kind) ([]string, []Lower) {
	named := map[string]bool{}  // every permission that a call with data names, least or higher
	needed := map[string]bool{} // the permissions that cover a call as a least alternative
	for _, r := range rows {
		for _, alt := range slices.Concat(r.Least, r.Higher) {
			for _, p := range permissions.Alternatives(alt) {
				named[strings.ToLower(p)] = true
			}
		}
		if r.Status == "covered" && slices.Contains(r.Least, r.CoveredBy) {
			for _, p := range permissions.Alternatives(r.CoveredBy) {
				needed[strings.ToLower(p)] = true
			}
		}
	}
	unused := []string{}
	for _, p := range granted {
		if !named[strings.ToLower(p)] {
			unused = append(unused, p)
		}
	}
	sort.Strings(unused)

	type group struct {
		least [][]string
		calls int
	}
	groups := map[string]*group{}
	blocked := map[string]bool{}
	for _, r := range rows {
		if r.Status != "covered" || slices.Contains(r.Least, r.CoveredBy) || strings.Contains(r.CoveredBy, " and ") || len(r.Least) == 0 {
			continue
		}
		if needed[strings.ToLower(r.CoveredBy)] {
			continue // another call needs it as it is
		}
		// only the alternatives that dollycli can grant: a lower permission that it does not know is no advice
		grantable := slices.DeleteFunc(slices.Clone(r.Least), func(alt string) bool {
			return catalog != nil && slices.ContainsFunc(permissions.Alternatives(alt), func(p string) bool {
				_, ok := catalog.Find(kind, p)
				return !ok
			})
		})
		if len(grantable) == 0 {
			blocked[r.CoveredBy] = true // lowering it would leave this call with nothing that dollycli can grant
			continue
		}
		g := groups[r.CoveredBy]
		if g == nil {
			g = &group{}
			groups[r.CoveredBy] = g
		}
		g.least = append(g.least, grantable)
		g.calls += r.Calls
	}
	lower := []Lower{}
	for from, g := range groups {
		if blocked[from] {
			continue
		}
		lower = append(lower, Lower{From: from, To: minimumSet(g.least, have), Calls: g.calls})
	}
	sort.Slice(lower, func(i, j int) bool { return lower[i].From < lower[j].From })
	return unused, lower
}

func allIn(parts []string, have map[string]bool) bool {
	for _, p := range parts {
		if !have[strings.ToLower(p)] {
			return false
		}
	}
	return true
}

// minimumSet picks permissions so that every call has one alternative that is covered: a simple search, and not the best set in every case.
// Each element of calls is the alternatives of one call ("A and B" needs both). `have` are permissions that cost nothing (the client has them
// already), and they are not in the result. The calls with the fewest alternatives go first, and an alternative with the fewest new permissions wins,
// then the one with permissions that more calls can use. Last, a permission that no call needs any more is taken out.
func minimumSet(calls [][]string, have map[string]bool) []string {
	if len(calls) == 0 {
		return []string{}
	}
	lower := func(s string) string { return strings.ToLower(s) }
	// how many calls each permission can serve
	popularity := map[string]int{}
	for _, alts := range calls {
		seen := map[string]bool{}
		for _, alt := range alts {
			for _, p := range permissions.Alternatives(alt) {
				if !seen[lower(p)] {
					seen[lower(p)] = true
					popularity[lower(p)]++
				}
			}
		}
	}
	order := make([]int, len(calls))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return len(calls[order[a]]) < len(calls[order[b]]) })

	chosen := map[string]string{} // in lower case, with the spelling of Microsoft
	// a permission is held when it is had or chosen, or when one that is chosen is assumed to include it (implies.json): so a permission that
	// includes another one is taken, and not both. `without` is a chosen permission to leave out of the count.
	holds := func(p, without string) bool {
		l := lower(p)
		if have[l] || (chosen[l] != "" && l != without) {
			return true
		}
		for c := range chosen {
			if c != without && slices.Contains(permissions.Includes(c), l) {
				return true
			}
		}
		return false
	}
	has := func(p string) bool { return holds(p, "") }
	for _, i := range order {
		if slices.ContainsFunc(calls[i], func(alt string) bool { return allInFunc(permissions.Alternatives(alt), has) }) {
			continue
		}
		best, bestNew, bestPop := "", 0, 0
		for _, alt := range calls[i] {
			n, pop := 0, 0
			for _, p := range permissions.Alternatives(alt) {
				if !has(p) {
					n++
					pop += popularity[lower(p)]
				}
			}
			if best == "" || n < bestNew || (n == bestNew && (pop > bestPop || (pop == bestPop && alt < best))) {
				best, bestNew, bestPop = alt, n, pop
			}
		}
		for _, p := range permissions.Alternatives(best) {
			if !has(p) {
				chosen[lower(p)] = p
			}
		}
	}
	// take out a permission that nothing needs, the ones that serve few calls first
	names := make([]string, 0, len(chosen))
	for p := range chosen {
		names = append(names, p)
	}
	sort.Slice(names, func(a, b int) bool {
		if popularity[names[a]] != popularity[names[b]] {
			return popularity[names[a]] < popularity[names[b]]
		}
		return names[a] < names[b]
	})
	covered := func(without string) bool {
		for _, alts := range calls {
			ok := slices.ContainsFunc(alts, func(alt string) bool {
				return allInFunc(permissions.Alternatives(alt), func(p string) bool { return holds(p, without) })
			})
			if !ok {
				return false
			}
		}
		return true
	}
	for _, p := range names {
		if covered(p) {
			delete(chosen, p)
		}
	}
	out := make([]string, 0, len(chosen))
	for _, p := range chosen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func allInFunc(parts []string, ok func(string) bool) bool {
	for _, p := range parts {
		if !ok(p) {
			return false
		}
	}
	return true
}
