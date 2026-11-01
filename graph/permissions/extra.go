package permissions

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// A person can add the permissions of their own tenant. The service principal Microsoft Graph of a tenant lists all of its permissions (more than the
// built-in list has), and the scripts in docs/permissions.md save that answer to a file. dollycli does not ship it: it reads the file that the person
// made, adds its permissions to the catalog, and a permission of the file wins over the built-in one with the same value.

var (
	extraMu   sync.Mutex
	extraPath string
)

// SetExtra says which file Load adds to the catalog. An empty path adds nothing. Call it before the first Load.
func SetExtra(path string) {
	extraMu.Lock()
	defer extraMu.Unlock()
	extraPath = path
}

// Extra says what Load added from the file of the person.
type Extra struct {
	Path        string    `json:"path"`
	Date        time.Time `json:"date"` // when the file was written
	Delegated   int       `json:"delegated"`
	Application int       `json:"application"`
}

// tenantRole and tenantScope are the app roles and scopes as the service principal of a tenant has them (Graph: appRole, permissionScope).
type tenantRole struct {
	ID                 string   `json:"id"`
	Value              string   `json:"value"`
	DisplayName        string   `json:"displayName"`
	Description        string   `json:"description"`
	IsEnabled          *bool    `json:"isEnabled"`
	AllowedMemberTypes []string `json:"allowedMemberTypes"`
}

type tenantScope struct {
	ID                      string `json:"id"`
	Value                   string `json:"value"`
	Type                    string `json:"type"` // Admin or User: who can consent
	IsEnabled               *bool  `json:"isEnabled"`
	AdminConsentDisplayName string `json:"adminConsentDisplayName"`
	AdminConsentDescription string `json:"adminConsentDescription"`
	UserConsentDisplayName  string `json:"userConsentDisplayName"`
	UserConsentDescription  string `json:"userConsentDescription"`
}

type tenantFile struct {
	AppRoles []tenantRole  `json:"appRoles"`
	Scopes   []tenantScope `json:"oauth2PermissionScopes"`
	Value    []struct {
		AppRoles []tenantRole  `json:"appRoles"`
		Scopes   []tenantScope `json:"oauth2PermissionScopes"`
	} `json:"value"` // the answer of a $filter: a list with one service principal
}

// ParseTenant reads the answer of GET /servicePrincipals(appId='00000003-0000-0000-c000-000000000000')?$select=appRoles,oauth2PermissionScopes, or the
// same with $filter (a list of one). The permissions come out in the shape of the catalog. A permission with no ID or no value is an error.
func ParseTenant(data []byte) (delegated, application []Permission, err error) {
	var f tenantFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, nil, fmt.Errorf("not JSON that Graph answered: %w", err)
	}
	roles, scopes := f.AppRoles, f.Scopes
	if len(f.Value) > 0 {
		roles, scopes = f.Value[0].AppRoles, f.Value[0].Scopes
	}
	if len(roles) == 0 && len(scopes) == 0 {
		return nil, nil, fmt.Errorf("it has no appRoles and no oauth2PermissionScopes: save the answer of the service principal Microsoft Graph")
	}
	enabled := func(b *bool) bool { return b == nil || *b }
	for _, r := range roles {
		if r.ID == "" || r.Value == "" {
			return nil, nil, fmt.Errorf("an app role has no id or no value: %+v", r)
		}
		application = append(application, Permission{
			ID: r.ID, Value: r.Value, IsAdmin: true, IsEnabled: enabled(r.IsEnabled),
			AdminConsentDisplayName: r.DisplayName, AdminConsentDescription: r.Description,
			AllowedMemberTypes: r.AllowedMemberTypes,
		})
	}
	for _, s := range scopes {
		if s.ID == "" || s.Value == "" {
			return nil, nil, fmt.Errorf("a scope has no id or no value: %+v", s)
		}
		delegated = append(delegated, Permission{
			ID: s.ID, Value: s.Value, IsAdmin: strings.EqualFold(s.Type, "Admin"), IsEnabled: enabled(s.IsEnabled),
			AdminConsentDisplayName: s.AdminConsentDisplayName, AdminConsentDescription: s.AdminConsentDescription,
			ConsentDisplayName: s.UserConsentDisplayName, ConsentDescription: s.UserConsentDescription,
		})
	}
	return delegated, application, nil
}

// addExtra adds the file of the person, when SetExtra was called. A permission of the file replaces the one of the catalog with the same value.
func (c *Catalog) addExtra() error {
	extraMu.Lock()
	path := extraPath
	extraMu.Unlock()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("permissions: %w", err)
	}
	delegated, application, err := ParseTenant(data)
	if err != nil {
		return fmt.Errorf("permissions: %s: %w", path, err)
	}
	merge := func(list []Permission, extra []Permission) []Permission {
		at := map[string]int{}
		for i, p := range list {
			at[strings.ToLower(p.Value)] = i
		}
		for _, p := range extra {
			if i, ok := at[strings.ToLower(p.Value)]; ok {
				list[i] = p
				continue
			}
			at[strings.ToLower(p.Value)] = len(list)
			list = append(list, p)
		}
		return list
	}
	c.Delegated, c.Application = merge(c.Delegated, delegated), merge(c.Application, application)
	c.Extra = &Extra{Path: path, Delegated: len(delegated), Application: len(application)}
	if st, err := os.Stat(path); err == nil {
		c.Extra.Date = st.ModTime().UTC()
	}
	if err := c.index(); err != nil {
		return fmt.Errorf("permissions: %s: %w", path, err)
	}
	return nil
}
