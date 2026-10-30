package ui_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/ui"
)

func get(t *testing.T, h http.Handler, path string) (*http.Response, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	res := rec.Result()
	body, _ := io.ReadAll(res.Body)
	return res, string(body)
}

func TestHandler_ServesTheBuiltApp(t *testing.T) {
	h := ui.Handler()
	res, body := get(t, h, "/")
	if res.StatusCode != 200 || !strings.Contains(body, `<div id="app">`) || res.Header.Get("Cache-Control") != "no-cache" {
		t.Fatalf("index: %d %q %s", res.StatusCode, res.Header.Get("Cache-Control"), body)
	}

	// the page names its script, and the script is served with a cache that does not end
	m := regexp.MustCompile(`src="\./(assets/[^"]+\.js)"`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("index.html has no script from assets/: run 'make ui'\n%s", body)
	}
	res, js := get(t, h, "/"+m[1])
	if res.StatusCode != 200 || len(js) < 1000 || !strings.Contains(res.Header.Get("Cache-Control"), "immutable") {
		t.Errorf("script: %d, %d bytes, cache %q", res.StatusCode, len(js), res.Header.Get("Cache-Control"))
	}
	if !strings.Contains(res.Header.Get("Content-Type"), "javascript") {
		t.Errorf("content type of the script: %q", res.Header.Get("Content-Type"))
	}

	if res, _ = get(t, h, "/nothing"); res.StatusCode != 404 {
		t.Errorf("a path that is not a file: %d", res.StatusCode)
	}
}
