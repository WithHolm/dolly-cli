package graph

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/withholm/dolly-cli/graph/api"
	"github.com/withholm/dolly-cli/graph/cost"
	"github.com/withholm/dolly-cli/shared"
)

// Server answers Graph requests from a World. The Graph API itself is in the api
// package, one file for each resource (see mount.go). The server is what is the
// same for all of them: the token, the faults, the list of calls, the counts, and $batch.
type Server struct {
	world *World
	mux   *http.ServeMux // the Graph API, under BasePath
	admin *http.ServeMux // the admin API, see admin.go

	bmu sync.RWMutex
	beh Behavior

	calls     callLog
	graphURL  string
	endpoints []Endpoint // the operations that dolly supports, from mountAPI
	stats     tracker

	costs   *cost.Data    // what a call costs, and the limits (see package cost)
	limiter *cost.Limiter // counts the cost of the calls of each client

	verify TokenVerifier // the sign-in API, when it runs. Without it, any token works.

	docs docSource // the text of the reference (docs.go)
}

// TokenInfo is what the sign-in API says about a token.
type TokenInfo struct {
	AppID string // the application (client) ID in the token
	// UserID is the object ID of the user in a delegated token. It is empty for an application token.
	UserID string
	// Problem is why the server must refuse the token, as the message of the 401. It is empty
	// for a token that is good.
	Problem string
}

// TokenVerifier checks a bearer token. It reports false for a token that the sign-in API did not
// make: such a token still works, and is counted by the application ID that it says.
type TokenVerifier func(token string) (TokenInfo, bool)

// SetTokenVerifier makes the server check the tokens that the sign-in API made. Call it before the
// server runs.
func (s *Server) SetTokenVerifier(v TokenVerifier) { s.verify = v }

type meKey struct{}

// meEndpoints lists /me for the reference: every operation on /users/{id} that Graph also has on
// /me. They are not routes of their own (meRequest turns /me into /users/{id}), so they are made
// from the list of the routes, and they cannot differ from it. Graph has no DELETE /me.
func meEndpoints(eps []Endpoint) []Endpoint {
	var out []Endpoint
	for _, e := range eps {
		rest, ok := strings.CutPrefix(e.Path, BasePath+"/users/{id}")
		if !ok || e.Method == http.MethodDelete {
			continue
		}
		e.Path = BasePath + "/me" + rest
		e.Summary = "The same as " + e.Method + " /users/{id}" + rest + ", for the user of the token. It needs a delegated token: an application token gets 400."
		out = append(out, e)
	}
	return out
}

// meRequest makes /me the user of the token: /me/manager is /users/{id}/manager. A token that has
// no user (an application token, or a token that Dolly did not make) cannot use /me, as in Graph.
// It gives r as it is for any other path.
func (s *Server) meRequest(r *http.Request) (*http.Request, *apiError) {
	rest, ok := strings.CutPrefix(r.URL.Path, BasePath+"/me")
	if !ok || (rest != "" && rest[0] != '/') {
		return r, nil
	}
	info, ours := s.token(r)
	if !ours || info.UserID == "" {
		return r, apiErr(http.StatusBadRequest, "BadRequest", "/me request is only valid with delegated authentication flow.")
	}
	r2 := r.Clone(context.WithValue(r.Context(), meKey{}, true)) // track counts the call as /me
	r2.URL.Path, r2.URL.RawPath = BasePath+"/users/"+info.UserID+rest, ""
	return r2, nil
}

// token gives what the sign-in API says about the bearer token of a request.
func (s *Server) token(r *http.Request) (TokenInfo, bool) {
	if s.verify == nil {
		return TokenInfo{}, false
	}
	return s.verify(bearer(r))
}

// ServerOptions changes how the server answers.
type ServerOptions struct {
	// PageSize forces the size of every page. A small number makes a tool
	// follow @odata.nextLink often, which is a way to test paging.
	PageSize int
	// Throttle makes the server answer 429 when a client passes the limits of Graph. Without it,
	// the server only counts the cost of each call. A person can change it at run time.
	Throttle bool
	// Docs is a folder with the markdown files of the reference (see package shared/docs). The
	// default is the folder that is built into the program. A folder that is given is read again at each
	// request, so a person who writes the docs sees a change without a rebuild. It is for development.
	Docs fs.FS
}

// New makes a server for a world.
func New(w *World, opt ServerOptions) *Server {
	s := &Server{world: w, mux: http.NewServeMux(), admin: http.NewServeMux()}
	s.beh.PageSize = opt.PageSize
	s.beh.Throttle = opt.Throttle
	s.docs.init(opt.Docs)
	if d, err := cost.Load(); err != nil {
		slog.Error("Dolly cannot read the data about the cost of a call. It does not count the cost.", "error", err)
	} else {
		s.costs, s.limiter = d, cost.NewLimiter(d, nil)
	}
	s.mountAPI()
	s.adminRoutes()
	return s
}

// SetGraphURL tells the server which address people use to reach it. The admin
// API shows it, so the page can print the address to copy.
func (s *Server) SetGraphURL(u string) {
	s.graphURL = u
	s.world.SetAddress(strings.TrimSuffix(u, BasePath)) // the lock file says where this dolly listens
}

// World gives the world of the server.
func (s *Server) World() *World { return s.world }

// BasePath is the path that a Graph URL starts with.
const BasePath = "/v1.0"

// Handler is the http.Handler of the server.
func (s *Server) Handler() http.Handler { return http.HandlerFunc(s.serve) }

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	// The admin API is for the person who runs the server. It needs no token,
	// and the call log does not show it.
	if strings.HasPrefix(r.URL.Path, "/_dolly/") {
		// This path is for a server that runs alone. Under shared.Router, the admin API
		// is at /_dolly/graph/ and the same guard is in front of it.
		shared.Guard(http.StripPrefix("/_dolly", s.admin)).ServeHTTP(w, r)
		return
	}
	id := s.calls.reserve()
	start := time.Now()
	call := Call{
		ID: id, Time: start, Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery,
		Consistency: r.Header.Get("ConsistencyLevel"), Agent: r.UserAgent(), Request: readBody(r),
	}
	rec := &recorder{ResponseWriter: w}
	api.Stamp(rec.Header(), r) // request-id, client-request-id, Cache-Control, on every answer
	defer func() {
		call.Status, call.Bytes, call.Response = rec.status, rec.bytes, cut(rec.head.String())
		call.Millis = float64(time.Since(start).Microseconds()) / 1000
		s.logCall(r, call)
		slog.Debug("dolly: request", "method", call.Method, "url", r.URL.String(), "status", call.Status)
	}()

	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Authorization")), "bearer ") {
		writeError(rec, apiErr(http.StatusUnauthorized, "InvalidAuthenticationToken", "Access token is empty."))
		return
	}
	if info, ours := s.token(r); ours && info.Problem != "" {
		writeError(rec, apiErr(http.StatusUnauthorized, "InvalidAuthenticationToken", info.Problem))
		return
	}
	if r.URL.Path != BasePath+"/$batch" {
		var e *apiError
		if r, e = s.meRequest(r); e != nil {
			writeError(rec, e)
			return
		}
	}
	// A $batch call is not failed as a whole. Each request in it can fail on its own, and
	// costs on its own.
	if r.URL.Path != BasePath+"/$batch" {
		if call.Fault = s.fault(rec, r); call.Fault != "" {
			return
		}
		var throttled bool
		if call.RU, throttled = s.charge(rec.Header(), r); throttled {
			call.Fault = "throttled"
			writeError(rec, apiErr(http.StatusTooManyRequests, "TooManyRequests", "Too many requests. (dolly: the limit of Graph for this client)"))
			return
		}
	} else if s.behavior().Unauthorized {
		call.Fault = s.fault(rec, r)
		return
	}
	// the ID goes in the context, so the parts of a $batch can name the call
	req := r.WithContext(context.WithValue(r.Context(), callIDKey{}, id))
	s.mux.ServeHTTP(rec, req)
	s.track(req, rec.status)
}

type callIDKey struct{}

// graphActor names the tool behind a request, for the file of changes.
func graphActor(r *http.Request) Actor {
	id, _ := r.Context().Value(callIDKey{}).(int64)
	return Actor{Source: "graph", Agent: r.UserAgent(), Call: id}
}

// adminActor names a person who uses the admin API.
var adminActor = Actor{Source: "admin"}
