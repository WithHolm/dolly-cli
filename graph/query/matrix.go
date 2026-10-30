// Package query has the rules that Microsoft Graph applies to $filter and $orderby on directory objects
// (users, groups, service principals, and others): for each property, which operators work, and whether
// they work by default or only in an advanced query (the header ConsistencyLevel: eventual and $count=true).
//
// The rules are Microsoft's own tables in the article "Advanced query capabilities on Microsoft Entra ID
// objects". 'make update-query' downloads them (raw/) and Parse reads them into matrix.json, which this
// package embeds. Dolly applies the same rules, so a call that Graph refuses, Dolly refuses with the same
// error, and a call that Graph answers, Dolly answers.
package query

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Mode says when an operator works on a property.
type Mode string

const (
	Default         Mode = "default"          // works, and needs no advanced query
	DefaultAdvanced Mode = "default+advanced" // works with and without an advanced query
	DefaultOnly     Mode = "defaultOnly"      // works only without an advanced query
	Advanced        Mode = "advanced"         // works only in an advanced query
	None            Mode = "none"             // Graph does not support it
)

// The operators that the tables have a column for.
const (
	OpEq         = "eq"
	OpStartsWith = "startsWith"
	OpGeLe       = "ge/le"
	OpEqNull     = "eqNull"
	OpCount0     = "count0" // /$count eq 0
	OpCount1     = "count1" // /$count eq 1
)

// Matrix is the rules for the directory objects.
type Matrix struct {
	// Commit is the commit of Microsoft's documents that the rules were read from.
	Commit string `json:"commit"`
	// Filter is, for each resource (user, group, ...), each property, and each operator, the mode.
	// A property that is a collection is "proxyAddresses/any:" (a list of text) or "assignedLicenses/any:skuId".
	Filter map[string]map[string]map[string]Mode `json:"filter"`
	// OrderBy is, for each resource and property, the mode of $orderby.
	OrderBy map[string]map[string]Mode `json:"orderby"`

	lower sync.Once
	fold  map[string]map[string]string // resource -> lower case property -> property
}

//go:embed matrix.json
var matrixJSON []byte

var loaded = sync.OnceValues(func() (*Matrix, error) {
	var m Matrix
	if err := json.Unmarshal(matrixJSON, &m); err != nil {
		return nil, fmt.Errorf("matrix.json: %w", err)
	}
	return &m, nil
})

// Load gives the rules that are built into the program.
func Load() *Matrix {
	m, err := loaded()
	if err != nil {
		panic(err) // a test reads the file, so this cannot reach a release
	}
	return m
}

// ---- asking ----

// HasResource says whether the rules cover a resource (user, group, servicePrincipal, ...). A resource that
// they do not cover has no rules of this kind.
func (m *Matrix) HasResource(resource string) bool { return len(m.Filter[resource]) > 0 }

func (m *Matrix) folded() map[string]map[string]string {
	m.lower.Do(func() {
		m.fold = map[string]map[string]string{}
		for res, props := range m.Filter {
			m.fold[res] = map[string]string{}
			for p := range props {
				m.fold[res][strings.ToLower(p)] = p
			}
		}
		for res, props := range m.OrderBy {
			if m.fold["orderby:"+res] == nil {
				m.fold["orderby:"+res] = map[string]string{}
			}
			for p := range props {
				m.fold["orderby:"+res][strings.ToLower(p)] = p
			}
		}
	})
	return m.fold
}

// FilterMode gives the mode of an operator on a property. A property that the tables do not list, and an
// operator that its row has no cell for, are None: Graph does not support them.
func (m *Matrix) FilterMode(resource, property, op string) Mode {
	key, ok := m.folded()[resource][strings.ToLower(property)]
	if !ok {
		return None
	}
	if mode, ok := m.Filter[resource][key][op]; ok {
		return mode
	}
	return None
}

// KnowsProperty says whether the tables list the property for $filter.
func (m *Matrix) KnowsProperty(resource, property string) bool {
	_, ok := m.folded()[resource][strings.ToLower(property)]
	return ok
}

// OrderByMode gives the mode of $orderby on a property. A property that is not listed is None.
func (m *Matrix) OrderByMode(resource, property string) Mode {
	key, ok := m.folded()["orderby:"+resource][strings.ToLower(property)]
	if !ok {
		return None
	}
	return m.OrderBy[resource][key]
}

// Row is one property with its operators, for the page.
type Row struct {
	Property  string          `json:"property"`
	Operators map[string]Mode `json:"operators"`
	OrderBy   Mode            `json:"orderby,omitempty"`
}

// Rows lists the properties of a resource, by name, with every operator that has a cell and its sorting.
func (m *Matrix) Rows(resource string) []Row {
	var out []Row
	for p, ops := range m.Filter[resource] {
		out = append(out, Row{Property: p, Operators: ops, OrderBy: m.OrderByMode(resource, p)})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Property) < strings.ToLower(out[j].Property) })
	return out
}

// ---- reading Microsoft's tables ----

var (
	resourceNames = map[string]string{
		"Administrative unit":    "administrativeUnit",
		"Application":            "application",
		"Contract":               "contract",
		"Device":                 "device",
		"Directory role":         "directoryRole",
		"Group":                  "group",
		"Organizational contact": "orgContact",
		"Service principal":      "servicePrincipal",
		"User":                   "user",
	}
	columnOps = map[string]string{
		"eq": OpEq, "startsWith": OpStartsWith, "ge/le": OpGeLe, "eq Null": OpEqNull,
		"eq Count 0": OpCount0, "eq Count 1": OpCount1,
	}
	tagModes = map[string]Mode{"RDS+AQP": DefaultAdvanced, "RDSOnly": DefaultOnly, "AQP": Advanced, "NS": None, "RDS": Default}

	headingRe = regexp.MustCompile(`^## (.+) properties\s*$`)
	cellRe    = regexp.MustCompile(`^!\[[^\]]*\]\[([^\]]+)\]$`)
	lambdaRe  = regexp.MustCompile(`^(.+)/any\((\w+):(\w+)(?:/(.+))?\)$`)
)

// propertyKey writes a property of the tables the way the rest of the code asks for it:
// "assignedLicenses/any(a:a/skuId)" is "assignedLicenses/any:skuId", and "proxyAddresses/any(p:p)" is "proxyAddresses/any:".
func propertyKey(p string) (string, error) {
	p = strings.TrimSpace(p)
	m := lambdaRe.FindStringSubmatch(p)
	if m == nil {
		if strings.Contains(p, "(") {
			return "", fmt.Errorf("the property %q has a form that Parse does not know", p)
		}
		return p, nil
	}
	if m[2] != m[3] {
		return "", fmt.Errorf("the property %q uses a variable that it does not define", p)
	}
	return m[1] + "/any:" + m[4], nil
}

func cells(line string) []string {
	line = strings.TrimSpace(strings.ReplaceAll(line, " ", " ")) // the documents have non-breaking spaces
	line = strings.TrimSuffix(strings.TrimPrefix(line, "|"), "|")
	parts := strings.Split(line, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// Parse reads the two include files of Microsoft (directory-objects-filter.md, directory-objects-orderby.md)
// into the rules. It fails on a form that it does not know, so a change in the documents is seen, and not
// ignored.
func Parse(filterMD, orderByMD string) (*Matrix, error) {
	m := &Matrix{Filter: map[string]map[string]map[string]Mode{}, OrderBy: map[string]map[string]Mode{}}

	var resource string
	var columns []string // the operator of each column after the property, "" when the table has no such column
	for n, line := range strings.Split(filterMD, "\n") {
		trim := strings.TrimSpace(line)
		if h := headingRe.FindStringSubmatch(trim); h != nil {
			key, ok := resourceNames[h[1]]
			if !ok {
				return nil, fmt.Errorf("filter line %d: the heading %q is a resource that Parse does not know", n+1, trim)
			}
			resource, columns = key, nil
			continue
		}
		if !strings.HasPrefix(trim, "|") || resource == "" {
			continue
		}
		row := cells(trim)
		switch {
		case strings.EqualFold(row[0], "Property"):
			columns = columns[:0]
			for _, c := range row[1:] {
				op, ok := columnOps[c]
				if !ok {
					return nil, fmt.Errorf("filter line %d: the column %q is one that Parse does not know", n+1, c)
				}
				columns = append(columns, op)
			}
		case strings.HasPrefix(row[0], "---") || strings.HasPrefix(row[0], ":--"):
		case strings.EqualFold(row[0], "Extension type"):
			columns = nil // the table about extension properties: its rows are not about a property of the object
		default:
			if columns == nil {
				continue
			}
			// a row may end early: the cells that it does not have are blank
			if len(row) > len(columns)+1 {
				return nil, fmt.Errorf("filter line %d: the row has %d cells, and the table has %d columns", n+1, len(row), len(columns)+1)
			}
			key, err := propertyKey(row[0])
			if err != nil {
				return nil, fmt.Errorf("filter line %d: %w", n+1, err)
			}
			if m.Filter[resource] == nil {
				m.Filter[resource] = map[string]map[string]Mode{}
			}
			ops := m.Filter[resource][key]
			if ops == nil {
				ops = map[string]Mode{}
				m.Filter[resource][key] = ops
			}
			for i, c := range row[1:] {
				if c == "" {
					continue // a blank cell: the query is not valid for the property
				}
				tag := cellRe.FindStringSubmatch(c)
				if tag == nil || tagModes[tag[1]] == "" {
					return nil, fmt.Errorf("filter line %d: the cell %q is one that Parse does not know", n+1, c)
				}
				ops[columns[i]] = tagModes[tag[1]]
			}
		}
	}
	if len(m.Filter["user"]) == 0 || len(m.Filter["group"]) == 0 || len(m.Filter["servicePrincipal"]) == 0 {
		return nil, fmt.Errorf("the filter tables have no user, group, or servicePrincipal: the document has changed its form")
	}

	for n, line := range strings.Split(orderByMD, "\n") {
		trim := strings.TrimSpace(line)
		if !strings.HasPrefix(trim, "|") {
			continue
		}
		row := cells(trim)
		if len(row) != 3 || strings.EqualFold(row[0], "Directory object") || strings.HasPrefix(row[0], "---") || strings.HasPrefix(row[0], "[*") {
			continue // the row "all others" says that a property that is not listed has no sorting
		}
		tag := cellRe.FindStringSubmatch(row[2])
		if tag == nil || tagModes[tag[1]] == "" {
			return nil, fmt.Errorf("orderby line %d: the cell %q is one that Parse does not know", n+1, row[2])
		}
		if m.OrderBy[row[0]] == nil {
			m.OrderBy[row[0]] = map[string]Mode{}
		}
		m.OrderBy[row[0]][row[1]] = tagModes[tag[1]]
	}
	if len(m.OrderBy["user"]) == 0 {
		return nil, fmt.Errorf("the orderby table has no user: the document has changed its form")
	}
	return m, nil
}
