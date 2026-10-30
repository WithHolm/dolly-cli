package graph_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

// The headers and the error body of every answer of Graph (checked through the Graph Explorer proxy on 2026-10-29).
func TestAnswers_HaveTheHeadersOfGraph(t *testing.T) {
	h := smallServer(false)
	get := func(path string, headers map[string]string) *httptest.ResponseRecorder {
		return sendWith(h, "GET", path, headers)
	}
	guid := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

	rec := get("/v1.0/users?$top=1", nil)
	for name, want := range map[string]string{
		"Content-Type":  "application/json; odata.metadata=minimal; odata.streaming=true; IEEE754Compatible=false; charset=utf-8",
		"Odata-Version": "4.0",
		"Cache-Control": "no-cache",
	} {
		if got := rec.Header().Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	// with no client-request-id from the client, the two ids are the same
	id := rec.Header().Get("request-id")
	if !guid.MatchString(id) || rec.Header().Get("client-request-id") != id {
		t.Errorf("request-id %q, client-request-id %q", id, rec.Header().Get("client-request-id"))
	}
	// the id that the client sent comes back, and the id of the request stays its own
	rec = get("/v1.0/users?$top=1", map[string]string{"client-request-id": "my-id"})
	if rec.Header().Get("client-request-id") != "my-id" || !guid.MatchString(rec.Header().Get("request-id")) {
		t.Errorf("client-request-id: %v", rec.Header())
	}

	// an error has the innerError of Graph: the time, and the two ids
	rec = get("/v1.0/users/00000000-0000-0000-0000-000000000000", map[string]string{"client-request-id": "my-id"})
	var body struct {
		Error struct {
			Code, Message string
			InnerError    struct {
				Date            string
				RequestID       string `json:"request-id"`
				ClientRequestID string `json:"client-request-id"`
			}
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusNotFound {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	e := body.Error
	if e.Code != "Request_ResourceNotFound" || e.Message != "Resource '00000000-0000-0000-0000-000000000000' does not exist or one of its queried reference-property objects are not present." {
		t.Errorf("the error: %+v", e)
	}
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}$`).MatchString(e.InnerError.Date) || e.InnerError.RequestID != rec.Header().Get("request-id") || e.InnerError.ClientRequestID != "my-id" {
		t.Errorf("innerError: %+v, headers %v", e.InnerError, rec.Header())
	}
	if rec.Header().Get("Content-Type") != "application/json" || rec.Header().Get("Odata-Version") != "" {
		t.Errorf("an error is plain JSON, with no OData-Version: %v", rec.Header())
	}
}
