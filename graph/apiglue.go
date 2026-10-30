package graph

import (
	"encoding/json"
	"net/http"

	"github.com/withholm/dolly-cli/graph/api"
)

// The names of the api package, as the rest of this package uses them. The types
// are aliases, so a World method that returns *apiError is a method that returns
// *api.Error, and the world satisfies the interfaces of the resources.

type (
	// Object is one Graph object as JSON, with the camelCase names that Graph writes.
	Object = api.Object
	// Actor says who made a change.
	Actor = api.Actor
	// Endpoint is one operation that dolly supports.
	Endpoint = api.Endpoint
	// QueryOption is a query option or a header that dolly knows.
	QueryOption = api.QueryOption

	apiError = api.Error
)

// QueryOptions are the query options and headers of Graph that dolly knows.
var QueryOptions = api.QueryOptions

// FilterForms and SearchDoc say which forms of $filter and $search dolly answers. The code that
// answers them is the table that these read, so the text and the behavior are the same.
var (
	FilterForms = api.FilterForms
	SearchDoc   = api.SearchDoc
)

const (
	maxPage = api.MaxPage
)

// writeJSON is for the admin API and $batch: plain JSON. The resources of Graph use api.WriteJSON, which has the
// content type and the OData-Version of Graph.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, e *apiError)   { api.WriteError(w, e) }
func badRequest(format string, args ...any) *apiError { return api.BadRequest(format, args...) }

// apiErr makes an error with a status and a code that Graph uses. (A literal of
// the type would need the names of the fields, because it is from another package.)
func apiErr(status int, code, message string) *apiError {
	return &api.Error{Status: status, Code: code, Message: message}
}
