package shared

import (
	"encoding/json"
	"net/http"
)

// Router puts the APIs and the portal on one address:
//
//	<prefix>            the API (for example /v1.0/ for Graph)
//	/_dolly/<name>/     the admin API of the API, behind Guard
//	/_dolly/apis        the names of the APIs, for the portal
//	/                   the portal (ui), for every path that is not one of the above
//
// An API that is a Matcher gets the paths that it owns before all of these.
//
// The calls to the portal and the admin paths are not in the lists of calls.
func Router(ui http.Handler, apis ...API) http.Handler { return RouterWith(Options{}, ui, apis...) }

// Options changes how the router guards the admin paths.
type Options struct {
	// AllowedHosts are host names (not loopback) that the admin paths accept in the Host header, for a Dolly that
	// other containers reach by a service name, or that has a domain. See GuardHosts.
	AllowedHosts []string
}

// RouterWith is Router with options.
func RouterWith(opt Options, ui http.Handler, apis ...API) http.Handler {
	guard := GuardHosts(opt.AllowedHosts)
	mux := http.NewServeMux()
	names := []string{}
	for _, a := range apis {
		n := a.Name()
		names = append(names, n)
		for _, p := range a.Prefixes() {
			mux.Handle(p, a.Handler())
		}
		mux.Handle("/_dolly/"+n+"/", guard(http.StripPrefix("/_dolly/"+n, a.AdminHandler())))
	}
	mux.Handle("GET /_dolly/apis", guard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"apis": names})
	})))
	mux.Handle("/", ui)
	var owners []API
	for _, a := range apis {
		if _, ok := a.(Matcher); ok {
			owners = append(owners, a)
		}
	}
	if len(owners) == 0 {
		return mux
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, a := range owners {
			if a.(Matcher).Owns(r) {
				a.Handler().ServeHTTP(w, r)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}
