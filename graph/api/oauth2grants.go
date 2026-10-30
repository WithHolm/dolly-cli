package api

import "net/http"

// OAuth2GrantsBackend is what the consents to delegated permissions need from the data.
type OAuth2GrantsBackend interface {
	ListOAuth2Grants() []Object
	GetOAuth2Grant(id string) (Object, bool)
	CreateOAuth2Grant(props map[string]any, a Actor) (Object, *Error)
	UpdateOAuth2Grant(id string, props map[string]any, a Actor) *Error
	DeleteOAuth2Grant(id string, a Actor) *Error
}

// OAuth2PermissionGrants is the resource for /oauth2PermissionGrants: the delegated permissions
// (scopes) of a resource that a client service principal has, for all users or for one.
func OAuth2PermissionGrants(b OAuth2GrantsBackend) Resource { return oauth2Grants{b} }

type oauth2Grants struct{ b OAuth2GrantsBackend }

func (g oauth2Grants) Register(r *Router) {
	env := r.Env()
	s := r.Scope("/oauth2PermissionGrants", "Permission grants")

	s.Handle("GET", "", "List the consents to delegated permissions. A $filter on clientId, resourceId, consentType, or principalId keeps some.", []string{"$select", "$top", "$filter", "$count", "$orderby"},
		func(w http.ResponseWriter, req *http.Request) {
			ServeList(w, req, env, g.b.ListOAuth2Grants(), ListOptions{Filterable: true, NoConsistency: true, Context: "oauth2PermissionGrants"})
		})
	s.Handle("GET", "/$count", "Count the consents.", []string{"$filter"},
		func(w http.ResponseWriter, req *http.Request) {
			ServeCount(w, req, g.b.ListOAuth2Grants(), ListOptions{Filterable: true, NoConsistency: true})
		})
	s.Handle("GET", "/{id}", "Get one consent.", []string{"$select"},
		func(w http.ResponseWriter, req *http.Request) {
			o, ok := g.b.GetOAuth2Grant(req.PathValue("id"))
			if !ok {
				WriteError(w, NotFound("oAuth2PermissionGrant", req.PathValue("id")))
				return
			}
			WriteJSON(w, http.StatusOK, one("oauth2PermissionGrants", o, req, nil, nil, ""))
		})
	s.Handle("POST", "", "Make a consent. Needs clientId and resourceId (object IDs of service principals), consentType (AllPrincipals, or Principal with the principalId of a user), and scope: delegated permissions of the resource, with a space between. A scope that the resource does not have is refused. Gives 201.", nil,
		func(w http.ResponseWriter, req *http.Request) {
			props, e := ReadJSON(req)
			if e == nil {
				var o Object
				if o, e = g.b.CreateOAuth2Grant(props, env.Actor(req)); e == nil {
					WriteJSON(w, http.StatusCreated, o)
					return
				}
			}
			WriteError(w, e)
		})
	s.Handle("PATCH", "/{id}", "Change the scope of a consent. Gives 204.", nil,
		func(w http.ResponseWriter, req *http.Request) {
			props, e := ReadJSON(req)
			if e == nil {
				e = g.b.UpdateOAuth2Grant(req.PathValue("id"), props, env.Actor(req))
			}
			respond(w, e)
		})
	s.Handle("DELETE", "/{id}", "Take a consent away. Gives 204.", nil,
		func(w http.ResponseWriter, req *http.Request) {
			respond(w, g.b.DeleteOAuth2Grant(req.PathValue("id"), env.Actor(req)))
		})
}
