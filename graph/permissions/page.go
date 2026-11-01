package permissions

import (
	"regexp"
	"strings"
)

// A page of Learn can have more than one table of permissions, each under a heading: "Permissions for the signed-in user's direct memberships" and
// "Permissions for another user's direct memberships". ParsePage reads all of them. One table is the permissions of the call. More than one are
// kept as scenarios (Variant with a Scenario), and Resolve picks the one that a path fits, when it can.

// includeRe finds the include file with a table of permissions: [!INCLUDE [permissions-table](../includes/permissions/x-permissions.md)]
var includeRe = regexp.MustCompile(`\[!INCLUDE \[[^\]]*\]\((\.\./includes/permissions/[^)]+\.md)\)\]`)

// Include reads an include file. rel is the link in the page: ../includes/permissions/x-permissions.md. It gives missing when the file is not there.
type Include func(rel string) (body []byte, missing bool, err error)

// section gives the text of "## Permissions" up to the next heading of that level. A page that has no such section is read whole.
func section(md string) string {
	lines := strings.Split(md, "\n")
	start := -1
	for i, l := range lines {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(l)), "## permissions") {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return md
	}
	end := len(lines)
	for i := start; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}

type chunk struct{ heading, text string }

// chunks splits a text at its headings (### and smaller). The text before the first heading has an empty heading.
func chunks(text string) []chunk {
	out := []chunk{{}}
	for _, l := range strings.Split(text, "\n") {
		if t := strings.TrimSpace(l); strings.HasPrefix(t, "###") {
			out = append(out, chunk{heading: strings.TrimSpace(strings.ReplaceAll(strings.TrimLeft(t, "#"), "*", ""))})
			continue
		}
		out[len(out)-1].text += l + "\n"
	}
	return out
}

// ParsePage reads the permissions of a page of Learn: the tables in the page, and in the include files that it links. dropped are the parts of
// cells that could not be read, and notFound the include files that are missing.
func ParsePage(md []byte, include Include) (out CallPermissions, dropped, notFound []string, err error) {
	type table struct {
		heading string
		perms   CallPermissions
	}
	var tables []table
	for _, c := range chunks(section(string(md))) {
		if perms, d, ok := ParseTable([]byte(c.text)); ok {
			dropped = append(dropped, d...)
			tables = append(tables, table{c.heading, perms})
		}
		for _, m := range includeRe.FindAllStringSubmatch(c.text, -1) {
			body, missing, e := include(m[1])
			switch {
			case e != nil:
				return out, dropped, notFound, e
			case missing:
				notFound = append(notFound, m[1])
				continue
			}
			if perms, d, ok := ParseTable(body); ok {
				dropped = append(dropped, d...)
				tables = append(tables, table{c.heading, perms})
			}
		}
	}
	switch len(tables) {
	case 0:
		return out, dropped, notFound, nil
	case 1:
		return tables[0].perms, dropped, notFound, nil
	}
	out.Table = true
	for i, t := range tables {
		scenario := t.heading
		if scenario == "" {
			scenario = "table " + string(rune('1'+i)) // the page has more than one table and no heading for this one
		}
		if len(t.perms.Variants) > 0 {
			for _, v := range t.perms.Variants {
				v.Scenario = scenario
				out.Variants = append(out.Variants, v)
			}
			continue
		}
		out.Variants = append(out.Variants, Variant{Scenario: scenario, Delegated: t.perms.Delegated, Application: t.perms.Application})
	}
	return out, dropped, notFound, nil
}

var (
	meRe    = regexp.MustCompile(`(?i)signed-in|\bself\b|\bme\b`)
	otherRe = regexp.MustCompile(`(?i)another|other users|other user|specific user|a user\b`)
)

// Resolve gives the permissions of a call by its path ("/v1.0/me/memberOf"). When the page has one table, those are the permissions. When it has
// scenarios, the path can decide between two: a /me path takes the scenario for the signed-in user, and another path the one for another user.
// The bool is false when the page depends on something else (a type of object, a channel or a chat): then Variants has the answers.
func (c CallPermissions) Resolve(path string) (delegated, application Need, ok bool) {
	if len(c.Variants) == 0 {
		return c.Delegated, c.Application, true
	}
	me := path == "/me" || strings.HasPrefix(path, "/me/") || strings.HasPrefix(path, "/v1.0/me/") || path == "/v1.0/me"
	var picked []Variant
	for _, v := range c.Variants {
		if v.Resource != "" || v.Scenario == "" {
			return Need{}, Need{}, false // rows by type of object: the object decides
		}
		if me && meRe.MatchString(v.Scenario) || !me && otherRe.MatchString(v.Scenario) && !meRe.MatchString(v.Scenario) {
			picked = append(picked, v)
		}
	}
	if len(picked) != 1 {
		return Need{}, Need{}, false
	}
	return picked[0].Delegated, picked[0].Application, true
}
