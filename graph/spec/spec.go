// Package spec is the part of Microsoft's OpenAPI description of Graph (v1.0) that
// dolly is checked against: the operations of the resources that dolly has, and
// the properties of the user, the group, and the app. Tests use it to find a route or
// a property that dolly has and Graph does not, and to list what Graph has and
// dolly does not. It is made by gen (see `make graph-spec`).
//
// The description is not the truth. It leaves out behavior (the exact errors, the
// limits, when a value is null or missing), so the notes in .plans/dolly-graph-notes.md
// hold what the docs say in words.
package spec

import (
	_ "embed"
	"encoding/json"
	"slices"
	"strings"
	"sync"
)

//go:embed graph-v1.0.json
var raw []byte

//go:embed metadata-v1.0.xml
var metadata []byte

// Metadata is the OData description (CSDL, the XML of /$metadata) of the part of Graph that dolly serves: the types that its resources need, the navigation
// properties between them, and the entity sets. It is made from the description of Microsoft by gen (see `make graph-spec`), with the types written
// in full (microsoft.graph.user), as the real Graph writes them. The source is msgraph-metadata, clean_v10_metadata/cleanMetadata.xml (MIT).
func Metadata() []byte { return metadata }

// Operation is one call that Graph has.
type Operation struct {
	Method  string   `json:"method"`
	Path    string   `json:"path"`
	Summary string   `json:"summary"`
	Doc     string   `json:"doc"`
	Query   []string `json:"query"`
	Headers []string `json:"headers"`
}

// Property is one property of a schema.
type Property struct {
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
	ReadOnly bool   `json:"readOnly"`
	// Default is true when Graph returns the property without $select.
	Default bool   `json:"default"`
	Filter  string `json:"filter"`
}

// Spec is the extract.
type Spec struct {
	Source     string                         `json:"source"`
	Operations []Operation                    `json:"operations"`
	Schemas    map[string]map[string]Property `json:"schemas"`
}

var (
	once   sync.Once
	loaded *Spec
)

// Load reads the extract. It panics when the file is bad: the file is embedded, and a
// test finds the problem.
func Load() *Spec {
	once.Do(func() {
		loaded = &Spec{}
		if err := json.Unmarshal(raw, loaded); err != nil {
			panic("spec: graph-v1.0.json is not valid: " + err.Error())
		}
	})
	return loaded
}

// Normal writes a path in the form that is the same in Graph and in dolly: every
// {name} is {}, a start of /v1.0 is gone, the case does not matter (Graph does not
// care), and a type cast is written as the description writes it (graph.room, and
// the Learn pages write microsoft.graph.room).
func Normal(path string) string {
	path = strings.ToLower(strings.TrimPrefix(path, "/v1.0"))
	path = strings.ReplaceAll(path, "microsoft.graph.", "graph.")
	var b strings.Builder
	for i := 0; i < len(path); i++ {
		if path[i] != '{' {
			b.WriteByte(path[i])
			continue
		}
		end := strings.IndexByte(path[i:], '}')
		if end < 0 {
			b.WriteString(path[i:])
			break
		}
		b.WriteString("{}")
		i += end
	}
	return b.String()
}

// Has says whether Graph has this call. The path can be in dolly's form.
func (s *Spec) Has(method, path string) bool {
	_, ok := s.index()[strings.ToUpper(method)+" "+Normal(path)]
	return ok
}

// Operation finds a call of Graph, or nil.
func (s *Spec) Operation(method, path string) *Operation {
	return s.index()[strings.ToUpper(method)+" "+Normal(path)]
}

// ForDolly finds the call of Graph that matches a path of dolly. Dolly has one route with a {type}
// where Graph has one path for each type, so the first type that Graph has is used.
func (s *Spec) ForDolly(method, path string) *Operation {
	if !strings.Contains(path, "/{type}") {
		return s.Operation(method, path)
	}
	for _, t := range []string{"microsoft.graph.user", "microsoft.graph.group", "microsoft.graph.servicePrincipal"} {
		if o := s.Operation(method, strings.Replace(path, "/{type}", "/"+t, 1)); o != nil {
			return o
		}
	}
	return nil
}

// Declared says whether a schema declares a property, by name, without regard to capitals.
func (s *Spec) Declared(schema, name string) bool {
	_, ok := s.PropertyType(schema, name)
	return ok
}

// PropertyType gives the type that a schema declares for a property ("string", "array", ...), by name, without regard to
// capitals. The bool is false when the schema does not declare it.
func (s *Spec) PropertyType(schema, name string) (string, bool) {
	for n, p := range s.Schemas[schema] {
		if strings.EqualFold(n, name) {
			return p.Type, true
		}
	}
	return "", false
}

// FilterProperty is a property that Graph says $filter works on, with the words of Graph about it.
type FilterProperty struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Filter string `json:"filter"` // for example: eq, ne, not, ge, le, in, startsWith, and eq on null values
}

// FilterProperties lists the properties of a schema that have a note about $filter, by name.
func (s *Spec) FilterProperties(schema string) []FilterProperty {
	var out []FilterProperty
	for name, p := range s.Schemas[schema] {
		if p.Filter != "" {
			out = append(out, FilterProperty{Name: name, Type: p.Type, Filter: p.Filter})
		}
	}
	slices.SortFunc(out, func(a, b FilterProperty) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return out
}

var (
	indexOnce sync.Once
	byKey     map[string]*Operation
)

func (s *Spec) index() map[string]*Operation {
	indexOnce.Do(func() {
		byKey = make(map[string]*Operation, len(s.Operations))
		for i := range s.Operations {
			o := &s.Operations[i]
			byKey[o.Method+" "+Normal(o.Path)] = o
		}
	})
	return byKey
}
