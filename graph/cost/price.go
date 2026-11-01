package cost

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Price is what a request costs.
type Price struct {
	RU    int    // resource units
	Write int    // write units (1 for a request that changes data, on a path that the table does not name)
	Route string // the row of the table that gave the cost, or "METHOD (other)" for the default
}

type basePattern struct {
	method string
	parts  []string // "{id}" matches one part
	cost   BaseCost
}

func split(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func (d *Data) prepare() error {
	for _, b := range d.Identity.BaseCosts {
		d.base = append(d.base, basePattern{strings.ToUpper(b.Method), split(b.Path), b})
	}
	if d.Identity.MinimumRU < 1 {
		return fmt.Errorf("identity.minimumRu must be 1 or more")
	}
	for _, m := range []string{"GET", "POST", "PATCH", "PUT", "DELETE"} {
		if _, ok := d.defaultFor(m); !ok {
			return fmt.Errorf("identity.defaults has no row for %s", m)
		}
	}
	return nil
}

func (d *Data) defaultFor(method string) (Default, bool) {
	for _, x := range d.Identity.Defaults {
		if strings.EqualFold(x.Method, method) {
			return x, true
		}
	}
	return Default{}, false
}

func matches(pattern, parts []string) bool {
	if len(pattern) != len(parts) {
		return false
	}
	for i, p := range pattern {
		if !(strings.HasPrefix(p, "{") && strings.HasSuffix(p, "}")) && !strings.EqualFold(p, parts[i]) {
			return false
		}
	}
	return true
}

// Price gives the cost of a request to the identity service. The path is without the version
// (users/{id}, not /v1.0/users/{id}).
//
// A cost for a path that starts with me/ is also the cost of the same path under
// users/{id | userPrincipalName}/. The options of the query change the cost, and it is never
// lower than the minimum.
func (d *Data) Price(method, path string, q url.Values) Price {
	method = strings.ToUpper(method)
	parts := split(path)
	alt := parts
	if len(parts) > 2 && strings.EqualFold(parts[0], "users") {
		alt = append([]string{"me"}, parts[2:]...)
	}

	p := Price{Route: method + " (other)"}
	if def, ok := d.defaultFor(method); ok {
		p.RU, p.Write = def.RU, def.Write
	} else {
		p.RU, p.Write = d.Identity.MinimumRU, 1
	}
	// a cost that a person reported beats the table of Microsoft, and the table beats the default
	found := false
	for _, b := range d.over {
		if b.method == method && (matches(b.parts, parts) || matches(b.parts, alt)) {
			p.RU, p.Write, p.Route, found = b.cost.RU, b.cost.Write, method+" "+b.cost.Path+" (reported)", true
			break
		}
	}
	for _, b := range d.base {
		if found {
			break
		}
		if b.method == method && (matches(b.parts, parts) || matches(b.parts, alt)) {
			p.RU, p.Write, p.Route = b.cost.RU, b.cost.Write, method+" "+b.cost.Path
			break
		}
	}

	_, hasSelect := q["$select"]
	for _, m := range d.Identity.Modifiers {
		if m.RUSet != 0 {
			if m.When == "$select" && hasSelect && matches(split(m.Path), parts) {
				return d.floor(p, m.RUSet)
			}
			continue
		}
		switch m.When {
		case "$select":
			if hasSelect {
				p.RU += m.RU
			}
		case "$expand":
			if _, ok := q["$expand"]; ok {
				p.RU += m.RU
			}
		case "$top<20":
			if n, err := strconv.Atoi(q.Get("$top")); err == nil && n < 20 {
				p.RU += m.RU
			}
		}
	}
	return d.floor(p, p.RU)
}

func (d *Data) floor(p Price, ru int) Price {
	p.RU = max(ru, d.Identity.MinimumRU)
	return p
}
