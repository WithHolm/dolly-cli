package rule

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// node is a part of a rule that is true or false for a user. (eval is the name of the method that
// works out the answer for one user from the parsed rule. It does not run code: a rule is text
// that Parse turns into these nodes, and the only things that it can do are to compare values.)
type node interface{ eval(c *ctx) bool }

// ctx is what a node needs to work out its answer: the user, the world, and, inside -any and
// -all, the item of the list that _ stands for.
type ctx struct {
	user map[string]any
	env  Env
	item any
}

type (
	andNode struct{ l, r node }
	orNode  struct{ l, r node }
	notNode struct{ inner node }
)

func (n andNode) eval(c *ctx) bool { return n.l.eval(c) && n.r.eval(c) }
func (n orNode) eval(c *ctx) bool  { return n.l.eval(c) || n.r.eval(c) }
func (n notNode) eval(c *ctx) bool { return !n.inner.eval(c) }

// anyAllNode is property -any (...) or property -all (...) on a list of values.
type anyAllNode struct {
	spec  propSpec
	all   bool
	inner node
}

func (n anyAllNode) eval(c *ctx) bool {
	list, _ := n.spec.get(c.user).([]any)
	for _, item := range list {
		sub := *c
		sub.item = item
		matched := n.inner.eval(&sub)
		if n.all && !matched {
			return false
		}
		if !n.all && matched {
			return true
		}
	}
	return n.all // no item is a counter-example to -all, and no item is an example for -any
}

// comparison is property operator value.
type comparison struct {
	prop  string
	spec  propSpec
	op    string
	val   value
	re    *regexp.Regexp // for -match and -notMatch
	wants []string       // the values of -in and -notIn, in lowercase
}

var stringOps = map[string]bool{
	"eq": true, "ne": true, "startswith": true, "notstartswith": true, "endswith": true, "notendswith": true,
	"contains": true, "notcontains": true, "match": true, "notmatch": true, "in": true, "notin": true,
}

// newComparison checks a comparison, like Entra does when a rule is saved: the operator must fit the
// type of the property, and the value must fit the operator.
func newComparison(prop string, spec propSpec, op string, val value) (node, error) {
	c := &comparison{prop: prop, spec: spec, op: op, val: val}
	var err error
	switch spec.kind {
	case kBool:
		err = c.checkBool()
	case kDate:
		err = c.checkDate()
	default: // a string, or an item of a list
		err = c.checkString()
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (c *comparison) checkBool() error {
	if c.op != "eq" && c.op != "ne" {
		return fmt.Errorf("%s is true or false. It takes -eq and -ne, and not -%s", c.prop, c.op)
	}
	v := c.val
	isText := v.kind == vString && (strings.EqualFold(v.s, "true") || strings.EqualFold(v.s, "false"))
	if v.kind != vBool && v.kind != vNull && !isText {
		return fmt.Errorf("%s is true or false, so its value must be true or false", c.prop)
	}
	return nil
}

func (c *comparison) checkDate() error {
	if c.op != "eq" && c.op != "ne" && c.op != "ge" && c.op != "le" {
		return fmt.Errorf("%s is a date. It takes -eq, -ne, -ge, and -le, and not -%s", c.prop, c.op)
	}
	if c.val.kind == vNumber { // a date without quotes: 2020-06-10T18:13:20Z
		c.val.kind = vString
	}
	switch c.val.kind {
	case vNow, vNull:
	case vString:
		if _, err := time.Parse(time.RFC3339, c.val.s); err != nil {
			return fmt.Errorf("%s needs a date like 2020-06-10T18:13:20Z, and %q is not one", c.prop, c.val.s)
		}
	default:
		return fmt.Errorf("%s needs a date like 2020-06-10T18:13:20Z, or system.now", c.prop)
	}
	return nil
}

func (c *comparison) checkString() error {
	if !stringOps[c.op] {
		if c.op == "ge" || c.op == "le" {
			return fmt.Errorf("-%s is only for employeeHireDate, and %s is not a date", c.op, c.prop)
		}
		return fmt.Errorf("the operator -%s is not known. The operators are -eq, -ne, -startsWith, -notStartsWith, -endsWith, -notEndsWith, -contains, -notContains, -match, -notMatch, -in, and -notIn", c.op)
	}
	return c.checkStringValue()
}

func (c *comparison) checkStringValue() error {
	list := c.op == "in" || c.op == "notin"
	if list != (c.val.kind == vList) {
		if list {
			return fmt.Errorf("-%s takes a list in brackets, for example [\"a\",\"b\"]", c.op)
		}
		return fmt.Errorf("-%s takes one value. Use -in for a list", c.op)
	}
	if c.val.kind == vNull && c.op != "eq" && c.op != "ne" {
		return fmt.Errorf("null can only be used with -eq and -ne (user.mail -ne null)")
	}
	switch c.op {
	case "match", "notmatch":
		re, err := regexp.Compile("(?i)" + c.val.text())
		if err != nil {
			return fmt.Errorf("the regular expression %q is not valid: %v", c.val.text(), err)
		}
		c.re = re
	case "in", "notin":
		for _, v := range c.val.list {
			c.wants = append(c.wants, strings.ToLower(v))
		}
	}
	return nil
}

// text is the value as text: a number and a bool are text in a string comparison.
func (v value) text() string { return v.s }

func (c *comparison) eval(cx *ctx) bool {
	var got any
	if c.spec.get == nil {
		got = cx.item
	} else {
		got = c.spec.get(cx.user)
	}
	switch c.spec.kind {
	case kBool:
		return c.evalBool(got)
	case kDate:
		return c.evalDate(got, cx.env)
	}
	return c.evalString(got)
}

func (c *comparison) evalBool(got any) bool {
	have, _ := got.(bool) // null is false
	want := c.val.b || (c.val.kind == vString && strings.EqualFold(c.val.s, "true"))
	eq := c.val.kind != vNull && have == want
	if c.val.kind == vNull {
		eq = got == nil
	}
	if c.op == "ne" {
		return !eq
	}
	return eq
}

func (c *comparison) evalDate(got any, env Env) bool {
	s, _ := got.(string)
	if c.val.kind == vNull {
		return (s == "") == (c.op == "eq")
	}
	have, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return c.op == "ne" // no date: not equal to anything
	}
	var want time.Time
	if c.val.kind == vNow {
		want = env.Now().Add(c.val.offset)
	} else {
		want, _ = time.Parse(time.RFC3339, c.val.s)
	}
	switch c.op {
	case "eq":
		return have.Equal(want)
	case "ne":
		return !have.Equal(want)
	case "ge":
		return !have.Before(want)
	}
	return !have.After(want) // le
}

// evalString works out a string operator. Strings are compared without regard to case. A null value
// (or an empty one) is only equal to null. The "not" operators are true for null.
func (c *comparison) evalString(got any) bool {
	s, isString := got.(string)
	isNull := got == nil || (isString && s == "")
	lower := strings.ToLower(s)
	want := strings.ToLower(c.val.text())

	if c.val.kind == vNull { // -eq null and -ne null
		return isNull == (c.op == "eq")
	}
	switch c.op {
	case "eq":
		return !isNull && lower == want
	case "ne":
		return isNull || lower != want
	case "startswith":
		return !isNull && strings.HasPrefix(lower, want)
	case "notstartswith":
		return isNull || !strings.HasPrefix(lower, want)
	case "endswith":
		return !isNull && strings.HasSuffix(lower, want)
	case "notendswith":
		return isNull || !strings.HasSuffix(lower, want)
	case "contains":
		return !isNull && strings.Contains(lower, want)
	case "notcontains":
		return isNull || !strings.Contains(lower, want)
	case "match":
		return !isNull && c.re.MatchString(s)
	case "notmatch":
		return isNull || !c.re.MatchString(s)
	}
	in := false
	for _, w := range c.wants {
		in = in || (!isNull && lower == w)
	}
	return in == (c.op == "in") // in, notin
}
