package permissions

import (
	"regexp"
	"strings"
)

// The permissions that a call of Graph needs, as the page of Microsoft Learn about the call says them. A page has a table in one of three forms:
//
//	|Permission type|Least privileged permissions|Higher privileged permissions|     the table of an include file
//	|Permission type|Permissions (from least to most privileged)|                    the table inside the page, the older form
//	|Supported resource|Delegated (work or school account)|...|Application|          one row for each type of object: the permission depends on it
//
// ParseTable reads all three. It keeps the names of the permissions, and the short words that Microsoft puts after a name when it holds only
// for a case ("for online meetings", or a star for resource-specific consent). It leaves the rest of the text out.

// Need is what a call asks of one kind of permission.
type Need struct {
	// Least are the permissions that allow the call with the least privilege. Any one of them is enough. "A and B" needs both.
	Least []string `json:"least,omitempty"`
	// Higher are the permissions that allow it too, with more privilege than the call needs.
	Higher []string `json:"higher,omitempty"`
	// NotSupported is true when the page says that the call does not work with this kind of permission.
	NotSupported bool `json:"notSupported,omitempty"`
	// Conditions are the words after a permission that say when it holds: "for online meetings".
	Conditions map[string]string `json:"conditions,omitempty"`
}

// Variant is a row of a table by type of object: the call needs other permissions for a user than for a group.
type Variant struct {
	// Scenario is the heading of the table, when the page has more than one table: "Permissions for another user's direct memberships".
	Scenario string `json:"scenario,omitempty"`
	Resource string `json:"resource,omitempty"` // device, group, user, "user calendar"
	// Detail is what Microsoft says about the row besides the type: a path, or a few words.
	Detail      string `json:"detail,omitempty"`
	Delegated   Need   `json:"delegated"`
	Application Need   `json:"application"`
}

// CallPermissions are the permissions of one call, for a signed-in user (delegated, work or school account) and for an application.
// The delegated permissions of a personal Microsoft account are not kept: a tenant of dollycli has no such account.
type CallPermissions struct {
	// Table is false for a page that has no table of permissions.
	Table       bool `json:"table"`
	Delegated   Need `json:"delegated"`
	Application Need `json:"application"`
	// Variants are the rows of a table by type of object. Then Delegated and Application are empty: what the call needs depends on the object.
	Variants []Variant `json:"variants,omitempty"`
}

var (
	// a name of a permission: User.Read.All, Group-Conversation.ReadWrite.All, Application.ReadWrite.OwnedBy
	nameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.\-]*$`)
	// [text](link) in a cell
	linkRe = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	// a name with a star after it: resource-specific consent
	starRe = regexp.MustCompile(`([A-Za-z][A-Za-z0-9_.\-]*)\*`)
	// a name with words in parentheses after it: OnlineMeetingRecording.Read.All (for online meetings)
	noteRe = regexp.MustCompile(`^(.+?)\s*\(([^)]*)\)$`)
	// the line under the header of a table
	separatorRe = regexp.MustCompile(`^:?-+:?$`)
)

// cells splits a row of a table: |a|b|c| gives a, b, c.
func cells(line string) []string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") {
		return nil
	}
	parts := strings.Split(strings.Trim(line, "|"), "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// And joins the permissions of one alternative that needs all of them: Microsoft writes "AppRoleAssignment.ReadWrite.All and
// Application.Read.All". An alternative is one string, with this word between the names.
const And = " and "

// Alternatives splits an alternative into the permissions that it needs together.
func Alternatives(alt string) []string { return strings.Split(alt, And) }

// names reads the permissions of a cell: alternatives that are separated by commas, and each one is a name, or names that are joined by "and".
// After a name there can be a star (resource-specific consent) or words in parentheses: they are given in conditions, by name of the permission.
// "Not supported." gives notSupported. A part that is none of these is not kept, and it is given in dropped, so a tool can stop and say that
// the form of the table has changed.
func names(cell string) (list []string, notSupported bool, conditions map[string]string, dropped []string) {
	cell = strings.ReplaceAll(cell, "<br>", ",")
	cell = strings.ReplaceAll(cell, "<br />", ",")
	cell = linkRe.ReplaceAllString(cell, "$1")
	cell = strings.ReplaceAll(cell, "`", "")
	if low := strings.ToLower(cell); strings.Contains(low, "not supported") || strings.Contains(low, "not available") || strings.Contains(low, "not applicable") {
		return nil, true, nil, nil
	}
	conditions = map[string]string{}
	cell = starRe.ReplaceAllStringFunc(cell, func(m string) string {
		name := strings.TrimSuffix(m, "*")
		conditions[name] = "resource-specific consent"
		return name
	})
	cell = strings.ReplaceAll(cell, "*", "") // the rest of the emphasis of markdown
	for _, p := range splitOutside(cell) {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		note := ""
		if m := noteRe.FindStringSubmatch(p); m != nil {
			p, note = strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
		}
		parts, ok := strings.Split(p, And), true
		for j := range parts {
			parts[j] = strings.TrimSpace(parts[j])
			ok = ok && nameRe.MatchString(parts[j])
		}
		if !ok {
			dropped = append(dropped, p)
			continue
		}
		alt := strings.Join(parts, And)
		list = append(list, alt)
		if note != "" {
			conditions[alt] = note
		}
	}
	if len(conditions) == 0 {
		conditions = nil
	}
	return list, false, conditions, dropped
}

// splitOutside splits at the commas that are not inside parentheses.
func splitOutside(s string) []string {
	var out []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth = max(0, depth-1)
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

// readNeed reads the cell of a kind of permission into a need. A cell of the form of the older table (the first is the least) or of a table by
// type of object has the permissions from least to most privileged.
func readNeed(cell string, dropped *[]string) Need {
	list, ns, conditions, d := names(cell)
	*dropped = append(*dropped, d...)
	n := Need{NotSupported: ns, Conditions: conditions}
	if len(list) > 0 {
		n.Least, n.Higher = list[:1], list[1:]
	}
	return n
}

// resourceOf reads the first cell of a table by type of object: "[callRecording](../x.md) <br /> `path` <br /> words" gives callRecording, and
// "path · words".
func resourceOf(cell string) (resource, detail string) {
	cell = linkRe.ReplaceAllString(cell, "$1")
	parts := regexp.MustCompile(`\s*<br\s*/?>\s*`).Split(cell, -1)
	resource = strings.TrimSpace(parts[0])
	var rest []string
	for _, p := range parts[1:] {
		if p = strings.TrimSpace(strings.ReplaceAll(p, "`", "")); p != "" {
			rest = append(rest, p)
		}
	}
	return resource, strings.Join(rest, " · ")
}

// ParseTable reads the table of permissions of a page or an include file of Microsoft Learn. It gives false when there is none. dropped has the
// parts of cells that it could not read.
func ParseTable(md []byte) (out CallPermissions, dropped []string, found bool) {
	var header []string
	delegatedCol, applicationCol := -1, -1 // in a table by type of object
	for _, line := range strings.Split(string(md), "\n") {
		row := cells(line)
		if row == nil { // a table ends at the first line that is not a row
			header, delegatedCol, applicationCol = nil, -1, -1
			continue
		}
		if len(row) > 0 && separatorRe.MatchString(row[0]) {
			continue
		}
		if strings.HasPrefix(strings.ToLower(row[0]), "permission type") {
			header, delegatedCol, applicationCol = row, -1, -1
			found = true
			continue
		}
		if header == nil && delegatedCol < 0 { // maybe the header of a table by type of object: it has a column for each kind of permission
			d, a := -1, -1
			for i, c := range row {
				switch low := strings.ToLower(c); {
				case strings.HasPrefix(low, "delegated (work or school"):
					d = i
				case low == "application":
					a = i
				}
			}
			if d > 0 && a > 0 {
				delegatedCol, applicationCol = d, a
				found = true
			}
			continue
		}
		if delegatedCol > 0 { // a row of a table by type of object
			if len(row) <= max(delegatedCol, applicationCol) {
				continue
			}
			resource, detail := resourceOf(row[0])
			out.Variants = append(out.Variants, Variant{
				Resource: resource, Detail: detail,
				Delegated: readNeed(row[delegatedCol], &dropped), Application: readNeed(row[applicationCol], &dropped),
			})
			out.Table = true
			continue
		}
		if len(row) < 2 {
			continue
		}
		kind := strings.ToLower(row[0])
		var need *Need
		switch {
		case strings.HasPrefix(kind, "delegated (work or school"):
			need = &out.Delegated
		case kind == "application":
			need = &out.Application
		default: // the delegated permissions of a personal account
			continue
		}
		if len(header) >= 3 && len(row) >= 3 { // least privileged, then higher privileged
			l, ns1, c1, d1 := names(row[1])
			h, ns2, c2, d2 := names(row[2])
			need.Least, need.Higher = l, h
			need.NotSupported = ns1 && (ns2 || len(h) == 0)
			need.Conditions = mergeConditions(c1, c2)
			dropped = append(dropped, append(d1, d2...)...)
		} else { // from least to most privileged: the first one is the least
			*need = readNeed(row[1], &dropped)
		}
		out.Table = true
	}
	return out, dropped, found
}

func mergeConditions(a, b map[string]string) map[string]string {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	m := map[string]string{}
	for k, v := range a {
		m[k] = v
	}
	for k, v := range b {
		m[k] = v
	}
	return m
}
