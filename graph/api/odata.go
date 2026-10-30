package api

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/withholm/dolly-cli/graph/query"
	"github.com/withholm/dolly-cli/graph/spec"
)

// $filter, as OData and Graph have it: and, or, not, eq ne gt ge lt le in, startsWith endsWith contains,
// any and all on a collection, /$count, the values null, true, false, numbers, dates, GUIDs and 'text'.
// A filter is read into a tree (parseFilter). For a directory object, the tree is then checked against
// the rules of Graph (checkFilter): which operator works on which property, and whether it needs
// an advanced query. Only then does it run (eval). So what Graph refuses, Dolly refuses, with the
// error that Graph gives.

// ---- the words of a filter ----

type tokKind int

const (
	tEOF tokKind = iota
	tIdent
	tString
	tNumber
	tGUID
	tTime
	tPunct // ( ) , / :
)

type token struct {
	kind tokKind
	text string // for a string: the text with '' made '
	pos  int
}

var (
	guidTok = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	timeTok = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}(?:T\d{2}:\d{2}(?::\d{2}(?:\.\d+)?)?(?:Z|[+-]\d{2}:\d{2})?)?`)
	numTok  = regexp.MustCompile(`^-?\d+(?:\.\d+)?`)
)

func lex(s string) ([]token, error) {
	var out []token
	for i := 0; i < len(s); {
		c := rune(s[i])
		switch {
		case unicode.IsSpace(c):
			i++
		case strings.ContainsRune("(),/:", c):
			out = append(out, token{tPunct, string(c), i})
			i++
		case c == '\'':
			var b strings.Builder
			j := i + 1
			for ; j < len(s); j++ {
				if s[j] == '\'' {
					if j+1 < len(s) && s[j+1] == '\'' {
						b.WriteByte('\'')
						j++
						continue
					}
					break
				}
				b.WriteByte(s[j])
			}
			if j >= len(s) {
				return nil, &parseError{detail: "Unterminated string literal", pos: i}
			}
			out = append(out, token{tString, b.String(), i})
			i = j + 1
		case unicode.IsDigit(c) || (c == '-' && i+1 < len(s) && unicode.IsDigit(rune(s[i+1]))):
			rest := s[i:]
			kind, m := tNumber, numTok.FindString(rest)
			if g := guidTok.FindString(rest); g != "" {
				kind, m = tGUID, g
			} else if t := timeTok.FindString(rest); t != "" && len(t) > len(m) {
				kind, m = tTime, t
			}
			out = append(out, token{kind, m, i})
			i += len(m)
		case unicode.IsLetter(c) || c == '$' || c == '_':
			j := i
			for j < len(s) && (unicode.IsLetter(rune(s[j])) || unicode.IsDigit(rune(s[j])) || strings.ContainsRune("$_.", rune(s[j]))) {
				j++
			}
			if g := guidTok.FindString(s[i:]); g != "" { // a GUID that starts with a letter
				out = append(out, token{tGUID, g, i})
				i += len(g)
				continue
			}
			out = append(out, token{tIdent, s[i:j], i})
			i = j
		default:
			return nil, &parseError{detail: fmt.Sprintf("Unexpected character '%c'", c), pos: i}
		}
	}
	return append(out, token{kind: tEOF, pos: len(s)}), nil
}

// ---- the tree ----

type litKind int

const (
	lNull litKind = iota
	lString
	lNumber
	lBool
	lTime
)

type literal struct {
	kind litKind
	s    string
	n    float64
	b    bool
	t    time.Time
}

// An expr is a test of one item.
type expr interface{}

type (
	logicExpr struct {
		and  bool
		l, r expr
	}
	notExpr struct{ x expr }
	// cmpExpr is path op literal. op is eq ne gt ge lt le, or in (then list has the values).
	cmpExpr struct {
		path []string
		op   string
		lit  literal
		list []literal
	}
	// fnExpr is startswith, endswith, or contains: fn(path,'text').
	fnExpr struct {
		fn   string
		path []string
		text string
	}
	// lambdaExpr is path/any(v:body) or path/all(v:body). The body of any() is nil.
	lambdaExpr struct {
		path []string
		all  bool
		v    string
		body expr
	}
)

// ---- reading ----

type parser struct {
	toks []token
	i    int
	src  string
}

func (p *parser) peek() token { return p.toks[p.i] }
func (p *parser) next() token {
	t := p.toks[p.i]
	if t.kind != tEOF { // the end stays the end, so a filter that stops early is an error and not a panic
		p.i++
	}
	return t
}
func (p *parser) is(kind tokKind, text string) bool {
	t := p.peek()
	return t.kind == kind && (text == "" || strings.EqualFold(t.text, text))
}
func (p *parser) eat(kind tokKind, text string) bool {
	if p.is(kind, text) {
		p.i++
		return true
	}
	return false
}

// parseError is a filter that cannot be read: what is wrong, and where. Graph says it as
// "Invalid filter clause: <detail> at position <n> in '<filter>'."
type parseError struct {
	detail string
	pos    int
}

func (e *parseError) Error() string { return fmt.Sprintf("%s at position %d", e.detail, e.pos) }

func (p *parser) fail(format string, a ...any) error {
	return &parseError{detail: fmt.Sprintf(format, a...), pos: p.peek().pos}
}
func (p *parser) want(kind tokKind, text string) error {
	if !p.eat(kind, text) {
		return p.fail("'%s' expected", text)
	}
	return nil
}

// parseFilter reads a $filter.
func parseFilter(s string) (expr, error) {
	toks, err := lex(s)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks, src: s}
	e, err := p.or()
	if err != nil {
		return nil, err
	}
	if !p.is(tEOF, "") {
		return nil, p.fail("unexpected %q", p.peek().text)
	}
	return e, nil
}

func (p *parser) or() (expr, error) {
	l, err := p.and()
	for err == nil && p.eat(tIdent, "or") {
		var r expr
		if r, err = p.and(); err == nil {
			l = logicExpr{false, l, r}
		}
	}
	return l, err
}

func (p *parser) and() (expr, error) {
	l, err := p.unary()
	for err == nil && p.eat(tIdent, "and") {
		var r expr
		if r, err = p.unary(); err == nil {
			l = logicExpr{true, l, r}
		}
	}
	return l, err
}

func (p *parser) unary() (expr, error) {
	if p.eat(tIdent, "not") {
		x, err := p.unary()
		return notExpr{x}, err
	}
	if p.eat(tPunct, "(") {
		e, err := p.or()
		if err == nil {
			err = p.want(tPunct, ")")
		}
		return e, err
	}
	return p.primary()
}

var functions = map[string]bool{"startswith": true, "endswith": true, "contains": true}

func (p *parser) primary() (expr, error) {
	t := p.peek()
	if t.kind == tEOF {
		return nil, p.fail("Expression expected") // Graph says this for a filter that stops after and, or, or not
	}
	if t.kind != tIdent {
		return nil, p.fail("Syntax error, unexpected '%s'", t.text)
	}
	if functions[strings.ToLower(t.text)] && p.toks[p.i+1].text == "(" && p.toks[p.i+1].kind == tPunct {
		p.i += 2
		path, err := p.path()
		if err != nil {
			return nil, err
		}
		if !p.eat(tPunct, ",") {
			return nil, p.fail("')' or ',' expected")
		}
		arg := p.next()
		if arg.kind != tString {
			return nil, p.fail("the second argument of %s must be a text in quotes", t.text)
		}
		return fnExpr{strings.ToLower(t.text), path, arg.text}, p.want(tPunct, ")")
	}
	path, err := p.path()
	if err != nil {
		return nil, err
	}
	// path/any(v:body), path/all(v:body)
	if len(path) > 1 && p.peek().text == "(" && p.peek().kind == tPunct {
		kind := strings.ToLower(path[len(path)-1])
		if kind != "any" && kind != "all" {
			return nil, p.fail("%q is not a function", path[len(path)-1])
		}
		p.i++
		l := lambdaExpr{path: path[:len(path)-1], all: kind == "all"}
		if !p.eat(tPunct, ")") {
			v := p.next()
			if v.kind != tIdent {
				return nil, p.fail("a name for the item is missing")
			}
			l.v = v.text
			if err := p.want(tPunct, ":"); err != nil {
				return nil, err
			}
			if l.body, err = p.or(); err != nil {
				return nil, err
			}
			if err := p.want(tPunct, ")"); err != nil {
				return nil, err
			}
		}
		return l, nil
	}
	op := p.next()
	if op.kind != tIdent {
		return nil, p.fail("an operator is missing after %s", strings.Join(path, "/"))
	}
	switch o := strings.ToLower(op.text); o {
	case "eq", "ne", "gt", "ge", "lt", "le":
		lit, err := p.literal()
		return cmpExpr{path: path, op: o, lit: lit}, err
	case "in":
		if err := p.want(tPunct, "("); err != nil {
			return nil, err
		}
		c := cmpExpr{path: path, op: "in"}
		for {
			lit, err := p.literal()
			if err != nil {
				return nil, err
			}
			c.list = append(c.list, lit)
			if !p.eat(tPunct, ",") {
				break
			}
		}
		return c, p.want(tPunct, ")")
	default:
		return nil, p.fail("%q is not an operator", op.text)
	}
}

// path reads name/name/name. A segment can be $count.
func (p *parser) path() ([]string, error) {
	var out []string
	for {
		t := p.next()
		if t.kind != tIdent {
			return nil, p.fail("a property name is missing")
		}
		out = append(out, t.text)
		if !(p.is(tPunct, "/") && p.toks[p.i+1].kind == tIdent) {
			return out, nil
		}
		p.i++
	}
}

func (p *parser) literal() (literal, error) {
	t := p.next()
	switch t.kind {
	case tString, tGUID:
		return literal{kind: lString, s: t.text}, nil
	case tNumber:
		n, err := strconv.ParseFloat(t.text, 64)
		return literal{kind: lNumber, n: n}, err
	case tTime:
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02"} {
			if tm, err := time.Parse(layout, t.text); err == nil {
				return literal{kind: lTime, t: tm}, nil
			}
		}
		return literal{}, p.fail("%q is not a date", t.text)
	case tIdent:
		switch strings.ToLower(t.text) {
		case "null":
			return literal{kind: lNull}, nil
		case "true":
			return literal{kind: lBool, b: true}, nil
		case "false":
			return literal{kind: lBool}, nil
		}
	}
	return literal{}, p.fail("a value is missing, or %q is not one", t.text)
}

// ---- the rules of Graph ----

// rules says what Graph accepts for one resource.
type rules struct {
	m        *query.Matrix
	resource string // user, group, servicePrincipal
	eventual bool   // the call has the header ConsistencyLevel: eventual
	advanced bool   // the call has the header, and $count=true: an advanced query
}

// resourceTitle is the name of the resource in the errors of Graph: User, Group, ServicePrincipal.
func resourceTitle(r string) string {
	if r == "" {
		return r
	}
	return strings.ToUpper(r[:1]) + r[1:]
}

func unsupportedClause(property, resource string) *Error {
	return Unsupported("Unsupported or invalid query filter clause specified for property '%s' of resource '%s'.", property, resourceTitle(resource))
}

// needsHeader is what Graph says to a call that uses an operator that needs ConsistencyLevel: eventual (not, endsWith).
func needsHeader(op string) *Error {
	return Unsupported("Operator '%s' is not supported because the 'ConsistencyLevel:eventual' header is missing. Refer to https://aka.ms/graph-docs/advanced-queries for more information", op)
}

// the properties that endsWith works on
var endsWithProps = map[string]bool{"mail": true, "othermails": true, "userprincipalname": true, "proxyaddresses/any:": true}

// invalid is the error for a property that the tables of Graph do not list for the operator. Graph tells apart a property that the
// resource does not declare at all, and a property that it declares and does not filter on.
func (r rules) invalid(property string) *Error {
	name, _, _ := strings.Cut(property, "/")
	if !r.m.KnowsProperty(r.resource, property) && !spec.Load().Declared(r.resource, name) {
		return Unsupported("Property '%s' does not exist as a declared property or extension property.", name)
	}
	return unsupportedClause(property, r.resource)
}

// singular makes "assignedLicenses" the name of the type of its items: "AssignedLicense".
func singular(collection string) string {
	s := strings.TrimSuffix(collection, "s")
	if strings.HasSuffix(collection, "ies") {
		s = strings.TrimSuffix(collection, "ies") + "y"
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// leaf is one use of an operator on a property. The rules are the tables of Microsoft, and these answers of the real Graph:
//   - ne needs an advanced query (the header and $count=true), and says "Filter operator 'NotEqualsMatch' is not supported."
//   - not and endsWith need only the header, and say that it is missing
//   - a property that needs an advanced query, without one, is "Unsupported or invalid query filter clause"
//   - eq on a date works where the table has ge/le
func (r rules) leaf(property, op string, negated bool, lit *literal) *Error {
	var col string
	switch op {
	case "eq", "ne", "in":
		col = query.OpEq
		if lit != nil && lit.kind == lNull {
			col = query.OpEqNull
		}
	case "startswith":
		col = query.OpStartsWith
	case "ge", "gt", "le", "lt":
		col = query.OpGeLe
	case "endswith":
		if r.resource != "user" || !endsWithProps[strings.ToLower(property)] {
			return r.invalid(property)
		}
		if !r.eventual {
			return needsHeader("endsWith")
		}
		return nil
	case "count":
		col = query.OpCount0
		if lit != nil && lit.kind == lNumber && lit.n == 1 {
			col = query.OpCount1
		} else if lit == nil || lit.kind != lNumber || lit.n != 0 {
			return unsupportedClause(property, r.resource)
		}
	default: // contains
		return Unsupported("Operator: 'Contains' is not supported.")
	}
	mode := r.m.FilterMode(r.resource, property, col)
	if mode == query.None && col == query.OpEq {
		mode = r.m.FilterMode(r.resource, property, query.OpGeLe) // a date: eq works where ge and le do
	}
	if mode == query.None {
		// The tables of Microsoft are not complete: Graph answers a filter on a property that they do not list (checked with
		// showInAddressList). So a property that the resource declares and the tables do not list is allowed. A property that
		// the tables list, with no cell for this operator, is refused, and so is one that the resource does not declare.
		name, _, _ := strings.Cut(property, "/")
		if r.m.KnowsProperty(r.resource, property) || !spec.Load().Declared(r.resource, name) {
			return r.invalid(property)
		}
		mode = query.DefaultAdvanced
	}
	if op == "count" && !r.advanced {
		coll, _, _ := strings.Cut(property, "/")
		return BadRequest("Expect simple name=value query, but observe property '%s' of complex type '%s'.", coll, singular(coll))
	}
	if op == "ne" && !r.advanced {
		return Unsupported("Filter operator 'NotEqualsMatch' is not supported.")
	}
	if negated && op != "ne" && !r.eventual {
		return needsHeader("not")
	}
	if (mode == query.Advanced && !r.advanced) || (mode == query.DefaultOnly && r.advanced) {
		return unsupportedClause(property, r.resource)
	}
	return nil
}

// check walks the tree and checks every use of an operator.
func (r rules) check(e expr, negated bool, lambda *lambdaScope) *Error {
	switch x := e.(type) {
	case logicExpr:
		if err := r.check(x.l, negated, lambda); err != nil {
			return err
		}
		return r.check(x.r, negated, lambda)
	case notExpr:
		return r.check(x.x, !negated, lambda)
	case cmpExpr:
		key, op := lambda.key(x.path), x.op
		if len(x.path) > 0 && x.path[len(x.path)-1] == "$count" {
			op = "count"
		}
		var lit *literal
		if x.op != "in" {
			lit = &x.lit
		}
		return r.leaf(key, op, negated, lit)
	case fnExpr:
		return r.leaf(lambda.key(x.path), x.fn, negated, nil)
	case lambdaExpr:
		scope := &lambdaScope{coll: lambda.key(x.path), v: x.v}
		if x.body == nil {
			// any() with no body: is the collection empty? That is /$count, which needs an advanced query
			return r.leaf(scope.coll+"/$count", "count", negated, &literal{kind: lNumber})
		}
		return r.check(x.body, negated, scope)
	}
	return nil
}

// A lambdaScope says what the name of the item in any(v:...) means, to write the key of the tables:
// "assignedLicenses/any:skuId".
type lambdaScope struct{ coll, v string }

func (s *lambdaScope) key(path []string) string {
	if s == nil || len(path) == 0 || path[0] != s.v {
		return strings.Join(path, "/")
	}
	return s.coll + "/any:" + strings.Join(path[1:], "/")
}

// ---- running ----

type evalCtx struct {
	root  Object
	vars  map[string]any
	exact bool // a text is compared with its capitals (the tags of a service principal)
}

func (c *evalCtx) resolve(path []string) any {
	var cur any = c.root
	rest := path
	if v, ok := c.vars[path[0]]; ok {
		cur, rest = v, path[1:]
	}
	for _, seg := range rest {
		m, ok := asMap(cur)
		if !ok {
			return nil
		}
		cur = nil
		for k, v := range m { // the names of properties are not case sensitive
			if strings.EqualFold(k, seg) {
				cur = v
				break
			}
		}
	}
	return cur
}

func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any) // Object is the same type
	return m, ok
}

func elems(v any) []any {
	switch l := v.(type) {
	case []any:
		return l
	case []string:
		out := make([]any, len(l))
		for i, s := range l {
			out[i] = s
		}
		return out
	case []map[string]any: // []Object is the same type
		out := make([]any, len(l))
		for i, s := range l {
			out[i] = s
		}
		return out
	}
	return nil
}

// test says whether the item passes the tree. It reads a tree that parseFilter made: it never runs text as code.
func (c *evalCtx) test(e expr) bool {
	switch x := e.(type) {
	case logicExpr:
		if x.and {
			return c.test(x.l) && c.test(x.r)
		}
		return c.test(x.l) || c.test(x.r)
	case notExpr:
		return !c.test(x.x)
	case fnExpr:
		s, ok := c.resolve(x.path).(string)
		if !ok {
			return false
		}
		a, b := s, x.text
		if !c.exact {
			a, b = strings.ToLower(a), strings.ToLower(b)
		}
		switch x.fn {
		case "startswith":
			return strings.HasPrefix(a, b)
		case "endswith":
			return strings.HasSuffix(a, b)
		}
		return strings.Contains(a, b)
	case lambdaExpr:
		items := elems(c.resolve(x.path))
		if x.body == nil {
			return len(items) > 0
		}
		inner := &evalCtx{root: c.root, vars: map[string]any{}, exact: c.exact || strings.EqualFold(x.path[len(x.path)-1], "tags")}
		for k, v := range c.vars {
			inner.vars[k] = v
		}
		for _, it := range items {
			inner.vars[x.v] = it
			if inner.test(x.body) != x.all {
				return !x.all
			}
		}
		return x.all
	case cmpExpr:
		if x.path[len(x.path)-1] == "$count" {
			n := float64(len(elems(c.resolve(x.path[:len(x.path)-1]))))
			return compare(n, x.op, x.lit, c.exact)
		}
		v := c.resolve(x.path)
		if x.op == "in" {
			for _, l := range x.list {
				if compare(v, "eq", l, c.exact) {
					return true
				}
			}
			return false
		}
		return compare(v, x.op, x.lit, c.exact)
	}
	return false
}

func asNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

// compare is value op literal. A property that is missing is null: it is equal to null and to nothing else.
func compare(v any, op string, l literal, exact bool) bool {
	if v == nil {
		switch {
		case l.kind == lNull:
			return op == "eq"
		case op == "ne":
			return true
		}
		return false
	}
	var c int // the order of v and the literal
	switch l.kind {
	case lNull:
		return op == "ne"
	case lBool:
		b, ok := v.(bool)
		if !ok {
			return op == "ne"
		}
		if b == l.b {
			c = 0
		} else {
			c = 1
		}
	case lNumber:
		n, ok := asNumber(v)
		if !ok {
			return op == "ne"
		}
		c = cmpFloat(n, l.n)
	case lTime:
		s, ok := v.(string)
		t, err := time.Parse(time.RFC3339Nano, s)
		if !ok || err != nil {
			return op == "ne"
		}
		c = t.Compare(l.t)
	default:
		s, ok := v.(string)
		if !ok {
			return op == "ne"
		}
		a, b := s, l.s
		if !exact {
			a, b = strings.ToLower(a), strings.ToLower(b)
		}
		c = strings.Compare(a, b)
	}
	switch op {
	case "eq":
		return c == 0
	case "ne":
		return c != 0
	case "gt":
		return c > 0
	case "ge":
		return c >= 0
	case "lt":
		return c < 0
	case "le":
		return c <= 0
	}
	return false
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
