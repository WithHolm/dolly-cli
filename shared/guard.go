package shared

import (
	"net"
	"net/http"
	"strings"
)

// Guard protects an admin API. Another web page must not reach it: the Host
// header must name this machine (DNS rebinding), and a call that writes needs a
// JSON content type (a web page cannot send one to another host without a preflight).
func Guard(next http.Handler) http.Handler { return GuardHosts(nil)(next) }

// GuardHosts is Guard for a Dolly that is reached by a name that is not loopback: a service name in a compose
// file (dolly), or a domain. The names in allowed are accepted as the Host header too, without a port.
func GuardHosts(allowed []string) func(http.Handler) http.Handler {
	names := map[string]bool{}
	for _, n := range allowed {
		if n = strings.ToLower(strings.TrimSpace(n)); n != "" {
			names[n] = true
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !knownHost(r.Host, names) {
				http.Error(w, "unknown host", http.StatusForbidden)
				return
			}
			if r.Method != http.MethodGet && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnsupportedMediaType)
				_, _ = w.Write([]byte(`{"error":{"code":"Request_BadRequest","message":"Send JSON: the admin API needs the content type application/json."}}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// knownHost reports whether a Host header names this machine, or is one of the names that were allowed.
func knownHost(hostport string, allowed map[string]bool) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	host = strings.ToLower(host)
	switch host {
	case "127.0.0.1", "localhost", "[::1]", "::1":
		return true
	}
	return allowed[host]
}
