package graph

import (
	"net/http"

	"github.com/withholm/dolly-cli/graph/api"
)

// env is what the resources ask of the server.
type env struct{ s *Server }

func (e env) PageSize() int               { return e.s.behavior().PageSize }
func (e env) Actor(r *http.Request) Actor { return graphActor(r) }

// mountAPI puts the resources of the Graph API on the mux. This is the one place
// that lists them: to add a resource, write its file in the api package and add
// one line here. $batch is not a resource: it runs the other resources, so it is
// part of the server (batch.go).
func (s *Server) mountAPI() {
	router := api.NewRouter(s.mux, BasePath, env{s})
	router.Mount(
		api.Users(s.world),
		api.Groups(s.world),
		api.ServicePrincipals(s.world),
		api.Applications(s.world),
		api.OAuth2PermissionGrants(s.world),
		api.Places(s.world),
		api.Sites(s.world),
		api.Drives(s.world),
		api.Organization(s.world),
		api.Subscriptions(subscriptions{s}),
	)
	s.startNotifier(router.Subscribables()) // after the resources: they say what can be subscribed to
	s.endpoints = router.Endpoints()
	s.endpoints = append(s.endpoints, meEndpoints(s.endpoints)...)

	s.mux.HandleFunc("GET "+api.DownloadPrefix+"{drive}/{item}/{tag}/{name}", s.download)
	s.mux.HandleFunc("GET "+metadataPath, s.metadata)
	s.endpoints = append(s.endpoints, Endpoint{
		Method: http.MethodGet, Path: metadataPath, Area: "Metadata",
		Summary: "The OData description of the API (CSDL, XML): the types, the navigation properties, and the entity sets that dollycli has. It needs no token.",
	})
	s.mux.HandleFunc("POST "+BasePath+"/$batch", s.batch)
	s.endpoints = append(s.endpoints, Endpoint{
		Method: http.MethodPost, Path: BasePath + "/$batch", Area: "Batch",
		Summary: "Run up to 20 calls in one. Each call is answered by the same code as a call of its own, and can fail on its own.",
	})
}
