package graph_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

// send sends one request straight to the handler. It is fast, and the limits are by time.
func send(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://127.0.0.1"+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer any")
	if method != http.MethodGet { // the admin API needs it for a write
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func smallServer(throttle bool) http.Handler {
	// 10 users is a small tenant: 3500 resource units in 10 seconds
	return graph.New(graph.Generate(graph.Options{Users: 10, Seed: 1}), graph.ServerOptions{Throttle: throttle}).Handler()
}

func TestCost_HeadersShowWhatACallCosts(t *testing.T) {
	h := smallServer(false)
	for path, want := range map[string]string{
		"/v1.0/users":                          "2",
		"/v1.0/users?$select=id":               "1",
		"/v1.0/users?$top=5":                   "1",
		"/v1.0/organization":                   "1",
		"/v1.0/groups?$top=50&$expand=members": "2", // 1 + 1
	} {
		if got := send(h, "GET", path, "").Header().Get("x-ms-resource-unit"); got != want {
			t.Errorf("%s: x-ms-resource-unit %q, want %q", path, got, want)
		}
	}
}

func TestThrottle_Over3500RUIn10SecondsIs429(t *testing.T) {
	h := smallServer(true)
	for i := range 1750 {
		if rec := send(h, "GET", "/v1.0/users", ""); rec.Code != 200 {
			t.Fatalf("request %d: %d", i+1, rec.Code)
		}
	}
	rec := send(h, "GET", "/v1.0/users", "")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("request 1751: %d, want 429", rec.Code)
	}
	if ra, _ := strconv.Atoi(rec.Header().Get("Retry-After")); ra < 1 || ra > 10 {
		t.Errorf("Retry-After %q", rec.Header().Get("Retry-After"))
	}
	if sc := rec.Header().Get("x-ms-throttle-scope"); !strings.HasPrefix(sc, "Tenant_Application/ReadWrite/anonymous/") {
		t.Errorf("x-ms-throttle-scope %q", sc)
	}
	if rec.Header().Get("x-ms-throttle-information") != "ResourceUnitLimitExceeded" {
		t.Errorf("x-ms-throttle-information %q", rec.Header().Get("x-ms-throttle-information"))
	}

	// each request of a $batch costs on its own, and is throttled on its own
	rec = send(h, "POST", "/v1.0/$batch", `{"requests":[{"id":"a","method":"GET","url":"/users"}]}`)
	var out struct {
		Responses []struct {
			Status  int
			Headers map[string]string
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || rec.Code != 200 || len(out.Responses) != 1 {
		t.Fatalf("batch: %d %s", rec.Code, rec.Body)
	}
	if out.Responses[0].Status != 429 || out.Responses[0].Headers["Retry-After"] == "" {
		t.Errorf("batch part: %+v", out.Responses[0])
	}

	// the admin API shows the use, and does not count itself
	rec = send(h, "GET", "/_dolly/usage", "")
	var u struct {
		ServicePrincipals []struct {
			AppID     string
			RU        int
			Throttled int
		}
		Throttle bool
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &u); err != nil || len(u.ServicePrincipals) != 1 {
		t.Fatalf("usage: %d %s", rec.Code, rec.Body)
	}
	if u.ServicePrincipals[0].RU != 3500 || u.ServicePrincipals[0].Throttled != 2 || !u.Throttle {
		t.Errorf("usage: %+v", u)
	}
}

func TestThrottle_OffOnlyCounts(t *testing.T) {
	h := smallServer(false)
	var last *httptest.ResponseRecorder
	for range 2000 {
		if last = send(h, "GET", "/v1.0/users", ""); last.Code != 200 {
			t.Fatalf("status %d with throttle off", last.Code)
		}
	}
	if p := last.Header().Get("x-ms-throttle-limit-percentage"); p == "" {
		t.Error("the app is over its limit, and x-ms-throttle-limit-percentage is missing")
	}
}

func TestThrottle_CanBeTurnedOnWhileRunning(t *testing.T) {
	h := smallServer(false)
	if rec := send(h, "PUT", "/_dolly/behavior", `{"throttle":true}`); rec.Code != 200 {
		t.Fatalf("behavior: %d %s", rec.Code, rec.Body)
	}
	for range 1750 {
		send(h, "GET", "/v1.0/users", "")
	}
	if rec := send(h, "GET", "/v1.0/users", ""); rec.Code != 429 {
		t.Errorf("after the switch: %d, want 429", rec.Code)
	}
	send(h, "DELETE", "/_dolly/usage", "")
	if rec := send(h, "GET", "/v1.0/users", ""); rec.Code != 200 {
		t.Errorf("after the reset of the use: %d, want 200", rec.Code)
	}
}
