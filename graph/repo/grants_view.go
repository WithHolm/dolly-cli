package repo

import (
	"slices"
	"strings"
	"sync"

	"github.com/withholm/dolly-cli/graph/permissions"
)

// What the admin API and the page need about permissions: the resources, the permissions of a
// resource to pick from, what one service principal has, and the two changes in one call each.

// ResourceInfo is a service principal that has permissions to give.
type ResourceInfo struct {
	ID          string `json:"id"`
	AppID       string `json:"appId"`
	Name        string `json:"displayName"`
	Namespace   string `json:"namespace"` // the start of its scopes: https://graph.microsoft.com
	Builtin     string `json:"builtin,omitempty"`
	Application int    `json:"application"` // how many application permissions (roles) it has
	Delegated   int    `json:"delegated"`   // how many delegated permissions (scopes) it has
}

// PermissionInfo is a permission of a resource, with the scope that a client writes to ask for it.
type PermissionInfo struct {
	ID          string `json:"id"`
	Value       string `json:"value"`
	Name        string `json:"displayName"`
	Description string `json:"description"`
	Admin       bool   `json:"admin"` // only an administrator can consent to it
	// Scope is what the client asks for. A delegated permission is asked for by name:
	// https://graph.microsoft.com/User.Read. Application permissions cannot be asked for one by one:
	// the client asks for .default, and the token has the roles that are assigned.
	Scope string `json:"scope"`
}

// AppGrant is an application permission that a service principal has.
type AppGrant struct {
	AssignmentID  string `json:"assignmentId"`
	ResourceID    string `json:"resourceId"`
	ResourceAppID string `json:"resourceAppId"`
	ResourceName  string `json:"resourceName"`
	PermissionInfo
}

// DelegatedGrant is a consent: delegated permissions of a resource that a service principal has.
type DelegatedGrant struct {
	GrantID       string           `json:"grantId"`
	ResourceID    string           `json:"resourceId"`
	ResourceAppID string           `json:"resourceAppId"`
	ResourceName  string           `json:"resourceName"`
	ConsentType   string           `json:"consentType"`
	PrincipalID   string           `json:"principalId,omitempty"`
	PrincipalName string           `json:"principalName,omitempty"`
	Permissions   []PermissionInfo `json:"permissions"`
	// Scope is the value of the scope parameter that asks for all of them.
	Scope string `json:"scope"`
}

// GrantsInfo is what one service principal has.
type GrantsInfo struct {
	Application []AppGrant       `json:"application"`
	Delegated   []DelegatedGrant `json:"delegated"`
}

// sorted lists of the catalog, by value
var graphSorted = sync.OnceValue(func() map[permissions.Kind][]permissions.Permission {
	c := catalogOnce()
	by := func(a, b permissions.Permission) int {
		return strings.Compare(strings.ToLower(a.Value), strings.ToLower(b.Value))
	}
	d, ap := slices.Clone(c.Delegated), slices.Clone(c.Application)
	slices.SortFunc(d, by)
	slices.SortFunc(ap, by)
	return map[permissions.Kind][]permissions.Permission{permissions.Delegated: d, permissions.Application: ap}
})

func (sp *ServicePrincipal) resourceInfo() ResourceInfo {
	r := ResourceInfo{ID: sp.ID, AppID: sp.AppID, Name: sp.Name, Namespace: sp.namespace(), Builtin: sp.Builtin,
		Application: len(sp.Roles), Delegated: len(sp.Scopes)}
	if sp.Builtin == BuiltinGraph {
		r.Application, r.Delegated = len(catalogOnce().Application), len(catalogOnce().Delegated)
	}
	return r
}

func scopeFor(ns string, kind permissions.Kind, value string) string {
	if kind == permissions.Application {
		return ns + "/.default"
	}
	return ns + "/" + value
}

// Resources lists the service principals that have permissions to give: Microsoft Graph first, then the others.
func (w *World) Resources() []ResourceInfo {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var out []ResourceInfo
	for _, sp := range w.SPs {
		if sp.isResource() {
			out = append(out, sp.resourceInfo())
		}
	}
	slices.SortStableFunc(out, func(a, b ResourceInfo) int {
		switch {
		case a.Builtin != "" && b.Builtin == "":
			return -1
		case a.Builtin == "" && b.Builtin != "":
			return 1
		}
		return 0
	})
	return out
}

func permissionInfo(ns string, kind permissions.Kind, id, value, name, desc string, admin bool) PermissionInfo {
	return PermissionInfo{ID: id, Value: value, Name: name, Description: desc, Admin: admin, Scope: scopeFor(ns, kind, value)}
}

// ResourcePermissions lists the permissions of a resource of one kind, for a person to pick from. query
// keeps the ones that have the text in the value or the name (any case). limit cuts the list (0 is all);
// the number gives how many there are before the cut. The bool is false when there is no such resource.
func (w *World) ResourcePermissions(resourceAppID string, kind permissions.Kind, query string, limit int) ([]PermissionInfo, int, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	res := w.spByAppID(resourceAppID)
	if res == nil || !res.isResource() {
		return nil, 0, false
	}
	ns := res.namespace()
	var all []PermissionInfo
	switch {
	case res.Builtin == BuiltinGraph:
		for _, p := range graphSorted()[kind] {
			all = append(all, permissionInfo(ns, kind, p.ID, p.Value, p.AdminConsentDisplayName, p.AdminConsentDescription, p.IsAdmin))
		}
	case kind == permissions.Application:
		for _, r := range res.Roles {
			all = append(all, permissionInfo(ns, kind, r.ID, strings.ToLower(r.Name), r.Name, "", true))
		}
	default:
		for _, s := range res.Scopes {
			all = append(all, permissionInfo(ns, kind, s.ID, s.Value, s.Name, s.Description, true))
		}
	}
	if q := strings.ToLower(strings.TrimSpace(query)); q != "" {
		all = slices.DeleteFunc(all, func(p PermissionInfo) bool {
			return !strings.Contains(strings.ToLower(p.Value), q) && !strings.Contains(strings.ToLower(p.Name), q)
		})
	}
	total := len(all)
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, total, true
}

// GrantsOf gives what the service principal with this application ID has: its application
// permissions (app role assignments) and its consents. The bool is false when there is none.
func (w *World) GrantsOf(appID string) (GrantsInfo, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	client := w.spByAppID(appID)
	if client == nil {
		return GrantsInfo{}, false
	}
	out := GrantsInfo{Application: []AppGrant{}, Delegated: []DelegatedGrant{}}
	for _, a := range w.Assignments {
		res := w.sps[strings.ToLower(a.ResourceID)]
		if !isID(a.PrincipalID, client.ID) || res == nil {
			continue
		}
		r, ok := res.role(a.RoleID)
		if !ok {
			continue
		}
		out.Application = append(out.Application, AppGrant{
			AssignmentID: assignmentID(a), ResourceID: res.ID, ResourceAppID: res.AppID, ResourceName: res.Name,
			PermissionInfo: permissionInfo(res.namespace(), permissions.Application, r.ID, r.Value, r.Name, r.Description, true),
		})
	}
	for _, g := range w.Grants {
		res := w.sps[strings.ToLower(g.ResourceID)]
		if !isID(g.ClientID, client.ID) || res == nil {
			continue
		}
		d := DelegatedGrant{GrantID: g.ID, ResourceID: res.ID, ResourceAppID: res.AppID, ResourceName: res.Name,
			ConsentType: g.ConsentType, PrincipalID: g.PrincipalID, PrincipalName: w.principalName(g.PrincipalID), Permissions: []PermissionInfo{}}
		var values []string
		for _, v := range strings.Fields(g.Scope) {
			if s, ok := res.scope(v); ok {
				d.Permissions = append(d.Permissions, permissionInfo(res.namespace(), permissions.Delegated, s.ID, s.Value, s.Name, s.Description, s.Admin))
				values = append(values, d.Permissions[len(d.Permissions)-1].Scope)
			}
		}
		d.Scope = strings.Join(values, " ")
		out.Delegated = append(out.Delegated, d)
	}
	return out, true
}

// GrantPermission gives a service principal one permission of a resource, as an administrator does.
// An application permission becomes an app role assignment. A delegated permission goes into the
// consent for all users of the client and the resource, which is made when there is none.
func (w *World) GrantPermission(clientAppID, resourceAppID string, kind permissions.Kind, value string, a Actor) *apiError {
	w.mu.RLock()
	client, res := w.spByAppID(clientAppID), w.spByAppID(resourceAppID)
	var roleID, canonical, grantID, scope string
	var found bool
	if client != nil && res != nil {
		if kind == permissions.Application {
			for _, r := range roleList(res) {
				if strings.EqualFold(r.Value, value) {
					roleID, found = r.ID, true
					break
				}
			}
		} else if s, ok := res.scope(value); ok {
			canonical, found = s.Value, true
			for _, g := range w.Grants {
				if isID(g.ClientID, client.ID) && isID(g.ResourceID, res.ID) && g.ConsentType == ConsentAll {
					grantID, scope = g.ID, g.Scope
				}
			}
		}
	}
	w.mu.RUnlock()
	switch {
	case client == nil:
		return notFound("servicePrincipal", clientAppID)
	case res == nil:
		return notFound("servicePrincipal", resourceAppID)
	case !found:
		return badRequest("The permission '%s' was not found on the resource '%s'.", value, res.Name)
	case kind == permissions.Application:
		_, e := w.AssignAppRole(res.ID, client.ID, roleID, a)
		return e
	case grantID == "":
		_, e := w.CreateOAuth2Grant(map[string]any{"clientId": client.ID, "resourceId": res.ID, "consentType": ConsentAll, "scope": canonical}, a)
		return e
	case slices.ContainsFunc(strings.Fields(scope), func(v string) bool { return strings.EqualFold(v, canonical) }):
		return badRequest("Permission being assigned already exists on the object.")
	}
	return w.UpdateOAuth2Grant(grantID, map[string]any{"scope": scope + " " + canonical}, a)
}

// RevokePermission takes one permission of a resource away from a service principal. It is the opposite of GrantPermission.
func (w *World) RevokePermission(clientAppID, resourceAppID string, kind permissions.Kind, value string, a Actor) *apiError {
	w.mu.RLock()
	client, res := w.spByAppID(clientAppID), w.spByAppID(resourceAppID)
	var assignment, grantID, rest string
	var found bool
	if client != nil && res != nil {
		if kind == permissions.Application {
			for _, x := range w.Assignments {
				if r, ok := res.role(x.RoleID); ok && isID(x.PrincipalID, client.ID) && isID(x.ResourceID, res.ID) && strings.EqualFold(r.Value, value) {
					assignment, found = assignmentID(x), true
				}
			}
		} else {
			for _, g := range w.Grants {
				if isID(g.ClientID, client.ID) && isID(g.ResourceID, res.ID) && g.ConsentType == ConsentAll {
					var keep []string
					for _, v := range strings.Fields(g.Scope) {
						if strings.EqualFold(v, value) {
							found = true
						} else {
							keep = append(keep, v)
						}
					}
					if found {
						grantID, rest = g.ID, strings.Join(keep, " ")
						break
					}
				}
			}
		}
	}
	w.mu.RUnlock()
	switch {
	case client == nil:
		return notFound("servicePrincipal", clientAppID)
	case res == nil:
		return notFound("servicePrincipal", resourceAppID)
	case !found:
		return badRequest("The service principal does not have the permission '%s' of '%s'.", value, res.Name)
	case kind == permissions.Application:
		return w.DeleteAppRoleAssignment(client.ID, assignment, false, a)
	case rest == "":
		return w.DeleteOAuth2Grant(grantID, a)
	}
	return w.UpdateOAuth2Grant(grantID, map[string]any{"scope": rest}, a)
}

// roleList gives the roles of a service principal as roleInfo. For Microsoft Graph that is the catalog.
func roleList(sp *ServicePrincipal) []roleInfo {
	if sp.Builtin == BuiltinGraph {
		list := catalogOnce().Application
		out := make([]roleInfo, len(list))
		for i, p := range list {
			out[i] = roleInfo{p.ID, p.Value, p.AdminConsentDisplayName, p.AdminConsentDescription}
		}
		return out
	}
	out := make([]roleInfo, len(sp.Roles))
	for i, r := range sp.Roles {
		out[i] = roleInfo{r.ID, strings.ToLower(r.Name), r.Name, ""}
	}
	return out
}
