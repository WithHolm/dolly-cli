package graph

import (
	"github.com/withholm/dolly-cli/graph/api"
	"github.com/withholm/dolly-cli/shared"
)

// The server is an API of the shared router (see package shared).
var _ shared.API = (*Server)(nil)

// Name is the name of the API: graph.
func (s *Server) Name() string { return "graph" }

// Prefixes are the path starts that the Graph API answers. The second is the receiver of change notifications (subscriptions.go), and the third is where
// the download addresses of files are (@microsoft.graph.downloadUrl, see drive_download.go).
func (s *Server) Prefixes() []string {
	return []string{BasePath + "/", ReceiverPath, api.DownloadPrefix}
}
