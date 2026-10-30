package docs_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/withholm/dolly-cli/shared/docs"
)

const users = "---\nname: Users\norder: 2\ncovers: me=Signed-in user\n---\n" +
	"The people of the tenant.\n\nSecond paragraph.\n\n" +
	"## GET /v1.0/users\nList the users.\n\n### Filtering\nText only.\n\n### Search\nOnly displayName.\n\n" +
	"## GET /v1.0/users/{id}\nOne user.\n\n```\n## this is code, not a call\n### nor this\n```\n"

func load(t *testing.T, files map[string]string) ([]docs.Page, error) {
	t.Helper()
	m := fstest.MapFS{}
	for name, text := range files {
		m["docs/"+name] = &fstest.MapFile{Data: []byte(text)}
	}
	return docs.Load(m, "docs")
}

func TestLoad_ReadsPagesCallsAndSections(t *testing.T) {
	pages, err := load(t, map[string]string{
		"users.md":  users,
		"batch.md":  "---\nname: Batch\nresource: $batch\norder: 1\n---\nRun 20 calls in one.\n\n## POST /v1.0/$batch\nA call.\n",
		"README.md": "not a page, and not read: ---",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 || pages[0].Name != "Batch" || pages[1].Name != "Users" {
		t.Fatalf("the pages are in the order of order: %+v", pages)
	}
	b, u := pages[0], pages[1]
	if b.Resource != "$batch" || b.Description != "Run 20 calls in one." || u.Resource != "users" {
		t.Errorf("resource and text: %+v %+v", b, u)
	}
	if u.Description != "The people of the tenant.\n\nSecond paragraph." {
		t.Errorf("description: %q", u.Description)
	}
	if len(u.Covers) != 1 || u.Covers[0].Segment != "me" || u.Covers[0].Section != "Signed-in user" {
		t.Errorf("covers: %+v", u.Covers)
	}
	if len(u.Ops) != 2 || u.Ops[0].Key != "GET /v1.0/users" || u.Ops[0].Body != "List the users." {
		t.Fatalf("ops: %+v", u.Ops)
	}
	if s := u.Ops[0].Sections; len(s) != 2 || s[0].Title != "Filtering" || s[0].Body != "Text only." || s[1].Title != "Search" {
		t.Errorf("sections: %+v", s)
	}
	// a title inside a code block is code
	if one := u.Ops[1]; one.Key != "GET /v1.0/users/{id}" || !strings.Contains(one.Body, "## this is code, not a call") || len(one.Sections) != 0 {
		t.Errorf("code block: %+v", one)
	}
}

func TestLoad_ErrorsSayWhatToDo(t *testing.T) {
	for name, c := range map[string]struct{ text, want string }{
		"no front matter":     {"just text", "must start with"},
		"no name":             {"---\norder: 1\n---\ntext", "needs a name"},
		"a key that is wrong": {"---\nname: A\ncolour: red\n---\n", `no "colour"`},
		"order not a number":  {"---\nname: A\norder: first\n---\n", "whole number"},
		"covers wrong":        {"---\nname: A\ncovers: me\n---\n", "segment=Section title"},
		"title is not a call": {"---\nname: A\n---\n## Users\ntext", "method and the path"},
		"method is wrong":     {"---\nname: A\n---\n## FETCH /v1.0/x\ntext", "method and the path"},
		"title of level 1":    {"---\nname: A\n---\n# Users\n", "level 1"},
		"part without a call": {"---\nname: A\n---\n### Filtering\ntext", "part of a call"},
		"code is not closed":  {"---\nname: A\n---\n```\ntext", "not closed"},
	} {
		if _, err := load(t, map[string]string{"a.md": c.text}); err == nil || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), "a.md") {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
}

func TestCheck_FindsWhatDoesNotFitTheRoutes(t *testing.T) {
	routes := []string{"GET /v1.0/users", "GET /v1.0/users/{id}", "GET /v1.0/me", "POST /v1.0/$batch", "GET /v1.0/groups"}
	pages, _ := load(t, map[string]string{
		"users.md": users,
		"batch.md": "---\nname: Batch\nresource: $batch\n---\n",
	})
	problems := strings.Join(docs.Check(pages, routes, "/v1.0"), "\n")
	// groups has no page
	if !strings.Contains(problems, "calls of /groups have no page") {
		t.Errorf("a resource without a page: %q", problems)
	}
	if strings.Contains(problems, "/me") || strings.Contains(problems, "$batch") {
		t.Errorf("me is covered by users, and $batch has its page: %q", problems)
	}

	pages, _ = load(t, map[string]string{
		"users.md":  users + "\n## DELETE /v1.0/users/{id}\nA call that is not there.\n\n## GET /v1.0/users\nTwice.\n\n## GET /v1.0/me\n",
		"batch.md":  "---\nname: Batch\nresource: $batch\n---\n",
		"groups.md": "---\nname: Groups\n---\n",
		"other.md":  "---\nname: Other\nresource: users\n---\n",
		"gone.md":   "---\nname: Gone\n---\n",
	})
	problems = strings.Join(docs.Check(pages, routes, "/v1.0"), "\n")
	for _, want := range []string{
		`"DELETE /v1.0/users/{id}" is not a call of the API`,
		`"GET /v1.0/users" is there twice`,
		`"GET /v1.0/me" has no text`,
		"both cover",
		"gone.md covers /gone, and no call starts with it",
	} {
		if !strings.Contains(problems, want) {
			t.Errorf("missing %q in:\n%s", want, problems)
		}
	}
}
