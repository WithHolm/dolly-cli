// Package permissions is the catalog of the permissions of Microsoft Graph: the names, the IDs, and
// the descriptions of the delegated permissions (scopes) and the application permissions (app roles)
// that the service principal Microsoft Graph has in a real tenant. The data is raw/ (see README.md),
// read as it is. Load refuses a field that these types do not have.
//
// This is the catalog of one resource, Microsoft Graph. A grant to another resource (an API that
// one service principal exposes to another) uses the same shapes, but its permissions are made
// by a person, not read from here.
package permissions

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

// Microsoft Graph as a resource.
const (
	GraphAppID         = "00000003-0000-0000-c000-000000000000" // the application (client) ID of the service principal
	GraphIdentifierURI = "https://graph.microsoft.com"          // the namespace of the scopes: https://graph.microsoft.com/User.Read
	GraphName          = "Microsoft Graph"
)

//go:embed raw/permissions-descriptions.json
var rawData []byte

//go:embed raw/SOURCE.json
var rawSource []byte

// Kind is the kind of a permission.
type Kind string

const (
	// Delegated permissions are scopes. An app has them on behalf of a signed-in user, and a user or an
	// administrator consents to them.
	Delegated Kind = "delegated"
	// Application permissions are app roles. An app has them with no user, and an administrator grants them.
	Application Kind = "application"
)

// Permission is one permission. A text that the source does not give (the application permissions
// have no consent texts for a user) is empty.
type Permission struct {
	ID                      string   `json:"id"`
	Value                   string   `json:"value"` // User.Read
	IsAdmin                 bool     `json:"isAdmin"`
	IsEnabled               bool     `json:"isEnabled"`
	AdminConsentDisplayName string   `json:"adminConsentDisplayName"`
	AdminConsentDescription string   `json:"adminConsentDescription"`
	ConsentDisplayName      string   `json:"consentDisplayName"`
	ConsentDescription      string   `json:"consentDescription"`
	AllowedMemberTypes      []string `json:"allowedMemberTypes,omitempty"` // application permissions: ["Application"]

	Kind Kind `json:"-"`
}

// Scope is the name of the permission as a client asks for it: the namespace of the resource, a
// slash, and the value (https://graph.microsoft.com/User.Read).
func (p Permission) Scope() string { return GraphIdentifierURI + "/" + p.Value }

// Source says where the catalog is from (SOURCE.json).
type Source struct {
	Commit      string `json:"commit"`
	CommitDate  string `json:"commitDate"`
	FetchedDate string `json:"fetchedDate"`
	License     string `json:"license"`
	File        struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	} `json:"file"`
	Delegated   int `json:"delegated"`
	Application int `json:"application"`
}

// Catalog is the list of the permissions.
type Catalog struct {
	Delegated   []Permission `json:"delegatedScopesList"`
	Application []Permission `json:"applicationScopesList"`
	Source      Source       `json:"-"`

	byValue map[string]*Permission
	byID    map[string]*Permission
}

func key(k Kind, s string) string { return string(k) + "|" + strings.ToLower(s) }

// Parse reads the file of Microsoft. It refuses a field that the types do not have, a permission with
// no ID or value, and a value or an ID that is twice in a kind. (One ID is in both kinds: User.Export.All.)
func Parse(data []byte) (*Catalog, error) {
	var c Catalog
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("permissions: %w", err)
	}
	c.byValue, c.byID = map[string]*Permission{}, map[string]*Permission{}
	for k, list := range map[Kind][]Permission{Delegated: c.Delegated, Application: c.Application} {
		for i := range list {
			p := &list[i]
			p.Kind = k
			if p.ID == "" || p.Value == "" {
				return nil, fmt.Errorf("permissions: a %s permission has no ID or no value: %+v", k, *p)
			}
			if c.byValue[key(k, p.Value)] != nil || c.byID[key(k, p.ID)] != nil {
				return nil, fmt.Errorf("permissions: the %s permission %s (%s) is twice", k, p.Value, p.ID)
			}
			c.byValue[key(k, p.Value)], c.byID[key(k, p.ID)] = p, p
		}
	}
	return &c, nil
}

// Load reads the catalog that is in the program.
func Load() (*Catalog, error) {
	c, err := Parse(rawData)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(rawSource, &c.Source); err != nil {
		return nil, fmt.Errorf("permissions: SOURCE.json: %w", err)
	}
	return c, nil
}

// Find gives the permission with this value, in any case: user.read finds User.Read.
func (c *Catalog) Find(k Kind, value string) (Permission, bool) {
	if p := c.byValue[key(k, value)]; p != nil {
		return *p, true
	}
	return Permission{}, false
}

// ByID gives the permission with this ID.
func (c *Catalog) ByID(k Kind, id string) (Permission, bool) {
	if p := c.byID[key(k, id)]; p != nil {
		return *p, true
	}
	return Permission{}, false
}

// ParseScope reads a scope as a client writes it. https://graph.microsoft.com/User.Read and
// User.Read (with no namespace, as for Microsoft Graph) both give the value User.Read. The
// namespace is not checked here: it names the resource, and the caller knows which resources there are.
// For .default, the value is ".default".
func ParseScope(scope string) (namespace, value string) {
	if i := strings.LastIndex(scope, "/"); i >= 0 {
		return scope[:i], scope[i+1:]
	}
	return "", scope
}
