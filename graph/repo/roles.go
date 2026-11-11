package repo

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// The app roles of an app registration and of a service principal.
//
// The rules are from the page of Microsoft Learn "appRole resource type" (the commit of graph/cost/raw/SOURCE.json):
//   - A new role needs an id that is a GUID. The value has at most 120 characters, from a list of characters (no space), and it does not start with a dot.
//   - isEnabled: a role is disabled in one call, and removed in a later one.
//   - origin is read-only. The roles of an application are on its service principal with the origin Application. A role that is only for one tenant is
//     defined on the service principal (origin ServicePrincipal), and the member type Application is for roles of an application only.
//
// The texts of the errors are dollycli's own: the Explorer proxy allows no write, so the real ones could not be tried.

// The origins of a role, and the member types.
const (
	OriginApplication      = "Application"
	OriginServicePrincipal = "ServicePrincipal"
	MemberUser             = "User"
	MemberApplication      = "Application"
)

// roleValueRe is the characters that Learn allows in a value.
var roleValueRe = regexp.MustCompile("^[0-9A-Za-z:!#$%&'()*+,./;<=>?@\\[\\]^_`{|}~-]*$")

// legacy says the role has no member types: it is from a data folder from before the roles had them. Then the role is enabled, it is for users and
// applications, its value is its name in lower case, and its description is its name.
func (r AppRole) legacy() bool { return len(r.MemberTypes) == 0 }

func (r AppRole) value() string {
	if r.legacy() && r.Value == "" {
		return strings.ToLower(r.Name)
	}
	return r.Value
}

func (r AppRole) description() string {
	if r.legacy() && r.Description == "" {
		return r.Name
	}
	return r.Description
}

func (r AppRole) memberTypes() []string {
	if r.legacy() {
		return []string{MemberUser, MemberApplication}
	}
	return r.MemberTypes
}

func (r AppRole) origin() string {
	if r.Origin == "" {
		return OriginApplication
	}
	return r.Origin
}

// memberTypeOf is the member type that a kind of principal needs: a group is assigned a role as a user is.
func memberTypeOf(kind string) string {
	if kind == PrincipalService {
		return MemberApplication
	}
	return MemberUser
}

// object gives the role as Graph writes it.
func (r AppRole) object() Object {
	var value any
	if v := r.value(); v != "" {
		value = v
	}
	types := make([]any, 0, 2)
	for _, t := range r.memberTypes() {
		types = append(types, t)
	}
	return Object{
		"allowedMemberTypes": types, "description": r.description(), "displayName": r.Name, "id": r.ID,
		"isEnabled": !r.Disabled, "origin": r.origin(), "value": value,
	}
}

func roleObjects(roles []AppRole) []any {
	out := make([]any, 0, len(roles))
	for _, r := range roles {
		out = append(out, r.object())
	}
	return out
}

// roleFromObject reads a role that was checked (parseRoleList), or that the data has. It does not check.
func roleFromObject(m map[string]any) AppRole {
	r := AppRole{ID: str(m["id"]), Name: str(m["displayName"]), Description: str(m["description"]), Value: str(m["value"]), MemberTypes: anyStrings(m["allowedMemberTypes"])}
	if on, ok := m["isEnabled"].(bool); ok && !on {
		r.Disabled = true
	}
	if str(m["origin"]) == OriginServicePrincipal {
		r.Origin = OriginServicePrincipal
	}
	return r
}

func rolesFromList(v any) []AppRole {
	var out []AppRole
	for _, m := range anyObjects(v) {
		out = append(out, roleFromObject(m))
	}
	return out
}

// parseRoleList reads and checks a list of roles from a call. resource is the name for the texts of the errors. onApp is true for an application. The
// origin of a role is as the call wrote it (empty, Application, or ServicePrincipal): a call may send back what it read.
func parseRoleList(v any, resource string, onApp bool) ([]AppRole, *apiError) {
	bad := func(format string, a ...any) *apiError {
		return badRequest("Invalid value specified for property 'appRoles' of resource '%s': %s", resource, fmt.Sprintf(format, a...))
	}
	list, ok := v.([]any)
	if !ok {
		return nil, bad("it must be a list of app roles.")
	}
	var out []AppRole
	for i, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, bad("item %d is not an app role.", i+1)
		}
		for k := range m {
			switch k {
			case "id", "displayName", "description", "value", "allowedMemberTypes", "isEnabled", "origin":
			default:
				return nil, bad("an app role has no property '%s'.", k)
			}
		}
		r := AppRole{ID: str(m["id"]), Name: str(m["displayName"]), Description: str(m["description"])}
		if !IsGUID(r.ID) {
			return nil, bad("item %d needs an id that is a GUID.", i+1)
		}
		if strings.TrimSpace(r.Name) == "" {
			return nil, bad("the role %s needs a displayName.", r.ID)
		}
		types := anyStrings(m["allowedMemberTypes"])
		if len(types) == 0 {
			return nil, bad("the role %s needs allowedMemberTypes: User, Application, or both.", r.ID)
		}
		for _, t := range types {
			if t != MemberUser && t != MemberApplication {
				return nil, bad("allowedMemberTypes of the role %s has '%s'. Use User, Application, or both.", r.ID, t)
			}
		}
		r.MemberTypes = types
		if len(types) == 2 && types[0] == types[1] {
			r.MemberTypes = types[:1]
		}
		if v, has := m["value"]; has && v != nil {
			s, ok := v.(string)
			switch {
			case !ok:
				return nil, bad("the value of the role %s must be a string.", r.ID)
			case len(s) > 120:
				return nil, bad("the value of the role %s has more than 120 characters.", r.ID)
			case strings.HasPrefix(s, "."):
				return nil, bad("the value of the role %s must not start with a dot.", r.ID)
			case !roleValueRe.MatchString(s):
				return nil, bad("the value '%s' of the role %s has a character that is not allowed (no space; letters, digits, and : ! # $ %% & ' ( ) * + , - . / ; < = > ? @ [ ] ^ _ ` { | } ~).", s, r.ID)
			}
			r.Value = s
		}
		if v, has := m["isEnabled"]; has {
			on, ok := v.(bool)
			if !ok {
				return nil, bad("isEnabled of the role %s must be true or false.", r.ID)
			}
			r.Disabled = !on
		}
		switch o := str(m["origin"]); {
		case o == "" || o == OriginApplication:
			r.Origin = o
		case o == OriginServicePrincipal && !onApp:
			r.Origin = o
		default:
			return nil, bad("origin of the role %s is read-only.", r.ID)
		}
		out = append(out, r)
	}
	return out, checkRoleSet(out, resource)
}

// checkRoleSet checks that no two roles have the same id, or the same value.
func checkRoleSet(roles []AppRole, resource string) *apiError {
	ids, values := map[string]bool{}, map[string]bool{}
	for _, r := range roles {
		id := strings.ToLower(r.ID)
		if ids[id] {
			return badRequest("Invalid value specified for property 'appRoles' of resource '%s': two roles have the same id %s.", resource, r.ID)
		}
		ids[id] = true
		if v := strings.ToLower(r.Value); v != "" {
			if values[v] {
				return badRequest("Invalid value specified for property 'appRoles' of resource '%s': two roles have the same value '%s'.", resource, r.Value)
			}
			values[v] = true
		}
	}
	return nil
}

// checkRoleRemoval checks that every role that goes away was disabled before.
func checkRoleRemoval(old, now []AppRole, resource string) *apiError {
	for _, o := range old {
		if o.Disabled || slices.ContainsFunc(now, func(r AppRole) bool { return isID(r.ID, o.ID) }) {
			continue
		}
		return badRequest("Invalid value specified for property 'appRoles' of resource '%s': the role '%s' must be disabled (isEnabled false) in one call before a later call removes it.", resource, firstNonEmpty(o.Name, o.ID))
	}
	return nil
}

// normalizeApplicationRoles checks the appRoles of a call on an application, and writes them as the data keeps them (all the fields). old are the roles
// that the application has now (nil for a new application).
func normalizeApplicationRoles(body map[string]any, old []AppRole) *apiError {
	v, has := body["appRoles"]
	if !has {
		return nil
	}
	roles, e := parseRoleList(v, "Application", true)
	if e != nil {
		return e
	}
	if e := checkRoleRemoval(old, roles, "Application"); e != nil {
		return e
	}
	for i := range roles {
		roles[i].Origin = ""
	}
	body["appRoles"] = roleObjects(roles)
	return nil
}

// normalizeServicePrincipalRoles checks the appRoles of a call on a service principal. The roles of the application stay as they are (a service principal
// cannot change them, or remove them: that is done on the application), and the other roles are the service principal's own.
func normalizeServicePrincipalRoles(sp *ServicePrincipal, body map[string]any) *apiError {
	v, has := body["appRoles"]
	if !has {
		return nil
	}
	roles, e := parseRoleList(v, "ServicePrincipal", false)
	if e != nil {
		return e
	}
	var final, ownNow, ownOld []AppRole
	for _, r := range sp.Roles {
		if r.origin() == OriginServicePrincipal {
			ownOld = append(ownOld, r)
		} else {
			final = append(final, r)
		}
	}
	for _, r := range roles {
		if slices.ContainsFunc(final, func(x AppRole) bool { return isID(x.ID, r.ID) }) {
			continue // a role of the application: it stays as it is
		}
		if r.Origin == OriginApplication {
			return badRequest("Invalid value specified for property 'appRoles' of resource 'ServicePrincipal': origin of the role %s is read-only.", r.ID)
		}
		if slices.Contains(r.MemberTypes, MemberApplication) {
			return badRequest("Invalid value specified for property 'appRoles' of resource 'ServicePrincipal': allowedMemberTypes of the role %s has Application. That is for the roles of an application only.", r.ID)
		}
		r.Origin = OriginServicePrincipal
		ownNow = append(ownNow, r)
	}
	if e := checkRoleRemoval(ownOld, ownNow, "ServicePrincipal"); e != nil {
		return e
	}
	final = append(final, ownNow...)
	if e := checkRoleSet(final, "ServicePrincipal"); e != nil {
		return e
	}
	body["appRoles"] = roleObjects(final)
	return nil
}

// ---- what the portal does with the roles of an app registration ----
//
// The portal adds, edits, and removes one role at a time. Each of these writes the whole list through UpdateApplication, so the rules are those of a call of
// Graph, and the history has the same change. (The list is read, and then written: two people who change the roles of the same app at the same moment can
// lose one change. The portal has one person at a time.)

// roleProps are the properties of a role that a person can set. The id is made, and the origin is read-only.
var roleProps = []string{"displayName", "value", "description", "allowedMemberTypes", "isEnabled"}

// AppRoles gives the roles of an application, found by object ID or client ID. The bool is false when there is no such application.
func (w *World) AppRoles(ref string) ([]Object, bool) {
	app, ok := w.GetApplication(ref)
	if !ok {
		return nil, false
	}
	out := []Object{}
	for _, r := range anyObjects(app["appRoles"]) {
		out = append(out, r)
	}
	return out, true
}

func (w *World) changeAppRoles(ref string, change func(roles []map[string]any) ([]map[string]any, *apiError), a Actor) *apiError {
	app, ok := w.GetApplication(ref)
	if !ok {
		return notFound("application", ref)
	}
	next, e := change(anyObjects(app["appRoles"]))
	if e != nil {
		return e
	}
	list := make([]any, len(next))
	for i, r := range next {
		list[i] = r
	}
	return w.UpdateApplication(ref, map[string]any{"appRoles": list}, a)
}

func checkRoleProps(props map[string]any) *apiError {
	for k := range props {
		if !slices.Contains(roleProps, k) {
			return badRequest("Property '%s' does not exist on an app role, or it cannot be set.", k)
		}
	}
	return nil
}

// AddAppRole adds a role to an application. The role gets an id, and it is enabled unless the call says otherwise. It gives the role as the data keeps it.
func (w *World) AddAppRole(ref string, props map[string]any, a Actor) (Object, *apiError) {
	if e := checkRoleProps(props); e != nil {
		return nil, e
	}
	role := map[string]any{"id": newGUID(), "isEnabled": true}
	for k, v := range props {
		role[k] = v
	}
	if e := w.changeAppRoles(ref, func(roles []map[string]any) ([]map[string]any, *apiError) { return append(roles, role), nil }, a); e != nil {
		return nil, e
	}
	roles, _ := w.AppRoles(ref)
	for _, r := range roles {
		if str(r["id"]) == role["id"] {
			return r, nil
		}
	}
	return nil, nil
}

// EditAppRole changes the properties that a call names, in one role of an application. A role is disabled this way (isEnabled false).
func (w *World) EditAppRole(ref, roleID string, props map[string]any, a Actor) *apiError {
	if e := checkRoleProps(props); e != nil {
		return e
	}
	return w.changeAppRoles(ref, func(roles []map[string]any) ([]map[string]any, *apiError) {
		for _, r := range roles {
			if isID(str(r["id"]), roleID) {
				for k, v := range props {
					r[k] = v
				}
				return roles, nil
			}
		}
		return nil, notFound("appRole", roleID)
	}, a)
}

// RemoveAppRole takes a role out of an application. As in Graph, the role must have been disabled before.
func (w *World) RemoveAppRole(ref, roleID string, a Actor) *apiError {
	return w.changeAppRoles(ref, func(roles []map[string]any) ([]map[string]any, *apiError) {
		i := slices.IndexFunc(roles, func(r map[string]any) bool { return isID(str(r["id"]), roleID) })
		if i < 0 {
			return nil, notFound("appRole", roleID)
		}
		return slices.Delete(roles, i, i+1), nil
	}, a)
}

// RoleAssignments gives the roles of a service principal (found by object ID or client ID) and who is assigned to them. The bool is false when there is no such
// service principal. The roles of Microsoft Graph are the permissions, and they are not listed here.
func (w *World) RoleAssignments(ref string) (roles, assignments []Object, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	sp := w.findServicePrincipal(ref)
	if sp == nil {
		return nil, nil, false
	}
	roles, assignments = []Object{}, []Object{}
	if sp.Builtin == "" {
		for _, r := range sp.Roles {
			roles = append(roles, r.object())
		}
	}
	for _, a := range w.Assignments {
		if isID(a.ResourceID, sp.ID) {
			assignments = append(assignments, w.assignmentObject(a))
		}
	}
	return roles, assignments, true
}

// setRoles sets the roles of a service principal, and takes the assignments of a role that is gone with it. Call it with the lock held.
func (w *World) setRoles(sp *ServicePrincipal, roles []AppRole) {
	old := sp.Roles
	sp.Roles = roles
	for _, o := range old {
		if !slices.ContainsFunc(roles, func(r AppRole) bool { return isID(r.ID, o.ID) }) {
			w.Assignments = slices.DeleteFunc(w.Assignments, func(a Assignment) bool { return isID(a.ResourceID, sp.ID) && isID(a.RoleID, o.ID) })
		}
	}
}

// syncRolesFromApp gives the service principal of an application the roles of the application. The service principal keeps its own roles. Call it with
// the lock held, after the application or the service principal is there.
func (w *World) syncRolesFromApp(app Object) {
	sp := w.byAppID[strings.ToLower(str(app["appId"]))]
	if sp == nil || sp.Builtin != "" {
		return
	}
	roles := rolesFromList(app["appRoles"])
	for _, r := range sp.Roles {
		if r.origin() == OriginServicePrincipal {
			roles = append(roles, r)
		}
	}
	w.setRoles(sp, roles)
}
