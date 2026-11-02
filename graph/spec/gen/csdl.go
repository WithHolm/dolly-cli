package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// The OData description of Graph (CSDL, the XML that /$metadata gives) is 3.4 MB for v1.0, with thousands of types that dollycli does not have. trimMetadata keeps
// the part that dollycli serves: the types that the roots need (their base types and the types of their properties, as far as they go), the navigation
// properties that point at a type that is kept, and the entity sets and singletons that dollycli has. Everything else is left out: the other types, the
// actions and functions (dollycli serves few of them, and a description that lists them would promise calls that it does not answer), and the annotations.
//
// The description of Microsoft writes the types with an alias (graph.user). The real Graph writes the full name (microsoft.graph.user), and so does the
// result. The source is msgraph-metadata, clean_v10_metadata/cleanMetadata.xml (MIT).

const (
	nsEdmx = "http://docs.oasis-open.org/odata/ns/edmx"
	nsEdm  = "http://docs.oasis-open.org/odata/ns/edm"
)

// node is an element of the description, with the attributes that are not namespace declarations.
type node struct {
	space string
	name  string
	attrs []xml.Attr
	kids  []*node
}

func (n *node) attr(name string) string {
	for _, a := range n.attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func (n *node) setAttr(name, value string) {
	for i, a := range n.attrs {
		if a.Name.Local == name {
			n.attrs[i].Value = value
			return
		}
	}
	n.attrs = append(n.attrs, xml.Attr{Name: xml.Name{Local: name}, Value: value})
}

func (n *node) children(name string) []*node {
	var out []*node
	for _, k := range n.kids {
		if k.name == name {
			out = append(out, k)
		}
	}
	return out
}

func parseNodes(data []byte) (*node, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var stack []*node
	var root *node
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("the description is not XML: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &node{space: t.Name.Space, name: t.Name.Local}
			for _, a := range t.Attr {
				if a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns") {
					continue
				}
				n.attrs = append(n.attrs, a)
			}
			if len(stack) > 0 {
				p := stack[len(stack)-1]
				p.kids = append(p.kids, n)
			} else {
				root = n
			}
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		}
	}
	if root == nil {
		return nil, fmt.Errorf("the description has no elements")
	}
	return root, nil
}

// find gives the first descendant with this name, in the order of the document.
func (n *node) find(name string) *node {
	if n.name == name {
		return n
	}
	for _, k := range n.kids {
		if f := k.find(name); f != nil {
			return f
		}
	}
	return nil
}

// resolver turns the aliases of the schemas into the namespaces, and a type name into the qualified name of the type without Collection( ).
type resolver map[string]string

func (r resolver) full(t string) string {
	for alias, ns := range r {
		if strings.HasPrefix(t, alias+".") {
			return ns + t[len(alias):]
		}
	}
	return t
}

// typeOf gives the qualified name that a type attribute is about: Collection(graph.user) is microsoft.graph.user. A name of the Edm namespace is "".
func (r resolver) typeOf(t string) string {
	if inner, ok := strings.CutPrefix(t, "Collection("); ok {
		t = strings.TrimSuffix(inner, ")")
	}
	if strings.HasPrefix(t, "Edm.") {
		return ""
	}
	return r.full(t)
}

// rewrite writes a type attribute with the full namespace, keeping Collection( ).
func (r resolver) rewrite(t string) string {
	if inner, ok := strings.CutPrefix(t, "Collection("); ok {
		return "Collection(" + r.rewrite(strings.TrimSuffix(inner, ")")) + ")"
	}
	if strings.HasPrefix(t, "Edm.") {
		return t
	}
	return r.full(t)
}

// typeAttrs are the attributes that hold the name of a type.
var typeAttrs = []string{"Type", "BaseType", "EntityType", "UnderlyingType"}

func (r resolver) rewriteAll(n *node) {
	for _, a := range typeAttrs {
		if v := n.attr(a); v != "" {
			n.setAttr(a, r.rewrite(v))
		}
	}
	for _, k := range n.kids {
		r.rewriteAll(k)
	}
}

// dropAnnotations takes the annotations (the descriptions and the capabilities) out of an element and of its children.
func dropAnnotations(n *node) {
	kids := n.kids[:0]
	for _, k := range n.kids {
		if k.name == "Annotation" || k.name == "Annotations" {
			continue
		}
		dropAnnotations(k)
		kids = append(kids, k)
	}
	n.kids = kids
}

// trimMetadata makes the description that dollycli serves from the description of Microsoft. roots are the names of the types of microsoft.graph that dollycli
// has (user, group, ...), and sets the names of the entity sets and singletons of the container that it has (users, groups, me, ...).
func trimMetadata(data []byte, roots, sets []string) ([]byte, error) {
	doc, err := parseNodes(data)
	if err != nil {
		return nil, err
	}
	services := doc.find("DataServices")
	if services == nil {
		return nil, fmt.Errorf("the description has no DataServices")
	}
	schemas := services.children("Schema")
	r := resolver{}
	types := map[string]*node{}
	for _, s := range schemas {
		ns := s.attr("Namespace")
		if alias := s.attr("Alias"); alias != "" {
			r[alias] = ns
		}
		for _, k := range s.kids {
			switch k.name {
			case "EntityType", "ComplexType", "EnumType":
				types[ns+"."+k.attr("Name")] = k
			}
		}
	}

	// the types that the roots need: the base types, and the types of the properties
	keep := map[string]bool{}
	var walk func(q, why string) error
	walk = func(q, why string) error {
		if q == "" || keep[q] {
			return nil
		}
		t, ok := types[q]
		if !ok {
			return fmt.Errorf("the type %s (%s) is not in the description", q, why)
		}
		keep[q] = true
		if b := t.attr("BaseType"); b != "" {
			if err := walk(r.typeOf(b), "the base type of "+q); err != nil {
				return err
			}
		}
		for _, p := range t.children("Property") {
			if err := walk(r.typeOf(p.attr("Type")), "the property "+p.attr("Name")+" of "+q); err != nil {
				return err
			}
		}
		return nil
	}
	for _, name := range roots {
		if err := walk("microsoft.graph."+name, "a root"); err != nil {
			return nil, err
		}
	}

	wantedSets := map[string]bool{}
	for _, s := range sets {
		wantedSets[s] = true
	}
	// the entity sets and singletons that are kept: the name is wanted, and the type is kept
	keptSets := map[string]bool{}
	for _, s := range schemas {
		for _, c := range s.children("EntityContainer") {
			for _, e := range c.kids {
				if (e.name == "EntitySet" || e.name == "Singleton") && wantedSets[e.attr("Name")] {
					typ := e.attr("EntityType")
					if typ == "" {
						typ = e.attr("Type")
					}
					if keep[r.typeOf(typ)] {
						keptSets[e.attr("Name")] = true
					}
				}
			}
		}
	}

	// build the result: the same schemas, with only the kept types, the full names, and no annotations
	out := &node{space: nsEdmx, name: "Edmx", attrs: []xml.Attr{{Name: xml.Name{Local: "Version"}, Value: "4.0"}}}
	data2 := &node{space: nsEdmx, name: "DataServices"}
	out.kids = []*node{data2}
	for _, s := range schemas {
		ns := s.attr("Namespace")
		ns2 := &node{space: nsEdm, name: "Schema", attrs: []xml.Attr{{Name: xml.Name{Local: "Namespace"}, Value: ns}}}
		for _, k := range s.kids {
			switch k.name {
			case "EnumType", "ComplexType", "EntityType":
				if !keep[ns+"."+k.attr("Name")] {
					continue
				}
				dropAnnotations(k)
				if k.name == "EntityType" || k.name == "ComplexType" { // a complex type can have navigation properties too
					navs := k.kids[:0]
					for _, c := range k.kids {
						// a navigation property to a type that is not kept is left out
						if c.name == "NavigationProperty" && !keep[r.typeOf(c.attr("Type"))] {
							continue
						}
						navs = append(navs, c)
					}
					k.kids = navs
				}
				r.rewriteAll(k)
				ns2.kids = append(ns2.kids, k)
			case "EntityContainer":
				c := &node{space: nsEdm, name: "EntityContainer", attrs: k.attrs}
				for _, e := range k.kids {
					if (e.name != "EntitySet" && e.name != "Singleton") || !keptSets[e.attr("Name")] {
						continue
					}
					dropAnnotations(e)
					bindings := e.kids[:0]
					for _, b := range e.kids {
						target, _, _ := strings.Cut(b.attr("Target"), "/")
						if b.name == "NavigationPropertyBinding" && !keptSets[target] {
							continue
						}
						bindings = append(bindings, b)
					}
					e.kids = bindings
					r.rewriteAll(e)
					c.kids = append(c.kids, e)
				}
				if len(c.kids) > 0 {
					ns2.kids = append(ns2.kids, c)
				}
			}
		}
		if len(ns2.kids) > 0 {
			data2.kids = append(data2.kids, ns2)
		}
	}
	var b bytes.Buffer
	b.WriteString(xml.Header)
	writeNode(&b, out, 0)
	return b.Bytes(), nil
}

// writeNode writes an element: the edmx namespace has the prefix, the Edm namespace is the default one (declared on each Schema, as the real Graph does).
func writeNode(b *bytes.Buffer, n *node, depth int) {
	indent := strings.Repeat("  ", depth)
	name := n.name
	if n.space == nsEdmx {
		name = "edmx:" + name
	}
	b.WriteString(indent + "<" + name)
	for _, a := range n.attrs {
		b.WriteString(" " + a.Name.Local + `="`)
		_ = xml.EscapeText(b, []byte(a.Value))
		b.WriteString(`"`)
	}
	if n.name == "Edmx" {
		b.WriteString(` xmlns:edmx="` + nsEdmx + `"`)
	}
	if n.name == "Schema" {
		b.WriteString(` xmlns="` + nsEdm + `"`)
	}
	if len(n.kids) == 0 {
		b.WriteString(" />\n")
		return
	}
	b.WriteString(">\n")
	for _, k := range n.kids {
		writeNode(b, k, depth+1)
	}
	b.WriteString(indent + "</" + name + ">\n")
}
