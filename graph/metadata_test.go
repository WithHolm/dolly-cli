package graph_test

import (
	"encoding/xml"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
	"github.com/withholm/dolly-cli/graph/spec"
)

type csdlProperty struct {
	Name string `xml:"Name,attr"`
	Type string `xml:"Type,attr"`
}

type csdlType struct {
	Name       string         `xml:"Name,attr"`
	BaseType   string         `xml:"BaseType,attr"`
	Properties []csdlProperty `xml:"Property"`
	Navigation []csdlProperty `xml:"NavigationProperty"`
}

type csdl struct {
	Schemas []struct {
		Namespace string     `xml:"Namespace,attr"`
		Entities  []csdlType `xml:"EntityType"`
		Complex   []csdlType `xml:"ComplexType"`
		Enums     []struct {
			Name string `xml:"Name,attr"`
		} `xml:"EnumType"`
		Container []struct {
			Name string `xml:"Name,attr"`
			Sets []struct {
				Name       string `xml:"Name,attr"`
				EntityType string `xml:"EntityType,attr"`
			} `xml:"EntitySet"`
			Singletons []struct {
				Name string `xml:"Name,attr"`
				Type string `xml:"Type,attr"`
			} `xml:"Singleton"`
		} `xml:"EntityContainer"`
	} `xml:"DataServices>Schema"`
}

// /$metadata is the OData description of what dollycli serves (the part of Microsoft's description that it has, see graph/spec/gen/csdl.go). The real Graph answers
// 200 with application/xml and the header OData-Version: 4.0 (checked with the Graph Explorer proxy: 1,241 entity types, 1.75 MB; dollycli has the types that it serves).
func TestMetadata_IsServedAsXML(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	get := func(token string) (*http.Response, []byte) {
		req, _ := http.NewRequest("GET", ts.URL+"/v1.0/$metadata", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, b
	}
	res, body := get("x")
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "application/xml") || res.Header.Get("OData-Version") != "4.0" {
		t.Fatalf("%d %q %q", res.StatusCode, res.Header.Get("Content-Type"), res.Header.Get("OData-Version"))
	}
	var doc csdl
	if err := xml.Unmarshal(body, &doc); err != nil || len(doc.Schemas) == 0 {
		t.Fatalf("not an OData description: %v", err)
	}
	// the description of the API is public: a client reads it before it signs in (a guess: it comes from what the real Graph is known to do, and is not checked)
	if res, _ := get(""); res.StatusCode != 200 {
		t.Errorf("with no token: %d", res.StatusCode)
	}
}

// The description has the entity sets that dollycli serves, the types of them, and the base types, and every type that it names is in it.
func TestMetadata_DescribesWhatDollyServes(t *testing.T) {
	var doc csdl
	if err := xml.Unmarshal(spec.Metadata(), &doc); err != nil {
		t.Fatal(err)
	}
	types := map[string]csdlType{}
	entity := map[string]bool{}
	defined := map[string]bool{}
	for _, s := range doc.Schemas {
		for _, e := range s.Entities {
			types[s.Namespace+"."+e.Name], entity[s.Namespace+"."+e.Name], defined[s.Namespace+"."+e.Name] = e, true, true
		}
		for _, e := range s.Complex {
			types[s.Namespace+"."+e.Name], defined[s.Namespace+"."+e.Name] = e, true
		}
		for _, e := range s.Enums {
			defined[s.Namespace+"."+e.Name] = true
		}
	}
	for _, want := range []string{"user", "group", "servicePrincipal", "application", "organization", "oAuth2PermissionGrant", "appRoleAssignment", "directoryObject", "entity", "place", "room", "building"} {
		if !entity["microsoft.graph."+want] {
			t.Errorf("no entity type %s", want)
		}
	}
	if types["microsoft.graph.user"].BaseType != "microsoft.graph.directoryObject" || types["microsoft.graph.directoryObject"].BaseType != "microsoft.graph.entity" {
		t.Errorf("the hierarchy: user is a %q, directoryObject is a %q", types["microsoft.graph.user"].BaseType, types["microsoft.graph.directoryObject"].BaseType)
	}
	sets := map[string]string{}
	for _, s := range doc.Schemas {
		for _, c := range s.Container {
			for _, e := range c.Sets {
				sets[e.Name] = e.EntityType
			}
			for _, e := range c.Singletons {
				sets[e.Name] = e.Type
			}
		}
	}
	for name, typ := range map[string]string{"users": "microsoft.graph.user", "groups": "microsoft.graph.group", "servicePrincipals": "microsoft.graph.servicePrincipal", "applications": "microsoft.graph.application",
		"organization": "microsoft.graph.organization", "oauth2PermissionGrants": "microsoft.graph.oAuth2PermissionGrant", "directoryObjects": "microsoft.graph.directoryObject", "places": "microsoft.graph.place", "me": "microsoft.graph.user"} {
		if sets[name] != typ {
			t.Errorf("the entity set %s is %q, want %s", name, sets[name], typ)
		}
	}
	// every type that is named is defined here (or is of the Edm namespace)
	named := func(t string) string {
		t = strings.TrimSuffix(strings.TrimPrefix(t, "Collection("), ")")
		if strings.HasPrefix(t, "Edm.") {
			return ""
		}
		return t
	}
	for q, typ := range types {
		for _, p := range append(slices.Clone(typ.Properties), typ.Navigation...) {
			if n := named(p.Type); n != "" && !defined[n] {
				t.Errorf("%s.%s is of the type %s, and the description does not define it", q, p.Name, n)
			}
		}
		if typ.BaseType != "" && !defined[typ.BaseType] {
			t.Errorf("%s has the base type %s, and the description does not define it", q, typ.BaseType)
		}
	}
	for q, typ := range sets {
		if !defined[typ] {
			t.Errorf("the entity set %s is of the type %s, and the description does not define it", q, typ)
		}
	}
}

// The OData description and the OpenAPI description are two views of the same API: a property of a type in the first is a property of the schema in the second.
// (The other way is not true: the OpenAPI schema has the navigation properties, and the description of dollycli leaves out those that point at types it does not have.)
func TestMetadata_AgreesWithTheOpenAPIExtract(t *testing.T) {
	var doc csdl
	if err := xml.Unmarshal(spec.Metadata(), &doc); err != nil {
		t.Fatal(err)
	}
	byName := map[string]csdlType{}
	for _, s := range doc.Schemas {
		for _, e := range s.Entities {
			byName[s.Namespace+"."+e.Name] = e
		}
	}
	var all func(q string) []csdlProperty // the properties of a type and of its base types
	all = func(q string) []csdlProperty {
		e, ok := byName[q]
		if !ok {
			return nil
		}
		return append(slices.Clone(e.Properties), all(e.BaseType)...)
	}
	for schema, fullName := range map[string]string{"user": "microsoft.graph.user", "group": "microsoft.graph.group", "servicePrincipal": "microsoft.graph.servicePrincipal", "organization": "microsoft.graph.organization",
		"application": "microsoft.graph.application", "oAuth2PermissionGrant": "microsoft.graph.oAuth2PermissionGrant", "appRoleAssignment": "microsoft.graph.appRoleAssignment"} {
		props := spec.Load().Schemas[schema]
		if len(props) == 0 {
			t.Errorf("the OpenAPI extract has no schema %s", schema)
			continue
		}
		var missing []string
		for _, p := range all(fullName) {
			if _, ok := props[p.Name]; !ok {
				missing = append(missing, p.Name)
			}
		}
		if len(missing) > 0 {
			t.Errorf("%s: properties of the OData description that the OpenAPI schema does not have: %v", schema, missing)
		}
	}
}
