// Package cost is what Microsoft says about the cost of a Graph call and the limits that
// throttle it. The data is cost.json. The types in this file are its form: Load refuses a
// file that has a field these types do not know, so a change in the form is a change in this
// file. cost.json is made from Microsoft's documents (see README.md).
//
// Dolly reads only a part of it: the global limit and the identity service (the resource
// unit model). The other services are in the types so that the file is checked in full.
package cost

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed cost.json
var raw []byte

// Data is cost.json.
type Data struct {
	Source   Source    `json:"source"`
	Global   Global    `json:"global"`
	Identity Identity  `json:"identity"`
	Services []Service `json:"services"`

	// made by Load: the patterns of Identity.BaseCosts
	base []basePattern
}

// Source says where the data is from.
type Source struct {
	Commit     string `json:"commit"`
	CommitDate string `json:"commitDate"`
	License    string `json:"license"`
}

// Global is the limit for all services.
type Global struct {
	Requests int     `json:"requests"`
	Per      string  `json:"per"`
	Scope    string  `json:"scope"`
	Methods  Methods `json:"methods"`
}

// Identity is the identity and access service. It counts resource units (RU).
type Identity struct {
	Unit          string            `json:"unit"`
	Limits        []Limit           `json:"limits"`
	TenantSize    map[string]string `json:"tenantSize"`
	BaseCosts     []BaseCost        `json:"baseCosts"`
	Defaults      []Default         `json:"defaults"`
	Modifiers     []Modifier        `json:"modifiers"`
	MinimumRU     int               `json:"minimumRu"`
	SpecialLimits []SpecialLimit    `json:"specialLimits"`
	Headers       Headers           `json:"headers"`
	Notes         []string          `json:"notes"`
}

// BaseCost is the cost of a request path that the table names.
type BaseCost struct {
	Method string `json:"method"`
	Path   string `json:"path"` // with {id} for a part that changes
	RU     int    `json:"ru"`
	Write  int    `json:"write"`
}

// Default is the cost of a path that the table does not name.
type Default struct {
	Method string `json:"method"`
	RU     int    `json:"ru"`
	Write  int    `json:"write"`
}

// Modifier changes the cost for a query option. When is $select, $expand, $top<20, or
// b2cUserCreate. With RU it adds to the cost; with RUSet (and Path) it sets the cost.
type Modifier struct {
	When  string `json:"when"`
	Path  string `json:"path,omitempty"`
	RU    int    `json:"ru,omitempty"`
	RUSet int    `json:"ruSet,omitempty"`
}

// SpecialLimit is a limit on one path that depends on a property of the body.
type SpecialLimit struct {
	Path     string   `json:"path"`
	Methods  []string `json:"methods"`
	When     string   `json:"when"`
	Requests int      `json:"requests"`
	Per      string   `json:"per"`
}

// Headers are the throttle headers that the source describes.
type Headers struct {
	Request   []Header `json:"request"`
	Response  []Header `json:"response"`
	Throttled []Header `json:"throttled"`
}

// Header is one header.
type Header struct {
	Name    string   `json:"name"`
	Values  []string `json:"values,omitempty"`
	Default string   `json:"default,omitempty"`
	Notes   string   `json:"notes"`
}

// Limit is a limit. A number the source does not give is zero. The limit of a service has
// Requests (or one of the Min and Concurrent forms) and Per. The limit of the identity
// service has RU and Per, and Writes and WritesPer.
type Limit struct {
	Scope         string   `json:"scope"`
	RU            *BySize  `json:"ru,omitempty"`
	Per           string   `json:"per,omitempty"`
	Writes        int      `json:"writes,omitempty"`
	WritesPer     string   `json:"writesPer,omitempty"`
	Requests      int      `json:"requests,omitempty"`
	Concurrent    int      `json:"concurrent,omitempty"`
	UploadMB      int      `json:"uploadMB,omitempty"`
	MinRequests   int      `json:"minRequests,omitempty"`
	MinConcurrent int      `json:"minConcurrent,omitempty"`
	MinRecords    int      `json:"minRecords,omitempty"`
	Methods       *Methods `json:"methods,omitempty"`
	Path          string   `json:"path,omitempty"`
	Resource      string   `json:"resource,omitempty"`
	Notes         string   `json:"notes,omitempty"`
	From          string   `json:"from,omitempty"`
	Until         string   `json:"until,omitempty"`
}

// Service is a service other than identity.
type Service struct {
	Name          string            `json:"name"`
	Include       string            `json:"include"`
	Per           string            `json:"per,omitempty"`
	Dimensions    map[string]string `json:"dimensions,omitempty"`
	Limits        []Limit           `json:"limits"`
	Routes        []Route           `json:"routes,omitempty"`
	RouteDefaults []Route           `json:"routeDefaults,omitempty"`
	Notes         string            `json:"notes"`
}

// Route is a limit by request, in the Teams table. The numbers are requests per Service.Per.
type Route struct {
	Group     string         `json:"group,omitempty"`
	Methods   Methods        `json:"methods"`
	Path      string         `json:"path,omitempty"`
	App       int            `json:"app,omitempty"`
	AppTenant int            `json:"appTenant,omitempty"`
	User      int            `json:"user,omitempty"`
	Resource  *RouteResource `json:"resource,omitempty"`
	Notes     string         `json:"notes,omitempty"`
}

// RouteResource is the limit for one team, channel, or chat.
type RouteResource struct {
	Requests int    `json:"requests"`
	Per      string `json:"per"`
	Of       string `json:"of"`
}

// Methods is "any", or a list of HTTP methods.
type Methods struct {
	Any  bool
	List []string
}

// UnmarshalJSON reads "any" or a list.
func (m *Methods) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if s != "any" {
			return fmt.Errorf(`methods must be "any" or a list, not %q`, s)
		}
		m.Any = true
		return nil
	}
	return json.Unmarshal(b, &m.List)
}

// MarshalJSON writes what UnmarshalJSON reads.
func (m Methods) MarshalJSON() ([]byte, error) {
	if m.Any {
		return []byte(`"any"`), nil
	}
	return json.Marshal(m.List)
}

// Has reports whether the limit is for the method.
func (m Methods) Has(method string) bool {
	if m.Any {
		return true
	}
	for _, x := range m.List {
		if strings.EqualFold(x, method) {
			return true
		}
	}
	return false
}

// BySize is a number that depends on the size of the tenant. One number in the file is the
// same for all sizes.
type BySize struct{ S, M, L int }

// UnmarshalJSON reads a number, or {"S":…,"M":…,"L":…}.
func (b *BySize) UnmarshalJSON(data []byte) error {
	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		*b = BySize{n, n, n}
		return nil
	}
	var o struct{ S, M, L int }
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&o); err != nil {
		return err
	}
	*b = BySize(o)
	return nil
}

// MarshalJSON writes what UnmarshalJSON reads.
func (b BySize) MarshalJSON() ([]byte, error) {
	if b.S == b.M && b.M == b.L {
		return json.Marshal(b.S)
	}
	return json.Marshal(struct{ S, M, L int }(b))
}

// Size gives the number for a tenant size: "S", "M", or "L".
func (b BySize) Size(size string) int {
	switch size {
	case "S":
		return b.S
	case "M":
		return b.M
	}
	return b.L
}

// TenantSize gives the size of a tenant by its number of users: S is under 50, M is 50 to 500,
// and L is over 500 (the identity service, see Identity.TenantSize).
func TenantSize(users int) string {
	switch {
	case users < 50:
		return "S"
	case users <= 500:
		return "M"
	}
	return "L"
}

// Parse reads cost.json data. It refuses a field that the types do not have.
func Parse(data []byte) (*Data, error) {
	var d Data
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("cost.json: %w", err)
	}
	if err := d.prepare(); err != nil {
		return nil, fmt.Errorf("cost.json: %w", err)
	}
	return &d, nil
}

// Load reads the cost.json that is in the program.
func Load() (*Data, error) { return Parse(raw) }
