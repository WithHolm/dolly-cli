// Package repo is the data of dolly: the world (users, groups, service principals), the
// history of changes, and the two files that keep them. It has one file for each
// resource, as the api package has: users.go, groups.go (with group_members.go,
// group_owners.go, and group_apps.go), and apps.go. A resource file holds the
// functions that read and change that resource, the history events that it makes,
// how it describes them, and how a replay applies them. The world in world.go is
// what they share: the data, one lock, and the lookups by ID.
//
// The world satisfies the interfaces of the api package (api.UsersBackend,
// api.GroupsBackend, api.ServicePrincipalsBackend), so the Graph API reads and changes the
// data through the same functions that the admin API and the page use.
package repo

import "github.com/withholm/dolly-cli/graph/api"

// The names of the api package, as the files of this package use them. The types
// are aliases, so a method that returns *apiError returns *api.Error, and the
// world satisfies the interfaces of the resources.

type (
	// Object is one Graph object as JSON, with the camelCase names that Graph writes.
	Object = api.Object
	// Actor says who made a change.
	Actor = api.Actor

	apiError = api.Error
)

const (
	typeUser  = api.TypeUser
	typeGroup = api.TypeGroup
	typeSP    = api.TypeServicePrincipal
)

func notFound(what, id string) *apiError              { return api.NotFound(what, id) }
func badRequest(format string, args ...any) *apiError { return api.BadRequest(format, args...) }
func str(v any) string                                { return api.Str(v) }
