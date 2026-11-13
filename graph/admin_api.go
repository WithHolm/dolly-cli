package graph

import (
	"net/http"
)

// The admin API of the page "Expose an API" in the portal: the Application ID URI, the scopes, and the clients that need no consent. The rules and the history
// are those of the repo package (repo/scopes.go), which are those of PATCH /applications: the Graph API reads what these calls change, and the other way round.
// An app is found by object ID or client ID.
func (s *Server) adminAPIRoutes() {
	a := s.admin
	missingApp := func(w http.ResponseWriter, ref string) {
		writeError(w, apiErr(http.StatusNotFound, "Request_ResourceNotFound", "No app registration has the ID '"+ref+"'."))
	}
	a.HandleFunc("GET /applications/{id}/api", func(w http.ResponseWriter, r *http.Request) {
		settings, ok := s.world.APISettings(r.PathValue("id"))
		if !ok {
			missingApp(w, r.PathValue("id"))
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})
	a.HandleFunc("PUT /applications/{id}/identifier-uris", func(w http.ResponseWriter, r *http.Request) {
		props, e := readAdminJSON(r)
		if e == nil {
			var uris []string
			for _, u := range anySliceOf(props["identifierUris"]) {
				uris = append(uris, str(u))
			}
			e = s.world.SetIdentifierURIs(r.PathValue("id"), uris, adminActor)
		}
		s.adminChanged(w, e)
	})
	// the version of the access tokens of this API: 1, 2, or null (1)
	a.HandleFunc("PUT /applications/{id}/access-token-version", func(w http.ResponseWriter, r *http.Request) {
		props, e := readAdminJSON(r)
		if e == nil {
			v, has := props["version"]
			n, isNumber := v.(float64)
			switch {
			case !has:
				e = badRequest("Invalid value specified for property 'requestedAccessTokenVersion' of resource 'Application': send version: null, 1, or 2.")
			case v != nil && !isNumber:
				e = badRequest("Invalid value specified for property 'requestedAccessTokenVersion' of resource 'Application': it must be null, 1, or 2.")
			default:
				e = s.world.SetAccessTokenVersion(r.PathValue("id"), int(n), adminActor)
			}
		}
		s.adminChanged(w, e)
	})
	a.HandleFunc("POST /applications/{id}/api-scopes", func(w http.ResponseWriter, r *http.Request) {
		props, e := readAdminJSON(r)
		if e != nil {
			writeError(w, e)
			return
		}
		scope, e := s.world.AddScope(r.PathValue("id"), props, adminActor)
		if e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, http.StatusCreated, scope)
	})
	a.HandleFunc("PATCH /applications/{id}/api-scopes/{scopeId}", func(w http.ResponseWriter, r *http.Request) {
		props, e := readAdminJSON(r)
		if e == nil {
			e = s.world.EditScope(r.PathValue("id"), r.PathValue("scopeId"), props, adminActor)
		}
		s.adminChanged(w, e)
	})
	a.HandleFunc("DELETE /applications/{id}/api-scopes/{scopeId}", func(w http.ResponseWriter, r *http.Request) {
		s.adminChanged(w, s.world.RemoveScope(r.PathValue("id"), r.PathValue("scopeId"), adminActor))
	})
	a.HandleFunc("PUT /applications/{id}/pre-authorized", func(w http.ResponseWriter, r *http.Request) {
		props, e := readAdminJSON(r)
		if e == nil {
			var ids []string
			for _, x := range anySliceOf(props["delegatedPermissionIds"]) {
				ids = append(ids, str(x))
			}
			e = s.world.SetPreAuthorized(r.PathValue("id"), str(props["appId"]), ids, adminActor)
		}
		s.adminChanged(w, e)
	})
	a.HandleFunc("DELETE /applications/{id}/pre-authorized/{appId}", func(w http.ResponseWriter, r *http.Request) {
		s.adminChanged(w, s.world.RemovePreAuthorized(r.PathValue("id"), r.PathValue("appId"), adminActor))
	})
}

func anySliceOf(v any) []any {
	l, _ := v.([]any)
	return l
}
