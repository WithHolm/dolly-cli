// Package shared is what every Dolly API has in common: the interface that an API
// implements, the router that puts them on one address with the portal, and the guard of the
// admin paths. An API package (graph, later az) imports shared. Shared imports no API.
//
// The portal is one app (the package ui). Its page for an API reads the admin API of that API.
package shared

import "net/http"

// API is one fake API, such as Graph.
type API interface {
	// Name is the short name: "graph". It is in the admin path (/_dolly/graph/), the address of the
	// page of the portal (#/graph), and the subcommand.
	Name() string
	// Prefixes are the path starts that the API answers, such as "/v1.0/". The
	// API gets the full path.
	Prefixes() []string
	// Handler is the API itself.
	Handler() http.Handler
	// AdminHandler is the admin API, with paths like /state. The router mounts it
	// at /_dolly/<name>/, behind Guard.
	AdminHandler() http.Handler
}

// Matcher is for an API whose paths start with a variable part, such as /{tenant}/oauth2/v2.0/token. A
// path like that cannot be a prefix: it would conflict with the prefix of another API. The router asks
// the Matcher before it looks at the prefixes. An API that is a Matcher can return no prefixes.
type Matcher interface {
	Owns(r *http.Request) bool
}
