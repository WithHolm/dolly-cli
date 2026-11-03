package graph_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/withholm/dolly-cli/graph"
)

// rawCall sends a request with a body of bytes, and gives the status, the headers, and the body. It does not follow a redirect.
func rawCall(t *testing.T, ts *httptest.Server, method, path string, body []byte, token bool) (int, http.Header, []byte) {
	t.Helper()
	req, _ := http.NewRequest(method, ts.URL+path, bytes.NewReader(body))
	if token {
		req.Header.Set("Authorization", "Bearer x")
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header, out
}

// driveIDs gives the default drive of the site hr-policies, and the site.
func hrDrive(t *testing.T, call func(method, path, body string) (int, map[string]any)) (drive, site string) {
	t.Helper()
	hr := siteByName(t, call, "hr-policies")
	_, d := call("GET", "/sites/"+hr["id"].(string)+"/drive", "")
	return d["id"].(string), hr["id"].(string)
}

// The owner of every world, in the test servers (Acme): see repo.OwnerName.
const (
	ownerName = "Philip Meholm"
	ownerUPN  = "philip.meholm@acme.example"
	maxUpload = 4 << 20
)

func itemNames(out map[string]any) []string {
	var n []string
	for _, v := range out["value"].([]any) {
		n = append(n, v.(map[string]any)["name"].(string))
	}
	return n
}

func TestDrives_ReachedManyWays(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	call := func(method, path, body string) (int, map[string]any) { return graphCall(t, ts, method, path, body) }
	id, _ := hrDrive(t, call)

	// the same drive by its ID, and through the site
	if code, d := call("GET", "/drives/"+id, ""); code != 200 || d["id"] != id || d["driveType"] != "documentLibrary" {
		t.Errorf("/drives/{id}: %d %v", code, d)
	}
	// the drive of a group is the default drive of its team site
	_, g := call("GET", "/groups?$filter=displayName%20eq%20'grp.m365.announcements'", "")
	gid := ids(g)[0]
	_, site := call("GET", "/groups/"+gid+"/sites/root", "")
	_, want := call("GET", "/sites/"+site["id"].(string)+"/drive", "")
	if code, d := call("GET", "/groups/"+gid+"/drive", ""); code != 200 || d["id"] != want["id"] {
		t.Errorf("the drive of a group: %d %v, want %v", code, d["id"], want["id"])
	}
	// the drive of a user is OneDrive. It is found by the ID and by the user principal name.
	_, u := call("GET", "/users/"+ownerUPN+"?$select=id", "")
	uid := u["id"].(string)
	code, mine := call("GET", "/users/"+uid+"/drive", "")
	owner, _ := mine["owner"].(map[string]any)["user"].(map[string]any)
	if code != 200 || mine["driveType"] != "business" || mine["name"] != "OneDrive" || owner["id"] != uid || owner["displayName"] != ownerName {
		t.Fatalf("the drive of a user: %d %v", code, mine)
	}
	if _, again := call("GET", "/users/"+ownerUPN+"/drive", ""); again["id"] != mine["id"] {
		t.Errorf("by the user principal name: %v", again["id"])
	}
	if code, d := call("GET", "/drives/"+mine["id"].(string), ""); code != 200 || d["id"] != mine["id"] {
		t.Errorf("the drive of a user by its ID: %d", code)
	}

	// the errors of the real Graph (checked with the proxy of Graph Explorer): a user that is not there, and an ID that is not an ID of a drive
	if code, out := call("GET", "/users/nobody@fjordvik.example/drive", ""); code != 404 || out["error"].(map[string]any)["code"] != "ResourceNotFound" || errText(out) != "User not found" {
		t.Errorf("an unknown user: %d %v", code, out)
	}
	for _, path := range []string{"/drives/b!nothing", "/drives/b!nothing/root"} {
		if code, out := call("GET", path, ""); code != 400 || out["error"].(map[string]any)["code"] != "invalidRequest" ||
			errText(out) != "The provided drive id appears to be malformed, or does not represent a valid drive." {
			t.Errorf("%s: %d %v", path, code, out)
		}
	}
	// guess: an ID with the form of a drive that is not there is a 404
	if code, out := call("GET", "/drives/b!"+strings.Repeat("A", 64), ""); code != 404 || out["error"].(map[string]any)["code"] != "itemNotFound" {
		t.Errorf("an unknown drive: %d %v", code, out)
	}
}

func TestDrives_ReadItems(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	call := func(method, path, body string) (int, map[string]any) { return graphCall(t, ts, method, path, body) }
	id, _ := hrDrive(t, call)
	base := "/drives/" + id

	code, root := call("GET", base+"/root", "")
	if code != 200 || root["name"] != "root" || root["root"] == nil || root["folder"].(map[string]any)["childCount"] != float64(3) || root["size"].(float64) < 300000 {
		t.Fatalf("root: %d %v", code, root)
	}
	_, kids := call("GET", base+"/root/children", "")
	if got := itemNames(kids); len(got) != 3 || !slices.Contains(got, "Archive") || !slices.Contains(got, "General") || !slices.Contains(got, "Welcome.docx") {
		t.Fatalf("the items in the root: %v", got)
	}
	var docx, general map[string]any
	for _, v := range kids["value"].([]any) {
		switch v.(map[string]any)["name"] {
		case "Welcome.docx":
			docx = v.(map[string]any)
		case "General":
			general = v.(map[string]any)
		}
	}
	// a file: size, type, the address of the download, and where it is
	if docx["size"] != float64(18432) || docx["file"].(map[string]any)["mimeType"] != "application/vnd.openxmlformats-officedocument.wordprocessingml.document" ||
		docx["folder"] != nil || !strings.HasPrefix(docx["@microsoft.graph.downloadUrl"].(string), ts.URL+"/dolly-download/") {
		t.Errorf("a file: %v", docx)
	}
	pr := docx["parentReference"].(map[string]any)
	if pr["driveId"] != id || pr["driveType"] != "documentLibrary" || pr["id"] != root["id"] || pr["path"] != "/drives/"+id+"/root:" {
		t.Errorf("parentReference of a file in the root: %v", pr)
	}
	// a folder: the count of what is in it, and the size of what is in it
	if general["folder"].(map[string]any)["childCount"] != float64(2) || general["file"] != nil || general["size"] != float64(612+52224) || general["@microsoft.graph.downloadUrl"] != nil {
		t.Errorf("a folder: %v", general)
	}

	// by ID, with $select, and by path (with the colon forms of the Learn pages)
	if code, got := call("GET", base+"/items/"+docx["id"].(string)+"?$select=name,size", ""); code != 200 || got["name"] != "Welcome.docx" || got["eTag"] != nil || got["id"] == nil {
		t.Errorf("get by ID: %d %v", code, got)
	}
	for _, p := range []string{"/root:/General/Readme.txt", "/root:/general/README.TXT", "/root:/General/Readme.txt:"} {
		code, got := call("GET", base+p, "")
		if pr, _ := got["parentReference"].(map[string]any); code != 200 || got["name"] != "Readme.txt" || pr["path"] != "/drives/"+id+"/root:/General" || pr["name"] != "General" {
			t.Errorf("get %s: %d %v", p, code, got)
		}
	}
	if code, got := call("GET", base+"/root:/General:/children", ""); code != 200 || len(ids(got)) != 2 {
		t.Errorf("children by path: %d %v", code, got)
	}
	if code, got := call("GET", base+"/items/"+general["id"].(string)+"/children?$top=1", ""); code != 200 || len(ids(got)) != 1 || got["@odata.nextLink"] == nil {
		t.Errorf("children with $top: %d %v", code, got)
	}
	// from the real Graph: a path that is not there is 404 itemNotFound
	for _, p := range []string{"/root:/Nope", "/root:/General/Nope.txt", "/items/01NOTANITEM"} {
		if code, out := call("GET", base+p, ""); code != 404 || out["error"].(map[string]any)["code"] != "itemNotFound" {
			t.Errorf("%s: %d %v", p, code, out)
		}
	}
	// guess: a file has no children
	if code, _ := call("GET", base+"/items/"+docx["id"].(string)+"/children", ""); code != 400 {
		t.Errorf("the children of a file: %d", code)
	}

	// the same items by the other ways to the drive
	hr := siteByName(t, call, "hr-policies")
	if code, got := call("GET", "/sites/"+hr["id"].(string)+"/drive/root:/General/Readme.txt", ""); code != 200 || got["name"] != "Readme.txt" {
		t.Errorf("by the site: %d %v", code, got)
	}
	if code, got := call("GET", "/users/"+ownerUPN+"/drive/root:/Documents/Projects/Ideas.md", ""); code != 200 || got["size"] != float64(2048) {
		t.Errorf("in the OneDrive of the owner: %d %v", code, got)
	}
}

func TestDrives_Download(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	call := func(method, path, body string) (int, map[string]any) { return graphCall(t, ts, method, path, body) }
	id, _ := hrDrive(t, call)
	base := "/v1.0/drives/" + id

	// /content is a 302 to the address of the download, which needs no token
	code, hdr, _ := rawCall(t, ts, "GET", base+"/root:/General/Readme.txt:/content", nil, true)
	loc := hdr.Get("Location")
	if code != 302 || !strings.Contains(loc, "/dolly-download/") {
		t.Fatalf("content: %d %q", code, loc)
	}
	u, _ := url.Parse(loc)
	code, hdr, body := rawCall(t, ts, "GET", u.RequestURI(), nil, false)
	if code != 200 || len(body) != 612 || !strings.HasPrefix(string(body), "dollycli sample content for Readme.txt\n") || !strings.HasPrefix(hdr.Get("Content-Type"), "text/plain") {
		t.Errorf("download: %d %d bytes %q %v", code, len(body), body[:20], hdr.Get("Content-Type"))
	}
	// the address answers a range, as the Learn page says
	req, _ := http.NewRequest("GET", ts.URL+u.RequestURI(), nil)
	req.Header.Set("Range", "bytes=0-9")
	res, _ := http.DefaultClient.Do(req)
	part, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 206 || string(part) != "dollycli s" {
		t.Errorf("range: %d %q", res.StatusCode, part)
	}

	// new content makes a new address, and the old one stops
	if code, _, _ := rawCall(t, ts, "PUT", base+"/root:/General/Readme.txt:/content", []byte("new text"), true); code != 200 {
		t.Fatalf("put: %d", code)
	}
	if code, _, _ := rawCall(t, ts, "GET", u.RequestURI(), nil, false); code != 404 {
		t.Errorf("the old address after new content: %d", code)
	}
	_, hdr, _ = rawCall(t, ts, "GET", base+"/root:/General/Readme.txt:/content", nil, true)
	if _, _, body := rawCall(t, ts, "GET", strings.TrimPrefix(hdr.Get("Location"), ts.URL), nil, false); string(body) != "new text" {
		t.Errorf("the new content: %q", body)
	}
	// a folder has no content
	if code, _, _ := rawCall(t, ts, "GET", base+"/root:/General:/content", nil, true); code != 400 {
		t.Errorf("the content of a folder: %d", code)
	}
}

func TestDrives_WriteItems(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	call := func(method, path, body string) (int, map[string]any) { return graphCall(t, ts, method, path, body) }
	id, _ := hrDrive(t, call)
	base := "/drives/" + id

	// a folder: 201, and the same name again is 409 nameAlreadyExists (the default is fail). rename makes "Reports 1", replace takes the place.
	code, f := call("POST", base+"/root/children", `{"name":"Reports","folder":{}}`)
	if code != 201 || f["folder"] == nil || f["name"] != "Reports" {
		t.Fatalf("make a folder: %d %v", code, f)
	}
	if code, out := call("POST", base+"/root/children", `{"name":"reports","folder":{}}`); code != 409 || out["error"].(map[string]any)["code"] != "nameAlreadyExists" {
		t.Errorf("a name that is taken (the case does not matter): %d %v", code, out)
	}
	if code, r := call("POST", base+"/root/children", `{"name":"Reports","folder":{},"@microsoft.graph.conflictBehavior":"rename"}`); code != 201 || r["name"] != "Reports 1" {
		t.Errorf("rename: %d %v", code, r)
	}
	for _, bad := range []string{`{"name":"a/b","folder":{}}`, `{"name":"a:b","folder":{}}`, `{"name":" a","folder":{}}`, `{"name":"a.","folder":{}}`, `{"folder":{}}`, `{"name":"x"}`, `{"name":"x","folder":{},"file":{}}`} {
		if code, _ := call("POST", base+"/root/children", bad); code != 400 {
			t.Errorf("%s: %d", bad, code)
		}
	}

	// a file by path: the folders that are not there are made. 201 for a new file, 200 for the same name again.
	content := []byte("a,b\n1,2\n")
	code, hdr, out := rawCall(t, ts, "PUT", "/v1.0"+base+"/root:/Reports/2026/Q1.csv:/content", content, true)
	_ = hdr
	if code != 201 || !bytes.Contains(out, []byte(`"size":8`)) || !bytes.Contains(out, []byte(`"mimeType":"text/csv`)) {
		t.Fatalf("put by path: %d %s", code, out)
	}
	if code, got := call("GET", base+"/root:/Reports/2026", ""); code != 200 || got["folder"].(map[string]any)["childCount"] != float64(1) {
		t.Errorf("the folder that the path made: %d %v", code, got)
	}
	if code, _, _ := rawCall(t, ts, "PUT", "/v1.0"+base+"/root:/Reports/2026/Q1.csv:/content", []byte("x,y\n"), true); code != 200 {
		t.Errorf("put again: %d", code)
	}
	if code, _, _ := rawCall(t, ts, "PUT", "/v1.0"+base+"/root:/Reports/2026/Q1.csv:/content?@microsoft.graph.conflictBehavior=fail", []byte("x"), true); code != 409 {
		t.Errorf("put with fail: %d", code)
	}
	// a file by the ID of the folder and a name
	if code, _, _ := rawCall(t, ts, "PUT", "/v1.0"+base+"/items/"+f["id"].(string)+":/notes.txt:/content", []byte("hello"), true); code != 201 {
		t.Errorf("put by folder ID and name: %d", code)
	}
	code, got := call("GET", base+"/root:/Reports/notes.txt", "")
	if code != 200 || got["size"] != float64(5) {
		t.Fatalf("the file: %d %v", code, got)
	}
	// the content comes back as it went in
	_, hdr, _ = rawCall(t, ts, "GET", "/v1.0"+base+"/items/"+got["id"].(string)+"/content", nil, true)
	if _, _, b := rawCall(t, ts, "GET", strings.TrimPrefix(hdr.Get("Location"), ts.URL), nil, false); string(b) != "hello" {
		t.Errorf("content: %q", b)
	}
	// an empty file, from a folder call
	if code, e := call("POST", base+"/root:/Reports:/children", `{"name":"empty.txt","file":{}}`); code != 201 || e["size"] != float64(0) {
		t.Errorf("an empty file: %d %v", code, e)
	}

	// rename, move, and the conflicts of both
	if code, r := call("PATCH", base+"/items/"+got["id"].(string), `{"name":"notes 2.txt"}`); code != 200 || r["name"] != "notes 2.txt" || r["eTag"] == got["eTag"] || r["cTag"] != got["cTag"] {
		t.Errorf("rename (the eTag changes, the cTag does not): %d %v", code, r)
	}
	_, q1 := call("GET", base+"/root:/Reports/2026/Q1.csv", "")
	if code, _ := call("PATCH", base+"/items/"+got["id"].(string), `{"name":"empty.txt"}`); code != 409 {
		t.Errorf("rename to a name that is taken: %d", code)
	}
	_, y := call("GET", base+"/root:/Reports/2026", "")
	if code, m := call("PATCH", base+"/items/"+got["id"].(string), `{"parentReference":{"id":"`+y["id"].(string)+`"}}`); code != 200 || m["parentReference"].(map[string]any)["path"] != "/drives/"+id+"/root:/Reports/2026" {
		t.Errorf("move: %d %v", code, m)
	}
	if code, _ := call("PATCH", base+"/items/"+f["id"].(string), `{"parentReference":{"id":"`+y["id"].(string)+`"}}`); code != 400 {
		t.Errorf("a folder into a folder in itself: %d", code)
	}
	if code, _ := call("PATCH", base+"/items/"+q1["id"].(string), `{"size":1}`); code != 400 {
		t.Errorf("size cannot change: %d", code)
	}
	if code, _ := call("PATCH", base+"/root:/Reports/2026/Q1.csv", `{"name":"Q1 final.csv"}`); code != 200 {
		t.Errorf("rename by path: %d", code)
	}

	// too large, and a file is not a folder
	if code, _, _ := rawCall(t, ts, "PUT", "/v1.0"+base+"/root:/big.bin:/content", make([]byte, maxUpload+1), true); code != 413 {
		t.Errorf("a file that is too large: %d", code)
	}
	// guess: a file where a folder should be is the same as a name that is taken
	if code, _, _ := rawCall(t, ts, "PUT", "/v1.0"+base+"/root:/Reports/2026/Q1 final.csv/inside.txt:/content", []byte("x"), true); code != 409 {
		t.Errorf("a file as a folder: %d", code)
	}

	// delete: a folder goes with what is in it, and the root stays
	if code, _ := call("DELETE", base+"/items/"+f["id"].(string), ""); code != 204 {
		t.Errorf("delete a folder: %d", code)
	}
	for _, p := range []string{"/root:/Reports", "/root:/Reports/2026/Q1 final.csv", "/items/" + got["id"].(string)} {
		if code, _ := call("GET", base+p, ""); code != 404 {
			t.Errorf("%s after the delete: %d", p, code)
		}
	}
	if code, _ := call("DELETE", base+"/root", ""); code == 204 {
		t.Error("the root cannot be deleted")
	}
	if code, _ := call("DELETE", base+"/items/"+f["id"].(string), ""); code != 404 {
		t.Errorf("delete again: %d", code)
	}
	// a site that is deleted takes its items
	_, siteID := hrDrive(t, call)
	_ = siteID
}

func TestDrives_KeptAndReplayed(t *testing.T) {
	dir := t.TempDir()
	opt := graph.OpenOptions{Users: 10, Seed: 3, Now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }}
	w, err := graph.Open(dir, opt)
	if err != nil {
		t.Fatal(err)
	}
	a := graph.Actor{}
	site, e := w.CreateSite(map[string]any{"name": "keep"}, a)
	if e != nil {
		t.Fatal(e)
	}
	drives, _ := w.SiteDrives(site["id"].(string))
	d := drives[0]["id"].(string)
	if _, e := w.CreateChild(d, "root", map[string]any{"name": "Docs", "folder": map[string]any{}}, a); e != nil {
		t.Fatal(e)
	}
	file, _, e := w.PutFile(d, "/Docs", "a.txt", []byte("kept content"), "", a)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := w.UpdateItem(d, file["id"].(string), map[string]any{"name": "b.txt"}, a); e != nil {
		t.Fatal(e)
	}
	gone, _, e := w.PutFile(d, "root", "gone.txt", []byte("x"), "", a)
	if e != nil {
		t.Fatal(e)
	}
	if e := w.DeleteItem(d, gone["id"].(string), a); e != nil {
		t.Fatal(e)
	}
	_ = w.Close()

	w2, err := graph.Open(dir, opt)
	if err != nil {
		t.Fatal(err)
	}
	defer w2.Close()
	it, data, e := w2.Content(d, "/Docs/b.txt")
	if e != nil || string(data) != "kept content" || it["id"] != file["id"] || it["size"] != float64(12) {
		t.Errorf("after the restart: %v %q %v", e, data, it)
	}
	if _, e := w2.GetItem(d, gone["id"].(string)); e == nil {
		t.Error("the deleted file is back")
	}
	h := w2.History()
	want := []string{"Made the site keep", "Made the folder Docs on Documents on keep", "Made the file a.txt on Documents on keep", "Changed the item b.txt on Documents on keep",
		"Made the file gone.txt on Documents on keep", "Deleted the item gone.txt on Documents on keep"}
	if len(h) != len(want) {
		t.Fatalf("history: %d entries", len(h))
	}
	for i, text := range want {
		if h[i].Text() != text {
			t.Errorf("history %d: %q, want %q", i, h[i].Text(), text)
		}
	}
	// a rollback to before the file: the folder is there and the file is not
	if err := w2.Rollback(2, a); err != nil {
		t.Fatal(err)
	}
	if _, e := w2.GetItem(d, "/Docs"); e != nil {
		t.Errorf("the folder after the rollback: %v", e)
	}
	if _, e := w2.GetItem(d, "/Docs/b.txt"); e == nil {
		t.Error("the file must be gone after the rollback")
	}
	// a site that is deleted takes its drive and its items
	if e := w2.DeleteSite(site["id"].(string), a); e == nil {
		if _, e := w2.GetDrive(d); e == nil {
			t.Error("the drive of a deleted site")
		}
	}
}

// The shared router sends only the prefixes of an API to it. The download addresses must be one of them, or a client that follows the 302 of /content gets a 404
// from the router (the test servers above call the handler of the API, and do not go through the router).
func TestDrives_DownloadPathIsRouted(t *testing.T) {
	_, _, srv := startServer(t, graph.ServerOptions{})
	if !slices.Contains(srv.Prefixes(), "/dolly-download/") {
		t.Errorf("the prefixes of the API: %v", srv.Prefixes())
	}
}

// A body larger than what the log keeps (1 MB) must reach the handler whole.
func TestDrives_LargeUploadIsNotCut(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	call := func(method, path, body string) (int, map[string]any) { return graphCall(t, ts, method, path, body) }
	id, _ := hrDrive(t, call)
	data := bytes.Repeat([]byte("0123456789abcdef"), 3*65536) // 3 MB
	if code, _, out := rawCall(t, ts, "PUT", "/v1.0/drives/"+id+"/root:/three.bin:/content", data, true); code != 201 || !bytes.Contains(out, []byte(`"size":3145728`)) {
		t.Fatalf("put 3 MB: %d %s", code, out)
	}
	_, hdr, _ := rawCall(t, ts, "GET", "/v1.0/drives/"+id+"/root:/three.bin:/content", nil, true)
	if _, _, got := rawCall(t, ts, "GET", strings.TrimPrefix(hdr.Get("Location"), ts.URL), nil, false); !bytes.Equal(got, data) {
		t.Errorf("the content that came back is %d bytes, and the file was %d", len(got), len(data))
	}
}
