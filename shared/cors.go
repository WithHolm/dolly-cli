package shared

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// CORS lets a web app in a browser call an API from another origin, as a single-page app does with Graph and with the token
// endpoint of Entra. It answers the preflight (OPTIONS), and it adds the headers to the answer.
//
// Only an origin on this machine gets them: localhost, a loopback address, or a host in allowHosts (a service name in a compose
// file, for example). A web page from the internet is not let in, so it cannot read or change the data of dollycli by way of the
// browser of the person who visits it. The headers are for the API, not for the admin paths.
func CORS(allowHosts []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" || !originAllowed(origin, allowHosts) {
				next.ServeHTTP(w, r)
				return
			}
			h := w.Header()
			h.Add("Vary", "Origin")
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Expose-Headers", "request-id, client-request-id, x-ms-resource-unit, retry-after, location, odata-version")
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				h.Add("Vary", "Access-Control-Request-Headers")
				h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				if reqHeaders := r.Header.Get("Access-Control-Request-Headers"); reqHeaders != "" {
					h.Set("Access-Control-Allow-Headers", reqHeaders)
				} else {
					h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, ConsistencyLevel, Prefer")
				}
				h.Set("Access-Control-Max-Age", "3600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func originAllowed(origin string, allowHosts []string) bool {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	for _, a := range allowHosts {
		if strings.EqualFold(a, host) {
			return true
		}
	}
	return false
}
