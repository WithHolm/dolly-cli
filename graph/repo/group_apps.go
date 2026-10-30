package repo

import "strings"

// The apps that a group is assigned to. The data side of api/group_apps.go. It
// satisfies api.GroupApps. (The service principals themselves are in apps.go.)

// GroupAppRoleAssignments lists the apps that a group is assigned to. The bool is
// false when there is no such group.
func (w *World) GroupAppRoleAssignments(groupID string) ([]Object, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	id := strings.ToLower(groupID)
	if w.groups[id] == nil {
		return nil, false
	}
	var items []Object
	for _, a := range w.Assignments {
		if !isID(a.PrincipalID, id) {
			continue
		}
		name := ""
		if sp := w.sps[strings.ToLower(a.ResourceID)]; sp != nil {
			name = sp.Name
		}
		items = append(items, Object{
			"id": a.PrincipalID + "." + a.RoleID, "resourceId": a.ResourceID, "resourceDisplayName": name,
			"principalId": a.PrincipalID, "appRoleId": a.RoleID,
		})
	}
	return items, true
}
