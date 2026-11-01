package repo

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/withholm/dolly-cli/graph/rule"
)

// The groups: the group itself (reads, create, update, delete). This file is the
// data side of api/groups.go. Its members, owners, and apps are in group_members.go,
// group_owners.go, and group_apps.go. The world satisfies api.GroupsBackend.

// Events that the groups make.
const (
	evGroupCreate = "group.create" // a new group (the whole group is in the event)
	evGroupUpdate = "group.update" // fields of one group changed
	evGroupDelete = "group.delete" // a group is gone, and so is every reference to it
)

// Group is a group in the fake world.
type Group struct {
	ID             string `json:"id"`
	Name           string `json:"displayName"`
	Security       bool   `json:"securityEnabled"`
	Mail           bool   `json:"mailEnabled"`
	Unified        bool   `json:"unified"`
	Dynamic        bool   `json:"dynamic"`
	RoleAssignable bool   `json:"roleAssignable"`
	Rule           string `json:"membershipRule,omitempty"`
	// RuleProcessing is On or Paused, for a dynamic group (dynamic.go). A paused group keeps its members.
	RuleProcessing string `json:"ruleProcessing,omitempty"`
	Description    string `json:"description,omitempty"`
	MailNickname   string `json:"mailNickname,omitempty"`
	// Created is when the group was made. The seed groups have none.
	Created time.Time `json:"created,omitzero"`
	// rule is the rule, read. It is read again when the text changes (dynamic.go).
	rule     *rule.Rule
	ruleText string
	// Owners are object IDs of users and apps that own the group.
	Owners []string `json:"owners,omitempty"`
	// Members are object IDs (users, groups, service principals), in the order
	// they were added.
	Members []string `json:"members"`
}

// groupProps are the properties of a group that a call can set.
var groupProps = map[string]propKind{
	"displayName": kString, "description": kString, "mailNickname": kString, "membershipRule": kString,
	"membershipRuleProcessingState": kString, "visibility": kString,
	"mailEnabled": kBool, "securityEnabled": kBool, "isAssignableToRole": kBool,
	"groupTypes": kList, "members@odata.bind": kList, "owners@odata.bind": kList,
}

// object gives the group as a Graph object.
func (g *Group) object() Object {
	types := []string{}
	if g.Unified {
		types = append(types, "Unified")
	}
	if g.Dynamic {
		types = append(types, "DynamicMembership")
	}
	o := Object{
		"@odata.type":        typeGroup,
		"id":                 g.ID,
		"displayName":        g.Name,
		"groupTypes":         types,
		"securityEnabled":    g.Security,
		"mailEnabled":        g.Mail,
		"isAssignableToRole": g.RoleAssignable,
	}
	if g.Rule != "" {
		o["membershipRule"] = g.Rule
	}
	if g.Dynamic {
		o["membershipRuleProcessingState"] = g.processing()
	}
	if g.Description != "" {
		o["description"] = g.Description
	}
	if g.MailNickname != "" {
		o["mailNickname"] = g.MailNickname
	}
	if !g.Created.IsZero() {
		o["createdDateTime"] = g.Created.Format(time.RFC3339)
	}
	return o
}

func copyGroup(g *Group) *Group {
	c := *g
	c.Members = append([]string{}, g.Members...)
	c.Owners = append([]string(nil), g.Owners...)
	return &c
}

// ---- reads ----

// ListGroups gives every group as an object, in the order of the data.
func (w *World) ListGroups() []Object {
	w.mu.RLock()
	defer w.mu.RUnlock()
	items := make([]Object, len(w.Groups))
	for i, g := range w.Groups {
		items[i] = g.object()
	}
	return items
}

// GetGroup gives a group as an object.
func (w *World) GetGroup(id string) (Object, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	g := w.groups[strings.ToLower(id)]
	if g == nil {
		return nil, false
	}
	return g.object(), true
}

// addGroup puts a new group in the world. Call it with the lock held.
func (w *World) addGroup(g *Group) {
	w.Groups = append(w.Groups, g)
	w.groups[strings.ToLower(g.ID)] = g
}

// ---- create, update, delete ----

// CreateGroup makes a group and gives it as an object. Graph needs displayName,
// mailEnabled, mailNickname, and securityEnabled. groupTypes can hold Unified
// (Microsoft 365) and DynamicMembership (which needs membershipRule; dolly keeps
// the rule and does not run it). members@odata.bind puts members in the new group,
// and owners@odata.bind makes owners.
func (w *World) CreateGroup(props map[string]any, a Actor) (Object, *apiError) {
	g, e := newGroup(props)
	if e != nil {
		return nil, e
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if e := w.bindMembers(g, props); e != nil {
		return nil, e
	}
	if e := w.bindOwners(g, props); e != nil {
		return nil, e
	}
	g.Created = w.now()
	w.addGroup(g)
	w.refreshGroup(g, w.ruleEnv()) // a dynamic group has its members at once
	w.record(Event{Type: evGroupCreate, Actor: a, Group: g.ID, GroupName: g.Name, GroupObject: copyGroup(g)})
	return g.object(), nil
}

// newGroup makes the group of a create call, with the checks of Graph. It has no
// members and no owners yet.
func newGroup(props map[string]any) (*Group, *apiError) {
	if e := checkProps(props, groupProps, "Group"); e != nil {
		return nil, e
	}
	name, _ := props["displayName"].(string)
	nick, _ := props["mailNickname"].(string)
	mailEnabled, hasMail := props["mailEnabled"].(bool)
	security, hasSecurity := props["securityEnabled"].(bool)
	switch {
	case strings.TrimSpace(name) == "":
		return nil, missing("displayName")
	case strings.TrimSpace(nick) == "":
		return nil, missing("mailNickname")
	case !hasMail:
		return nil, missing("mailEnabled")
	case !hasSecurity:
		return nil, missing("securityEnabled")
	}

	g := &Group{ID: newGUID(), Name: name, Mail: mailEnabled, Security: security, MailNickname: nick, Members: []string{}}
	g.Description, _ = props["description"].(string)
	g.Rule, _ = props["membershipRule"].(string)
	g.RoleAssignable, _ = props["isAssignableToRole"].(bool)
	if e := setGroupTypes(g, props["groupTypes"]); e != nil {
		return nil, e
	}
	switch {
	case g.Dynamic && strings.TrimSpace(g.Rule) == "":
		return nil, badRequest("A group with dynamic membership needs the property 'membershipRule'.")
	case !g.Dynamic && g.Rule != "":
		return nil, badRequest("The property 'membershipRule' is only for a group with dynamic membership.")
	case g.RoleAssignable && g.Dynamic:
		return nil, badRequest("A group that can get Entra roles cannot have dynamic membership.")
	case g.RoleAssignable && !g.Security:
		return nil, badRequest("A group that can get Entra roles must be a security group (securityEnabled).")
	}
	return g, newDynamicSettings(g, props)
}

// newDynamicSettings checks the rule of a new dynamic group, and sets its processing state. It gives
// the error as the second result of newGroup (a nil *apiError when there is none).
func newDynamicSettings(g *Group, props map[string]any) *apiError {
	state, hasState := props["membershipRuleProcessingState"]
	if !g.Dynamic {
		if hasState {
			return badRequest("The property 'membershipRuleProcessingState' is only for a group with dynamic membership.")
		}
		return nil
	}
	if e := checkRule(g.Rule); e != nil {
		return e
	}
	g.RuleProcessing = processingOn
	if hasState {
		var e *apiError
		if g.RuleProcessing, e = checkProcessing(state); e != nil {
			return e
		}
	}
	return nil
}

// setGroupTypes reads groupTypes: Unified (Microsoft 365) and DynamicMembership.
func setGroupTypes(g *Group, groupTypes any) *apiError {
	types, _ := groupTypes.([]any)
	for _, t := range types {
		switch s, _ := t.(string); s {
		case "Unified":
			g.Unified = true
		case "DynamicMembership":
			g.Dynamic = true
		default:
			return badRequest("Invalid value specified for property 'groupTypes' of resource 'Group'.")
		}
	}
	return nil
}

// bindIDs reads the object IDs at the end of the references in members@odata.bind
// or owners@odata.bind.
func bindIDs(refs any) []string {
	list, _ := refs.([]any)
	ids := make([]string, 0, len(list))
	for _, b := range list {
		ref, _ := b.(string)
		ids = append(ids, ref[strings.LastIndex(ref, "/")+1:])
	}
	return ids
}

// setFields puts the fields of a group update in the group.
func (g *Group) setFields(set map[string]any) {
	for k, v := range set {
		s, _ := v.(string)
		b, _ := v.(bool)
		switch k {
		case "displayName":
			g.Name = s
		case "description":
			g.Description = s
		case "mailNickname":
			g.MailNickname = s
		case "membershipRule":
			g.Rule = s
		case "membershipRuleProcessingState":
			g.RuleProcessing = s
		case "mailEnabled":
			g.Mail = b
		case "securityEnabled":
			g.Security = b
		}
	}
}

// UpdateGroup changes properties of a group. As in Graph, a group cannot change
// its groupTypes or isAssignableToRole after it is made, and membershipRule is
// only for a group with dynamic membership. The members and the owners change with
// their own calls, not here.
func (w *World) UpdateGroup(id string, props map[string]any, a Actor) *apiError {
	for _, fixed := range []string{"groupTypes", "isAssignableToRole"} {
		if _, has := props[fixed]; has {
			return badRequest("The property '%s' cannot be changed after the group is made.", fixed)
		}
	}
	if _, has := props["members@odata.bind"]; has {
		return badRequest("Use the members calls to change the members of a group.")
	}
	if _, has := props["owners@odata.bind"]; has {
		return badRequest("Use the owners calls to change the owners of a group.")
	}
	if e := checkProps(props, groupProps, "Group"); e != nil {
		return e
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	g := w.groups[strings.ToLower(id)]
	if g == nil {
		return notFound("group", id)
	}
	set, e := groupChanges(g, props)
	if e != nil || len(set) == 0 {
		return e
	}
	g.setFields(set)
	w.refreshGroup(g, w.ruleEnv()) // a new rule, or a state that is On again, gives new members
	w.record(Event{Type: evGroupUpdate, Actor: a, Group: g.ID, GroupName: g.Name, Set: deepCopy(set)})
	return nil
}

// groupChanges works out what an update does to a group, or why it cannot.
func groupChanges(g *Group, props map[string]any) (map[string]any, *apiError) {
	set := map[string]any{}
	for k, v := range props {
		switch k {
		case "visibility":
			continue // accepted, and not kept
		case "membershipRuleProcessingState":
			if !g.Dynamic {
				return nil, badRequest("The property 'membershipRuleProcessingState' is only for a group with dynamic membership.")
			}
			state, e := checkProcessing(v)
			if e != nil {
				return nil, e
			}
			v = state
		case "displayName", "mailNickname":
			if s, _ := v.(string); strings.TrimSpace(s) == "" {
				return nil, badRequest("Invalid value specified for property '%s' of resource 'Group'.", k)
			}
		case "membershipRule":
			if !g.Dynamic {
				return nil, badRequest("The property 'membershipRule' is only for a group with dynamic membership.")
			}
			if s, _ := v.(string); strings.TrimSpace(s) == "" {
				return nil, badRequest("A group with dynamic membership needs the property 'membershipRule'.")
			}
			if e := checkRule(str(v)); e != nil {
				return nil, e
			}
		case "mailEnabled", "securityEnabled":
			if v == nil {
				return nil, badRequest("Invalid value specified for property '%s' of resource 'Group'.", k)
			}
		}
		set[k] = v
	}
	return set, nil
}

// DeleteGroup deletes a group.
func (w *World) DeleteGroup(id string, a Actor) *apiError {
	w.mu.Lock()
	defer w.mu.Unlock()
	g := w.groups[strings.ToLower(id)]
	if g == nil {
		return notFound("group", id)
	}
	gid, name := g.ID, g.Name
	n := w.removeGroup(gid)
	w.record(Event{Type: evGroupDelete, Actor: a, Group: gid, GroupName: name, Changed: n})
	return nil
}

// removeGroup takes a group out of the world and out of every group that held it.
// It returns in how many groups it was a member. Call it with the lock held.
func (w *World) removeGroup(id string) int {
	key := strings.ToLower(id)
	if w.groups[key] == nil {
		return 0
	}
	delete(w.groups, key)
	w.Groups = slices.DeleteFunc(w.Groups, func(g *Group) bool { return isID(g.ID, id) })
	return w.dropReferences(id)
}

// dropReferences takes an object ID out of every member list, every owner list,
// and every app role assignment. It returns the number of groups that changed. A
// delete of a user or of a group uses it. Call it with the lock held.
func (w *World) dropReferences(id string) int {
	n := 0
	for _, g := range w.Groups {
		before := len(g.Members) + len(g.Owners)
		if slices.ContainsFunc(g.Members, func(m string) bool { return isID(m, id) }) {
			w.noteMember(g.ID, id, true) // delta query: the group lost this member
		}
		g.removeMember(id)
		g.Owners = slices.DeleteFunc(g.Owners, func(o string) bool { return isID(o, id) })
		if len(g.Members)+len(g.Owners) != before {
			n++
		}
	}
	w.Assignments = slices.DeleteFunc(w.Assignments, func(a Assignment) bool { return isID(a.PrincipalID, id) })
	return n
}

// ---- the history of groups ----

// groupEventText says what an event of the groups did. It reports whether it is one.
func groupEventText(e Event) (string, bool) {
	switch e.Type {
	case evGroupCreate:
		return fmt.Sprintf("Made the group %s", firstNonEmpty(e.GroupName, e.Group)), true
	case evGroupUpdate:
		return fmt.Sprintf("Changed the group %s: %s", firstNonEmpty(e.GroupName, e.Group), setText(e.Set)), true
	case evGroupDelete:
		return fmt.Sprintf("Deleted the group %s (it was in %d %s)", firstNonEmpty(e.GroupName, e.Group), e.Changed, plural(e.Changed, "group", "groups")), true
	}
	return "", false
}

// applyGroupEvent does an event of the groups on a world that nobody else uses. It
// reports whether it is one.
func (w *World) applyGroupEvent(e Event) bool {
	switch e.Type {
	case evGroupCreate:
		if e.GroupObject != nil {
			w.addGroup(copyGroup(e.GroupObject))
		}
	case evGroupUpdate:
		if g := w.groups[strings.ToLower(e.Group)]; g != nil {
			g.setFields(e.Set)
		}
	case evGroupDelete:
		w.removeGroup(e.Group)
	default:
		return false
	}
	return true
}
