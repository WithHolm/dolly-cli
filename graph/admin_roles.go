package graph

import (
	"encoding/json"
	"net/http"
)

// readAdminJSON reads the JSON object of the body.
func readAdminJSON(r *http.Request) (map[string]any, *apiError) {
	var props map[string]any
	if err := json.NewDecoder(r.Body).Decode(&props); err != nil || props == nil {
		return nil, badRequest("The body must be a JSON object.")
	}
	return props, nil
}

func str(v any) string { s, _ := v.(string); return s }

// The admin API of the app roles in the portal: the roles of an app registration (they are defined there), and who is assigned to the roles of a service
// principal. The rules and the history are those of the repo package (repo/roles.go), which are those of the Graph API: the Graph API reads what these
// calls change, and the other way round. An app is found by object ID or client ID.
func (s *Server) adminRoleRoutes() {
	a := s.admin
	a.HandleFunc("GET /applications/{id}/app-roles", func(w http.ResponseWriter, r *http.Request) {
		roles, ok := s.world.AppRoles(r.PathValue("id"))
		if !ok {
			writeError(w, apiErr(http.StatusNotFound, "Request_ResourceNotFound", "No app registration has the ID '"+r.PathValue("id")+"'."))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"roles": roles})
	})
	a.HandleFunc("POST /applications/{id}/app-roles", func(w http.ResponseWriter, r *http.Request) {
		props, e := readAdminJSON(r)
		if e != nil {
			writeError(w, e)
			return
		}
		role, e := s.world.AddAppRole(r.PathValue("id"), props, adminActor)
		if e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, http.StatusCreated, role)
	})
	a.HandleFunc("PATCH /applications/{id}/app-roles/{roleId}", func(w http.ResponseWriter, r *http.Request) {
		props, e := readAdminJSON(r)
		if e == nil {
			e = s.world.EditAppRole(r.PathValue("id"), r.PathValue("roleId"), props, adminActor)
		}
		s.adminChanged(w, e)
	})
	a.HandleFunc("DELETE /applications/{id}/app-roles/{roleId}", func(w http.ResponseWriter, r *http.Request) {
		s.adminChanged(w, s.world.RemoveAppRole(r.PathValue("id"), r.PathValue("roleId"), adminActor))
	})

	// the other side: who is assigned to the roles of a service principal
	a.HandleFunc("GET /service-principals/{appId}/role-assignments", func(w http.ResponseWriter, r *http.Request) {
		roles, assignments, ok := s.world.RoleAssignments(r.PathValue("appId"))
		if !ok {
			writeError(w, apiErr(http.StatusNotFound, "Request_ResourceNotFound", "No service principal has the client ID '"+r.PathValue("appId")+"'."))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"roles": roles, "assignments": assignments})
	})
	a.HandleFunc("POST /service-principals/{appId}/role-assignments", func(w http.ResponseWriter, r *http.Request) {
		props, e := readAdminJSON(r)
		if e == nil {
			principal, role := str(props["principalId"]), str(props["appRoleId"])
			if principal == "" || role == "" {
				e = badRequest("An assignment needs principalId and appRoleId.")
			} else if sp, ok := s.world.GetServicePrincipal(r.PathValue("appId")); !ok {
				e = apiErr(http.StatusNotFound, "Request_ResourceNotFound", "No service principal has the client ID '"+r.PathValue("appId")+"'.")
			} else {
				_, e = s.world.AssignAppRole(str(sp["id"]), principal, role, adminActor)
			}
		}
		s.adminChanged(w, e)
	})
	a.HandleFunc("DELETE /service-principals/{appId}/role-assignments/{assignmentId}", func(w http.ResponseWriter, r *http.Request) {
		var e *apiError
		if sp, ok := s.world.GetServicePrincipal(r.PathValue("appId")); !ok {
			e = apiErr(http.StatusNotFound, "Request_ResourceNotFound", "No service principal has the client ID '"+r.PathValue("appId")+"'.")
		} else {
			e = s.world.DeleteAppRoleAssignment(str(sp["id"]), r.PathValue("assignmentId"), true, adminActor)
		}
		s.adminChanged(w, e)
	})
}
