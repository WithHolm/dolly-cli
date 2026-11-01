package graph_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

// ---- helpers for a delta cycle ----

type deltaPage struct {
	Context   string           `json:"@odata.context"`
	NextLink  string           `json:"@odata.nextLink"`
	DeltaLink string           `json:"@odata.deltaLink"`
	Value     []map[string]any `json:"value"`
}

// deltaGet makes one call to a link (or a path), with headers, and gives the status, the headers, and the body.
func deltaGet(t *testing.T, ts *httptest.Server, link string, header map[string]string) (int, http.Header, []byte) {
	t.Helper()
	if strings.HasPrefix(link, "/") {
		link = ts.URL + "/v1.0" + link
	}
	req, _ := http.NewRequest("GET", link, nil)
	req.Header.Set("Authorization", "Bearer x")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header, b
}

// deltaRound follows the links of one round, from the first call to the page with the deltaLink. It gives every item, the pages it took,
// and the deltaLink. The links point at the test server, and keep the form of the first call.
func deltaRound(t *testing.T, ts *httptest.Server, first string, header map[string]string) (items []map[string]any, pages []deltaPage, deltaLink string) {
	t.Helper()
	link := first
	for i := 0; i < 50; i++ {
		code, _, body := deltaGet(t, ts, link, header)
		if code != 200 {
			t.Fatalf("%s: %d %s", link, code, body)
		}
		var p deltaPage
		if err := json.Unmarshal(body, &p); err != nil {
			t.Fatal(err)
		}
		pages = append(pages, p)
		items = append(items, p.Value...)
		if p.NextLink != "" && p.DeltaLink != "" {
			t.Fatalf("a page has both links: %v", p)
		}
		switch {
		case p.DeltaLink != "":
			return items, pages, p.DeltaLink
		case p.NextLink != "":
			link = p.NextLink
		default:
			t.Fatalf("a page with neither link: %s", body)
		}
	}
	t.Fatal("a round that does not end")
	return
}

func byID(items []map[string]any) map[string]map[string]any {
	m := map[string]map[string]any{}
	for _, it := range items {
		m[it["id"].(string)] = it
	}
	return m
}

func serverOf(t *testing.T, users int) (*httptest.Server, *graph.World) {
	t.Helper()
	w := graph.Generate(graph.Options{Users: users, Seed: 5})
	ts := httptest.NewServer(graph.New(w, graph.ServerOptions{}).Handler())
	t.Cleanup(ts.Close)
	return ts, w
}

// ---- users ----

// The first round gives every user in pages of 200, then an empty page with the deltaLink. A round with no changes takes two calls: an empty
// page with a nextLink, and an empty page with the next deltaLink (checked with the real Graph). The links keep the form of the first call.
func TestDelta_Users_FirstRoundAndAnEmptyRound(t *testing.T) {
	ts, _ := serverOf(t, 450)
	items, pages, deltaLink := deltaRound(t, ts, "/users/delta", nil)
	if len(items) != 450 || len(pages) != 4 { // 200 + 200 + 50 + the empty last page
		t.Fatalf("%d users in %d pages", len(items), len(pages))
	}
	last := pages[len(pages)-1]
	if len(last.Value) != 0 || last.DeltaLink == "" || last.NextLink != "" || last.Value == nil {
		t.Errorf("the last page is empty, with the deltaLink and an empty list: %+v", last)
	}
	if pages[0].Context != "https://graph.microsoft.com/v1.0/$metadata#users" {
		t.Errorf("context: %s", pages[0].Context)
	}
	if !strings.Contains(pages[0].NextLink, "/v1.0/users/delta?$skiptoken=") || !strings.Contains(deltaLink, "/v1.0/users/delta?$deltatoken=") {
		t.Errorf("links: %s %s", pages[0].NextLink, deltaLink)
	}
	// the default properties of delta are four (checked with the real Graph), and a property that has no value is not there
	for _, it := range items {
		for k := range it {
			if k != "id" && k != "displayName" && k != "mail" && k != "userPrincipalName" {
				t.Fatalf("a property that is not tracked: %v", it)
			}
		}
	}

	items2, pages2, _ := deltaRound(t, ts, deltaLink, nil)
	if len(items2) != 0 || len(pages2) != 2 || pages2[0].NextLink == "" || pages2[1].DeltaLink == "" {
		t.Errorf("a round with no changes: %d items in %d pages: %+v", len(items2), len(pages2), pages2)
	}

	// the form with the brackets keeps its form in the links
	_, pg, _ := deltaRound(t, ts, "/users/delta()", nil)
	if !strings.Contains(pg[0].NextLink, "/users/delta()?$skiptoken=") {
		t.Errorf("the form delta(): %s", pg[0].NextLink)
	}
}

// What changed since the deltaLink: a user that was made (with its properties), a user that changed a tracked property, a user that was
// deleted (its id and @removed), and nothing for a change to a property that is not tracked.
func TestDelta_Users_Changes(t *testing.T) {
	ts, w := serverOf(t, 30)
	_, _, deltaLink := deltaRound(t, ts, "/users/delta", nil)

	users := w.ListUsers()
	changed, quiet, gone := users[0]["id"].(string), users[1]["id"].(string), users[2]["id"].(string)
	call := func(method, path, body string) int { code, _ := graphCall(t, ts, method, path, body); return code }
	if call("PATCH", "/users/"+changed, `{"displayName":"New Name"}`) != 204 ||
		call("PATCH", "/users/"+quiet, `{"officeLocation":"Nowhere"}`) != 204 || // not tracked by default
		call("DELETE", "/users/"+gone, "") != 204 {
		t.Fatal("the changes")
	}
	_, created := graphCall(t, ts, "POST", "/users", newUser)
	madeID := created["id"].(string)

	items, _, next := deltaRound(t, ts, deltaLink, nil)
	got := byID(items)
	if len(got) != 3 {
		t.Fatalf("%d items, want the changed, the deleted, and the made user: %v", len(items), items)
	}
	if got[changed]["displayName"] != "New Name" || got[changed]["userPrincipalName"] == nil {
		t.Errorf("a changed user has the tracked properties: %v", got[changed])
	}
	if rm := got[gone]["@removed"].(map[string]any); rm["reason"] != "deleted" || len(got[gone]) != 2 {
		t.Errorf("a deleted user is its id and @removed: %v", got[gone])
	}
	if got[madeID]["displayName"] != "Ada Test" || got[madeID]["userPrincipalName"] != "ada.test@acme.example" {
		t.Errorf("a made user has its properties: %v", got[madeID])
	}
	if _, has := got[quiet]; has {
		t.Errorf("a change to a property that is not tracked does not show the user: %v", got[quiet])
	}

	// the next round starts after these changes, so it is empty
	if again, _, _ := deltaRound(t, ts, next, nil); len(again) != 0 {
		t.Errorf("the changes come once: %v", again)
	}
}

// $select chooses the tracked properties in the first call. It is in the links, so the later calls repeat nothing.
func TestDelta_Users_SelectIsTracked(t *testing.T) {
	ts, w := serverOf(t, 30)
	first, pages, deltaLink := deltaRound(t, ts, "/users/delta?$select=officeLocation,department", nil)
	if pages[0].Context != "https://graph.microsoft.com/v1.0/$metadata#users(officeLocation,department)" {
		t.Errorf("context: %s", pages[0].Context)
	}
	for _, it := range first {
		if _, has := it["displayName"]; has {
			t.Fatalf("only the selected properties, and id: %v", it)
		}
	}

	users := w.ListUsers()
	a, b := users[0]["id"].(string), users[1]["id"].(string)
	graphCall(t, ts, "PATCH", "/users/"+a, `{"officeLocation":"Room 1"}`)
	graphCall(t, ts, "PATCH", "/users/"+b, `{"displayName":"Other"}`) // not tracked now
	items, _, _ := deltaRound(t, ts, deltaLink, nil)
	got := byID(items)
	if len(got) != 1 || got[a]["officeLocation"] != "Room 1" {
		t.Errorf("only the change to a selected property shows: %v", items)
	}

	// a rename changes the field of every user that has the value: each of them shows, when the field is tracked
	_, _, link := deltaRound(t, ts, "/users/delta?$select=department", nil)
	dept := ""
	for _, u := range w.ListUsers() {
		if d, _ := u["department"].(string); d != "" {
			dept = d
			break
		}
	}
	n := w.Rename("department", dept, "Renamed", graph.Actor{Source: "admin"})
	items, _, _ = deltaRound(t, ts, link, nil)
	if n == 0 || len(items) != n {
		t.Errorf("a rename of %d users gave %d items", n, len(items))
	}
	for _, it := range items {
		if it["department"] != "Renamed" {
			t.Errorf("a renamed user: %v", it)
		}
	}
}

// Prefer: return=minimal gives only the properties that changed (and id) for a changed user. A user that was made has all of them.
func TestDelta_Users_ReturnMinimal(t *testing.T) {
	ts, w := serverOf(t, 30)
	_, _, deltaLink := deltaRound(t, ts, "/users/delta?$select=displayName,officeLocation,mail", nil)
	id := w.ListUsers()[3]["id"].(string)
	graphCall(t, ts, "PATCH", "/users/"+id, `{"officeLocation":"Moved"}`)
	graphCall(t, ts, "POST", "/users", newUser)

	items, _, _ := deltaRound(t, ts, deltaLink, map[string]string{"Prefer": "return=minimal"})
	got := byID(items)
	if len(got) != 2 {
		t.Fatalf("%v", items)
	}
	if len(got[id]) != 2 || got[id]["officeLocation"] != "Moved" {
		t.Errorf("minimal: only id and what changed: %v", got[id])
	}
	for _, it := range items {
		if it["displayName"] == "Ada Test" && it["mail"] == nil && it["officeLocation"] == nil && len(it) < 2 {
			t.Errorf("a made user is not cut: %v", it)
		}
	}
}

// $deltatoken=latest skips the objects: an empty page with a deltaLink, and the changes after it come in the next round.
func TestDelta_Users_Latest(t *testing.T) {
	ts, w := serverOf(t, 30)
	items, pages, link := deltaRound(t, ts, "/users/delta?$deltatoken=latest", nil)
	if len(items) != 0 || len(pages) != 1 {
		t.Fatalf("latest: %d items in %d pages", len(items), len(pages))
	}
	id := w.ListUsers()[0]["id"].(string)
	graphCall(t, ts, "PATCH", "/users/"+id, `{"displayName":"After"}`)
	got, _, _ := deltaRound(t, ts, link, nil)
	if len(got) != 1 || got[0]["displayName"] != "After" {
		t.Errorf("the change after latest: %v", got)
	}
}

// $filter takes only id eq '…' (several with or). The other filters, a token that is not a token, and an id that is not there are the
// errors that Graph gives (checked with the real Graph).
func TestDelta_Users_FilterAndErrors(t *testing.T) {
	ts, w := serverOf(t, 30)
	u := w.ListUsers()
	a, b := u[0]["id"].(string), u[1]["id"].(string)

	items, _, link := deltaRound(t, ts, "/users/delta?$filter="+url.QueryEscape("id eq '"+a+"' or id eq '"+b+"'"), nil)
	if len(items) != 2 {
		t.Errorf("two ids: %v", items)
	}
	graphCall(t, ts, "PATCH", "/users/"+a, `{"displayName":"A2"}`)
	graphCall(t, ts, "PATCH", "/users/"+u[2]["id"].(string), `{"displayName":"C2"}`)
	if again, _, _ := deltaRound(t, ts, link, nil); len(again) != 1 || again[0]["id"] != a {
		t.Errorf("the filter is in the link: %v", again)
	}

	for _, c := range []struct{ path, code, text string }{
		{"/users/delta?$filter=" + url.QueryEscape("displayName eq 'x'"), "Request_UnsupportedQuery", "Unsupported query."},
		{"/users/delta?$skiptoken=garbage", "BadRequest", "Badly formed token."},
		{"/users/delta?$deltatoken=garbage", "BadRequest", "Badly formed token."},
	} {
		code, _, body := deltaGet(t, ts, c.path, nil)
		var out map[string]any
		_ = json.Unmarshal(body, &out)
		if code != 400 || errCode(out) != c.code || errText(out) != c.text {
			t.Errorf("%s: %d %s %q", c.path, code, errCode(out), errText(out))
		}
	}
	// $top, $orderby, $count, and $expand are accepted and change nothing (checked with the real Graph)
	for _, q := range []string{"$top=1", "$orderby=displayName", "$count=true", "$expand=manager"} {
		if got, _, _ := deltaRound(t, ts, "/users/delta?"+q, nil); len(got) != 30 {
			t.Errorf("%s: %d items", q, len(got))
		}
	}
}

// After a reset the data is another epoch: an old token is 410 Gone, with a Location that starts the cycle again. A world with no folder has
// a new epoch at each start, so a token from another world is 410 too.
func TestDelta_Users_OldTokensAreGone(t *testing.T) {
	ts, w := serverOf(t, 30)
	_, _, deltaLink := deltaRound(t, ts, "/users/delta", nil)
	_, pages, _ := deltaRound(t, ts, "/users/delta", nil)
	skip := pages[0].NextLink

	if err := w.Reset(graph.Actor{Source: "admin"}); err != nil {
		t.Fatal(err)
	}
	for _, link := range []string{deltaLink, skip} {
		code, hdr, body := deltaGet(t, ts, link, nil)
		var out map[string]any
		_ = json.Unmarshal(body, &out)
		if code != http.StatusGone || errCode(out) != "syncStateNotFound" || !strings.HasSuffix(hdr.Get("Location"), "/v1.0/users/delta?$deltatoken=") {
			t.Errorf("after a reset: %d %s %s", code, errCode(out), hdr.Get("Location"))
		}
	}
	// and the cycle starts again without a token
	if items, _, _ := deltaRound(t, ts, "/users/delta", nil); len(items) != 30 {
		t.Errorf("start again: %d", len(items))
	}

	other, _ := serverOf(t, 30)
	if code, _, _ := deltaGet(t, other, strings.Replace(deltaLink, ts.URL, other.URL, 1), nil); code != http.StatusGone {
		t.Errorf("a token of another world: %d", code)
	}
}

// With a folder the log of changes is made again from the history, so a deltaLink is good after a restart. A rollback is another epoch.
func TestDelta_Users_AfterARestartAndARollback(t *testing.T) {
	dir := t.TempDir()
	w, err := graph.Open(dir, graph.OpenOptions{Users: 30, Seed: 5})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(graph.New(w, graph.ServerOptions{}).Handler())
	id := w.ListUsers()[0]["id"].(string)
	graphCall(t, ts, "PATCH", "/users/"+id, `{"displayName":"Before"}`)
	_, _, deltaLink := deltaRound(t, ts, "/users/delta", nil)
	graphCall(t, ts, "PATCH", "/users/"+id, `{"displayName":"After"}`)
	graphCall(t, ts, "POST", "/users", newUser)
	w.Rename("department", "Sales", "Selling", graph.Actor{Source: "admin"})
	host := strings.TrimPrefix(ts.URL, "http://")

	ts.Close()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := graph.Open(dir, graph.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ts2 := httptest.NewServer(graph.New(again, graph.ServerOptions{}).Handler())
	defer ts2.Close()
	defer again.Close()

	link := strings.Replace(deltaLink, host, strings.TrimPrefix(ts2.URL, "http://"), 1)
	items, _, next := deltaRound(t, ts2, link, nil)
	got := byID(items)
	if got[id]["displayName"] != "After" || len(items) < 2 {
		t.Fatalf("after a restart the link still gives the changes: %v", items)
	}

	if err := again.Rollback(0, graph.Actor{Source: "admin"}); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := deltaGet(t, ts2, next, nil); code != http.StatusGone {
		t.Errorf("after a rollback: %d", code)
	}
}
