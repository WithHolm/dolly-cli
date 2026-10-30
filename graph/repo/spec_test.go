package repo

import (
	"sort"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph/permissions"
	"github.com/withholm/dolly-cli/graph/spec"
)

// These tests check the data of dolly against the description of Graph. A
// property that dolly has and Graph does not have would make a tool work here
// and fail in Entra.

// A user can set these properties, but Graph does not list them as read-write, or
// dolly keeps them in another way. Each one has the reason.
var notSettable = map[string]string{
	"passwordProfile": "accepted and not kept: dolly has no passwords",
}

func TestSpec_SettableUserProperties(t *testing.T) {
	props := spec.Load().Schemas["user"]
	for name := range userProps {
		p, ok := props[name]
		switch {
		case !ok:
			t.Errorf("userProps has %q, and Graph has no such property on a user", name)
		case p.ReadOnly && notSettable[name] == "":
			t.Errorf("userProps lets a call set %q, and Graph says it is read-only", name)
		}
	}
}

func TestSpec_SettableGroupProperties(t *testing.T) {
	props := spec.Load().Schemas["group"]
	for name := range groupProps {
		if strings.HasSuffix(name, "@odata.bind") {
			continue // a reference in the body of a create, not a property
		}
		p, ok := props[name]
		switch {
		case !ok:
			t.Errorf("groupProps has %q, and Graph has no such property on a group", name)
		case p.ReadOnly:
			t.Errorf("groupProps lets a call set %q, and Graph says it is read-only", name)
		}
	}
}

// Every property that a call can set on a place exists on that type in Graph.
func TestSpec_PlaceProperties(t *testing.T) {
	s := spec.Load()
	for kind, props := range PlaceTypes {
		schema := s.Schemas[kind]
		if schema == nil {
			t.Errorf("the description has no schema for %s", kind)
			continue
		}
		for name := range props {
			if p, ok := schema[name]; !ok {
				t.Errorf("%s: dolly lets a call set %q, and Graph has no such property", kind, name)
			} else if p.ReadOnly {
				t.Errorf("%s: dolly lets a call set %q, and Graph says it is read-only", kind, name)
			}
		}
	}
	w := Generate(Options{Users: 10, Seed: 2})
	for _, p := range w.Places {
		kind := placeKind(str(p["@odata.type"]))
		for k := range p {
			if _, ok := s.Schemas[kind][k]; !ok {
				t.Errorf("a %s has the property %q, and Graph has not", kind, k)
			}
		}
	}
}

// The data that dolly makes has only properties that Graph has.
func TestSpec_GeneratedObjectsHaveOnlyGraphProperties(t *testing.T) {
	s := spec.Load()
	w := Generate(Options{Users: 60, Seed: 2})

	unknown := map[string]bool{}
	for _, u := range w.Users {
		for k := range u {
			if _, ok := s.Schemas["user"][k]; !ok {
				unknown[k] = true
			}
		}
	}
	for _, g := range w.Groups {
		for k := range g.object() {
			if _, ok := s.Schemas["group"][k]; !ok && k != "@odata.type" {
				unknown["group."+k] = true
			}
		}
	}
	for k := range w.Organization {
		if _, ok := s.Schemas["organization"][k]; !ok {
			unknown["organization."+k] = true
		}
	}
	for _, d := range w.Organization["verifiedDomains"].([]any) {
		for k := range d.(Object) {
			if _, ok := s.Schemas["verifiedDomain"][k]; !ok {
				unknown["verifiedDomain."+k] = true
			}
		}
	}
	for _, sp := range w.SPs {
		for k := range sp.object(true) {
			if _, ok := s.Schemas["servicePrincipal"][k]; !ok && k != "@odata.type" {
				unknown["servicePrincipal."+k] = true
			}
		}
	}
	// consents and app role assignments
	if _, err := w.EnsureServicePrincipal(clientApp, Actor{}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []struct {
		kind  permissions.Kind
		value string
	}{{permissions.Delegated, "User.Read"}, {permissions.Application, "User.Read.All"}} {
		if err := w.GrantPermission(clientApp, permissions.GraphAppID, p.kind, p.value, Actor{}); err != nil {
			t.Fatal(err)
		}
	}
	for _, g := range w.ListOAuth2Grants() {
		for k := range g {
			if _, ok := s.Schemas["oAuth2PermissionGrant"][k]; !ok {
				unknown["oAuth2PermissionGrant."+k] = true
			}
		}
	}
	client, _ := w.FindServicePrincipal(clientApp)
	assignments, _ := w.AppRoleAssignments(client.ID)
	for _, a := range assignments {
		for k := range a {
			if _, ok := s.Schemas["appRoleAssignment"][k]; !ok {
				unknown["appRoleAssignment."+k] = true
			}
		}
	}
	var names []string
	for k := range unknown {
		names = append(names, k)
	}
	sort.Strings(names)
	if len(names) > 0 {
		t.Errorf("dolly makes properties that Graph does not have: %v", names)
	}
}
