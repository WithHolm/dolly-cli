package cost

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// overrides.json has the cost of a route that Microsoft's tables do not name, when a person reports it. It is written by hand and kept apart from
// cost.json, so that 'make update-cost' never removes it and the test that compares cost.json with Microsoft's table still holds. An override
// beats the row of Microsoft's table and the default. The query options still add to it, as they do to a row of the table.

//go:embed overrides.json
var overridesRaw []byte

// Override is the cost of one route, as a person reported it.
type Override struct {
	Method string `json:"method"`
	Path   string `json:"path"` // without the version, with {id} for a part that changes: applications/{id}
	RU     int    `json:"ru"`
	Write  int    `json:"write"`
	// Source says who reported it, and where: a link to an issue, a name and a date. It is required.
	Source string `json:"source"`
	// Note says why the override is there when Microsoft's table has a row for the same route.
	Note string `json:"note,omitempty"`
}

type overrideFile struct {
	Note      string     `json:"note"`
	Overrides []Override `json:"overrides"`
}

// Overrides gives the overrides that the data has.
func (d *Data) Overrides() []Override { return slices.Clone(d.overrides) }

// HasRow says whether Microsoft's table has a row for this route (method and path as in the table, with {id}).
func (d *Data) HasRow(method, path string) bool {
	method, parts := strings.ToUpper(method), split(path)
	return slices.ContainsFunc(d.base, func(b basePattern) bool { return b.method == method && matches(b.parts, parts) })
}

func (d *Data) addOverrides(data []byte) error {
	var f overrideFile
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return fmt.Errorf("overrides.json: %w", err)
	}
	seen := map[string]bool{}
	for _, o := range f.Overrides {
		method := strings.ToUpper(o.Method)
		key := method + " " + strings.ToLower(strings.Trim(o.Path, "/"))
		switch {
		case !slices.Contains([]string{"GET", "POST", "PATCH", "PUT", "DELETE"}, method):
			return fmt.Errorf("overrides.json: %q is not a method", o.Method)
		case strings.Trim(o.Path, "/") == "":
			return fmt.Errorf("overrides.json: %s has no path", method)
		case o.RU < 1:
			return fmt.Errorf("overrides.json: %s has ru %d: a request costs 1 or more", key, o.RU)
		case o.Write < 0:
			return fmt.Errorf("overrides.json: %s has a negative write", key)
		case strings.TrimSpace(o.Source) == "":
			return fmt.Errorf("overrides.json: %s has no source: say who reported the cost, and where", key)
		case seen[key]:
			return fmt.Errorf("overrides.json: %s is twice", key)
		}
		seen[key] = true
		o.Method = method
		d.overrides = append(d.overrides, o)
		d.over = append(d.over, basePattern{method, split(o.Path), BaseCost{Method: method, Path: o.Path, RU: o.RU, Write: o.Write}})
	}
	return nil
}
