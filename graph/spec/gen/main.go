// Command gen makes graph-v1.0.json, the part of Microsoft's OpenAPI description of
// Graph that dolly is checked against. It is not part of cohort. Run it with
// `make graph-spec`, which downloads the description first. The description is MIT
// licensed (github.com/microsoftgraph/msgraph-metadata).
//
// It keeps, for the resources that dolly has: every operation (method, path,
// summary, the page in Microsoft Learn, the query options and the headers), and the
// properties of the schemas (type, nullable, read-only, whether Graph returns it
// without $select). Everything else in the 44 MB file is left out.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// prefixes are the paths that dolly has, or may have soon.
var prefixes = []string{"/users", "/groups", "/servicePrincipals", "/applications", "/places", "/organization", "/oauth2PermissionGrants", "/subscriptions", "/sites", "/drives", "/me"}

// schemas are the schemas that give the properties.
var schemas = map[string]string{
	"user": "microsoft.graph.user", "group": "microsoft.graph.group",
	"servicePrincipal": "microsoft.graph.servicePrincipal", "directoryObject": "microsoft.graph.directoryObject",
	"application": "microsoft.graph.application", "subscription": "microsoft.graph.subscription",
	"place": "microsoft.graph.place", "room": "microsoft.graph.room", "roomList": "microsoft.graph.roomList",
	"building": "microsoft.graph.building", "floor": "microsoft.graph.floor", "section": "microsoft.graph.section",
	"desk": "microsoft.graph.desk", "workspace": "microsoft.graph.workspace",
	"physicalAddress": "microsoft.graph.physicalAddress", "outlookGeoCoordinates": "microsoft.graph.outlookGeoCoordinates",
	"organization": "microsoft.graph.organization", "verifiedDomain": "microsoft.graph.verifiedDomain",
	"oAuth2PermissionGrant": "microsoft.graph.oAuth2PermissionGrant", "appRoleAssignment": "microsoft.graph.appRoleAssignment",
	"site": "microsoft.graph.site", "siteCollection": "microsoft.graph.siteCollection", "drive": "microsoft.graph.drive",
	"baseItem": "microsoft.graph.baseItem", "root": "microsoft.graph.root", "sharepointIds": "microsoft.graph.sharepointIds",
	"driveItem": "microsoft.graph.driveItem", "itemReference": "microsoft.graph.itemReference", "folder": "microsoft.graph.folder",
	"file": "microsoft.graph.file", "fileSystemInfo": "microsoft.graph.fileSystemInfo", "quota": "microsoft.graph.quota",
}

type operation struct {
	Method  string   `json:"method"`
	Path    string   `json:"path"`
	Summary string   `json:"summary"`
	Doc     string   `json:"doc,omitempty"`
	Query   []string `json:"query,omitempty"`
	Headers []string `json:"headers,omitempty"`
}

type property struct {
	Type     string `json:"type"` // string, boolean, integer, array, object, or the name of a schema
	Nullable bool   `json:"nullable,omitempty"`
	ReadOnly bool   `json:"readOnly,omitempty"`
	// Default is true when Graph returns the property without $select.
	Default bool   `json:"default,omitempty"`
	Filter  string `json:"filter,omitempty"` // the text after "Supports $filter"
}

type extract struct {
	Source     string                         `json:"source"`
	Operations []operation                    `json:"operations"`
	Schemas    map[string]map[string]property `json:"schemas"`
}

var filterText = regexp.MustCompile(`Supports \$filter \(([^)]*)\)`)

// metadataRoots are the types of microsoft.graph that dollycli has, and metadataSets the entity sets and singletons of the container that it has. The OData
// description that dollycli serves at /$metadata is made from them (csdl.go).
var (
	metadataRoots = []string{"entity", "directoryObject", "user", "group", "servicePrincipal", "application", "organization", "oAuth2PermissionGrant", "appRoleAssignment",
		"place", "building", "floor", "section", "desk", "room", "workspace", "roomList", "subscription",
		"baseItem", "site", "drive", "siteCollection", "root", "sharepointIds", "driveItem", "itemReference", "folder", "file", "fileSystemInfo", "quota"}
	metadataSets = []string{"users", "groups", "servicePrincipals", "applications", "organization", "oauth2PermissionGrants", "directoryObjects", "places", "subscriptions", "sites", "drives", "me"}
)

func main() {
	in := flag.String("in", "openapi.yaml", "the OpenAPI file of Graph v1.0")
	out := flag.String("out", "graph-v1.0.json", "the extract to write")
	csdl := flag.String("csdl", "", "the OData description of Graph v1.0 (cleanMetadata.xml). With it, the part that dollycli serves is written to -csdl-out")
	csdlOut := flag.String("csdl-out", "metadata-v1.0.xml", "the trimmed OData description to write")
	flag.Parse()
	if err := run(*in, *out); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
	if *csdl != "" {
		data, err := os.ReadFile(*csdl)
		if err == nil {
			var trimmed []byte
			if trimmed, err = trimMetadata(data, metadataRoots, metadataSets); err == nil {
				fmt.Printf("%s: %d bytes from %d\n", *csdlOut, len(trimmed), len(data))
				err = os.WriteFile(*csdlOut, trimmed, 0o644)
			}
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "gen:", err)
			os.Exit(1)
		}
	}
}

func run(in, out string) error {
	data, err := os.ReadFile(in)
	if err != nil {
		return err
	}
	var doc struct {
		Paths      map[string]map[string]any `yaml:"paths"`
		Components struct {
			Schemas map[string]map[string]any `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}

	ex := extract{
		Source:  "https://github.com/microsoftgraph/msgraph-metadata, openapi/v1.0/openapi.yaml (MIT)",
		Schemas: map[string]map[string]property{},
	}
	for path, item := range doc.Paths {
		if !wanted(path) {
			continue
		}
		for method, raw := range item {
			if op, ok := readOperation(strings.ToUpper(method), path, raw); ok {
				ex.Operations = append(ex.Operations, op)
			}
		}
	}
	sort.Slice(ex.Operations, func(i, j int) bool {
		a, b := ex.Operations[i], ex.Operations[j]
		return a.Path < b.Path || a.Path == b.Path && a.Method < b.Method
	})
	for short, full := range schemas {
		ex.Schemas[short] = readProperties(doc.Components.Schemas, doc.Components.Schemas[full])
	}

	b, err := json.MarshalIndent(ex, "", " ")
	if err != nil {
		return err
	}
	fmt.Printf("%d operations, %d bytes\n", len(ex.Operations), len(b))
	return os.WriteFile(out, append(b, '\n'), 0o644)
}

func wanted(path string) bool {
	for _, p := range prefixes {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

func readOperation(method, path string, raw any) (operation, bool) {
	m, ok := raw.(map[string]any)
	if !ok || strings.HasPrefix(method, "X-") || method == "PARAMETERS" || method == "DESCRIPTION" {
		return operation{}, false
	}
	op := operation{Method: method, Path: path}
	op.Summary, _ = m["summary"].(string)
	if d, ok := m["externalDocs"].(map[string]any); ok {
		op.Doc, _ = d["url"].(string)
	}
	params, _ := m["parameters"].([]any)
	for _, p := range params {
		pm, _ := p.(map[string]any)
		name, _ := pm["name"].(string)
		switch pm["in"] {
		case "query":
			op.Query = append(op.Query, name)
		case "header":
			op.Headers = append(op.Headers, name)
		}
	}
	return op, true
}

func readProperties(all map[string]map[string]any, schema map[string]any) map[string]property {
	props := map[string]property{}
	collect(all, schema, props)
	return props
}

// collect reads the properties of a schema, and of the schemas that it is made of
// (allOf, and the schemas that it points to), as OpenAPI writes inheritance. The
// id of a user is in the base entity.
func collect(schemas map[string]map[string]any, node map[string]any, props map[string]property) {
	if ref, ok := node["$ref"].(string); ok {
		if base := schemas[ref[strings.LastIndex(ref, "/")+1:]]; base != nil {
			collect(schemas, base, props)
		}
	}
	if all, ok := node["allOf"].([]any); ok {
		for _, part := range all {
			if m, ok := part.(map[string]any); ok {
				collect(schemas, m, props)
			}
		}
	}
	p, _ := node["properties"].(map[string]any)
	for name, raw := range p {
		m, _ := raw.(map[string]any)
		desc, _ := m["description"].(string)
		// "Read-only." is the whole property. "Read-only for users synced from
		// on-premises" (mobilePhone) is read-only for some users, so a call may set it.
		prop := property{Type: typeOf(m), ReadOnly: strings.Contains(desc, "Read-only."), Default: !strings.Contains(desc, "Requires $select")}
		prop.Nullable, _ = m["nullable"].(bool)
		if f := filterText.FindStringSubmatch(desc); f != nil {
			prop.Filter = f[1]
		}
		// A property with no description is not a stored value (a navigation property, or the
		// type marker). Graph never returns a navigation property without $expand.
		if desc == "" && prop.Type != "string" {
			prop.Default = false
		}
		props[name] = prop
	}
}

func typeOf(m map[string]any) string {
	if t, ok := m["type"].(string); ok {
		return t
	}
	if r, ok := m["$ref"].(string); ok {
		return r[strings.LastIndex(r, "/")+1:]
	}
	for _, key := range []string{"anyOf", "oneOf"} {
		if list, ok := m[key].([]any); ok {
			for _, v := range list {
				if vm, ok := v.(map[string]any); ok {
					if t := typeOf(vm); t != "" && t != "object" {
						return t
					}
				}
			}
		}
	}
	return "object"
}
