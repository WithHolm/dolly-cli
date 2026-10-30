package repo

import (
	"slices"
	"strings"
)

// The apps of the tenant (service principals), and the roles that users and
// groups have on them. The data side of api/apps.go. It satisfies api.ServicePrincipalsBackend.

// AppRole is a role of a service principal: an application permission, or a role for users.
type AppRole struct {
	ID   string `json:"id"`
	Name string `json:"displayName"`
}

// AppScope is a delegated permission that a service principal gives.
type AppScope struct {
	ID          string `json:"id"`
	Value       string `json:"value"`
	Name        string `json:"displayName"`
	Description string `json:"description,omitempty"`
}

// ServicePrincipal is an app in the tenant. An enterprise application has the
// tag that Entra puts on it.
type ServicePrincipal struct {
	ID         string    `json:"id"`
	Name       string    `json:"displayName"`
	AppID      string    `json:"appId"`
	Enterprise bool      `json:"enterprise"`
	Roles      []AppRole `json:"appRoles"`

	// What the service principal gives to other service principals (grants.go): a resource has
	// roles (application permissions) or scopes (delegated permissions). Scopes and identifier URIs
	// are the namespace of the permissions: api://<id>/Files.Read.
	Scopes         []AppScope `json:"scopes,omitempty"`
	IdentifierURIs []string   `json:"identifierUris,omitempty"`
	// Builtin names a service principal whose roles and scopes are not in the data but in the
	// catalog (package permissions): graph is Microsoft Graph.
	Builtin string `json:"builtin,omitempty"`

	// What Dolly knows about the service principal apart from Graph (principal.go). None of it
	// is in the Graph object, except the tag dolly:auto.
	Auto bool       `json:"auto,omitempty"`       // made by the sign-in API
	Mode string     `json:"mode,omitempty"`       // open (when empty) or enforce
	Cred *CredState `json:"credential,omitempty"` // any (when empty), armed, or captured

	// System marks a service principal that Dolly makes for itself (the portal). The page shows the tag "built in". It changes
	// nothing else: Builtin is the one that says where the permissions are.
	System bool `json:"system,omitempty"`

	// How the Graph API treats the calls of this client (see FlagExemptFromThrottle and FlagHideCalls).
	ExemptFromThrottle bool `json:"exemptFromThrottle,omitempty"`
	HideCalls          bool `json:"hideCalls,omitempty"`
}

// Assignment gives a user or group a role on an app.
type Assignment struct {
	ResourceID    string `json:"resourceId"`
	PrincipalID   string `json:"principalId"`
	PrincipalType string `json:"principalType"` // User, Group, or ServicePrincipal
	RoleID        string `json:"appRoleId"`
}

// DefaultAccess is the role ID that means "no role" (Default access).
const DefaultAccess = "00000000-0000-0000-0000-000000000000"

// EnterpriseTag is the tag of an enterprise application.
const EnterpriseTag = "WindowsAzureActiveDirectoryIntegratedApp"

// object gives the app as a Graph object.
// object gives the service principal as a Graph object. Microsoft Graph has more than a thousand
// roles and scopes, so a list leaves them out and only the call for one (full) has them.
func (s *ServicePrincipal) object(full bool) Object {
	tags := []string{}
	if s.Enterprise {
		tags = append(tags, EnterpriseTag)
	}
	if s.Auto {
		tags = append(tags, "dolly:auto")
	}
	o := Object{
		"@odata.type":           typeSP,
		"id":                    s.ID,
		"displayName":           s.Name,
		"appId":                 s.AppID,
		"tags":                  tags,
		"servicePrincipalNames": append(slices.Clone(s.IdentifierURIs), s.AppID),
	}
	if s.Builtin == BuiltinGraph {
		if full {
			o["appRoles"], o["oauth2PermissionScopes"] = graphAppRoles(), graphScopes()
		}
		return o
	}
	roles := make([]Object, 0, len(s.Roles))
	for _, r := range s.Roles {
		roles = append(roles, Object{"id": r.ID, "displayName": r.Name, "value": strings.ToLower(r.Name), "isEnabled": true})
	}
	o["appRoles"] = roles
	if len(s.Scopes) > 0 {
		scopes := make([]Object, 0, len(s.Scopes))
		for _, c := range s.Scopes {
			scopes = append(scopes, Object{"id": c.ID, "value": c.Value, "type": "Admin", "isEnabled": true,
				"adminConsentDisplayName": c.Name, "adminConsentDescription": c.Description})
		}
		o["oauth2PermissionScopes"] = scopes
	}
	return o
}

// graphAppRoles and graphScopes give the application and the delegated permissions of Microsoft
// Graph as Graph writes them in a service principal.
func graphAppRoles() []Object {
	list := catalogOnce().Application
	out := make([]Object, 0, len(list))
	for _, p := range list {
		out = append(out, Object{"id": p.ID, "value": p.Value, "displayName": p.AdminConsentDisplayName, "description": p.AdminConsentDescription,
			"isEnabled": p.IsEnabled, "origin": "Application", "allowedMemberTypes": []string{"Application"}})
	}
	return out
}

func graphScopes() []Object {
	list := catalogOnce().Delegated
	out := make([]Object, 0, len(list))
	for _, p := range list {
		kind := "User"
		if p.IsAdmin {
			kind = "Admin"
		}
		out = append(out, Object{"id": p.ID, "value": p.Value, "type": kind, "isEnabled": p.IsEnabled,
			"adminConsentDisplayName": p.AdminConsentDisplayName, "adminConsentDescription": p.AdminConsentDescription,
			"userConsentDisplayName": p.ConsentDisplayName, "userConsentDescription": p.ConsentDescription})
	}
	return out
}

// ListServicePrincipals gives every app, in the order of the data.
func (w *World) ListServicePrincipals() []Object {
	w.mu.RLock()
	defer w.mu.RUnlock()
	items := make([]Object, len(w.SPs))
	for i, sp := range w.SPs {
		items[i] = sp.object(false)
	}
	return items
}

// GetServicePrincipal gives one app as an object.
func (w *World) GetServicePrincipal(id string) (Object, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	sp := w.sps[strings.ToLower(id)]
	if sp == nil {
		return nil, false
	}
	return sp.object(true), true
}

// AppRoleAssignedTo lists who is assigned to an app. The bool is false when there
// is no such app.
func (w *World) AppRoleAssignedTo(appID string) ([]Object, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	id := strings.ToLower(appID)
	if w.sps[id] == nil {
		return nil, false
	}
	var items []Object
	for _, a := range w.Assignments {
		if isID(a.ResourceID, id) {
			items = append(items, Object{
				"id": a.PrincipalID + "." + a.RoleID, "resourceId": a.ResourceID, "principalId": a.PrincipalID,
				"principalType": a.PrincipalType, "principalDisplayName": w.principalName(a.PrincipalID), "appRoleId": a.RoleID,
			})
		}
	}
	return items, true
}

// principalName is the display name of a user, a group, or a service principal. Call it with the
// lock held.
func (w *World) principalName(id string) string {
	_, _, name, _ := w.principal(id)
	return name
}
