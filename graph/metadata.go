package graph

import (
	"net/http"

	"github.com/withholm/dolly-cli/graph/spec"
)

// metadataPath is /v1.0/$metadata: the OData description of the API.
const metadataPath = BasePath + "/$metadata"

// metadata answers /$metadata with the OData description of what dollycli serves (spec.Metadata). The real Graph answers 200 with application/xml and the header
// OData-Version: 4.0 (checked with the Graph Explorer proxy). Its answer is the whole of Graph (1.75 MB); this one has the types that dollycli serves.
func (s *Server) metadata(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("OData-Version", "4.0")
	_, _ = w.Write(spec.Metadata())
}
