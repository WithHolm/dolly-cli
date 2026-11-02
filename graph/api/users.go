package api

import "net/http"

// UsersBackend is what the users resource needs from the data: the users
// themselves, and their managers (user_manager.go).
type UsersBackend interface {
	UserCore
	UserManagers
	UserRelations
}

// UserCore is what the users need from the data. The lists are copies, so a
// handler can read them while the data changes.
type UserCore interface {
	// ListUsers gives every user, in the order of the data.
	ListUsers() []Object
	// GetUser finds a user by object ID or by user principal name.
	GetUser(ref string) (Object, bool)
	// CreateUser makes a user and gives a copy of it.
	CreateUser(props Object, a Actor) (Object, *Error)
	// UpdateUser changes properties of a user, found by object ID or user principal name.
	UpdateUser(ref string, props Object, a Actor) *Error
	// DeleteUser deletes a user, and every reference to it.
	DeleteUser(ref string, a Actor) *Error
}

// DefaultUserProperties are the properties that Graph returns for a user when the
// call has no $select (the Learn page "Get user"; the real Graph gives id last, and null for a property with no value).
// Every other property needs $select. With a $select, id is not added: only what it names is returned.
var DefaultUserProperties = []string{
	"businessPhones", "displayName", "givenName", "jobTitle", "mail", "mobilePhone",
	"officeLocation", "preferredLanguage", "surname", "userPrincipalName", "id",
}

// DefaultUserDeltaProperties are the properties that /users/delta tracks and returns when the call has no $select. They are fewer than the
// ones of /users (checked with the real Graph).
var DefaultUserDeltaProperties = []string{"displayName", "mail", "userPrincipalName", "id"}

// Users is the resource for /users.
func Users(b UsersBackend) Resource { return users{b} }

type users struct{ b UsersBackend }

func (u users) Register(r *Router) {
	env := r.Env()
	s := r.Scope("/users", "Users")
	list := []string{"$select", "$top", "$filter", "$search", "$count", "$orderby", "$expand"}
	// a client can subscribe to all users, or to one (Learn: Supported resources)
	r.Subscribable(Subscribable{Pattern: "users", Kind: "user", MaxMinutes: directoryLifetime}, Subscribable{Pattern: "users/{id}", Kind: "user", MaxMinutes: directoryLifetime})

	s.Handle("GET", "", "List the users. $expand adds the manager or the direct reports of each user.", list, func(w http.ResponseWriter, req *http.Request) {
		items := u.b.ListUsers()
		if e := expandManager(req, u.b, false, items...); e != nil {
			WriteError(w, e)
			return
		}
		ServeList(w, req, env, items, ListOptions{Filterable: true, Resource: "user", Context: "users", Extra: []string{"manager", "directReports"}, Default: DefaultUserProperties})
	})
	if feed, ok := u.b.(DeltaFeed); ok { // the data has a log of changes (the fake of a test may not)
		src := DeltaSource{Feed: feed, Kind: "user", Set: "users", Tracked: DefaultUserDeltaProperties, List: u.b.ListUsers, Get: u.b.GetUser}
		serve := func(w http.ResponseWriter, req *http.Request) { ServeDelta(w, req, src) }
		const summary = "Read the users, and then only what changed since (delta query). The first call gives every user in pages, and ends with an @odata.deltaLink. Call that link to get the users that were made, changed, or deleted since. A deleted user is its id and @removed. $select says which properties are tracked, and $filter takes only id eq '…'."
		opts := []string{"$select", "$filter", "$deltatoken", "$skiptoken"}
		s.Handle("GET", "/delta", summary, opts, serve)
		s.Handle("GET", "/delta()", summary, opts, serve)
	}
	s.Handle("GET", "/$count", "Count the users. Needs ConsistencyLevel: eventual.", []string{"$filter", "$search"}, func(w http.ResponseWriter, req *http.Request) {
		ServeCount(w, req, u.b.ListUsers(), ListOptions{Filterable: true, Resource: "user"})
	})
	s.Handle("GET", "/{id}", "Get one user, by object ID or user principal name.", []string{"$select", "$expand"}, func(w http.ResponseWriter, req *http.Request) {
		o, ok := u.b.GetUser(req.PathValue("id"))
		if !ok {
			WriteError(w, NotFound("user", req.PathValue("id")))
			return
		}
		if e := expandManager(req, u.b, true, o); e != nil {
			WriteError(w, e)
			return
		}
		WriteJSON(w, http.StatusOK, one("users", o, req, DefaultUserProperties, []string{"manager", "directReports"}, "user"))
	})
	registerManager(s, env, u.b)
	registerRelations(s, env, u.b)
	s.Handle("POST", "", "Make a user. Needs accountEnabled, displayName, mailNickname, userPrincipalName, and passwordProfile. Gives 201 with the user. A user principal name that is taken, an unknown property, and a value of the wrong type give 400.", nil,
		func(w http.ResponseWriter, req *http.Request) {
			props, e := ReadJSON(req)
			if e == nil {
				var o Object
				if o, e = u.b.CreateUser(props, env.Actor(req)); e == nil {
					WriteJSON(w, http.StatusCreated, o)
					return
				}
			}
			WriteError(w, e)
		})
	s.Handle("PATCH", "/{id}", "Change properties of a user, by object ID or user principal name. null clears a property. onPremisesExtensionAttributes is merged. Gives 204.", nil,
		func(w http.ResponseWriter, req *http.Request) {
			props, e := ReadJSON(req)
			if e == nil {
				e = u.b.UpdateUser(req.PathValue("id"), props, env.Actor(req))
			}
			respond(w, e)
		})
	s.Handle("DELETE", "/{id}", "Delete a user, by object ID or user principal name. The user also goes out of every group (as a member and as an owner) and out of the app role assignments. Gives 204.", nil,
		func(w http.ResponseWriter, req *http.Request) {
			respond(w, u.b.DeleteUser(req.PathValue("id"), env.Actor(req)))
		})
}

// respond answers 204 when there is no error, and the error when there is one.
func respond(w http.ResponseWriter, e *Error) {
	if e != nil {
		WriteError(w, e)
		return
	}
	NoContent(w)
}
