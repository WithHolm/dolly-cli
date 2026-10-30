package graph_test

import (
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/withholm/dolly-cli/graph"
	"github.com/withholm/dolly-cli/shared/docs"
)

func newServer() *graph.Server {
	return graph.New(graph.NewMemory(graph.Options{Users: 10, Seed: 1}), graph.ServerOptions{})
}

// The markdown files must fit the routes: a text for a call that is not there, a page for a first part
// that no call has, or a call with no page, is a mistake in the files.
func TestDocs_FitTheRoutes(t *testing.T) {
	srv := newServer()
	eps, _ := srv.API()
	var routes []string
	for _, e := range eps {
		routes = append(routes, e.Method+" "+e.Path)
	}
	if problems := docs.Check(srv.DocPages(), routes, graph.BasePath); len(problems) > 0 {
		t.Errorf("the documentation does not fit the routes:\n  %s", strings.Join(problems, "\n  "))
	}
}

func TestDocs_AreOnTheOperations(t *testing.T) {
	eps, _ := newServer().API()
	by := map[string]graph.EndpointInfo{}
	for _, e := range eps {
		by[e.Method+" "+e.Path] = e
		if e.Resource == "" {
			t.Errorf("%s %s has no page", e.Method, e.Path)
		}
	}
	// the text of a call, and its parts
	users := by["GET /v1.0/users"]
	if users.Resource != "users" || users.Notes == nil || len(users.Notes.Sections) != 3 || users.Notes.Sections[0].Title != "Filtering" || !strings.Contains(users.Notes.Sections[0].Body, "advanced query") {
		t.Errorf("GET /v1.0/users: %+v", users.Notes)
	}
	// /me is on the page of the users, in its own section
	if me := by["GET /v1.0/me/manager"]; me.Resource != "users" || me.Section != "Signed-in user" {
		t.Errorf("GET /v1.0/me/manager: page %q, section %q", me.Resource, me.Section)
	}
	if by["GET /v1.0/users/{id}"].Section != "" || by["GET /v1.0/users/{id}"].Notes != nil {
		t.Errorf("a call with no text has none: %+v", by["GET /v1.0/users/{id}"])
	}
	if p := by["POST /v1.0/$batch"]; p.Resource != "$batch" || p.Notes == nil {
		t.Errorf("batch: %+v", p)
	}
	if g := by["GET /v1.0/oauth2PermissionGrants"]; g.Resource != "oauth2PermissionGrants" {
		t.Errorf("a resource that is not the name of its file: %q", g.Resource)
	}
}

// The example of $batch in the documentation is sent to a server, so it cannot say what the code does not do.
func TestDocs_BatchExampleWorks(t *testing.T) {
	srv := newServer()
	var body string
	for _, p := range srv.DocPages() {
		for _, op := range p.Ops {
			if op.Key == "POST /v1.0/$batch" {
				body = op.Body
			}
		}
	}
	// the code blocks of the text: the first is the request, and the second is the answer
	var blocks []string
	for _, part := range strings.Split(body, "```json\n")[1:] {
		blocks = append(blocks, strings.SplitN(part, "\n```", 2)[0])
	}
	if len(blocks) != 2 {
		t.Fatalf("the batch text has %d json blocks, want a request and an answer", len(blocks))
	}
	request := strings.TrimPrefix(blocks[0], "POST /v1.0/$batch\n")
	var in struct{ Requests []struct{ ID string } }
	if err := json.Unmarshal([]byte(request), &in); err != nil || len(in.Requests) != 3 {
		t.Fatalf("the example request: %v %s", err, request)
	}
	rec := send(srv.Handler(), "POST", "/v1.0/$batch", request)
	var out struct {
		Responses []struct {
			ID     string
			Status int
		}
	}
	decode(t, rec.Body.String(), &out)
	if rec.Code != 200 || len(out.Responses) != 3 {
		t.Fatalf("the example: %d %s", rec.Code, rec.Body)
	}
	for i, want := range []int{200, 200, 201} {
		if r := out.Responses[i]; r.ID != in.Requests[i].ID || r.Status != want {
			t.Errorf("response %d: %+v, want id %s and status %d", i, r, in.Requests[i].ID, want)
		}
	}
	// the example answer is JSON of the same shape
	var shown struct{ Responses []struct{ ID string } }
	if err := json.Unmarshal([]byte(blocks[1]), &shown); err != nil || len(shown.Responses) != 3 {
		t.Errorf("the example answer: %v %s", err, blocks[1])
	}
}

func TestDocs_AFolderThatAPersonGaveIsReadAtEachRequest(t *testing.T) {
	files := fstest.MapFS{"users.md": {Data: []byte("---\nname: People\nresource: users\n---\nFirst text.\n")}}
	srv := graph.New(graph.NewMemory(graph.Options{Users: 3, Seed: 1}), graph.ServerOptions{Docs: files})
	if p := srv.DocPages(); len(p) != 1 || p[0].Name != "People" || p[0].Description != "First text." {
		t.Fatalf("pages: %+v", p)
	}
	files["users.md"] = &fstest.MapFile{Data: []byte("---\nname: People\nresource: users\n---\nChanged.\n")}
	if p := srv.DocPages(); p[0].Description != "Changed." {
		t.Errorf("the change is not read: %+v", p)
	}
	// a file with a mistake does not take the page away: the last good one stays
	files["users.md"] = &fstest.MapFile{Data: []byte("not a page")}
	if p := srv.DocPages(); len(p) != 1 || p[0].Description != "Changed." {
		t.Errorf("after a mistake: %+v", p)
	}
}
