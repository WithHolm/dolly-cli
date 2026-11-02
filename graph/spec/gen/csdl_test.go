package main

import (
	"encoding/xml"
	"strings"
	"testing"
)

// A small description of our own, in the form of Microsoft's: an alias for the namespace, types that are not wanted, a navigation property to a type that is
// not wanted, a bound function, annotations, and a second schema.
const csdlFixture = `<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx Version="4.0" xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx">
  <edmx:DataServices>
    <Schema Namespace="microsoft.graph" Alias="graph" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <EnumType Name="color"><Member Name="red" Value="0" /><Member Name="blue" Value="1" /></EnumType>
      <EnumType Name="unusedEnum"><Member Name="x" Value="0" /></EnumType>
      <ComplexType Name="address">
        <Property Name="city" Type="Edm.String" />
        <Property Name="tint" Type="graph.color" />
        <NavigationProperty Name="nearestPart" Type="graph.part" />
        <NavigationProperty Name="nearestPerson" Type="graph.person" />
      </ComplexType>
      <ComplexType Name="unusedComplex"><Property Name="a" Type="Edm.String" /></ComplexType>
      <EntityType Name="entity" Abstract="true">
        <Key><PropertyRef Name="id" /></Key>
        <Property Name="id" Type="Edm.String" Nullable="false" />
      </EntityType>
      <EntityType Name="thing" BaseType="graph.entity">
        <Property Name="name" Type="Edm.String" />
        <Property Name="tags" Type="Collection(Edm.String)" />
        <Property Name="homes" Type="Collection(graph.address)" />
        <NavigationProperty Name="owner" Type="graph.person" />
        <NavigationProperty Name="parts" Type="Collection(graph.part)" ContainsTarget="true" />
        <Annotation Term="Org.OData.Core.V1.Description" String="dropped" />
      </EntityType>
      <EntityType Name="person" BaseType="graph.entity"><Property Name="displayName" Type="Edm.String" /></EntityType>
      <EntityType Name="part" BaseType="graph.entity" />
      <EntityType Name="other" BaseType="graph.entity" />
      <Function Name="hello" IsBound="true"><Parameter Name="bindingParameter" Type="graph.thing" /><ReturnType Type="Edm.String" /></Function>
      <Annotations Target="microsoft.graph.thing"><Annotation Term="Org.OData.Core.V1.Description" String="dropped" /></Annotations>
      <EntityContainer Name="GraphService">
        <EntitySet Name="things" EntityType="microsoft.graph.thing">
          <NavigationPropertyBinding Path="owner" Target="people" />
          <NavigationPropertyBinding Path="parts" Target="parts" />
        </EntitySet>
        <EntitySet Name="people" EntityType="microsoft.graph.person" />
        <EntitySet Name="others" EntityType="microsoft.graph.other" />
        <Singleton Name="me" Type="microsoft.graph.person" />
      </EntityContainer>
    </Schema>
    <Schema Namespace="microsoft.graph.security" Alias="self" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <EntityType Name="alert" BaseType="graph.entity"><Property Name="title" Type="Edm.String" /></EntityType>
    </Schema>
  </edmx:DataServices>
</edmx:Edmx>`

// names lists the types of a trimmed description, as "Kind Name" with the namespace in front.
func names(t *testing.T, doc []byte) map[string]bool {
	t.Helper()
	var root struct {
		Schemas []struct {
			Namespace string `xml:"Namespace,attr"`
			Alias     string `xml:"Alias,attr"`
			Entities  []struct {
				Name string `xml:"Name,attr"`
			} `xml:"EntityType"`
			Complex []struct {
				Name string `xml:"Name,attr"`
			} `xml:"ComplexType"`
			Enums []struct {
				Name string `xml:"Name,attr"`
			} `xml:"EnumType"`
		} `xml:"DataServices>Schema"`
	}
	if err := xml.Unmarshal(doc, &root); err != nil {
		t.Fatalf("the trimmed description is not well-formed XML: %v\n%s", err, doc)
	}
	out := map[string]bool{}
	for _, s := range root.Schemas {
		if s.Alias != "" {
			t.Errorf("the schema %s keeps its alias %q: the real Graph writes full names", s.Namespace, s.Alias)
		}
		for _, e := range s.Entities {
			out["entity "+s.Namespace+"."+e.Name] = true
		}
		for _, e := range s.Complex {
			out["complex "+s.Namespace+"."+e.Name] = true
		}
		for _, e := range s.Enums {
			out["enum "+s.Namespace+"."+e.Name] = true
		}
	}
	return out
}

func TestTrimMetadata_KeepsWhatTheRootsNeed(t *testing.T) {
	doc, err := trimMetadata([]byte(csdlFixture), []string{"thing", "person"}, []string{"things", "people", "me"})
	if err != nil {
		t.Fatal(err)
	}
	got := names(t, doc)
	want := []string{"entity microsoft.graph.entity", "entity microsoft.graph.thing", "entity microsoft.graph.person", "complex microsoft.graph.address", "enum microsoft.graph.color"}
	for _, w := range want {
		if !got[w] {
			t.Errorf("%s is missing: %v", w, got)
		}
	}
	if len(got) != len(want) {
		t.Errorf("types that nothing needs are kept: %v", got)
	}
	text := string(doc)
	for _, gone := range []string{"Alias", "unused", `Name="other"`, `Name="part"`, `Name="parts"`, "hello", "Annotation", "dropped", "microsoft.graph.security", `Name="others"`, `"graph.`} {
		if strings.Contains(text, gone) {
			t.Errorf("%q is in the trimmed description:\n%s", gone, text)
		}
	}
	for _, kept := range []string{
		`<EntityType Name="thing" BaseType="microsoft.graph.entity">`, `Type="Collection(microsoft.graph.address)"`, `Type="microsoft.graph.color"`, `Type="Collection(Edm.String)"`,
		`<NavigationProperty Name="owner" Type="microsoft.graph.person" />`, `<Key>`, `Nullable="false"`, `Abstract="true"`, `<EntitySet Name="things" EntityType="microsoft.graph.thing">`,
		`<NavigationPropertyBinding Path="owner" Target="people" />`, `<Singleton Name="me" Type="microsoft.graph.person" />`, `<EntityContainer Name="GraphService">`,
		`xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx"`, `<edmx:Edmx Version="4.0"`, `<edmx:DataServices>`, `xmlns="http://docs.oasis-open.org/odata/ns/edm"`,
	} {
		if !strings.Contains(text, kept) {
			t.Errorf("%q is not in the trimmed description:\n%s", kept, text)
		}
	}
	// the navigation property and the binding to a type that was not kept are gone, and the one to a kept type stays. A complex type can have navigation
	// properties too (userPrint in Graph has them).
	if strings.Contains(text, `Target="parts"`) {
		t.Error("a binding to a set that is not kept")
	}
	if strings.Contains(text, "nearestPart") || !strings.Contains(text, `<NavigationProperty Name="nearestPerson" Type="microsoft.graph.person" />`) {
		t.Errorf("the navigation properties of a complex type:\n%s", text)
	}
}

// A type that a kept type needs and the description does not have is an error: an alias that was not understood must not make a description with a hole.
func TestTrimMetadata_Errors(t *testing.T) {
	if _, err := trimMetadata([]byte(csdlFixture), []string{"nothing"}, nil); err == nil || !strings.Contains(err.Error(), "nothing") {
		t.Errorf("a root that is not there: %v", err)
	}
	broken := strings.Replace(csdlFixture, `Type="graph.color"`, `Type="graph.missingColor"`, 1)
	if _, err := trimMetadata([]byte(broken), []string{"thing"}, nil); err == nil || !strings.Contains(err.Error(), "missingColor") {
		t.Errorf("a property type that is not there: %v", err)
	}
	if _, err := trimMetadata([]byte("<not-xml"), []string{"thing"}, nil); err == nil {
		t.Error("not XML")
	}
}

// A second schema with a type that a root needs is kept, in its own schema.
func TestTrimMetadata_SeveralSchemas(t *testing.T) {
	withSecurity := strings.Replace(csdlFixture, `<Property Name="name" Type="Edm.String" />`, `<Property Name="name" Type="Edm.String" /><Property Name="alert" Type="microsoft.graph.security.flag" />`, 1)
	withSecurity = strings.Replace(withSecurity, `<EntityType Name="alert" BaseType="graph.entity"><Property Name="title" Type="Edm.String" /></EntityType>`,
		`<EnumType Name="flag"><Member Name="on" Value="0" /></EnumType><EntityType Name="alert" BaseType="graph.entity" />`, 1)
	doc, err := trimMetadata([]byte(withSecurity), []string{"thing"}, []string{"things"})
	if err != nil {
		t.Fatal(err)
	}
	got := names(t, doc)
	if !got["enum microsoft.graph.security.flag"] || got["entity microsoft.graph.security.alert"] {
		t.Errorf("the second schema keeps the type that is needed, and only that: %v", got)
	}
}
