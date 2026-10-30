package shared_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/shared"
)

// owner is an API with a variable start of its paths: /{tenant}/token.
type owner struct{ fake }

func (o owner) Prefixes() []string { return nil }
func (o owner) Owns(r *http.Request) bool {
	parts := strings.Split(r.URL.Path, "/")
	return len(parts) == 3 && parts[2] == "token"
}

func TestRouter_MatcherOwnsItsPathsBeforeThePrefixes(t *testing.T) {
	portal := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "portal "+r.URL.Path) })
	h := shared.Router(portal, fake{"one", "/v1.0/"}, owner{fake{"login", ""}})
	get := func(path string) string {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", path, nil)
		req.Host = "127.0.0.1:8650" // the guard of the admin API wants this machine
		h.ServeHTTP(rec, req)
		return rec.Body.String()
	}
	for path, want := range map[string]string{
		"/anything/token":          "api login /anything/token",
		"/v1.0/token":              "api login /v1.0/token", // a prefix of another API does not matter: the owner is asked first
		"/v1.0/users":              "api one /v1.0/users",
		"/_dolly/login/state":      "admin login /state",
		"/anything/token/but/more": "portal /anything/token/but/more",
		"/":                        "portal /",
	} {
		if got := get(path); got != want {
			t.Errorf("GET %s = %q, want %q", path, got, want)
		}
	}
}
