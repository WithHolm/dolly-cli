// Package rule reads and works out the membership rules of dynamic groups, in the rule language
// of Microsoft Entra ID (Learn: "Manage rules for dynamic membership groups"). It does what Entra
// does for a rule on users: it says that a rule is not valid, and it says which users a rule takes.
//
// It knows the whole language for users except two things that need more data than dolly has
// (the properties memberOf and assignedPlans), and the rules for devices. A rule that uses one of
// these is refused with a message that says so.
package rule

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// MaxLength is the longest rule that Entra takes.
const MaxLength = 3072

// Rule is a rule that has been read and checked.
type Rule struct {
	root          node
	directReports string // the manager ID of a "Direct Reports for" rule
}

// Env is what a rule needs to know about the world, besides the user.
type Env struct {
	// Now gives the time for system.now. nil uses the clock.
	Now func() time.Time
	// Manager gives the object ID of the manager of a user (lowercase), or "".
	Manager func(userID string) string
}

var directReportsForm = regexp.MustCompile(`(?is)^\s*direct\s+reports\s+for\s+"([^"]+)"\s*$`)

// Parse reads a rule. The error says what is wrong, in a way that a person can act on.
func Parse(text string) (*Rule, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("the membership rule is empty")
	}
	if len(text) > MaxLength {
		return nil, fmt.Errorf("the membership rule has %d characters, and the most is %d", len(text), MaxLength)
	}
	if m := directReportsForm.FindStringSubmatch(text); m != nil {
		return &Rule{directReports: strings.ToLower(m[1])}, nil
	}
	toks, err := lex(text)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	root, err := p.parseOr(false)
	if err != nil {
		return nil, err
	}
	if p.pos < len(p.toks) {
		return nil, fmt.Errorf("the rule has something that dolly does not understand after the expression: %q", p.toks[p.pos].text)
	}
	return &Rule{root: root}, nil
}

// Match says whether the rule takes the user. user is the user as Graph writes it.
func (r *Rule) Match(user map[string]any, env Env) bool {
	if r.directReports != "" {
		id, _ := user["id"].(string)
		id = strings.ToLower(id)
		// the manager is in the group too (Learn: "Create a rule for direct reports")
		return id == r.directReports || (env.Manager != nil && env.Manager(id) == r.directReports)
	}
	c := &ctx{user: user, env: env}
	if env.Now == nil {
		c.env.Now = func() time.Time { return time.Now().UTC() }
	}
	return r.root.eval(c)
}

// ---- tokens ----

type tokKind int

const (
	tWord tokKind = iota
	tString
	tLParen
	tRParen
	tLBrack
	tRBrack
	tComma
)

type token struct {
	kind tokKind
	text string
}

func isDash(r rune) bool { return r == '-' || r == '–' || r == '—' }

// lexString reads a value in quotes, from the character after the opening quote. It gives the value,
// the place after the closing quote, and whether there was a closing quote.
func lexString(rs []rune, i int) (string, int, bool) {
	var b strings.Builder
	for ; i < len(rs); i++ {
		switch {
		case rs[i] == '`' && i+1 < len(rs): // a backtick makes the next character part of the value
			i++
			b.WriteRune(rs[i])
		case rs[i] == '"':
			return b.String(), i + 1, true
		default:
			b.WriteRune(rs[i])
		}
	}
	return "", i, false
}

// lex cuts the rule into tokens. The Learn page writes operators with a hyphen, and some of its
// examples with an en dash, so both are the same.
func lex(s string) ([]token, error) {
	var toks []token
	rs := []rune(s)
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			i++
		case r == '(':
			toks, i = append(toks, token{tLParen, "("}), i+1
		case r == ')':
			toks, i = append(toks, token{tRParen, ")"}), i+1
		case r == '[':
			toks, i = append(toks, token{tLBrack, "["}), i+1
		case r == ']':
			toks, i = append(toks, token{tRBrack, "]"}), i+1
		case r == ',':
			toks, i = append(toks, token{tComma, ","}), i+1
		case r == '"':
			text, next, ok := lexString(rs, i+1)
			if !ok {
				return nil, fmt.Errorf("a value in quotes is not closed: a \" is missing")
			}
			toks, i = append(toks, token{tString, text}), next
		default:
			start := i
			for i < len(rs) && !strings.ContainsRune(" \t\n\r()[],\"", rs[i]) {
				i++
			}
			word := string(rs[start:i])
			if isDash([]rune(word)[0]) {
				word = "-" + string([]rune(word)[1:])
			}
			toks = append(toks, token{tWord, word})
		}
	}
	return toks, nil
}

// ---- the parser ----

type parser struct {
	toks []token
	pos  int
}

func (p *parser) peek() (token, bool) {
	if p.pos >= len(p.toks) {
		return token{}, false
	}
	return p.toks[p.pos], true
}

// isKeyword says whether the next token is this operator word (with or without the hyphen).
func (p *parser) isKeyword(names ...string) bool {
	t, ok := p.peek()
	if !ok || t.kind != tWord {
		return false
	}
	w := strings.ToLower(strings.TrimPrefix(t.text, "-"))
	for _, n := range names {
		if w == n {
			return true
		}
	}
	return false
}

func (p *parser) next() token {
	t := p.toks[p.pos]
	p.pos++
	return t
}

func (p *parser) expect(k tokKind, what string) error {
	if t, ok := p.peek(); !ok || t.kind != k {
		return fmt.Errorf("the rule needs %s here", what)
	}
	p.pos++
	return nil
}

// parseOr is the lowest level: -or. inItem is true inside the brackets of -any and -all.
func (p *parser) parseOr(inItem bool) (node, error) {
	left, err := p.parseAnd(inItem)
	if err != nil {
		return nil, err
	}
	for p.isKeyword("or") {
		p.pos++
		right, err := p.parseAnd(inItem)
		if err != nil {
			return nil, err
		}
		left = orNode{left, right}
	}
	return left, nil
}

func (p *parser) parseAnd(inItem bool) (node, error) {
	left, err := p.parseNot(inItem)
	if err != nil {
		return nil, err
	}
	for p.isKeyword("and") {
		p.pos++
		right, err := p.parseNot(inItem)
		if err != nil {
			return nil, err
		}
		left = andNode{left, right}
	}
	return left, nil
}

func (p *parser) parseNot(inItem bool) (node, error) {
	if p.isKeyword("not") {
		p.pos++
		inner, err := p.parseNot(inItem)
		if err != nil {
			return nil, err
		}
		return notNode{inner}, nil
	}
	return p.parsePrimary(inItem)
}

func (p *parser) parsePrimary(inItem bool) (node, error) {
	t, ok := p.peek()
	if !ok {
		return nil, fmt.Errorf("the rule ends where an expression is needed")
	}
	if t.kind == tLParen {
		p.pos++
		inner, err := p.parseOr(inItem)
		if err != nil {
			return nil, err
		}
		if err := p.expect(tRParen, "a closing parenthesis )"); err != nil {
			return nil, err
		}
		return inner, nil
	}
	return p.parseComparison(inItem)
}

// parseComparison reads property operator value, or property -any (...) and property -all (...).
func (p *parser) parseComparison(inItem bool) (node, error) {
	t := p.next()
	if t.kind != tWord {
		return nil, fmt.Errorf("the rule needs a property like user.department here, and has %q", t.text)
	}
	var spec propSpec
	propName := t.text
	switch {
	case inItem && t.text == "_":
		spec = propSpec{kString, nil} // the item of the list
	case strings.HasPrefix(strings.ToLower(t.text), "user."):
		var err error
		if spec, err = lookup(t.text[len("user."):]); err != nil {
			return nil, err
		}
	case strings.HasPrefix(strings.ToLower(t.text), "device."):
		return nil, fmt.Errorf("dolly works out rules for users only, and %s is a device property", t.text)
	default:
		return nil, fmt.Errorf("the property %q is not valid: a rule for users uses user.<property>, like user.department", t.text)
	}

	opTok, ok := p.peek()
	if !ok || opTok.kind != tWord {
		return nil, fmt.Errorf("the property %s needs an operator, like -eq", propName)
	}
	p.pos++
	op := strings.ToLower(strings.TrimPrefix(opTok.text, "-"))

	if op == "any" || op == "all" {
		return p.parseAnyAll(propName, spec, op == "all", inItem)
	}
	if spec.kind == kCollection {
		return nil, fmt.Errorf("user.%s is a list of values. Use -any or -all with it, for example (user.%s -any (_ -startsWith \"x\"))", strings.TrimPrefix(propName, "user."), strings.TrimPrefix(propName, "user."))
	}
	val, err := p.parseValue()
	if err != nil {
		return nil, fmt.Errorf("after %s %s: %w", propName, opTok.text, err)
	}
	cmp, err := newComparison(propName, spec, op, val)
	if err != nil {
		return nil, err
	}
	return cmp, nil
}

func (p *parser) parseAnyAll(propName string, spec propSpec, all, inItem bool) (node, error) {
	if inItem || spec.kind != kCollection {
		return nil, fmt.Errorf("-any and -all are for a list of values (user.otherMails, user.proxyAddresses), and %s is not one", propName)
	}
	if err := p.expect(tLParen, "an opening parenthesis ( after -any or -all"); err != nil {
		return nil, err
	}
	inner, err := p.parseOr(true)
	if err != nil {
		return nil, err
	}
	if err := p.expect(tRParen, "a closing parenthesis ) at the end of -any or -all"); err != nil {
		return nil, err
	}
	return anyAllNode{spec: spec, all: all, inner: inner}, nil
}

// value is the right side of a comparison.
type value struct {
	kind valueKind
	s    string
	b    bool
	list []string
	// for system.now: an offset
	now    bool
	offset time.Duration
}

type valueKind int

const (
	vString valueKind = iota
	vNull
	vBool
	vList
	vNumber
	vNow
)

var durationForm = regexp.MustCompile(`(?i)^p(\d+)([dwh])$`)

func (p *parser) parseValue() (value, error) {
	t, ok := p.peek()
	if !ok {
		return value{}, fmt.Errorf("the rule ends where a value is needed")
	}
	p.pos++
	switch t.kind {
	case tString:
		return value{kind: vString, s: t.text}, nil
	case tLBrack:
		return p.parseList()
	case tWord:
		switch lower := strings.ToLower(t.text); {
		case lower == "null" || lower == "$null":
			return value{kind: vNull}, nil
		case lower == "true" || lower == "false":
			return value{kind: vBool, b: lower == "true", s: lower}, nil
		case lower == "system.now":
			return p.parseNow()
		default:
			return value{kind: vNumber, s: t.text}, nil
		}
	}
	return value{}, fmt.Errorf("a value is needed, and the rule has %q", t.text)
}

func (p *parser) parseList() (value, error) {
	v := value{kind: vList}
	for {
		t, ok := p.peek()
		if !ok {
			return value{}, fmt.Errorf("a list in [ ] is not closed: a ] is missing")
		}
		if t.kind == tRBrack {
			p.pos++
			return v, nil
		}
		if len(v.list) > 0 {
			if t.kind != tComma {
				return value{}, fmt.Errorf("the values in a list are separated with commas")
			}
			p.pos++
			t, ok = p.peek()
			if !ok {
				return value{}, fmt.Errorf("a list in [ ] is not closed: a ] is missing")
			}
		}
		if t.kind != tString && t.kind != tWord {
			return value{}, fmt.Errorf("a list has values like \"a\", \"b\"")
		}
		p.pos++
		v.list = append(v.list, t.text)
	}
}

// parseNow reads system.now, with an optional -plus or -minus and a duration like p1d.
func (p *parser) parseNow() (value, error) {
	v := value{kind: vNow, now: true}
	if p.isKeyword("plus", "minus") {
		sign := time.Duration(1)
		if p.isKeyword("minus") {
			sign = -1
		}
		p.pos++
		t, ok := p.peek()
		m := durationForm.FindStringSubmatch(t.text)
		if !ok || t.kind != tWord || m == nil {
			return value{}, fmt.Errorf("system.now -plus and -minus need a duration like p1d, p2w, or p3h")
		}
		p.pos++
		var n int
		_, _ = fmt.Sscanf(m[1], "%d", &n)
		unit := map[string]time.Duration{"d": 24 * time.Hour, "w": 7 * 24 * time.Hour, "h": time.Hour}[strings.ToLower(m[2])]
		v.offset = sign * time.Duration(n) * unit
	}
	return v, nil
}
