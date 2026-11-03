package graph_test

import (
	"strings"
	"testing"
	"time"

	"github.com/withholm/dolly-cli/graph"
)

// siteByName finds a seed site with a search, and gives the site.
func siteByName(t *testing.T, call func(method, path, body string) (int, map[string]any), name string) map[string]any {
	t.Helper()
	code, out := call("GET", "/sites?search="+name, "")
	if code != 200 || len(ids(out)) == 0 {
		t.Fatalf("search %q: %d, %d sites", name, code, len(ids(out)))
	}
	for _, v := range out["value"].([]any) {
		if s := v.(map[string]any); s["name"] == name {
			return s
		}
	}
	t.Fatalf("no site named %q in %v", name, ids(out))
	return nil
}

func TestSites_RootAndGet(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	call := func(method, path, body string) (int, map[string]any) { return graphCall(t, ts, method, path, body) }

	// the root site: its ID is a host and two GUIDs, and it is the root of its site collection
	code, root := call("GET", "/sites/root", "")
	id, _ := root["id"].(string)
	parts := strings.Split(id, ",")
	if code != 200 || len(parts) != 3 || !strings.HasSuffix(parts[0], ".sharepoint.com") {
		t.Fatalf("root: %d %v", code, root)
	}
	if root["webUrl"] != "https://"+parts[0] || root["siteCollection"].(map[string]any)["hostname"] != parts[0] || root["root"] == nil {
		t.Errorf("root site: %v", root)
	}
	for _, k := range []string{"displayName", "name", "createdDateTime", "lastModifiedDateTime", "isPersonalSite"} {
		if _, has := root[k]; !has {
			t.Errorf("the site has no %s", k)
		}
	}
	for _, k := range []string{"kind", "group", "groupId", "parent", "path"} {
		if _, has := root[k]; has {
			t.Errorf("the site shows %q: Graph does not", k)
		}
	}

	// by ID, with $select, and by the host and the path (the link of the site)
	if code, got := call("GET", "/sites/"+id+"?$select=id,displayName", ""); code != 200 || got["id"] != id || got["webUrl"] != nil {
		t.Errorf("get by ID with $select: %d %v", code, got)
	}
	intranet := siteByName(t, call, "intranet")
	for _, path := range []string{"/sites/" + parts[0] + ":/sites/intranet", "/sites/" + parts[0] + ":/sites/intranet:", "/sites/" + strings.ToUpper(parts[0]) + ":/sites/Intranet/"} {
		if code, got := call("GET", path, ""); code != 200 || got["id"] != intranet["id"] {
			t.Errorf("get by link %s: %d %v", path, code, got)
		}
	}
	if code, got := call("GET", "/sites/"+parts[0]+":", ""); code != 200 || got["id"] != id {
		t.Errorf("the root site by its host: %d %v", code, got)
	}

	// what is not there: the real Graph answers 404 itemNotFound for SharePoint (checked with the proxy of Graph Explorer, for a drive)
	for _, path := range []string{"/sites/" + parts[0] + ",00000000-0000-0000-0000-000000000000,00000000-0000-0000-0000-000000000000", "/sites/" + parts[0] + ":/sites/nobody", "/sites/nobody"} {
		code, out := call("GET", path, "")
		if e, _ := out["error"].(map[string]any); code != 404 || e["code"] != "itemNotFound" || e["message"] != "Item not found" {
			t.Errorf("%s: %d %v", path, code, out)
		}
	}
}

func TestSites_SearchAndList(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	call := func(method, path, body string) (int, map[string]any) { return graphCall(t, ts, method, path, body) }

	_, all := call("GET", "/sites?search=*", "")
	if len(ids(all)) < 5 {
		t.Errorf("search=* gives %d sites, want the root site, the team sites, the SharePoint sites, and the subsite", len(ids(all)))
	}
	if _, every := call("GET", "/sites/getAllSites()", ""); len(ids(every)) != len(ids(all)) {
		t.Errorf("getAllSites gives %d sites, and search=* gives %d", len(ids(every)), len(ids(all)))
	}
	// the text is free: the name, the title, the description, and the address
	for text, want := range map[string]string{"intranet": "intranet", "HR POLICIES": "hr-policies", "newcomer": "", "forms": "hr-policies"} {
		_, out := call("GET", "/sites?search="+strings.ReplaceAll(text, " ", "%20"), "")
		found := false
		for _, v := range out["value"].([]any) {
			found = found || v.(map[string]any)["name"] == want
		}
		if want != "" && !found {
			t.Errorf("search %q does not find %q: %v", text, want, ids(out))
		}
	}
	if _, none := call("GET", "/sites?search=zzzznothing", ""); len(ids(none)) != 0 || none["value"] == nil {
		t.Errorf("a search with no match must give an empty list: %v", none)
	}
	// $orderby on the date of creation, as the Learn page says
	_, newest := call("GET", "/sites?search=*&$orderby=createdDateTime%20desc", "")
	first := newest["value"].([]any)[0].(map[string]any)["createdDateTime"].(string)
	for _, v := range newest["value"].([]any) {
		if v.(map[string]any)["createdDateTime"].(string) > first {
			t.Fatalf("$orderby=createdDateTime desc: %s comes after %s", first, v.(map[string]any)["createdDateTime"])
		}
	}
}

func TestSites_SubsitesAndDrives(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	call := func(method, path, body string) (int, map[string]any) { return graphCall(t, ts, method, path, body) }

	hr := siteByName(t, call, "hr-policies")
	hrID := hr["id"].(string)
	// the subsite is below the site, has the same site collection, and is not the root of one
	_, subs := call("GET", "/sites/"+hrID+"/sites", "")
	if len(ids(subs)) != 1 {
		t.Fatalf("subsites: %v", ids(subs))
	}
	sub := subs["value"].([]any)[0].(map[string]any)
	if sub["name"] != "onboarding" || sub["webUrl"] != hr["webUrl"].(string)+"/onboarding" || sub["root"] != nil || sub["siteCollection"] != nil {
		t.Errorf("subsite: %v", sub)
	}
	if strings.Split(sub["id"].(string), ",")[1] != strings.Split(hrID, ",")[1] {
		t.Errorf("a subsite is in the site collection of its parent: %v and %v", sub["id"], hrID)
	}
	if _, none := call("GET", "/sites/"+sub["id"].(string)+"/sites", ""); len(ids(none)) != 0 {
		t.Errorf("the subsite has subsites: %v", ids(none))
	}

	// the drives: a site has a library, Documents, and it is also the default drive
	code, drives := call("GET", "/sites/"+hrID+"/drives", "")
	if code != 200 || len(ids(drives)) != 1 {
		t.Fatalf("drives: %d %v", code, drives)
	}
	d := drives["value"].([]any)[0].(map[string]any)
	if d["name"] != "Documents" || d["driveType"] != "documentLibrary" || !strings.HasPrefix(d["id"].(string), "b!") || d["quota"] == nil || d["owner"] == nil {
		t.Errorf("drive: %v", d)
	}
	if !strings.HasPrefix(d["webUrl"].(string), hr["webUrl"].(string)+"/") {
		t.Errorf("the drive is at the address of the site: %v", d["webUrl"])
	}
	for _, path := range []string{"/sites/" + hrID + "/drive", "/sites/" + hrID + "/drives/" + d["id"].(string)} {
		if code, got := call("GET", path, ""); code != 200 || got["id"] != d["id"] {
			t.Errorf("%s: %d %v", path, code, got)
		}
	}
	// below the link of a site
	host := strings.Split(hrID, ",")[0]
	if code, got := call("GET", "/sites/"+host+":/sites/hr-policies:/drive", ""); code != 200 || got["id"] != d["id"] {
		t.Errorf("the drive by the link: %d %v", code, got)
	}
	if code, got := call("GET", "/sites/"+host+":/sites/hr-policies:/drives", ""); code != 200 || len(ids(got)) != 1 {
		t.Errorf("the drives by the link: %d %v", code, got)
	}
	if code, got := call("GET", "/sites/"+host+":/sites/hr-policies:/sites", ""); code != 200 || len(ids(got)) != 1 {
		t.Errorf("the subsites by the link: %d %v", code, got)
	}
	if code, out := call("GET", "/sites/"+hrID+"/drives/b!nothing", ""); code != 404 || out["error"].(map[string]any)["code"] != "itemNotFound" {
		t.Errorf("an unknown drive: %d %v", code, out)
	}
	if code, _ := call("GET", "/sites/nobody/drives", ""); code != 404 {
		t.Errorf("the drives of an unknown site: %d", code)
	}
}

// A Microsoft 365 group has a team site, and a call reaches it through the group. A site that has no group is found only by its link or by search.
func TestSites_GroupSite(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	call := func(method, path, body string) (int, map[string]any) { return graphCall(t, ts, method, path, body) }

	_, g := call("GET", "/groups?$filter=displayName%20eq%20'grp.m365.announcements'", "")
	if len(ids(g)) != 1 {
		t.Fatalf("the Microsoft 365 group of the seed: %v", ids(g))
	}
	gid := ids(g)[0]
	code, site := call("GET", "/groups/"+gid+"/sites/root", "")
	if code != 200 || site["displayName"] != "grp.m365.announcements" || site["siteCollection"] == nil {
		t.Fatalf("the site of the group: %d %v", code, site)
	}
	// the same site is in a search, and is drives are owned by the group
	if found := siteByName(t, call, site["name"].(string)); found["id"] != site["id"] {
		t.Errorf("search finds another site: %v", found)
	}
	_, drive := call("GET", "/sites/"+site["id"].(string)+"/drive", "")
	if owner, _ := drive["owner"].(map[string]any); owner["group"].(map[string]any)["id"] != gid {
		t.Errorf("the owner of the drive of a group site: %v", drive["owner"])
	}
	if code, got := call("GET", "/groups/"+gid+"/sites/"+site["id"].(string), ""); code != 200 || got["id"] != site["id"] {
		t.Errorf("the site of the group by its ID: %d %v", code, got)
	}
	if code, _ := call("GET", "/groups/"+gid+"/sites/"+siteByName(t, call, "intranet")["id"].(string), ""); code != 404 {
		t.Errorf("a site of no group is not the site of this group: %d", code)
	}

	// a group that is not a Microsoft 365 group has no site, and a group that is not there is a 404 of the directory
	_, other := call("GET", "/groups?$filter=startswith(displayName,'grp.dept')&$top=1", "")
	if code, out := call("GET", "/groups/"+ids(other)[0]+"/sites/root", ""); code != 404 || out["error"].(map[string]any)["code"] != "itemNotFound" {
		t.Errorf("a group with no site: %d %v", code, out)
	}
	if code, out := call("GET", "/groups/00000000-0000-0000-0000-000000000000/sites/root", ""); code != 404 || out["error"].(map[string]any)["code"] != "Request_ResourceNotFound" {
		t.Errorf("an unknown group: %d %v", code, out)
	}
}

func TestSites_CreateUpdateDelete(t *testing.T) {
	_, _, srv := startServer(t, graph.ServerOptions{})
	w := srv.World()
	a := graph.Actor{}

	// a site with no group, a subsite, and a drive
	s, e := w.CreateSite(map[string]any{"name": "projects", "displayName": "Projects", "description": "All of them"}, a)
	if e != nil || s["webUrl"] == nil || !strings.HasSuffix(s["webUrl"].(string), "/sites/projects") || s["root"] == nil {
		t.Fatalf("create: %v %v", e, s)
	}
	sid := s["id"].(string)
	if _, e := w.CreateSite(map[string]any{"name": "Projects"}, a); e == nil {
		t.Error("a name with capitals must be refused")
	}
	if _, e := w.CreateSite(map[string]any{"name": "projects"}, a); e == nil {
		t.Error("the same address twice must be refused")
	}
	sub, e := w.CreateSite(map[string]any{"name": "plans", "parentId": sid}, a)
	if e != nil || !strings.HasSuffix(sub["webUrl"].(string), "/sites/projects/plans") || sub["displayName"] != "plans" {
		t.Fatalf("subsite: %v %v", e, sub)
	}
	if got, _ := w.Subsites(sid); len(got) != 1 {
		t.Errorf("subsites: %d", len(got))
	}
	if drives, _ := w.SiteDrives(sid); len(drives) != 1 || drives[0]["name"] != "Documents" {
		t.Errorf("a new site has the drive Documents: %v", drives)
	}
	d, e := w.CreateDrive(sid, map[string]any{"name": "Contracts", "description": "Signed"}, a)
	if e != nil || d["driveType"] != "documentLibrary" {
		t.Fatalf("drive: %v %v", e, d)
	}
	if _, e := w.CreateDrive(sid, map[string]any{"name": "contracts"}, a); e == nil {
		t.Error("a second drive with the same name must be refused")
	}
	if _, e := w.CreateDrive(sid, map[string]any{}, a); e == nil {
		t.Error("a drive needs a name")
	}

	// the group site: a Microsoft 365 group, once
	g := graphGroup(t, w, true)
	gs, e := w.CreateSite(map[string]any{"kind": "group", "groupId": g}, a)
	if e != nil || gs["root"] == nil {
		t.Fatalf("group site: %v %v", e, gs)
	}
	if _, e := w.CreateSite(map[string]any{"kind": "group", "groupId": g}, a); e == nil {
		t.Error("a group has one site")
	}
	if _, e := w.CreateSite(map[string]any{"kind": "group", "groupId": graphGroup(t, w, false)}, a); e == nil {
		t.Error("only a Microsoft 365 group has a site")
	}
	if _, e := w.CreateSite(map[string]any{"kind": "sharepoint", "name": "x", "groupId": g}, a); e == nil {
		t.Error("a SharePoint site has no group")
	}
	if _, e := w.CreateSite(map[string]any{"kind": "team", "name": "x"}, a); e == nil {
		t.Error("an unknown kind must be refused")
	}

	// changes: the title and the description only
	got, e := w.UpdateSite(sid, map[string]any{"displayName": "Project hub", "description": ""}, a)
	if e != nil || got["displayName"] != "Project hub" || got["description"] != "" {
		t.Fatalf("update: %v %v", e, got)
	}
	for _, bad := range []map[string]any{{"name": "other"}, {"webUrl": "x"}, {"displayName": " "}} {
		if _, e := w.UpdateSite(sid, bad, a); e == nil {
			t.Errorf("update %v must be refused", bad)
		}
	}
	if _, e := w.UpdateDrive(d["id"].(string), map[string]any{"name": "Documents"}, a); e == nil {
		t.Error("a drive cannot take the name of another drive of the site")
	}
	if upd, e := w.UpdateDrive(d["id"].(string), map[string]any{"name": "Agreements"}, a); e != nil || upd["name"] != "Agreements" {
		t.Errorf("rename a drive: %v %v", e, upd)
	}

	// deleting: not a site with subsites, not the root, not the last drive. A site goes with its drives.
	if e := w.DeleteSite(sid, a); e == nil {
		t.Error("a site with subsites cannot be deleted")
	}
	root, _ := w.RootSite()
	if e := w.DeleteSite(root["id"].(string), a); e == nil {
		t.Error("the root site cannot be deleted")
	}
	drives, _ := w.SiteDrives(sid)
	if e := w.DeleteDrive(drives[1]["id"].(string), a); e != nil {
		t.Error(e)
	}
	if e := w.DeleteDrive(drives[0]["id"].(string), a); e == nil {
		t.Error("a site keeps one drive")
	}
	if e := w.DeleteSite(sub["id"].(string), a); e != nil {
		t.Fatal(e)
	}
	if e := w.DeleteSite(sid, a); e != nil {
		t.Fatal(e)
	}
	if _, ok := w.GetSite(sid); ok {
		t.Error("the site is not gone")
	}
	if _, ok := w.SiteDrives(sid); ok {
		t.Error("the drives of a deleted site must be gone")
	}
}

// graphGroup gives the ID of a group: a Microsoft 365 group with no site, or one that is not.
func graphGroup(t *testing.T, w *graph.World, unified bool) string {
	t.Helper()
	props := map[string]any{"displayName": "Plain", "mailNickname": "plain", "mailEnabled": false, "securityEnabled": true}
	if unified {
		// the seed has one Microsoft 365 group and it has its site: make another
		props = map[string]any{"displayName": "Extra", "mailNickname": "extra", "mailEnabled": true, "securityEnabled": false, "groupTypes": []any{"Unified"}}
	}
	g, e := w.CreateGroup(props, graph.Actor{})
	if e != nil {
		t.Fatal(e)
	}
	return g["id"].(string)
}

func TestSites_KeptAndReplayed(t *testing.T) {
	dir := t.TempDir()
	opt := graph.OpenOptions{Users: 10, Seed: 3, Now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }}
	w, err := graph.Open(dir, opt)
	if err != nil {
		t.Fatal(err)
	}
	s, e := w.CreateSite(map[string]any{"name": "keep", "displayName": "Keep"}, graph.Actor{})
	if e != nil {
		t.Fatal(e)
	}
	gone, e := w.CreateSite(map[string]any{"name": "gone"}, graph.Actor{})
	if e != nil {
		t.Fatal(e)
	}
	d, e := w.CreateDrive(s["id"].(string), map[string]any{"name": "Extra"}, graph.Actor{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e := w.UpdateSite(s["id"].(string), map[string]any{"description": "kept"}, graph.Actor{}); e != nil {
		t.Fatal(e)
	}
	if _, e := w.UpdateDrive(d["id"].(string), map[string]any{"name": "Renamed"}, graph.Actor{}); e != nil {
		t.Fatal(e)
	}
	if e := w.DeleteSite(gone["id"].(string), graph.Actor{}); e != nil {
		t.Fatal(e)
	}
	_ = w.Close()

	w2, err := graph.Open(dir, opt)
	if err != nil {
		t.Fatal(err)
	}
	defer w2.Close()
	got, ok := w2.GetSite(s["id"].(string))
	if !ok || got["description"] != "kept" || got["displayName"] != "Keep" {
		t.Errorf("after the restart: %v %v", ok, got)
	}
	if _, ok := w2.GetSite(gone["id"].(string)); ok {
		t.Error("the deleted site is back")
	}
	if drives, _ := w2.SiteDrives(s["id"].(string)); len(drives) != 2 || drives[1]["name"] != "Renamed" || drives[1]["id"] != d["id"] {
		t.Errorf("the drives after the restart: %v", drives)
	}
	h := w2.History()
	if len(h) != 6 || h[0].Text() != "Made the site Keep" || h[2].Text() != "Made the drive Extra on the site Keep" || h[5].Text() != "Deleted the site gone" {
		for _, e := range h {
			t.Log(e.Text())
		}
		t.Errorf("history: %d entries", len(h))
	}
	if h[3].Text() != "Changed the site Keep: description = kept" {
		t.Errorf("the change of a site: %q", h[3].Text())
	}

	// a rollback goes back to the seed, and the sites of the seed are there
	if err := w2.Rollback(0, graph.Actor{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := w2.GetSite(s["id"].(string)); ok {
		t.Error("the site must be gone after a rollback to the seed")
	}
	if root, ok := w2.RootSite(); !ok || root["id"] == nil {
		t.Error("the seed has a root site")
	}
}

// The portal makes and changes sites through the admin API, and the Graph API reads them.
func TestSites_AdminAPI(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	var list struct {
		Sites  []map[string]any `json:"sites"`
		Groups []map[string]any `json:"groups"`
	}
	admin(t, ts, "GET", "/sites", "", &list)
	if len(list.Sites) < 5 || list.Sites[0]["kind"] == nil || list.Sites[0]["drives"] == nil || list.Groups == nil {
		t.Fatalf("the list: %d sites, %v", len(list.Sites), list.Sites[0])
	}
	kinds := map[any]int{}
	for _, s := range list.Sites {
		kinds[s["kind"]]++
	}
	if kinds["group"] != 1 || kinds["sharepoint"] < 4 {
		t.Errorf("kinds of the seed sites: %v", kinds)
	}

	var made map[string]any
	if code := admin(t, ts, "POST", "/sites", `{"name":"labs","displayName":"Labs"}`, &made); code != 200 || made["displayName"] != "Labs" {
		t.Fatalf("make: %d %v", code, made)
	}
	id := made["id"].(string)
	if code, got := graphCall(t, ts, "GET", "/sites/"+id, ""); code != 200 || got["name"] != "labs" {
		t.Errorf("Graph reads the new site: %d %v", code, got)
	}
	var drive map[string]any
	if code := admin(t, ts, "POST", "/sites/"+id+"/drives", `{"name":"Specs"}`, &drive); code != 200 || drive["name"] != "Specs" {
		t.Fatalf("make a drive: %d %v", code, drive)
	}
	if code, got := graphCall(t, ts, "GET", "/sites/"+id+"/drives", ""); code != 200 || len(ids(got)) != 2 {
		t.Errorf("Graph reads the drives: %d %v", code, got)
	}
	if code := admin(t, ts, "PATCH", "/drives/"+drive["id"].(string), `{"description":"Technical"}`, &drive); code != 200 || drive["description"] != "Technical" {
		t.Errorf("change a drive: %d %v", code, drive)
	}
	if code := admin(t, ts, "PATCH", "/sites/"+id, `{"name":"x"}`, nil); code != 400 {
		t.Errorf("the name of a site cannot change: %d", code)
	}
	if code := admin(t, ts, "POST", "/sites", `[1]`, nil); code != 400 {
		t.Errorf("a body that is not an object: %d", code)
	}
	if code := admin(t, ts, "DELETE", "/drives/"+drive["id"].(string), "", nil); code != 204 {
		t.Errorf("delete a drive: %d", code)
	}
	if code := admin(t, ts, "DELETE", "/sites/"+id, "", nil); code != 204 {
		t.Errorf("delete the site: %d", code)
	}
	if code, _ := graphCall(t, ts, "GET", "/sites/"+id, ""); code != 404 {
		t.Errorf("Graph reads the deleted site: %d", code)
	}
	if code := admin(t, ts, "DELETE", "/sites/"+id, "", nil); code != 404 {
		t.Errorf("delete it again: %d", code)
	}
}

// The portal makes folders and files on a drive through the admin API, and the Graph API reads them.
func TestDrives_AdminAPI(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	call := func(method, path, body string) (int, map[string]any) { return graphCall(t, ts, method, path, body) }
	id, _ := hrDrive(t, call)

	var list struct {
		Items []map[string]any `json:"items"`
	}
	admin(t, ts, "GET", "/drives/"+id+"/items", "", &list)
	if len(list.Items) != 6 || list.Items[0]["name"] == nil {
		t.Fatalf("the items of the seed: %v", list.Items)
	}
	var folder, file map[string]any
	if code := admin(t, ts, "POST", "/drives/"+id+"/items", `{"name":"Plans","folder":true}`, &folder); code != 200 || folder["folder"] == nil {
		t.Fatalf("a folder: %d %v", code, folder)
	}
	if code := admin(t, ts, "POST", "/drives/"+id+"/items", `{"parentId":"`+folder["id"].(string)+`","name":"todo.txt","text":"one\ntwo"}`, &file); code != 200 || file["size"] != float64(7) {
		t.Fatalf("a file with text: %d %v", code, file)
	}
	if code := admin(t, ts, "POST", "/drives/"+id+"/items", `{"parentId":"`+folder["id"].(string)+`","name":"big.bin","size":2048}`, &file); code != 200 || file["size"] != float64(2048) {
		t.Fatalf("a file with a size: %d %v", code, file)
	}
	if code := admin(t, ts, "POST", "/drives/"+id+"/items", `{"name":"plans","folder":true}`, nil); code != 409 {
		t.Errorf("a name that is taken: %d", code)
	}
	if code := admin(t, ts, "POST", "/drives/"+id+"/items", `{"name":"huge.bin","size":99999999}`, nil); code != 400 {
		t.Errorf("a size above the limit: %d", code)
	}
	if code, got := call("GET", "/drives/"+id+"/root:/Plans/todo.txt", ""); code != 200 || got["size"] != float64(7) {
		t.Errorf("Graph reads the file: %d %v", code, got)
	}
	if code := admin(t, ts, "PATCH", "/drives/"+id+"/items/"+file["id"].(string), `{"name":"large.bin"}`, &file); code != 200 || file["name"] != "large.bin" {
		t.Errorf("rename: %d %v", code, file)
	}
	if code := admin(t, ts, "DELETE", "/drives/"+id+"/items/"+folder["id"].(string), "", nil); code != 204 {
		t.Errorf("delete: %d", code)
	}
	if code, _ := call("GET", "/drives/"+id+"/root:/Plans", ""); code != 404 {
		t.Errorf("Graph reads the deleted folder: %d", code)
	}
	if code := admin(t, ts, "GET", "/drives/b!nothing/items", "", nil); code != 404 && code != 400 {
		t.Errorf("an unknown drive: %d", code)
	}
}
