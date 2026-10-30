package shared_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/shared"
)

type fake struct{ name, prefix string }

func (f fake) Name() string       { return f.name }
func (f fake) Prefixes() []string { return []string{f.prefix} }
func (f fake) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "api "+f.name+" "+r.URL.Path) })
}
func (f fake) AdminHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "admin "+f.name+" "+r.URL.Path) })
}

func TestRouter_RoutesEachAPIAndThePortal(t *testing.T) {
	portal := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/assets/app.js" {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, "portal "+r.URL.Path)
	})
	ts := httptest.NewServer(shared.Router(portal, fake{"one", "/v1.0/"}, fake{"two", "/subscriptions/"}))
	defer ts.Close()

	send := func(method, path, contentType string) (int, string) {
		req, _ := http.NewRequest(method, ts.URL+path, nil)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}

	for _, c := range []struct{ method, path, ct, want string }{
		{"GET", "/v1.0/users", "", "api one /v1.0/users"},
		{"POST", "/v1.0/users", "application/json", "api one /v1.0/users"},
		{"GET", "/subscriptions/x", "", "api two /subscriptions/x"},
		{"GET", "/_dolly/two/state", "", "admin two /state"},
		{"POST", "/_dolly/one/rename", "application/json", "admin one /rename"},
		{"GET", "/_dolly/apis", "", `{"apis":["one","two"]}`},
		{"GET", "/", "", "portal /"},
		{"GET", "/assets/app.js", "", "portal /assets/app.js"},
	} {
		if code, body := send(c.method, c.path, c.ct); code != 200 || !strings.Contains(body, c.want) {
			t.Errorf("%s %s: %d %q, want %q", c.method, c.path, code, body, c.want)
		}
	}
	if code, _ := send("GET", "/nothing", ""); code != 404 {
		t.Errorf("unknown path: %d", code)
	}
	if code, _ := send("POST", "/_dolly/one/rename", ""); code != http.StatusUnsupportedMediaType {
		t.Errorf("a write with no JSON content type must be refused: %d", code)
	}
}
