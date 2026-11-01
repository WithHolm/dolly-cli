package api

import (
	"net/http"
	"strings"
)

// ServicePrincipalsBackend is what the service principals resource needs from the data.
type ServicePrincipalsBackend interface {
	// ListServicePrincipals gives every service principal, in the order of the data.
	ListServicePrincipals() []Object
	GetServicePrincipal(id string) (Object, bool)
	// CreateServicePrincipal makes the instance of an app (needs appId). UpdateServicePrincipal and DeleteServicePrincipal take the object ID or the appId.
	CreateServicePrincipal(props Object, a Actor) (Object, *Error)
	UpdateServicePrincipal(ref string, props Object, a Actor) *Error
	DeleteServicePrincipal(ref string, a Actor) *Error
	// AppRoleAssignedTo lists who is assigned to a service principal. The bool is false when
	// there is no such service principal.
	AppRoleAssignedTo(appID string) ([]Object, bool)
	// AppRoleAssignments lists the app roles that are assigned to a service principal.
	AppRoleAssignments(spID string) ([]Object, bool)
	AssignAppRole(resourceID, principalID, roleID string, a Actor) (Object, *Error)
	// DeleteAppRoleAssignment takes one away. asResource says that spID is the resource and not the principal.
	DeleteAppRoleAssignment(spID, assignmentID string, asResource bool, a Actor) *Error
}

// DefaultServicePrincipalDeltaProperties are the properties that /servicePrincipals/delta tracks and returns when the call has no $select
// (checked with the real Graph). Unlike the list, the delta has the roles and the scopes. A property that the object does not have is left out.
var DefaultServicePrincipalDeltaProperties = []string{
	"accountEnabled", "appDisplayName", "appId", "appOwnerOrganizationId", "createdDateTime", "displayName", "logoutUrl", "replyUrls",
	"servicePrincipalNames", "servicePrincipalType", "id", "appRoles", "keyCredentials", "oauth2PermissionScopes", "passwordCredentials",
	"resourceSpecificApplicationPermissions",
}

// ServicePrincipals is the resource for /servicePrincipals.
func ServicePrincipals(b ServicePrincipalsBackend) Resource { return servicePrincipals{b} }

type servicePrincipals struct{ b ServicePrincipalsBackend }

func (a servicePrincipals) Register(r *Router) {
	env := r.Env()
	s := r.Scope("/servicePrincipals", "Service principals")
	list := []string{"$select", "$top", "$filter", "$search", "$count", "$orderby"}

	s.Handle("GET", "", "List the service principals. A filter on the tag WindowsAzureActiveDirectoryIntegratedApp gives the enterprise applications.", list,
		func(w http.ResponseWriter, req *http.Request) {
			ServeList(w, req, env, a.b.ListServicePrincipals(), ListOptions{Filterable: true, Resource: "servicePrincipal", Context: "servicePrincipals"})
		})
	if feed, ok := a.b.(DeltaFeed); ok { // the data has a log of changes (the fake of a test may not)
		src := DeltaSource{Feed: feed, Kind: "servicePrincipal", Set: "servicePrincipals", Tracked: DefaultServicePrincipalDeltaProperties,
			// the list has no roles and scopes, and the delta does: every object is the one that GET /{id} gives
			List: func() []Object {
				var out []Object
				for _, o := range a.b.ListServicePrincipals() {
					if full, ok := a.b.GetServicePrincipal(Str(o["id"])); ok {
						out = append(out, full)
					}
				}
				return out
			},
			Get: a.b.GetServicePrincipal}
		serve := func(w http.ResponseWriter, req *http.Request) { ServeDelta(w, req, src) }
		const summary = "Read the service principals, and then only what changed since (delta query). It works as /users/delta. In dollycli a service principal is made (by the first sign-in of a client) and never changed or deleted, so a later round gives the new ones."
		opts := []string{"$select", "$filter", "$deltatoken", "$skiptoken"}
		s.Handle("GET", "/delta", summary, opts, serve)
		s.Handle("GET", "/delta()", summary, opts, serve)
	}
	s.Handle("GET", "/$count", "Count the service principals. Needs ConsistencyLevel: eventual.", []string{"$filter", "$search"},
		func(w http.ResponseWriter, req *http.Request) {
			ServeCount(w, req, a.b.ListServicePrincipals(), ListOptions{Filterable: true, Resource: "servicePrincipal"})
		})
	s.Handle("GET", "/{id}", "Get one service principal, with its roles and scopes. (Microsoft Graph has more than a thousand: only this call has them.)", []string{"$select"},
		func(w http.ResponseWriter, req *http.Request) {
			o, ok := a.b.GetServicePrincipal(req.PathValue("id"))
			if !ok {
				WriteError(w, NotFound("servicePrincipal", req.PathValue("id")))
				return
			}
			WriteJSON(w, http.StatusOK, one("servicePrincipals", o, req, nil, nil, "servicePrincipal"))
		})
	s.Handle("POST", "", "Make the service principal of an app: the instance of it in this tenant. Needs appId, and the app must be registered here (/applications), or be one of Microsoft. It takes its name, roles, scopes, identifier URIs, and reply URLs from the application. Gives 201 with the service principal.", nil,
		func(w http.ResponseWriter, req *http.Request) {
			props, e := ReadJSON(req)
			if e == nil {
				var o Object
				if o, e = a.b.CreateServicePrincipal(props, env.Actor(req)); e == nil {
					WriteJSON(w, http.StatusCreated, o)
					return
				}
			}
			WriteError(w, e)
		})
	s.Handle("PATCH", "/{id}", "Change properties of a service principal, by object ID or by appId (servicePrincipals(appId='...')). The tag WindowsAzureActiveDirectoryIntegratedApp in tags makes it an enterprise application. Gives 204.", nil,
		func(w http.ResponseWriter, req *http.Request) {
			props, e := ReadJSON(req)
			if e == nil {
				e = a.b.UpdateServicePrincipal(req.PathValue("id"), props, env.Actor(req))
			}
			respond(w, e)
		})
	s.Handle("DELETE", "/{id}", "Delete a service principal, by object ID or by appId. Its role assignments and consents go with it. Its app registration stays. Microsoft Graph and the service principals that dollycli needs cannot be deleted. Gives 204.", nil,
		func(w http.ResponseWriter, req *http.Request) {
			respond(w, a.b.DeleteServicePrincipal(req.PathValue("id"), env.Actor(req)))
		})
	s.Handle("GET", "/{id}/appRoleAssignedTo", "List who is assigned to a service principal, with the role.", []string{"$select", "$top", "$count", "$orderby"},
		func(w http.ResponseWriter, req *http.Request) {
			items, ok := a.b.AppRoleAssignedTo(req.PathValue("id"))
			if !ok {
				WriteError(w, NotFound("servicePrincipal", req.PathValue("id")))
				return
			}
			ServeList(w, req, env, items, ListOptions{})
		})

	// The two sides of an app role assignment: the principal (what a service principal may do with other
	// resources) and the resource (who may use it). An application permission of Microsoft Graph is an
	// assignment of one of its roles to a service principal.
	s.Handle("GET", "/{id}/appRoleAssignments", "List the app roles that are assigned to a service principal: the application permissions it has on other resources.", []string{"$select", "$top", "$count", "$orderby"},
		func(w http.ResponseWriter, req *http.Request) {
			items, ok := a.b.AppRoleAssignments(req.PathValue("id"))
			if !ok {
				WriteError(w, NotFound("servicePrincipal", req.PathValue("id")))
				return
			}
			ServeList(w, req, env, items, ListOptions{})
		})
	assign := func(asResource bool) http.HandlerFunc {
		return func(w http.ResponseWriter, req *http.Request) {
			props, e := ReadJSON(req)
			if e != nil {
				WriteError(w, e)
				return
			}
			id := req.PathValue("id")
			principal, resource, role := Str(props["principalId"]), Str(props["resourceId"]), Str(props["appRoleId"])
			switch {
			case principal == "" || resource == "" || role == "":
				WriteError(w, BadRequest("principalId, resourceId, and appRoleId are required."))
				return
			case asResource && !strings.EqualFold(resource, id):
				WriteError(w, BadRequest("The resourceId must be the ID in the path."))
				return
			case !asResource && !strings.EqualFold(principal, id):
				WriteError(w, BadRequest("The principalId must be the ID in the path."))
				return
			}
			o, e := a.b.AssignAppRole(resource, principal, role, env.Actor(req))
			if e != nil {
				WriteError(w, e)
				return
			}
			WriteJSON(w, http.StatusCreated, o)
		}
	}
	remove := func(asResource bool) http.HandlerFunc {
		return func(w http.ResponseWriter, req *http.Request) {
			respond(w, a.b.DeleteAppRoleAssignment(req.PathValue("id"), req.PathValue("assignmentId"), asResource, env.Actor(req)))
		}
	}
	s.Handle("POST", "/{id}/appRoleAssignments", "Assign an app role to this service principal: give it an application permission of a resource. Needs principalId (this one), resourceId, and appRoleId. The application permissions of Microsoft Graph are for a service principal only. Gives 201.", nil, assign(false))
	s.Handle("DELETE", "/{id}/appRoleAssignments/{assignmentId}", "Take an app role away from this service principal. Gives 204.", nil, remove(false))
	s.Handle("POST", "/{id}/appRoleAssignedTo", "Assign an app role of this resource to a user, a group, or a service principal. Needs principalId, resourceId (this one), and appRoleId. Gives 201.", nil, assign(true))
	s.Handle("DELETE", "/{id}/appRoleAssignedTo/{assignmentId}", "Take an app role of this resource away from a principal. Gives 204.", nil, remove(true))
}
