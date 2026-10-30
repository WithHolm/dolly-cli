package repo

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/withholm/dolly-cli/graph/permissions"
)

// What a service principal may do with a resource: the permissions that are granted to it.
//
// A resource is a service principal that has permissions to give. Microsoft Graph is one, and its
// permissions are the catalog (package permissions). Another service principal becomes a resource
// when it has roles or scopes: that is how one app uses the API of another.
//
//   - An application permission is an app role of the resource that is assigned to the client
//     service principal (Assignment). The token has it in the claim roles.
//   - A delegated permission is a scope of the resource in a consent of the client (Grant). The
//     token has it in the claim scp.
//
// The data side of api/apps.go and api/oauth2grants.go.

// Events that these make.
const (
	evAssignCreate = "assignment.create"  // an app role was assigned (Assignment)
	evAssignDelete = "assignment.delete"  // an app role assignment is gone (Assignment)
	evGrantCreate  = "oauth2grant.create" // a consent was made (Grant)
	evGrantUpdate  = "oauth2grant.update" // the scopes of a consent changed (Grant has the new ones)
	evGrantDelete  = "oauth2grant.delete" // a consent is gone (Grant)
)

// The types of principal of an assignment.
const (
	PrincipalUser    = "User"
	PrincipalGroup   = "Group"
	PrincipalService = "ServicePrincipal"
)

// The consent types of a grant.
const (
	ConsentAll       = "AllPrincipals" // an administrator consents for all the users
	ConsentPrincipal = "Principal"     // one user consents for himself or herself
)

// Grant is the consent to the delegated permissions of a resource, for a client.
type Grant struct {
	ID          string `json:"id"`
	ClientID    string `json:"clientId"` // the object ID of the client service principal
	ConsentType string `json:"consentType"`
	PrincipalID string `json:"principalId,omitempty"` // the user, when the consent type is Principal
	ResourceID  string `json:"resourceId"`            // the object ID of the resource service principal
	Scope       string `json:"scope"`                 // the values, with a space between
}

var catalogOnce = sync.OnceValue(func() *permissions.Catalog {
	c, err := permissions.Load()
	if err != nil {
		panic("dolly: the catalog of permissions is not valid: " + err.Error()) // the tests of that package find this
	}
	return c
})

// stableGUID makes a GUID from a text, so that the same text always gives the same ID.
func stableGUID(seed string) string {
	b := sha256.Sum256([]byte(seed))
	b[6] = b[6]&0x0f | 0x50
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// ensureGraphServicePrincipal gives a world that has none the service principal Microsoft Graph. It
// has the same ID in every dolly. Its roles and scopes are not kept in the data: they are the catalog.
// Call it after the lookups are made.
func (w *World) ensureGraphServicePrincipal() {
	if w.byAppID[permissions.GraphAppID] != nil {
		return
	}
	w.addServicePrincipal(&ServicePrincipal{
		ID: stableGUID("dolly/servicePrincipal/" + permissions.GraphAppID), AppID: permissions.GraphAppID,
		Name: permissions.GraphName, Builtin: BuiltinGraph, IdentifierURIs: []string{permissions.GraphIdentifierURI},
	})
}

// BuiltinGraph marks the service principal Microsoft Graph.
const BuiltinGraph = "graph"

// roleInfo is an app role, from the data or from the catalog.
type roleInfo struct{ ID, Value, Name, Description string }

// scopeInfo is a delegated permission, from the data or from the catalog.
type scopeInfo struct {
	ID, Value, Name, Description string
	Admin                        bool
}

func (sp *ServicePrincipal) role(id string) (roleInfo, bool) {
	if sp.Builtin == BuiltinGraph {
		if p, ok := catalogOnce().ByID(permissions.Application, id); ok {
			return roleInfo{p.ID, p.Value, p.AdminConsentDisplayName, p.AdminConsentDescription}, true
		}
		return roleInfo{}, false
	}
	for _, r := range sp.Roles {
		if isID(r.ID, id) {
			return roleInfo{r.ID, strings.ToLower(r.Name), r.Name, ""}, true
		}
	}
	return roleInfo{}, false
}

func (sp *ServicePrincipal) scope(value string) (scopeInfo, bool) {
	if sp.Builtin == BuiltinGraph {
		if p, ok := catalogOnce().Find(permissions.Delegated, value); ok {
			return scopeInfo{p.ID, p.Value, p.AdminConsentDisplayName, p.AdminConsentDescription, p.IsAdmin}, true
		}
		return scopeInfo{}, false
	}
	for _, s := range sp.Scopes {
		if strings.EqualFold(s.Value, value) {
			return scopeInfo{s.ID, s.Value, s.Name, s.Description, true}, true
		}
	}
	return scopeInfo{}, false
}

// namespace is the start of the scopes of a resource: its first identifier URI.
func (sp *ServicePrincipal) namespace() string {
	if len(sp.IdentifierURIs) > 0 {
		return sp.IdentifierURIs[0]
	}
	return "api://" + sp.AppID
}

func (sp *ServicePrincipal) isResource() bool {
	return sp.Builtin != "" || len(sp.Roles) > 0 || len(sp.Scopes) > 0
}

// principal finds a user, a group, or a service principal by ID. Call it with the lock held.
func (w *World) principal(id string) (kind, canonicalID, name string, ok bool) {
	id = strings.ToLower(id)
	if u := w.users[id]; u != nil {
		return PrincipalUser, str(u["id"]), str(u["displayName"]), true
	}
	if g := w.groups[id]; g != nil {
		return PrincipalGroup, g.ID, g.Name, true
	}
	if sp := w.sps[id]; sp != nil {
		return PrincipalService, sp.ID, sp.Name, true
	}
	return "", "", "", false
}

func assignmentID(a Assignment) string { return a.PrincipalID + "." + a.RoleID }

// assignmentObject gives an app role assignment as Graph writes it. Call it with the lock held.
func (w *World) assignmentObject(a Assignment) Object {
	o := Object{
		"id": assignmentID(a), "resourceId": a.ResourceID, "principalId": a.PrincipalID,
		"principalType": a.PrincipalType, "principalDisplayName": w.principalName(a.PrincipalID), "appRoleId": a.RoleID,
	}
	if sp := w.sps[strings.ToLower(a.ResourceID)]; sp != nil {
		o["resourceDisplayName"] = sp.Name
	}
	return o
}

// AppRoleAssignments lists the app roles that are assigned to a service principal: what it may
// do with other resources. The bool is false when there is no such service principal.
func (w *World) AppRoleAssignments(spID string) ([]Object, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	id := strings.ToLower(spID)
	if w.sps[id] == nil {
		return nil, false
	}
	var items []Object
	for _, a := range w.Assignments {
		if isID(a.PrincipalID, id) {
			items = append(items, w.assignmentObject(a))
		}
	}
	return items, true
}

// AssignAppRole assigns an app role of a resource to a user, a group, or a service principal. The
// application permissions of Microsoft Graph go to a service principal only. The role
// DefaultAccess (no role) is for a user or a group, on a resource with its own roles.
func (w *World) AssignAppRole(resourceID, principalID, roleID string, a Actor) (Object, *apiError) {
	w.mu.Lock()
	defer w.mu.Unlock()
	res := w.sps[strings.ToLower(resourceID)]
	if res == nil {
		return nil, notFound("servicePrincipal", resourceID)
	}
	kind, pid, pname, ok := w.principal(principalID)
	if !ok {
		return nil, badRequest("The principal '%s' does not exist.", principalID)
	}
	role, found := res.role(roleID)
	if isID(roleID, DefaultAccess) && res.Builtin == "" && kind != PrincipalService {
		role, found = roleInfo{ID: DefaultAccess, Value: "", Name: "Default access"}, true
	}
	switch {
	case !found:
		return nil, badRequest("Permission being assigned was not found on application '%s'.", res.AppID)
	case res.Builtin != "" && kind != PrincipalService:
		return nil, badRequest("The permissions of %s are for a service principal. A %s cannot have them.", res.Name, strings.ToLower(kind))
	}
	asg := Assignment{ResourceID: res.ID, PrincipalID: pid, PrincipalType: kind, RoleID: role.ID}
	if slices.ContainsFunc(w.Assignments, func(x Assignment) bool {
		return isID(x.ResourceID, asg.ResourceID) && isID(x.PrincipalID, asg.PrincipalID) && isID(x.RoleID, asg.RoleID)
	}) {
		return nil, badRequest("Permission being assigned already exists on the object.")
	}
	w.Assignments = append(w.Assignments, asg)
	cp := asg
	w.record(Event{Type: evAssignCreate, Actor: a, Assignment: &cp, PrincipalName: pname, ResourceName: res.Name, PermissionName: firstNonEmpty(role.Value, role.Name)})
	return w.assignmentObject(asg), nil
}

// DeleteAppRoleAssignment takes an app role assignment away. spID is the principal (asResource false)
// or the resource (asResource true), as in the two paths of Graph.
func (w *World) DeleteAppRoleAssignment(spID, assignmentID_ string, asResource bool, a Actor) *apiError {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.sps[strings.ToLower(spID)] == nil {
		return notFound("servicePrincipal", spID)
	}
	i := slices.IndexFunc(w.Assignments, func(x Assignment) bool {
		side := x.PrincipalID
		if asResource {
			side = x.ResourceID
		}
		return isID(side, spID) && isID(assignmentID(x), assignmentID_)
	})
	if i < 0 {
		return notFound("appRoleAssignment", assignmentID_)
	}
	asg := w.Assignments[i]
	w.Assignments = slices.Delete(w.Assignments, i, i+1)
	ev := Event{Type: evAssignDelete, Actor: a, Assignment: &asg, PrincipalName: w.principalName(asg.PrincipalID)}
	if res := w.sps[strings.ToLower(asg.ResourceID)]; res != nil {
		ev.ResourceName = res.Name
		if r, ok := res.role(asg.RoleID); ok {
			ev.PermissionName = firstNonEmpty(r.Value, r.Name)
		}
	}
	w.record(ev)
	return nil
}

// ---- the consents (oauth2PermissionGrants) ----

func (g Grant) object() Object {
	var principal any
	if g.PrincipalID != "" {
		principal = g.PrincipalID
	}
	return Object{"id": g.ID, "clientId": g.ClientID, "consentType": g.ConsentType, "principalId": principal, "resourceId": g.ResourceID, "scope": g.Scope}
}

// ListOAuth2Grants gives every consent.
func (w *World) ListOAuth2Grants() []Object {
	w.mu.RLock()
	defer w.mu.RUnlock()
	items := make([]Object, len(w.Grants))
	for i, g := range w.Grants {
		items[i] = g.object()
	}
	return items
}

// GetOAuth2Grant gives one consent.
func (w *World) GetOAuth2Grant(id string) (Object, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, g := range w.Grants {
		if isID(g.ID, id) {
			return g.object(), true
		}
	}
	return nil, false
}

// normalizeScopes checks that every value is a delegated permission of the resource, and gives them
// in the case of the resource, each once, with a space between.
func normalizeScopes(res *ServicePrincipal, scope string) (string, *apiError) {
	var out []string
	for _, v := range strings.Fields(scope) {
		s, ok := res.scope(v)
		if !ok {
			return "", badRequest("The permission '%s' was not found on the resource '%s' (%s).", v, res.Name, res.AppID)
		}
		if !slices.Contains(out, s.Value) {
			out = append(out, s.Value)
		}
	}
	return strings.Join(out, " "), nil
}

// CreateOAuth2Grant makes a consent. clientId and resourceId are object IDs of service principals,
// consentType is AllPrincipals, or Principal with the principalId of a user, and scope has delegated
// permissions of the resource.
func (w *World) CreateOAuth2Grant(props map[string]any, a Actor) (Object, *apiError) {
	for k := range props {
		switch k {
		case "clientId", "consentType", "principalId", "resourceId", "scope", "startTime", "expiryTime":
		default:
			return nil, badRequest("Property '%s' does not exist on the type oAuth2PermissionGrant.", k)
		}
	}
	g := Grant{ClientID: str(props["clientId"]), ConsentType: str(props["consentType"]), PrincipalID: str(props["principalId"]), ResourceID: str(props["resourceId"])}
	switch {
	case g.ClientID == "":
		return nil, missing("clientId")
	case g.ResourceID == "":
		return nil, missing("resourceId")
	case g.ConsentType != ConsentAll && g.ConsentType != ConsentPrincipal:
		return nil, badRequest("The consentType must be %s or %s.", ConsentAll, ConsentPrincipal)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	client, res := w.sps[strings.ToLower(g.ClientID)], w.sps[strings.ToLower(g.ResourceID)]
	switch {
	case client == nil:
		return nil, badRequest("The client '%s' does not exist.", g.ClientID)
	case res == nil:
		return nil, badRequest("The resource '%s' does not exist.", g.ResourceID)
	}
	g.ClientID, g.ResourceID = client.ID, res.ID
	if g.ConsentType == ConsentPrincipal {
		u := w.users[strings.ToLower(g.PrincipalID)]
		if u == nil {
			return nil, badRequest("The consentType Principal needs the principalId of a user that exists.")
		}
		g.PrincipalID = str(u["id"])
	} else if g.PrincipalID != "" {
		return nil, badRequest("The consentType %s has no principalId.", ConsentAll)
	}
	var e *apiError
	if g.Scope, e = normalizeScopes(res, str(props["scope"])); e != nil {
		return nil, e
	}
	if slices.ContainsFunc(w.Grants, func(x Grant) bool {
		return isID(x.ClientID, g.ClientID) && isID(x.ResourceID, g.ResourceID) && x.ConsentType == g.ConsentType && isID(x.PrincipalID, g.PrincipalID)
	}) {
		return nil, badRequest("Permission entry already exists.")
	}
	g.ID = newGUID()
	w.Grants = append(w.Grants, g)
	cp := g
	w.record(Event{Type: evGrantCreate, Actor: a, Grant: &cp, PrincipalName: client.Name, ResourceName: res.Name, PermissionName: g.Scope})
	return g.object(), nil
}

// UpdateOAuth2Grant changes the scopes of a consent. The scope is the only property that changes.
func (w *World) UpdateOAuth2Grant(id string, props map[string]any, a Actor) *apiError {
	for k := range props {
		if k != "scope" {
			return badRequest("Property '%s' is read-only. Only scope can change.", k)
		}
	}
	if _, has := props["scope"]; !has {
		return missing("scope")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	i := slices.IndexFunc(w.Grants, func(g Grant) bool { return isID(g.ID, id) })
	if i < 0 {
		return notFound("oAuth2PermissionGrant", id)
	}
	res := w.sps[strings.ToLower(w.Grants[i].ResourceID)]
	scope, e := normalizeScopes(res, str(props["scope"]))
	if e != nil {
		return e
	}
	w.Grants[i].Scope = scope
	cp := w.Grants[i]
	w.record(Event{Type: evGrantUpdate, Actor: a, Grant: &cp, PrincipalName: w.principalName(cp.ClientID), ResourceName: res.Name, PermissionName: scope})
	return nil
}

// DeleteOAuth2Grant takes a consent away.
func (w *World) DeleteOAuth2Grant(id string, a Actor) *apiError {
	w.mu.Lock()
	defer w.mu.Unlock()
	i := slices.IndexFunc(w.Grants, func(g Grant) bool { return isID(g.ID, id) })
	if i < 0 {
		return notFound("oAuth2PermissionGrant", id)
	}
	g := w.Grants[i]
	w.Grants = slices.Delete(w.Grants, i, i+1)
	ev := Event{Type: evGrantDelete, Actor: a, Grant: &g, PrincipalName: w.principalName(g.ClientID), PermissionName: g.Scope}
	if res := w.sps[strings.ToLower(g.ResourceID)]; res != nil {
		ev.ResourceName = res.Name
	}
	w.record(ev)
	return nil
}

// ---- the history ----

// principalKindText writes the type of a principal as a person says it: service principal.
func principalKindText(kind string) string {
	if kind == PrincipalService {
		return "service principal"
	}
	return strings.ToLower(kind)
}

func grantEventText(e Event) (string, bool) {
	switch e.Type {
	case evAssignCreate:
		return fmt.Sprintf("Assigned the role %s of %s to %s (%s)", e.PermissionName, e.ResourceName, e.PrincipalName, principalKindText(e.Assignment.PrincipalType)), true
	case evAssignDelete:
		return fmt.Sprintf("Took the role %s of %s away from %s", e.PermissionName, e.ResourceName, e.PrincipalName), true
	case evGrantCreate:
		return fmt.Sprintf("Consent for %s: the delegated permissions %q of %s (%s)", e.PrincipalName, e.PermissionName, e.ResourceName, e.Grant.ConsentType), true
	case evGrantUpdate:
		return fmt.Sprintf("Changed the delegated permissions of %s on %s to %q", e.PrincipalName, e.ResourceName, e.PermissionName), true
	case evGrantDelete:
		return fmt.Sprintf("Took the consent of %s for %s away", e.PrincipalName, e.ResourceName), true
	}
	return "", false
}

func (w *World) applyGrantEvent(e Event) bool {
	switch e.Type {
	case evAssignCreate:
		if e.Assignment != nil {
			w.Assignments = append(w.Assignments, *e.Assignment)
		}
	case evAssignDelete:
		if e.Assignment != nil {
			w.Assignments = slices.DeleteFunc(w.Assignments, func(x Assignment) bool {
				return isID(x.ResourceID, e.Assignment.ResourceID) && isID(assignmentID(x), assignmentID(*e.Assignment))
			})
		}
	case evGrantCreate:
		if e.Grant != nil {
			w.Grants = append(w.Grants, *e.Grant)
		}
	case evGrantUpdate:
		if e.Grant != nil {
			for i := range w.Grants {
				if isID(w.Grants[i].ID, e.Grant.ID) {
					w.Grants[i].Scope = e.Grant.Scope
				}
			}
		}
	case evGrantDelete:
		if e.Grant != nil {
			w.Grants = slices.DeleteFunc(w.Grants, func(g Grant) bool { return isID(g.ID, e.Grant.ID) })
		}
	default:
		return false
	}
	return true
}
