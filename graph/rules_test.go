package graph_test

import (
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

// $orderby, $search and $count on directory objects, as Microsoft's article describes them.

func rulesServer() http.Handler {
	return graph.New(graph.Generate(graph.Options{Users: 60, Seed: 3}), graph.ServerOptions{}).Handler()
}

type listAnswer struct {
	Value []map[string]any
	Count *int `json:"@odata.count"`
}

func list(t *testing.T, h http.Handler, path string, advanced bool) (int, listAnswer, string) {
	t.Helper()
	headers := map[string]string{}
	if advanced {
		headers["ConsistencyLevel"] = "eventual"
		path += "&$count=true"
	}
	rec := sendWith(h, "GET", path, headers)
	var out listAnswer
	if rec.Code == 200 {
		decode(t, rec.Body.String(), &out)
	}
	return rec.Code, out, rec.Body.String()
}

func names(l listAnswer, field string) []string {
	var out []string
	for _, u := range l.Value {
		s, _ := u[field].(string)
		out = append(out, s)
	}
	return out
}

func TestOrderBy_ByTheTablesOfGraph(t *testing.T) {
	h := rulesServer()
	// displayName and userPrincipalName sort by default
	code, asc, body := list(t, h, "/v1.0/users?$top=999&$orderby=displayName", false)
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	got := names(asc, "displayName")
	want := slices.Clone(got)
	sort.SliceStable(want, func(i, j int) bool { return strings.ToLower(want[i]) < strings.ToLower(want[j]) })
	if len(got) < 50 || !slices.Equal(got, want) {
		t.Errorf("$orderby=displayName is not in order: %v", got)
	}
	_, desc, _ := list(t, h, "/v1.0/users?$top=999&$orderby=displayName%20desc", false)
	slices.Reverse(want)
	if !slices.Equal(names(desc, "displayName"), want) {
		t.Errorf("$orderby=displayName desc is not the reverse")
	}
	if code, _, body = list(t, h, "/v1.0/users?$orderby=userPrincipalName,displayName%20desc", false); code != 200 {
		t.Errorf("two keys: %d %s", code, body)
	}

	// a property that is not in the table of sorting
	code, _, body = list(t, h, "/v1.0/users?$orderby=mail", true)
	if code != 400 || !strings.Contains(body, "Unsupported sort property 'mail' for 'User'.") {
		t.Errorf("$orderby=mail: %d %s", code, body)
	}
	// a property that sorts only in an advanced query
	if code, _, body = list(t, h, "/v1.0/users?$orderby=createdDateTime", false); code != 400 || !strings.Contains(body, "Unsupported sort property 'createdDateTime' for 'User'.") {
		t.Errorf("createdDateTime without an advanced query: %d %s", code, body)
	}
	if code, _, body = list(t, h, "/v1.0/users?$orderby=createdDateTime", true); code != 200 {
		t.Errorf("createdDateTime in an advanced query: %d %s", code, body)
	}
	// "Use of $filter and $orderby in the same query for directory objects always requires advanced query parameters"
	both := "/v1.0/users?$filter=" + url.QueryEscape("startsWith(displayName,'A')") + "&$orderby=displayName"
	if code, _, body = list(t, h, both, false); code != 400 || !strings.Contains(body, "Sorting not supported for current query.") {
		t.Errorf("$filter with $orderby, with no advanced query: %d %s", code, body)
	}
	if code, _, body = list(t, h, both, true); code != 200 {
		t.Errorf("$filter with $orderby, in an advanced query: %d %s", code, body)
	}
	// a group sorts by displayName by default, and a service principal only in an advanced query
	if code, _, _ = list(t, h, "/v1.0/groups?$orderby=displayName", false); code != 200 {
		t.Errorf("groups by displayName: %d", code)
	}
	if code, _, _ = list(t, h, "/v1.0/servicePrincipals?$orderby=displayName", false); code != 400 {
		t.Errorf("service principals by displayName need an advanced query: %d", code)
	}
	if code, _, _ = list(t, h, "/v1.0/servicePrincipals?$orderby=displayName", true); code != 200 {
		t.Errorf("service principals by displayName, advanced: %d", code)
	}
	if code, _, body = list(t, h, "/v1.0/users?$orderby=displayName%20sideways", false); code != 400 || !strings.Contains(body, "Syntax error at position 20 in 'displayName sideways'.") || !strings.Contains(body, `"code":"BadRequest"`) {
		t.Errorf("a direction that is not one: %d %s", code, body)
	}
	// a place is not a directory object: it sorts by any property
	if code, _, body = list(t, h, "/v1.0/places/microsoft.graph.room?$orderby=displayName", false); code != 200 {
		t.Errorf("places: %d %s", code, body)
	}
}

func TestSearch_ByTheArticleOfGraph(t *testing.T) {
	h := rulesServer()
	_, all, _ := list(t, h, "/v1.0/users?$top=999&$select=displayName,department,mail", false)
	pick := all.Value[3]
	word := strings.Fields(pick["displayName"].(string))[0] // the first name
	dept, _ := pick["department"].(string)

	search := func(s string) (int, listAnswer, string) {
		return list(t, h, "/v1.0/users?$top=999&$search="+url.QueryEscape(s), true)
	}
	// the clause of a property that is searched by words
	code, one, body := search(`"displayName:` + strings.ToLower(word) + `"`)
	if code != 200 || len(one.Value) == 0 {
		t.Fatalf("displayName: %d %s", code, body)
	}
	for _, u := range one.Value {
		if !strings.Contains(strings.ToLower(u["displayName"].(string)), strings.ToLower(word)) {
			t.Errorf("%v does not have the word %q", u["displayName"], word)
		}
	}
	// any other property that $filter works on is a startsWith
	if _, byMail, _ := search(`"mail:` + pick["mail"].(string)[:5] + `"`); len(byMail.Value) == 0 {
		t.Error(`"mail:..." is a startsWith on mail`)
	}
	// AND narrows, OR widens, and parentheses group
	_, and, _ := search(`"displayName:` + word + `" AND "department:` + dept + `"`)
	_, or, _ := search(`"displayName:` + word + `" OR "department:` + dept + `"`)
	_, deptOnly, _ := search(`"department:` + dept + `"`)
	if len(and.Value) == 0 || len(and.Value) > len(one.Value) || len(or.Value) < len(deptOnly.Value) || len(or.Value) < len(one.Value) {
		t.Errorf("one %d, department %d, AND %d, OR %d", len(one.Value), len(deptOnly.Value), len(and.Value), len(or.Value))
	}
	_, grouped, _ := search(`("displayName:` + word + `" OR "department:` + dept + `") AND "department:` + dept + `"`)
	if len(grouped.Value) != len(deptOnly.Value) {
		t.Errorf("parentheses: %d, want %d", len(grouped.Value), len(deptOnly.Value))
	}
	// $search and $filter are joined with AND
	filtered := "/v1.0/users?$top=999&$filter=" + url.QueryEscape("department eq '"+dept+"'") + "&$search=" + url.QueryEscape(`"displayName:`+word+`"`)
	if _, both, _ := list(t, h, filtered, true); len(both.Value) != len(and.Value) {
		t.Errorf("$filter and $search: %d, want %d", len(both.Value), len(and.Value))
	}
	for name, bad := range map[string]string{
		"a property that is not listed": `"nickname:x"`,
		"a clause with no property":     `"just words"`,
		"lower case and":                `"displayName:a" and "displayName:b"`,
		"a clause with no quotes":       `displayName:a`,
		"a parenthesis that stays open": `("displayName:a"`,
		"a quote that stays open":       `"displayName:a`,
	} {
		if code, _, body := search(bad); code != 400 {
			t.Errorf("%s: %q gave %d %s", name, bad, code, body)
		}
	}
	// without the header, with the message of Graph
	rec := sendWith(h, "GET", "/v1.0/users?$search="+url.QueryEscape(`"displayName:a"`), nil)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "Request with $search query parameter only works through MSGraph with a special request header: 'ConsistencyLevel: eventual'") {
		t.Errorf("$search without the header: %d %s", rec.Code, rec.Body)
	}
}

func TestCount_OnDirectoryObjects(t *testing.T) {
	h := rulesServer()
	// ?$count=true without the header is left out, with no error. With the header it is there.
	code, none, body := list(t, h, "/v1.0/users?$top=1", false)
	if code != 200 || none.Count != nil {
		t.Errorf("no count was asked for: %d %s", code, body)
	}
	rec := sendWith(h, "GET", "/v1.0/users?$count=true&$top=1", nil)
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "@odata.count") {
		t.Errorf("$count=true without the header: %d %s", rec.Code, rec.Body)
	}
	if code, out, _ := list(t, h, "/v1.0/users?$top=1", true); code != 200 || out.Count == nil || *out.Count != 60 {
		t.Errorf("$count=true with the header: %d %+v", code, out.Count)
	}
	// the URL segment /$count is an error without the header, and the number with it
	_, groups, _ := list(t, h, "/v1.0/groups?$top=1", false)
	id := groups.Value[0]["id"].(string)
	rec = sendWith(h, "GET", "/v1.0/groups/"+id+"/members/$count", nil)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "Request_UnsupportedQuery") || !strings.Contains(rec.Body.String(), "Raw count (/$count) is not supported because the 'ConsistencyLevel:eventual' header is missing.") {
		t.Errorf("/$count without the header: %d %s", rec.Code, rec.Body)
	}
	if rec = sendWith(h, "GET", "/v1.0/groups/"+id+"/members/$count", map[string]string{"ConsistencyLevel": "eventual"}); rec.Code != 200 {
		t.Errorf("/$count with the header: %d %s", rec.Code, rec.Body)
	}
	// the collections have /$count too, and a filter or a search keeps some
	for _, kind := range []string{"users", "groups", "servicePrincipals"} {
		if rec = sendWith(h, "GET", "/v1.0/"+kind+"/$count", nil); rec.Code != 400 || !strings.Contains(rec.Body.String(), "Raw count (/$count) is not supported") {
			t.Errorf("/%s/$count without the header: %d %s", kind, rec.Code, rec.Body)
		}
	}
	if rec = sendWith(h, "GET", "/v1.0/users/$count", map[string]string{"ConsistencyLevel": "eventual"}); rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != "60" || rec.Header().Get("Content-Type") != "text/plain" {
		t.Errorf("/users/$count: %d %q %s", rec.Code, rec.Body, rec.Header().Get("Content-Type"))
	}
	_, someUsers, _ := list(t, h, "/v1.0/users?$top=999&$filter="+url.QueryEscape("startsWith(displayName,'A')"), false)
	rec = sendWith(h, "GET", "/v1.0/users/$count?$filter="+url.QueryEscape("startsWith(displayName,'A')"), map[string]string{"ConsistencyLevel": "eventual"})
	if strings.TrimSpace(rec.Body.String()) != strconv.Itoa(len(someUsers.Value)) || len(someUsers.Value) == 0 {
		t.Errorf("/users/$count with a filter: %q, want %d", rec.Body, len(someUsers.Value))
	}
	// an operator that needs an advanced query works on the URL segment, because the header makes it advanced
	if rec = sendWith(h, "GET", "/v1.0/users/$count?$filter="+url.QueryEscape("department ne 'Legal'"), map[string]string{"ConsistencyLevel": "eventual"}); rec.Code != 200 {
		t.Errorf("/users/$count with ne: %d %s", rec.Code, rec.Body)
	}
	if rec = sendWith(h, "GET", "/v1.0/oauth2PermissionGrants/$count", nil); rec.Code != 200 {
		t.Errorf("/oauth2PermissionGrants/$count needs no header: %d %s", rec.Code, rec.Body)
	}
	// Graph does not count the organizations at all, with the header or without it
	for _, hdr := range []map[string]string{nil, {"ConsistencyLevel": "eventual"}} {
		if rec = sendWith(h, "GET", "/v1.0/organization/$count", hdr); rec.Code != 400 || !strings.Contains(rec.Body.String(), "unsupported entity 'Organization'") {
			t.Errorf("/organization/$count %v: %d %s", hdr, rec.Code, rec.Body)
		}
	}
	// something that is not a directory object counts with no header
	rec = sendWith(h, "GET", "/v1.0/places/microsoft.graph.room?$count=true&$top=1", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "@odata.count") {
		t.Errorf("places: %d %s", rec.Code, rec.Body)
	}
}
