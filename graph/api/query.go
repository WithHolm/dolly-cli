package api

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/withholm/dolly-cli/graph/query"
	"github.com/withholm/dolly-cli/graph/spec"
)

// The query options that every collection shares: $filter, $search, $orderby, $count,
// $select, $top, and paging with a next link. A resource gives its items to
// ServeList, and ServeList answers.
//
// The rules are the ones of Graph. For a directory object (users, groups, service principals) Graph decides
// by property and operator what works, and what needs an advanced query (ConsistencyLevel: eventual
// and $count=true). Dolly reads those rules from Microsoft's own tables (package query), so a call
// that Graph refuses is refused with the error of Graph, and a call that Graph answers is answered.

// ---- the forms that the portal shows ----

// FilterForm is one form of $filter, with an example that works. The portal shows these as help, and a
// test sends the Example of every form to a server, so the help is true. Which properties a form works on
// is in the tables of Graph (package query), and the portal shows that for each resource.
type FilterForm struct {
	Name             string `json:"name"`
	Syntax           string `json:"syntax"`  // field eq 'text'
	Example          string `json:"example"` // department eq 'Legal'
	Note             string `json:"note"`
	NeedsConsistency bool   `json:"needsConsistency"` // an advanced query: ConsistencyLevel: eventual and $count=true
	Resource         string `json:"resource"`         // the resource that the example is for: user, servicePrincipal
}

var filterForms = []FilterForm{
	{Name: "Equals", Syntax: "field eq value", Example: "department eq 'Legal'", Note: "Text, true, false, a number, null, a date, or a GUID. Capitals do not matter in text.", Resource: "user"},
	{Name: "Starts with", Syntax: "startsWith(field,'text')", Example: "startsWith(displayName,'Acme')", Note: "For a text property.", Resource: "user"},
	{Name: "In a list", Syntax: "field in ('a','b')", Example: "department in ('Legal','Support')", Note: "Works where eq works by default.", Resource: "user"},
	{Name: "And, or, not", Syntax: "a and (b or c)", Example: "department eq 'Legal' and (jobTitle eq 'Counsel' or jobTitle eq 'Paralegal')", Note: "With parentheses to group.", Resource: "user"},
	{Name: "Not", Syntax: "not(expression), field ne value", Example: "not(startsWith(displayName,'Acme'))", Note: "Works wherever eq works, but only in an advanced query.", NeedsConsistency: true, Resource: "user"},
	{Name: "Null", Syntax: "field ne null", Example: "mail ne null", Note: "For a property that can be empty.", NeedsConsistency: true, Resource: "user"},
	{Name: "Compare", Syntax: "field ge value, le, gt, lt", Example: "createdDateTime ge 2024-01-01T00:00:00Z", Note: "For a date. Only the properties that the table of Graph lists for ge/le.", Resource: "user"},
	{Name: "Ends with", Syntax: "endsWith(field,'text')", Example: "endsWith(mail,'@acme.example')", Note: "Only mail, otherMails, userPrincipalName, and proxyAddresses.", NeedsConsistency: true, Resource: "user"},
	{Name: "In a collection", Syntax: "collection/any(x:expression)", Example: "proxyAddresses/any(p:startsWith(p,'smtp:a'))", Note: "any and all. In the expression, x is one item, or x/property for an object.", Resource: "user"},
	{Name: "Empty collection", Syntax: "collection/$count eq 0", Example: "assignedLicenses/$count eq 0", Note: "Only the collections that the table of Graph lists.", NeedsConsistency: true, Resource: "user"},
	{Name: "A tag", Syntax: "tags/any(t:t eq 'text')", Example: "tags/any(t:t eq 'WindowsAzureActiveDirectoryIntegratedApp')", Note: "Only service principals have tags. Capitals matter here.", Resource: "servicePrincipal"},
}

// FilterForms gives the forms of $filter.
func FilterForms() []FilterForm { return filterForms }

// filterSummary says in one sentence what $filter does.
func filterSummary() string {
	return "OData: eq, ne, gt, ge, lt, le, in, and, or, not, startsWith, endsWith, and any and all on a collection. " +
		"On a directory object, what works depends on the property, and some of it needs an advanced query."
}

// ---- $filter ----

// compileFilter makes a test for a $filter. It reads the filter, checks it against the rules of Graph for the
// resource, and gives the test.
func compileFilter(f string, rl rules) (func(Object) bool, *Error) {
	f = strings.TrimSpace(f)
	if f == "" {
		return nil, nil
	}
	tree, err := parseFilter(f)
	if err != nil {
		return nil, SyntaxError("Invalid filter clause: %s in '%s'.", err, f)
	}
	if rl.m.HasResource(rl.resource) {
		if e := rl.check(tree, false, nil); e != nil {
			return nil, e
		}
	}
	return func(o Object) bool { return (&evalCtx{root: o}).test(tree) }, nil
}

// ---- $search ----

// tokens splits text like Entra search does: at spaces, symbols, a change from
// lower to upper case, and a change between letters and digits. The parts of a
// chunk that only symbols separate are also joined ("hello.world" gives
// "helloworld" too).
func tokens(s string) []string {
	var out []string
	for _, chunk := range strings.Fields(s) {
		var parts []string
		var cur []rune
		flush := func() {
			if len(cur) > 0 {
				parts = append(parts, strings.ToLower(string(cur)))
				cur = nil
			}
		}
		for _, r := range chunk {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				flush()
				continue
			}
			if len(cur) > 0 {
				prev := cur[len(cur)-1]
				if (unicode.IsLower(prev) && unicode.IsUpper(r)) || unicode.IsDigit(prev) != unicode.IsDigit(r) {
					flush()
				}
			}
			cur = append(cur, r)
		}
		flush()
		out = append(out, parts...)
		if len(parts) > 1 {
			out = append(out, strings.Join(parts, ""))
		}
	}
	return out
}

// searchMatches reports whether every word of the query starts a word of the text.
func searchMatches(text string, query []string) bool {
	have := tokens(text)
	for _, q := range query {
		found := false
		for _, h := range have {
			if strings.HasPrefix(h, q) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// SearchForm describes $search.
type SearchForm struct {
	Syntax           string   `json:"syntax"`
	Example          string   `json:"example"`
	Note             string   `json:"note"`
	Fields           []string `json:"fields"` // the properties that are searched by words
	NeedsConsistency bool     `json:"needsConsistency"`
}

// the properties that $search looks in by words. For any other property that $filter works on, a clause is a startsWith.
var searchWordFields = []string{"displayName", "description"}

// SearchDoc gives the form of $search.
func SearchDoc() SearchForm {
	return SearchForm{
		Syntax:  `"property:text" [AND|OR "property:text"]`,
		Example: `"displayName:acme portal" OR "mail:acme"`,
		Note: "AND and OR are in capitals, outside the quotes, and parentheses group. In displayName and description, the words can be in any order, " +
			`and each one must start a word of the text (so "acme" finds grp.app.acme.main). Any other property that $filter works on is a startsWith.`,
		Fields:           searchWordFields,
		NeedsConsistency: true,
	}
}

type (
	searchLogic struct {
		and  bool
		l, r any
	}
	searchClause struct{ prop, text string }
)

// parseSearch reads  "a:b" AND ("c:d" OR "e:f")
func parseSearch(s string) (any, error) {
	type tok struct{ kind, text string } // clause, (, ), AND, OR
	var toks []tok
	for i := 0; i < len(s); {
		switch c := s[i]; {
		case c == ' ' || c == '\t':
			i++
		case c == '(' || c == ')':
			toks = append(toks, tok{string(c), ""})
			i++
		case c == '"':
			var b strings.Builder
			j := i + 1
			for ; j < len(s) && s[j] != '"'; j++ {
				if s[j] == '\\' && j+1 < len(s) {
					j++
				}
				b.WriteByte(s[j])
			}
			if j >= len(s) {
				return nil, fmt.Errorf("a quote at %d is not closed", i)
			}
			toks = append(toks, tok{"clause", b.String()})
			i = j + 1
		case strings.HasPrefix(s[i:], "AND"):
			toks = append(toks, tok{"AND", ""})
			i += 3
		case strings.HasPrefix(s[i:], "OR"):
			toks = append(toks, tok{"OR", ""})
			i += 2
		default:
			return nil, fmt.Errorf("%q at %d: a clause is in double quotes, and AND and OR are in capitals", string(c), i)
		}
	}
	pos := 0
	var or func() (any, error)
	primary := func() (any, error) {
		if pos >= len(toks) {
			return nil, fmt.Errorf("a clause is missing")
		}
		t := toks[pos]
		pos++
		switch t.kind {
		case "(":
			e, err := or()
			if err == nil && (pos >= len(toks) || toks[pos].kind != ")") {
				err = fmt.Errorf("a ) is missing")
			}
			pos++
			return e, err
		case "clause":
			prop, text, ok := strings.Cut(t.text, ":")
			if !ok || strings.TrimSpace(prop) == "" {
				return nil, fmt.Errorf("the clause %q must be property:text", t.text)
			}
			return searchClause{strings.TrimSpace(prop), strings.TrimSpace(text)}, nil
		}
		return nil, fmt.Errorf("%s is not where a clause goes", t.kind)
	}
	and := func() (any, error) {
		l, err := primary()
		for err == nil && pos < len(toks) && toks[pos].kind == "AND" {
			pos++
			var r any
			if r, err = primary(); err == nil {
				l = searchLogic{true, l, r}
			}
		}
		return l, err
	}
	or = func() (any, error) {
		l, err := and()
		for err == nil && pos < len(toks) && toks[pos].kind == "OR" {
			pos++
			var r any
			if r, err = and(); err == nil {
				l = searchLogic{false, l, r}
			}
		}
		return l, err
	}
	e, err := or()
	if err == nil && pos < len(toks) {
		err = fmt.Errorf("%s is not where it can be", toks[pos].kind)
	}
	return e, err
}

func (rl rules) checkSearch(n any) *Error {
	switch x := n.(type) {
	case searchLogic:
		if e := rl.checkSearch(x.l); e != nil {
			return e
		}
		return rl.checkSearch(x.r)
	case searchClause:
		// any property that $filter works on can be searched
		if rl.m.HasResource(rl.resource) && !rl.m.KnowsProperty(rl.resource, x.prop) {
			return unsupportedClause(x.prop, rl.resource)
		}
	}
	return nil
}

func foldGet(o Object, name string) any {
	for k, v := range o {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return nil
}

func searchTest(n any, o Object) bool {
	switch x := n.(type) {
	case searchLogic:
		if x.and {
			return searchTest(x.l, o) && searchTest(x.r, o)
		}
		return searchTest(x.l, o) || searchTest(x.r, o)
	case searchClause:
		text := Str(foldGet(o, x.prop))
		for _, f := range searchWordFields {
			if strings.EqualFold(f, x.prop) {
				q := tokens(x.text)
				return len(q) > 0 && searchMatches(text, q)
			}
		}
		return text != "" && strings.HasPrefix(strings.ToLower(text), strings.ToLower(x.text))
	}
	return false
}

// compileSearch makes a test for a $search. A directory object needs the header ConsistencyLevel: eventual, and
// the error is the one of Graph.
func compileSearch(r *http.Request, s string, rl rules) (func(Object) bool, *Error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if !eventual(r) {
		return nil, Unsupported("Request with $search query parameter only works through MSGraph with a special request header: 'ConsistencyLevel: eventual'")
	}
	tree, err := parseSearch(s)
	if err != nil {
		return nil, BadRequest("Invalid $search clause: %s.", err)
	}
	if e := rl.checkSearch(tree); e != nil {
		return nil, e
	}
	return func(o Object) bool { return searchTest(tree, o) }, nil
}

// ---- $orderby ----

type sortKey struct {
	prop string
	desc bool
}

// parseOrderBy reads  displayName desc, createdDateTime. The errors are the ones that Graph gives.
func parseOrderBy(s string, rl rules, withFilter bool) ([]sortKey, *Error) {
	var keys []sortKey
	for _, part := range strings.Split(s, ",") {
		f := strings.Fields(part)
		if len(f) == 0 || len(f) > 2 || (len(f) == 2 && !strings.EqualFold(f[1], "asc") && !strings.EqualFold(f[1], "desc")) {
			return nil, SyntaxError("Syntax error at position %d in '%s'.", len(s), s)
		}
		k := sortKey{prop: f[0], desc: len(f) == 2 && strings.EqualFold(f[1], "desc")}
		if rl.m.HasResource(rl.resource) {
			// a property that sorts only in an advanced query is not sortable without one
			if mode := rl.m.OrderByMode(rl.resource, k.prop); mode == query.None || (mode == query.Advanced && !rl.advanced) {
				return nil, Unsupported("Unsupported sort property '%s' for '%s'.", k.prop, resourceTitle(rl.resource))
			}
			// $filter and $orderby together always need an advanced query
			if withFilter && !rl.advanced {
				return nil, Unsupported("Sorting not supported for current query.")
			}
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// order compares two values for a sort: nothing is first, and text is compared without its capitals.
func order(a, b any) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return -1
	case b == nil:
		return 1
	}
	if x, ok := asNumber(a); ok {
		if y, ok := asNumber(b); ok {
			return cmpFloat(x, y)
		}
	}
	if x, ok := a.(bool); ok {
		if y, ok := b.(bool); ok {
			switch {
			case x == y:
				return 0
			case !x:
				return -1
			}
			return 1
		}
	}
	as, bs := Str(a), Str(b)
	if ta, err := time.Parse(time.RFC3339Nano, as); err == nil {
		if tb, err := time.Parse(time.RFC3339Nano, bs); err == nil {
			return ta.Compare(tb)
		}
	}
	return strings.Compare(strings.ToLower(as), strings.ToLower(bs))
}

func sortItems(items []Object, keys []sortKey) []Object {
	out := append([]Object(nil), items...)
	sort.SliceStable(out, func(i, j int) bool {
		for _, k := range keys {
			c := order(foldGet(out[i], k.prop), foldGet(out[j], k.prop))
			if k.desc {
				c = -c
			}
			if c != 0 {
				return c < 0
			}
		}
		return false
	})
	return out
}

// ---- lists ----

// ListOptions says how one collection answers.
type ListOptions struct {
	// Extra names always go in the answer next to id, when $select is used.
	Extra []string
	// Filterable collections accept $filter and $search.
	Filterable bool
	// Resource is the kind of the items, for the rules of Graph: user, group, servicePrincipal. With one of
	// these, $filter, $search, and $orderby follow the tables of Graph for that kind. Otherwise they take any property.
	Resource string
	// NoConsistency says that the collection is not a directory object. $count needs no ConsistencyLevel header
	// for it, and no advanced query is needed. Users, groups, and service principals need it.
	NoConsistency bool
	// PageSize is the size of a page when the call has no $top. 0 uses 100.
	PageSize int
	// Default names the properties that Graph returns when the call has no $select.
	// nil returns every property.
	Default []string
	// Context is the set that the answer says in @odata.context: users, groups, directoryObjects. Empty says none.
	Context string
}

const (
	defaultPage = 100
	maxPage     = 999
)

// MaxPage is the largest size of a page, as in Graph.
const MaxPage = maxPage

// eventual says whether the call has the header ConsistencyLevel: eventual.
func eventual(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("ConsistencyLevel"), "eventual")
}

// ServeList answers a request for a collection: $filter, $search, $orderby, $count,
// $select, $top, and paging with a next link. items are in the order that the
// collection has. The items must not change while it runs (give copies).
func ServeList(w http.ResponseWriter, r *http.Request, env Env, items []Object, o ListOptions) {
	q := r.URL.Query()
	directory := !o.NoConsistency
	wantCount := strings.EqualFold(q.Get("$count"), "true")
	// An advanced query is the header ConsistencyLevel: eventual and $count=true together.
	rl := rules{m: query.Load(), resource: o.Resource, eventual: directory && eventual(r), advanced: directory && eventual(r) && wantCount}
	if e := checkTop(q.Get("$top"), directory); e != nil {
		WriteError(w, e)
		return
	}

	if o.Filterable {
		var e *Error
		if items, e = filterAndSearch(r, items, rl); e != nil {
			WriteError(w, e)
			return
		}
	}
	if ob := strings.TrimSpace(q.Get("$orderby")); ob != "" {
		keys, e := parseOrderBy(ob, rl, directory && o.Filterable && strings.TrimSpace(q.Get("$filter")) != "")
		if e != nil {
			WriteError(w, e)
			return
		}
		items = sortItems(items, keys)
	}
	// On a directory object, Graph counts only with the header, and without it leaves $count=true out, with no error.
	count := wantCount && (!directory || eventual(r))

	size := defaultPage
	if o.PageSize > 0 {
		size = o.PageSize
	}
	if t, err := strconv.Atoi(q.Get("$top")); err == nil && t > 0 {
		size = min(t, maxPage)
	}
	if forced := env.PageSize(); forced > 0 {
		size = forced
	}
	skip, _ := strconv.Atoi(q.Get("$skiptoken"))
	if s, err := strconv.Atoi(q.Get("$skip")); err == nil && skip == 0 {
		skip = s // $skip is the first page. The next link uses $skiptoken, which counts from the start.
	}
	skip = max(skip, 0)
	end := min(skip+size, len(items))
	page := []Object{}
	if skip < len(items) {
		page = items[skip:end]
	}

	sel := SplitList(q.Get("$select"))
	out := make([]Object, 0, len(page))
	for _, it := range page {
		out = append(out, selectProps(it, sel, o.Default, o.Extra, o.Resource))
	}
	body := map[string]any{"value": out}
	if o.Context != "" {
		body["@odata.context"] = contextURL(o.Context, sel, false)
	}
	if count {
		body["@odata.count"] = len(items)
	}
	if end < len(items) {
		next := cloneValues(q)
		next.Set("$skiptoken", strconv.Itoa(end))
		body["@odata.nextLink"] = fmt.Sprintf("http://%s%s?%s", r.Host, r.URL.Path, next.Encode())
	}
	WriteJSON(w, http.StatusOK, body)
}

// checkTop checks $top the way Graph does for a directory object: a whole number from 1 to 999. Anything else is an error,
// and not a page of another size.
func checkTop(top string, directory bool) *Error {
	if top == "" || !directory {
		return nil
	}
	n, err := strconv.Atoi(top)
	if err != nil {
		return SyntaxError("Invalid value '%s' for $top query option found. The $top query option requires a non-negative integer value.", top)
	}
	if n < 1 || n > maxPage {
		return Unsupported("Invalid page size specified: '%s'. Must be between 1 and %d inclusive.", top, maxPage)
	}
	return nil
}

// noRawCount is what Graph says to /$count on a directory object without the header.
func noRawCount() *Error {
	return Unsupported("Raw count (/$count) is not supported because the 'ConsistencyLevel:eventual' header is missing. Refer to https://aka.ms/graph-docs/advanced-queries for more information")
}

// ServeCount answers /$count on a collection: the number of the items that pass $filter and $search, as plain
// text. On a directory object, Graph counts only with the header ConsistencyLevel: eventual. The header takes the
// place of $count=true here: the URL segment is the count, so it makes an advanced query by itself.
func ServeCount(w http.ResponseWriter, r *http.Request, items []Object, o ListOptions) {
	directory := !o.NoConsistency
	if directory && !eventual(r) {
		WriteError(w, noRawCount())
		return
	}
	if o.Filterable {
		var e *Error
		if items, e = filterAndSearch(r, items, rules{m: query.Load(), resource: o.Resource, eventual: directory, advanced: directory}); e != nil {
			WriteError(w, e)
			return
		}
	}
	w.Header().Set("Content-Type", "text/plain")
	_, _ = fmt.Fprint(w, len(items))
}

// filterAndSearch keeps the items that pass $filter and $search.
func filterAndSearch(r *http.Request, items []Object, rl rules) ([]Object, *Error) {
	q := r.URL.Query()
	filter, e := compileFilter(q.Get("$filter"), rl)
	if e != nil {
		return nil, e
	}
	search, e := compileSearch(r, q.Get("$search"), rl)
	if e != nil {
		return nil, e
	}
	var kept []Object
	for _, it := range items {
		if (filter == nil || filter(it)) && (search == nil || search(it)) {
			kept = append(kept, it)
		}
	}
	return kept, nil
}

func cloneValues(v url.Values) url.Values {
	out := make(url.Values, len(v))
	for k, vals := range v {
		out[k] = append([]string(nil), vals...)
	}
	return out
}

// SplitList splits a comma list, like the value of $select.
func SplitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// SelectFields keeps the fields that $select names. Graph adds no id: $select=displayName gives only displayName.
// With no $select the whole object stays. The extra names are kept always, when the object has them.
func SelectFields(o Object, sel, extra []string) Object {
	return selectProps(o, sel, nil, extra, "")
}

// selectProps makes the object for an answer, as Graph does:
//   - with $select, the properties that it names, and no others (not even id)
//   - with no $select, the default properties of the resource (def); with neither, the whole object
//   - a property that the schema declares and the object has no value for is null (an empty list for a collection)
//   - a name that is not a property of the schema is dropped, with no error
//
// The extra names are kept when the object has them (the expanded manager, @odata.type). Without a schema, only the
// properties that the object has are kept.
func selectProps(o Object, sel, def, extra []string, schema string) Object {
	names := sel
	if len(names) == 0 {
		names = def
	}
	if len(names) == 0 {
		return o
	}
	out := Object{}
	for _, name := range names {
		if key, v, ok := foldLookup(o, name); ok {
			out[key] = v
		} else if typ, declared := spec.Load().PropertyType(schema, name); schema != "" && declared {
			if typ == "array" {
				out[name] = []any{}
			} else {
				out[name] = nil
			}
		}
	}
	for _, k := range extra {
		if v, ok := o[k]; ok {
			out[k] = v
		}
	}
	return out
}

// foldLookup finds a property of an object by name, without regard to capitals.
func foldLookup(o Object, name string) (string, any, bool) {
	if v, ok := o[name]; ok {
		return name, v, true
	}
	for k, v := range o {
		if strings.EqualFold(k, name) {
			return k, v, true
		}
	}
	return "", nil, false
}

// contextURL is the @odata.context of an answer: https://graph.microsoft.com/v1.0/$metadata#users(displayName,id) for a
// collection, and #users/$entity for one object. Graph writes its own address here.
func contextURL(set string, sel []string, entity bool) string {
	u := "https://graph.microsoft.com/v1.0/$metadata#" + set
	if len(sel) > 0 {
		u += "(" + strings.Join(sel, ",") + ")"
	}
	if entity {
		u += "/$entity"
	}
	return u
}

// one makes the answer for one object of a set (users, groups, ...): the properties of the call (see selectProps), and
// the @odata.context of Graph, https://graph.microsoft.com/v1.0/$metadata#users/$entity.
func one(set string, o Object, req *http.Request, def, extra []string, schema string) Object {
	sel := SplitList(req.URL.Query().Get("$select"))
	out := Object{"@odata.context": contextURL(set, sel, true)}
	for k, v := range selectProps(o, sel, def, extra, schema) {
		out[k] = v
	}
	return out
}
