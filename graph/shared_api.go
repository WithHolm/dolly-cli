package graph

import "github.com/withholm/dolly-cli/shared"

// The server is an API of the shared router (see package shared).
var _ shared.API = (*Server)(nil)

// Name is the name of the API: graph.
func (s *Server) Name() string { return "graph" }

// Prefixes are the path starts that the Graph API answers.
func (s *Server) Prefixes() []string { return []string{BasePath + "/"} }
