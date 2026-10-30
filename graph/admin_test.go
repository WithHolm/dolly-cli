package graph_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

// admin sends one request to the admin API and decodes the answer.
func admin(t *testing.T, ts *httptest.Server, method, path string, body string, out any) int {
	t.Helper()
	req, _ := http.NewRequest(method, ts.URL+"/_dolly"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil && res.StatusCode < 300 {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
	}
	return res.StatusCode
}

func startServer(t *testing.T, opt graph.ServerOptions) (*httptest.Server, *client, *graph.Server) {
	t.Helper()
	srv := graph.New(graph.Generate(graph.Options{Users: 200, Seed: 5}), opt)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, &client{BaseURL: ts.URL + graph.BasePath}, srv
}

type callsAnswer struct {
	Calls  []graph.Call `json:"calls"`
	Cursor int64        `json:"cursor"`
}

func TestCalls_AreLoggedWithTheirDetails(t *testing.T) {
	ts, c, _ := startServer(t, graph.ServerOptions{})
	ctx := context.Background()

	var out any
	if err := c.GetWith(ctx, "/groups?$search="+`"displayName:acme"`+"&$select=id,displayName", http.Header{"ConsistencyLevel": {"eventual"}}, &out); err != nil {
		t.Fatal(err)
	}
	var got callsAnswer
	admin(t, ts, "GET", "/requests", "", &got)
	if len(got.Calls) != 1 {
		t.Fatalf("calls: %+v (the admin calls must not be in the log)", got.Calls)
	}
	call := got.Calls[0]
	if call.Method != "GET" || call.Path != "/v1.0/groups" || call.Status != 200 || call.Consistency != "eventual" || !strings.Contains(call.Query, "search") {
		t.Errorf("call: %+v", call)
	}
	if !strings.Contains(call.Response, "grp.app.acme") || call.Bytes == 0 || call.Millis < 0 {
		t.Errorf("the answer is kept: %+v", call)
	}

	// the cursor: a second ask gives only the new calls
	_ = c.Get(ctx, "/users?$top=1", &out)
	var next callsAnswer
	admin(t, ts, "GET", "/requests?since="+itoa(got.Cursor), "", &next)
	if len(next.Calls) != 1 || next.Calls[0].Path != "/v1.0/users" {
		t.Errorf("new calls: %+v", next.Calls)
	}
	// a call with no token is in the log too, with its 401
	req, _ := http.NewRequest("GET", ts.URL+"/v1.0/users", nil)
	res, _ := http.DefaultClient.Do(req)
	res.Body.Close()
	admin(t, ts, "GET", "/requests?since="+itoa(next.Cursor), "", &next)
	if len(next.Calls) != 1 || next.Calls[0].Status != 401 {
		t.Errorf("a call with no token: %+v", next.Calls)
	}
	if admin(t, ts, "DELETE", "/requests", "", nil) != 204 {
		t.Error("clear")
	}
	admin(t, ts, "GET", "/requests", "", &got)
	if len(got.Calls) != 0 {
		t.Errorf("after clear: %d calls", len(got.Calls))
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// Another web page must not be able to change the data of Dolly.
func TestAdmin_KeepsOtherWebPagesOut(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	send := func(method, host, contentType string) int {
		req, _ := http.NewRequest(method, ts.URL+"/_dolly/reset", strings.NewReader("{}"))
		if host != "" {
			req.Host = host
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if s := send("POST", "", "application/json"); s != 200 {
		t.Errorf("the page and the CLI send JSON: %d", s)
	}
	for _, ct := range []string{"", "text/plain", "application/x-www-form-urlencoded"} {
		if s := send("POST", "", ct); s != http.StatusUnsupportedMediaType {
			t.Errorf("a write with content type %q must be refused: %d", ct, s)
		}
	}
	if s := send("POST", "evil.example", "application/json"); s != http.StatusForbidden {
		t.Errorf("a host that is not this machine must be refused (DNS rebinding): %d", s)
	}
}

func TestCalls_BatchPartsNameTheirBatch(t *testing.T) {
	ts, c, _ := startServer(t, graph.ServerOptions{})
	_, err := c.Batch(context.Background(), []batchRequest{
		{ID: "a", Method: "GET", URL: "/groups?$top=1"},
		{ID: "b", Method: "GET", URL: "/users?$top=1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got callsAnswer
	admin(t, ts, "GET", "/requests", "", &got)
	if len(got.Calls) != 3 {
		t.Fatalf("calls: %d, want the $batch and its 2 parts", len(got.Calls))
	}
	var batch graph.Call
	for _, c := range got.Calls {
		if c.Path == "/v1.0/$batch" {
			batch = c
		}
	}
	if batch.ID == 0 || !strings.Contains(batch.Request, `"requests"`) {
		t.Fatalf("the $batch call: %+v", batch)
	}
	parts := 0
	for _, c := range got.Calls {
		if c.Batch == batch.ID {
			parts++
		}
	}
	if parts != 2 {
		t.Errorf("%d parts name the batch, want 2", parts)
	}
}

func TestBehavior_FailuresAreRetriedByTheClient(t *testing.T) {
	ts, c, _ := startServer(t, graph.ServerOptions{})
	if s := admin(t, ts, "PUT", "/behavior", `{"failPercent":100,"failStatus":429,"retryAfter":0}`, nil); s != 200 {
		t.Fatalf("set behavior: %d", s)
	}
	var out any
	err := c.Get(context.Background(), "/users?$top=1", &out)
	var ge *graphError
	if !errors.As(err, &ge) || ge.Status != 429 {
		t.Fatalf("a 429 that never ends is a 429 error: %v", err)
	}
	var got callsAnswer
	admin(t, ts, "GET", "/requests", "", &got)
	if len(got.Calls) < 2 || got.Calls[0].Fault == "" || got.Calls[0].Status != 429 {
		t.Errorf("the client must try more than once, and the log names the fault: %+v", got.Calls)
	}

	// a failure inside a $batch is for one part, with Retry-After
	admin(t, ts, "DELETE", "/requests", "", nil)
	answers, err := c.Batch(context.Background(), []batchRequest{{ID: "a", Method: "GET", URL: "/users?$top=1"}})
	if err != nil || answers[0].Status != 429 {
		t.Errorf("batch part: %+v %v", answers, err)
	}

	// back to normal
	admin(t, ts, "PUT", "/behavior", `{}`, nil)
	if err := c.Get(context.Background(), "/users?$top=1", &out); err != nil {
		t.Errorf("after the reset: %v", err)
	}
}

func TestBehavior_Unauthorized_PageSize_AndBadValues(t *testing.T) {
	ts, c, _ := startServer(t, graph.ServerOptions{})
	var out any
	admin(t, ts, "PUT", "/behavior", `{"unauthorized":true}`, nil)
	if err := c.Get(context.Background(), "/users", &out); err == nil {
		t.Error("unauthorized must fail every request")
	}
	admin(t, ts, "PUT", "/behavior", `{"pageSize":7}`, nil)
	var page struct {
		Value []any  `json:"value"`
		Next  string `json:"@odata.nextLink"`
	}
	if err := c.Get(context.Background(), "/users?$top=100", &page); err != nil || len(page.Value) != 7 || page.Next == "" {
		t.Errorf("page size: %d items, next %q, %v", len(page.Value), page.Next, err)
	}
	for _, bad := range []string{`{"failPercent":101}`, `{"failStatus":418}`, `{"latencyMs":-1}`, `{"pageSize":5000}`} {
		if s := admin(t, ts, "PUT", "/behavior", bad, nil); s != 400 {
			t.Errorf("%s gave status %d, want 400", bad, s)
		}
	}
}

func TestAdmin_DataEditing(t *testing.T) {
	ts, c, _ := startServer(t, graph.ServerOptions{})
	ctx := context.Background()

	var first struct {
		Value []struct{ UserPrincipalName string }
	}
	if err := c.Get(ctx, "/users?$top=1&$select=userPrincipalName", &first); err != nil || len(first.Value) != 1 {
		t.Fatalf("a user: %v %+v", err, first)
	}

	// a change of a user shows in Graph, and save/reset work
	if s := admin(t, ts, "POST", "/user", `{"user":"`+first.Value[0].UserPrincipalName+`","set":{"department":"Moved"}}`, nil); s != 200 {
		t.Fatalf("set user: %d", s)
	}
	var one struct{ Value []map[string]any }
	if err := c.Get(ctx, "/users?$top=999&$select=id,department", &one); err != nil {
		t.Fatal(err)
	}
	moved := 0
	for _, v := range one.Value {
		if v["department"] == "Moved" {
			moved++
		}
	}
	if moved != 1 {
		t.Errorf("%d users in Moved, want 1", moved)
	}
	var st graph.State
	admin(t, ts, "POST", "/reset", "", &st)
	for _, d := range st.Departments {
		if d.Value == "Moved" {
			t.Error("reset must undo the change")
		}
	}
}
