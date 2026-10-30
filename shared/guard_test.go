package shared_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/shared"
)

func hostCode(h http.Handler, host string) int {
	req := httptest.NewRequest("GET", "http://"+host+"/x", nil)
	req.Host = host
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

// The admin paths accept this machine, and the names that a person allowed (a service name in a compose file).
func TestGuardHosts(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	for _, c := range []struct {
		host    string
		allowed []string
		want    int
	}{
		{"127.0.0.1:8650", nil, 204},
		{"localhost:8650", nil, 204},
		{"LOCALHOST", nil, 204},
		{"[::1]:8650", nil, 204},
		{"dolly:8650", nil, 403}, // another container's way to say it: refused until it is allowed
		{"dolly:8650", []string{"dolly"}, 204},
		{"Dolly", []string{" DOLLY "}, 204}, // capitals and spaces do not matter
		{"evil.example", []string{"dolly"}, 403},
		{"dolly.evil.example", []string{"dolly"}, 403}, // a name is not a prefix
		{"192.168.1.5:8650", []string{"dolly"}, 403},
	} {
		if got := hostCode(shared.GuardHosts(c.allowed)(ok), c.host); got != c.want {
			t.Errorf("host %q allowed %v: %d, want %d", c.host, c.allowed, got, c.want)
		}
	}
	// Guard is GuardHosts with no names
	if hostCode(shared.Guard(ok), "dolly:8650") != 403 || hostCode(shared.Guard(ok), "127.0.0.1") != 204 {
		t.Error("Guard accepts only this machine")
	}
	// the writes still need JSON, with an allowed name
	req := httptest.NewRequest("POST", "http://dolly/x", strings.NewReader("{}"))
	req.Host = "dolly"
	rec := httptest.NewRecorder()
	shared.GuardHosts([]string{"dolly"})(ok).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("a write that is not JSON: %d", rec.Code)
	}
}

// The router gives the admin paths the allowed names, and the API paths need none.
func TestRouterWith_AllowedHosts(t *testing.T) {
	portal := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	for host, want := range map[string]int{"dolly:8650": 200, "other:8650": 403} {
		h := shared.RouterWith(shared.Options{AllowedHosts: []string{"dolly"}}, portal, fake{"one", "/v1.0/"})
		req := httptest.NewRequest("GET", "http://"+host+"/_dolly/one/state", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("admin path from %s: %d, want %d", host, rec.Code, want)
		}
		req = httptest.NewRequest("GET", "http://"+host+"/v1.0/users", nil)
		req.Host = host
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Errorf("the API path needs no allowed name: %s %d", host, rec.Code)
		}
	}
}
