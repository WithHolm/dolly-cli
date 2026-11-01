package api

import "net/http"

// ApplicationsBackend is what the applications resource (the app registrations of the tenant) needs from the data.
type ApplicationsBackend interface {
	ListApplications() []Object
	// GetApplication finds an application by object ID, or by appId.
	GetApplication(ref string) (Object, bool)
	CreateApplication(props Object, a Actor) (Object, *Error)
	UpdateApplication(ref string, props Object, a Actor) *Error
	DeleteApplication(ref string, a Actor) *Error
}

// Applications is the resource for /applications. A call can also name an application by its appId: applications(appId='...') (the server
// rewrites it to /applications/{appId} before the route).
func Applications(b ApplicationsBackend) Resource { return applications{b} }

type applications struct{ b ApplicationsBackend }

func (a applications) Register(r *Router) {
	env := r.Env()
	s := r.Scope("/applications", "App registrations")
	list := []string{"$select", "$top", "$filter", "$search", "$count", "$orderby"}

	s.Handle("GET", "", "List the app registrations of the tenant. An application is the definition of an app, in its home tenant only. Its service principal (the instance in a tenant) has the same appId, and a different id.", list,
		func(w http.ResponseWriter, req *http.Request) {
			ServeList(w, req, env, a.b.ListApplications(), ListOptions{Filterable: true, Resource: "application", Context: "applications"})
		})
	s.Handle("GET", "/$count", "Count the app registrations. Needs ConsistencyLevel: eventual.", []string{"$filter", "$search"},
		func(w http.ResponseWriter, req *http.Request) {
			ServeCount(w, req, a.b.ListApplications(), ListOptions{Filterable: true, Resource: "application"})
		})
	s.Handle("GET", "/{id}", "Get one app registration, by object ID or by appId (applications(appId='...')).", []string{"$select"},
		func(w http.ResponseWriter, req *http.Request) {
			o, ok := a.b.GetApplication(req.PathValue("id"))
			if !ok {
				WriteError(w, NotFound("application", req.PathValue("id")))
				return
			}
			WriteJSON(w, http.StatusOK, one("applications", o, req, nil, nil, "application"))
		})
	s.Handle("POST", "", "Register an app. Needs displayName. id, appId, and createdDateTime are made. requiredResourceAccess says which permissions the app asks for: for Microsoft Graph, each permission ID must be one that dollycli knows (else 404, with what to do). Gives 201 with the application. The application has no service principal until a client signs in with its appId.", nil,
		func(w http.ResponseWriter, req *http.Request) {
			props, e := ReadJSON(req)
			if e == nil {
				var o Object
				if o, e = a.b.CreateApplication(props, env.Actor(req)); e == nil {
					WriteJSON(w, http.StatusCreated, o)
					return
				}
			}
			WriteError(w, e)
		})
	s.Handle("PATCH", "/{id}", "Change properties of an app registration, by object ID or by appId. requiredResourceAccess is replaced as a whole, and for Microsoft Graph each permission ID must be one that dollycli knows. An object property (web, spa, publicClient) changes only the properties that the call names. Gives 204.", nil,
		func(w http.ResponseWriter, req *http.Request) {
			props, e := ReadJSON(req)
			if e == nil {
				e = a.b.UpdateApplication(req.PathValue("id"), props, env.Actor(req))
			}
			respond(w, e)
		})
	s.Handle("DELETE", "/{id}", "Delete an app registration, by object ID or by appId. Its service principal stays. Gives 204.", nil,
		func(w http.ResponseWriter, req *http.Request) {
			respond(w, a.b.DeleteApplication(req.PathValue("id"), env.Actor(req)))
		})
}
