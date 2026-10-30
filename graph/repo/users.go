package repo

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

// The users: reads, create, update, delete, and the changes that the admin API
// makes. This file is the data side of api/users.go. It satisfies api.UsersBackend.

// Events that the users make.
const (
	evRename     = "rename"      // a value of a field changed for every user that had it
	evUser       = "user"        // fields of one user changed
	evUserCreate = "user.create" // a new user (the whole object is in the event)
	evUserDelete = "user.delete" // a user is gone, and so is every reference to it
)

// userProps are the properties of a user that a call can set, and their type.
// The ones that Graph computes (id, the on-premises properties) are not here.
var userProps = map[string]propKind{
	"displayName": kString, "givenName": kString, "surname": kString, "userPrincipalName": kString, "mail": kString,
	"mailNickname": kString, "department": kString, "jobTitle": kString, "officeLocation": kString, "city": kString,
	"country": kString, "companyName": kString, "usageLocation": kString, "postalCode": kString, "mobilePhone": kString,
	"preferredLanguage": kString, "employeeId": kString, "userType": kString, "passwordPolicies": kString,
	"employeeType": kString, "employeeHireDate": kString, "streetAddress": kString, "state": kString,
	"faxNumber": kString, "ageGroup": kString, "externalUserState": kString,
	"businessPhones": kList, "otherMails": kList,
	"employeeOrgData": kObject, "showInAddressList": kBool,
	"accountEnabled": kBool, "onPremisesExtensionAttributes": kObject,
	// accepted and not kept: dolly has no passwords
	"passwordProfile": kObject,
}

const upnTakenText = "Another object with the same value for property userPrincipalName already exists."

// ---- reads ----

// ListUsers gives every user, in the order of the data. Each is a copy.
func (w *World) ListUsers() []Object {
	w.mu.RLock()
	defer w.mu.RUnlock()
	items := make([]Object, len(w.Users))
	for i, u := range w.Users {
		// a shallow copy is enough: an update puts a new map for a nested value, and
		// never changes one in place
		items[i] = maps.Clone(u)
	}
	return items
}

// GetUser gives a copy of a user, found by object ID or user principal name.
func (w *World) GetUser(ref string) (Object, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	u := w.userByRef(ref)
	if u == nil {
		return nil, false
	}
	return deepCopy(u), true
}

// userByRef finds a user by object ID or by user principal name. Call it with the
// lock held.
func (w *World) userByRef(ref string) Object {
	if u := w.users[strings.ToLower(ref)]; u != nil {
		return u
	}
	for _, u := range w.Users {
		if strings.EqualFold(str(u["userPrincipalName"]), ref) {
			return u
		}
	}
	return nil
}

// upnTaken reports whether another user has this user principal name.
func (w *World) upnTaken(upn string, except Object) bool {
	for _, u := range w.Users {
		if u["id"] != nil && except != nil && u["id"] == except["id"] {
			continue
		}
		if strings.EqualFold(str(u["userPrincipalName"]), upn) {
			return true
		}
	}
	return false
}

// addUser puts a new user in the world. Call it with the lock held.
func (w *World) addUser(u Object) {
	w.Users = append(w.Users, u)
	w.users[strings.ToLower(str(u["id"]))] = u
}

// ---- create, update, delete ----

// CreateUser makes a user. Graph needs accountEnabled, displayName, mailNickname,
// userPrincipalName, and passwordProfile. It gives a copy of the new user.
func (w *World) CreateUser(props map[string]any, a Actor) (Object, *apiError) {
	if e := checkProps(props, userProps, "User"); e != nil {
		return nil, e
	}
	for _, name := range []string{"displayName", "mailNickname", "userPrincipalName"} {
		if s, _ := props[name].(string); strings.TrimSpace(s) == "" {
			return nil, missing(name)
		}
	}
	if _, ok := props["accountEnabled"].(bool); !ok {
		return nil, missing("accountEnabled")
	}
	if _, ok := props["passwordProfile"].(map[string]any); !ok {
		return nil, missing("passwordProfile")
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	upn := props["userPrincipalName"].(string)
	if w.upnTaken(upn, nil) {
		return nil, badRequest(upnTakenText)
	}
	u := Object{"id": newGUID(), "userType": "Member", "passwordPolicies": "None", "createdDateTime": w.now().Format(time.RFC3339)}
	for k, v := range props {
		if k != "passwordProfile" {
			u[k] = v
		}
	}
	if _, ok := u["mail"]; !ok {
		u["mail"] = nil
	}
	u = deepCopy(u)
	w.addUser(u)
	w.refreshDynamic()
	w.record(Event{Type: evUserCreate, Actor: a, User: str(u["id"]), UserName: upn, Object: deepCopy(u)})
	return deepCopy(u), nil
}

// UpdateUser changes properties of a user, found by object ID or user principal
// name. A value of null clears a property. The attributes in
// onPremisesExtensionAttributes are merged, as in Graph.
func (w *World) UpdateUser(ref string, props map[string]any, a Actor) *apiError {
	if _, has := props["id"]; has {
		return badRequest("Property 'id' is read-only.")
	}
	if e := checkProps(props, userProps, "User"); e != nil {
		return e
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	u := w.userByRef(ref)
	if u == nil {
		return notFound("user", ref)
	}
	set, e := w.userChanges(u, props)
	if e != nil || len(set) == 0 {
		return e
	}
	maps.Copy(u, set)
	w.refreshDynamic()
	w.record(Event{Type: evUser, Actor: a, User: str(u["id"]), UserName: str(u["userPrincipalName"]), Set: deepCopy(set)})
	return nil
}

// userChanges works out what an update does to a user: the properties to set (the
// final values), or why it cannot. Call it with the lock held.
func (w *World) userChanges(u Object, props map[string]any) (map[string]any, *apiError) {
	set := map[string]any{}
	for k, v := range props {
		switch k {
		case "passwordProfile":
			continue // dolly has no passwords
		case "userPrincipalName":
			s, _ := v.(string)
			if strings.TrimSpace(s) == "" {
				return nil, badRequest("Invalid value specified for property 'userPrincipalName' of resource 'User'.")
			}
			if w.upnTaken(s, u) {
				return nil, badRequest(upnTakenText)
			}
		case "onPremisesExtensionAttributes":
			if add, ok := v.(map[string]any); ok {
				old, _ := u[k].(map[string]any)
				v = mergeAttributes(old, add)
			}
		}
		set[k] = v
	}
	return set, nil
}

// mergeAttributes puts the attributes of add over the attributes of old, in a new
// map: an event keeps the maps that it was given, so a map is never changed in place.
func mergeAttributes(old, add map[string]any) map[string]any {
	merged := make(map[string]any, len(old)+len(add))
	maps.Copy(merged, old)
	maps.Copy(merged, add)
	return merged
}

// DeleteUser deletes a user, found by object ID or user principal name.
func (w *World) DeleteUser(ref string, a Actor) *apiError {
	w.mu.Lock()
	defer w.mu.Unlock()
	u := w.userByRef(ref)
	if u == nil {
		return notFound("user", ref)
	}
	id, upn := str(u["id"]), str(u["userPrincipalName"])
	n := w.removeUser(id)
	w.record(Event{Type: evUserDelete, Actor: a, User: id, UserName: upn, Changed: n})
	return nil
}

// removeUser takes a user out of the world and out of every group. It returns in
// how many groups the user was a member or an owner. Call it with the lock held.
func (w *World) removeUser(id string) int {
	key := strings.ToLower(id)
	if w.users[key] == nil {
		return 0
	}
	delete(w.users, key)
	w.Users = slices.DeleteFunc(w.Users, func(u Object) bool { return isID(str(u["id"]), id) })
	w.dropManagers(id)
	n := w.dropReferences(id)
	w.refreshDynamic() // the reports of a deleted manager have no manager now
	return n
}

// ---- what the admin API and the page change ----

// Rename changes a field of every user that has the value from. It returns how
// many users changed. The field is a Graph name, like department or jobTitle.
func (w *World) Rename(field, from, to string, a Actor) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := w.renameLocked(field, from, to)
	if n > 0 {
		w.refreshDynamic()
		w.record(Event{Type: evRename, Actor: a, Field: field, From: from, To: to, Changed: n})
	}
	return n
}

func (w *World) renameLocked(field, from, to string) int {
	n := 0
	for _, u := range w.Users {
		if str(u[field]) == from {
			u[field] = to
			n++
		}
	}
	return n
}

// SetUser changes fields of one user, found by ID or user principal name. It
// reports whether the user exists.
func (w *World) SetUser(who string, set map[string]any, a Actor) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	u := w.userByRef(who)
	if u == nil {
		return false
	}
	if add, ok := set["onPremisesExtensionAttributes"].(map[string]any); ok { // merged, as in Graph
		old, _ := u["onPremisesExtensionAttributes"].(map[string]any)
		set = maps.Clone(set)
		set["onPremisesExtensionAttributes"] = mergeAttributes(old, add)
	}
	maps.Copy(u, set)
	w.refreshDynamic()
	w.record(Event{Type: evUser, Actor: a, User: str(u["id"]), UserName: str(u["userPrincipalName"]), Set: set})
	return true
}

// UserInfo is a user in a list of the page.
type UserInfo struct {
	ID             string `json:"id"`
	Name           string `json:"displayName"`
	UPN            string `json:"userPrincipalName"`
	Department     string `json:"department"`
	JobTitle       string `json:"jobTitle"`
	OfficeLocation string `json:"officeLocation"`
	Enabled        bool   `json:"accountEnabled"`
	UserType       string `json:"userType"`
	// Fields are the other properties that the page can change (see EditableFields). A property
	// that the user does not have is an empty string. The extension attributes are
	// extensionAttribute1 to extensionAttribute15.
	Fields map[string]string `json:"fields"`
}

// EditableFields are the text properties of a user that the page can change, in the order that
// the page shows them.
var EditableFields = []string{
	"userPrincipalName", "mailNickname", "employeeHireDate", "givenName", "surname", "displayName", "mail", "employeeId", "companyName", "userType",
	"city", "state", "country", "streetAddress", "postalCode", "usageLocation", "preferredLanguage",
	"mobilePhone", "faxNumber",
}

func userInfo(u Object) UserInfo {
	enabled, _ := u["accountEnabled"].(bool)
	fields := map[string]string{}
	for _, f := range EditableFields {
		fields[f] = str(u[f])
	}
	ext, _ := u["onPremisesExtensionAttributes"].(map[string]any)
	for i := 1; i <= 15; i++ {
		name := fmt.Sprintf("extensionAttribute%d", i)
		fields[name] = str(ext[name])
	}
	return UserInfo{
		Fields: fields,
		ID:     str(u["id"]), Name: str(u["displayName"]), UPN: str(u["userPrincipalName"]), Department: str(u["department"]),
		JobTitle: str(u["jobTitle"]), OfficeLocation: str(u["officeLocation"]), Enabled: enabled, UserType: str(u["userType"]),
	}
}

// SearchUsers finds users whose name, user principal name, department, title,
// or office has the text. It gives at most limit users, and the number that match.
func (w *World) SearchUsers(q string, limit int) ([]UserInfo, int) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	q = strings.ToLower(strings.TrimSpace(q))
	var out []UserInfo
	total := 0
	for _, u := range w.Users {
		if q != "" && !userMatches(u, q) {
			continue
		}
		total++
		if limit <= 0 || len(out) < limit {
			out = append(out, userInfo(u))
		}
	}
	return out, total
}

func userMatches(u Object, q string) bool {
	for _, f := range []string{"displayName", "userPrincipalName", "department", "jobTitle", "officeLocation"} {
		if strings.Contains(strings.ToLower(str(u[f])), q) {
			return true
		}
	}
	return false
}

// ---- the history of users ----

// userEventText says what an event of the users did. It reports whether it is one.
func userEventText(e Event) (string, bool) {
	switch e.Type {
	case evRename:
		return fmt.Sprintf("Changed %s %q to %q for %d %s", e.Field, e.From, e.To, e.Changed, plural(e.Changed, "user", "users")), true
	case evUser:
		return fmt.Sprintf("Changed %s: %s", firstNonEmpty(e.UserName, e.User), setText(e.Set)), true
	case evUserCreate:
		return fmt.Sprintf("Made the user %s (%s)", firstNonEmpty(e.UserName, e.User), str(e.Object["displayName"])), true
	case evUserDelete:
		return fmt.Sprintf("Deleted the user %s (it was in %d %s)", firstNonEmpty(e.UserName, e.User), e.Changed, plural(e.Changed, "group", "groups")), true
	}
	return "", false
}

// applyUserEvent does an event of the users on a world that nobody else uses. It
// reports whether it is one.
func (w *World) applyUserEvent(e Event) bool {
	switch e.Type {
	case evRename:
		w.renameLocked(e.Field, e.From, e.To)
	case evUser:
		if u := w.users[strings.ToLower(e.User)]; u != nil {
			maps.Copy(u, e.Set)
		}
	case evUserCreate:
		w.addUser(deepCopy(e.Object))
	case evUserDelete:
		w.removeUser(e.User)
	default:
		return false
	}
	return true
}
