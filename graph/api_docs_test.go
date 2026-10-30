package graph_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
)

type apiDocs struct {
	Endpoints []struct {
		Method, Path, Area, Summary, Doc, Schema string
		Query, GraphQuery, GraphHeaders          []string
	}
	FilterForms []struct {
		Name, Syntax, Example, Note, Resource string
		NeedsConsistency                      bool
	}
	SearchForm struct {
		Syntax, Example  string
		Fields           []string
		NeedsConsistency bool
	}
	FilterProperties map[string][]struct{ Name, Type, Filter string }
	Options          []struct{ Name, Text string }
}

func readAPIDocs(t *testing.T, h http.Handler) apiDocs {
	t.Helper()
	rec := send(h, "GET", "/_dolly/api", "")
	if rec.Code != 200 {
		t.Fatalf("api: %d %s", rec.Code, rec.Body)
	}
	var d apiDocs
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	return d
}

// The portal shows the forms of $filter, and this test sends the example of each one to the resource that it is
// for, so the help cannot say what the code does not do. A form that needs an advanced query must be refused
// without one, with the error of Graph, and a form that does not must work without one.
func TestAPIDocs_EveryFilterExampleWorks(t *testing.T) {
	h := smallServer(false)
	d := readAPIDocs(t, h)
	if len(d.FilterForms) < 8 {
		t.Fatalf("filter forms: %+v", d.FilterForms)
	}
	advanced := map[string]string{"ConsistencyLevel": "eventual"}
	for _, f := range d.FilterForms {
		if f.Name == "" || f.Syntax == "" || f.Example == "" || f.Note == "" || f.Resource == "" {
			t.Errorf("a form with no text: %+v", f)
		}
		path := "/v1.0/" + map[string]string{"user": "users", "servicePrincipal": "servicePrincipals"}[f.Resource] + "?$filter=" + url.QueryEscape(f.Example)
		if rec := sendWith(h, "GET", path+"&$count=true", advanced); rec.Code != http.StatusOK {
			t.Errorf("the example of %q does not work: %d %s", f.Name, rec.Code, rec.Body)
		}
		rec := send(h, "GET", path, "")
		// the error is the one of Graph: "Filter operator 'NotEqualsMatch'...", or a text about the header, or about the type
		if f.NeedsConsistency && (rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"innerError"`)) {
			t.Errorf("%q needs an advanced query, and a call with none got %d %s", f.Name, rec.Code, rec.Body)
		}
		if !f.NeedsConsistency && rec.Code != http.StatusOK {
			t.Errorf("%q works by default, and a call with no advanced query got %d %s", f.Name, rec.Code, rec.Body)
		}
	}
	if rec := send(h, "GET", "/v1.0/users?$filter="+url.QueryEscape("displayName frobnicate 'x'"), ""); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Invalid filter clause") {
		t.Errorf("a filter that cannot be read: %d %s", rec.Code, rec.Body)
	}

	s := d.SearchForm
	if s.Example == "" || !s.NeedsConsistency || !slices.Equal(s.Fields, []string{"displayName", "description"}) {
		t.Errorf("search form: %+v", s)
	}
	for _, path := range []string{"/v1.0/groups", "/v1.0/users"} {
		if rec := sendWith(h, "GET", path+"?$search="+url.QueryEscape(s.Example), advanced); rec.Code != http.StatusOK {
			t.Errorf("the example of $search on %s does not work: %d %s", path, rec.Code, rec.Body)
		}
	}
	for _, o := range d.Options {
		if o.Name == "$filter" && !strings.Contains(o.Text, "OData") {
			t.Errorf("the text of the $filter option: %q", o.Text)
		}
	}
}

func TestAPIDocs_EachCallHasWhatGraphSays(t *testing.T) {
	d := readAPIDocs(t, smallServer(false))
	find := func(method, path string) int {
		for i, e := range d.Endpoints {
			if e.Method == method && e.Path == path {
				return i
			}
		}
		t.Fatalf("no endpoint %s %s", method, path)
		return -1
	}

	groups := d.Endpoints[find("GET", "/v1.0/groups")]
	if !strings.HasPrefix(groups.Doc, "https://learn.microsoft.com/graph/api/") || groups.Schema != "group" {
		t.Errorf("groups: %+v", groups)
	}
	// what Graph lists and dolly answers: $orderby. What it lists and dolly does not answer yet: $expand on groups
	if !slices.Contains(groups.GraphQuery, "$orderby") || !slices.Contains(groups.Query, "$orderby") {
		t.Errorf("groups: graph %v, dolly %v", groups.GraphQuery, groups.Query)
	}
	if !slices.Contains(groups.GraphQuery, "$expand") || slices.Contains(groups.Query, "$expand") {
		t.Errorf("groups $expand: graph %v, dolly %v", groups.GraphQuery, groups.Query)
	}
	if !slices.Contains(groups.GraphHeaders, "ConsistencyLevel") {
		t.Errorf("headers: %v", groups.GraphHeaders)
	}

	// a call with no $filter has no schema, and a {type} route has the description of its first type
	if m := d.Endpoints[find("GET", "/v1.0/groups/{id}/members")]; m.Schema != "" || slices.Contains(m.Query, "$filter") {
		t.Errorf("members: %+v", m)
	}
	if v := d.Endpoints[find("GET", "/v1.0/groups/{id}/members/{type}")]; v.Doc == "" {
		t.Errorf("the {type} variant has no page: %+v", v)
	}

	// what Graph says about $filter on the properties
	var display string
	for _, p := range d.FilterProperties["group"] {
		if p.Name == "displayName" {
			display = p.Filter
		}
	}
	if !strings.Contains(display, "startsWith") || !strings.Contains(display, "eq") {
		t.Errorf("filterProperties.group.displayName: %q", display)
	}
	if len(d.FilterProperties["user"]) < 20 || len(d.FilterProperties["servicePrincipal"]) < 5 {
		t.Errorf("user %d, servicePrincipal %d", len(d.FilterProperties["user"]), len(d.FilterProperties["servicePrincipal"]))
	}
	if _, has := d.FilterProperties[""]; has {
		t.Error("a call with no schema has no entry")
	}
}

// sendWith is send for a GET with more headers.
func sendWith(h http.Handler, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://127.0.0.1"+path, nil)
	req.Header.Set("Authorization", "Bearer any")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
