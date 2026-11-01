package shared

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A web app on this machine may call the API from the browser (with a preflight for a bearer token). A page from the internet
// gets no CORS headers, so the browser keeps it out.
func TestCORS(t *testing.T) {
	h := CORS([]string{"app.internal"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }))
	call := func(method, origin string, hdr map[string]string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/v1.0/me", nil)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		for k, v := range hdr {
			r.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec
	}

	for _, origin := range []string{"http://localhost:5173", "http://127.0.0.1:3000", "http://[::1]:3000", "https://app.internal", "http://web.localhost:8080"} {
		pre := call(http.MethodOptions, origin, map[string]string{"Access-Control-Request-Method": "GET", "Access-Control-Request-Headers": "authorization,consistencylevel"})
		if pre.Code != http.StatusNoContent || pre.Header().Get("Access-Control-Allow-Origin") != origin ||
			pre.Header().Get("Access-Control-Allow-Headers") != "authorization,consistencylevel" {
			t.Errorf("%s preflight: %d %v", origin, pre.Code, pre.Header())
		}
		if got := call(http.MethodGet, origin, nil); got.Code != http.StatusTeapot || got.Header().Get("Access-Control-Allow-Origin") != origin {
			t.Errorf("%s call: %d %v", origin, got.Code, got.Header())
		}
	}
	for _, origin := range []string{"https://evil.example", "http://localhost.evil.example", "https://10.0.0.5", "null", "file://"} {
		if got := call(http.MethodGet, origin, nil); got.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("%s must get no CORS headers: %v", origin, got.Header())
		}
		pre := call(http.MethodOptions, origin, map[string]string{"Access-Control-Request-Method": "POST"})
		if pre.Header().Get("Access-Control-Allow-Origin") != "" || pre.Code == http.StatusNoContent {
			t.Errorf("%s preflight must not pass: %d", origin, pre.Code)
		}
	}
	if got := call(http.MethodGet, "", nil); got.Header().Get("Access-Control-Allow-Origin") != "" || got.Code != http.StatusTeapot {
		t.Error("a call with no Origin passes through untouched")
	}
}
