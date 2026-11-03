package repo

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

// The SharePoint sites of the tenant, and the drives (document libraries) on them. The data side of api/sites.go. It satisfies api.SitesBackend.
//
// A site is one of two kinds. A "group" site is the team site of a Microsoft 365 group: a call reaches it through the group
// (/groups/{id}/sites/root). A "sharepoint" site has no group: a call finds it only by its link (/sites/{host}:/{path}), by search, or by a list
// of all sites. A site with a parent is a subsite. Graph shows none of this in the site: the kind is only for the portal.
//
// The rules are from the Learn pages "Get site", "Get a site resource", "Search for sites", "List subsites", and "List drives" (the pinned commit
// of graph/cost/raw/SOURCE.json). The real Graph was asked for the error of a drive and of a site it cannot show (see graph/docs/sites.md).

// Events that the sites and their drives make.
const (
	evSiteCreate  = "site.create"  // a new site (Site has it)
	evSiteUpdate  = "site.update"  // displayName or description changed (Set has the fields, and "modified")
	evSiteDelete  = "site.delete"  // a site is gone, with its drives
	evDriveCreate = "drive.create" // a new drive (Drive has it)
	evDriveUpdate = "drive.update" // name or description changed (Set)
	evDriveDelete = "drive.delete" // a drive is gone
)

// Site is a SharePoint site as dolly keeps it.
type Site struct {
	// ID is what Graph says: the host, the ID of the site collection, and the ID of the site, with commas between.
	ID          string `json:"id"`
	Name        string `json:"name"` // the last part of the path. The tenant root site has none.
	DisplayName string `json:"displayName"`
	Description string `json:"description,omitempty"`
	// Kind is "group" or "sharepoint".
	Kind string `json:"kind"`
	// Group is the object ID of the Microsoft 365 group of a group site.
	Group string `json:"group,omitempty"`
	// Parent is the ID of the site that has this one as a subsite.
	Parent string `json:"parent,omitempty"`
	// Path is the server relative URL: /sites/intranet. The tenant root site has "".
	Path     string    `json:"path"`
	Created  time.Time `json:"created"`
	Modified time.Time `json:"modified"`
}

// Drive is a document library of a site.
type Drive struct {
	ID          string `json:"id"` // b! and the characters that SharePoint gives
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Site        string `json:"site"` // the ID of the site
	// User is the object ID of the user whose drive this is (OneDrive). Such a drive is not stored: it is made from the user (drive_items.go).
	User     string    `json:"user,omitempty"`
	Created  time.Time `json:"created"`
	Modified time.Time `json:"modified"`
}

var siteName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// driveIDForm is what an ID of a drive looks like: b! and 64 characters. Anything else is "malformed" for the real Graph (checked with b!nothing).
var driveIDForm = regexp.MustCompile(`^b![A-Za-z0-9_-]{64}$`)

func (s *Site) host() string { return strings.SplitN(s.ID, ",", 2)[0] }

// collection gives the ID of the site collection, the middle part of the ID.
func (s *Site) collection() string {
	parts := strings.Split(s.ID, ",")
	if len(parts) == 3 {
		return parts[1]
	}
	return ""
}

func (s *Site) object() Object {
	host := s.host()
	o := Object{
		"id": s.ID, "name": s.Name, "displayName": s.DisplayName, "description": s.Description,
		"createdDateTime": s.Created.UTC().Format(time.RFC3339), "lastModifiedDateTime": s.Modified.UTC().Format(time.RFC3339),
		"webUrl": "https://" + host + s.Path, "isPersonalSite": false,
	}
	if s.Parent == "" { // the root site of its site collection
		o["root"] = Object{}
		o["siteCollection"] = Object{"hostname": host}
	}
	return o
}

func (d *Drive) object(w *World) Object {
	site := w.sitesByID[strings.ToLower(d.Site)]
	o := Object{
		"id": d.ID, "name": d.Name, "description": d.Description, "driveType": driveType(d),
		"createdDateTime": d.Created.UTC().Format(time.RFC3339), "lastModifiedDateTime": d.Modified.UTC().Format(time.RFC3339),
		"quota": Object{"total": float64(27487790694400), "used": float64(0), "remaining": float64(27487790694400), "deleted": float64(0), "state": "normal"},
	}
	system := w.identity(d)
	o["createdBy"], o["lastModifiedBy"] = system, system
	o["owner"] = system
	o["webUrl"] = w.driveWebURL(d)
	if site != nil {
		if site.Kind == "group" {
			if g := w.groups[strings.ToLower(site.Group)]; g != nil {
				o["owner"] = Object{"group": Object{"id": g.ID, "displayName": g.Name}}
			}
		}
	}
	return o
}

// ---- the lookups ----

func (w *World) indexSites() {
	w.sitesByID = make(map[string]*Site, len(w.Sites))
	for _, s := range w.Sites {
		w.sitesByID[strings.ToLower(s.ID)] = s
	}
	w.drivesByID = make(map[string]*Drive, len(w.Drives))
	for _, d := range w.Drives {
		w.drivesByID[strings.ToLower(d.ID)] = d
	}
	w.indexItems()
}

// sharepointHost is the host of the sites: the first part of the initial domain, and sharepoint.com. Call it with the lock held, or while
// the world is made.
func (w *World) sharepointHost() string {
	initial := ""
	if w.Profile != nil {
		initial = w.Profile.Organization.InitialDomain
	}
	if initial == "" {
		initial, _, _ = strings.Cut(w.defaultDomain(), ".")
	}
	label, _, _ := strings.Cut(initial, ".")
	return strings.ToLower(label) + ".sharepoint.com"
}

// guidFrom makes a GUID from a text. The seed uses it: a site must be the same in every world of a seed, and must draw nothing from the random generator.
func guidFrom(text string) string {
	sum := sha256.Sum256([]byte(text))
	b := sum[:16]
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// driveIDFrom makes the ID of a drive from a text: b! and 64 characters, as SharePoint gives.
func driveIDFrom(text string) string {
	a := sha256.Sum256([]byte("drive:" + text))
	b := sha256.Sum256([]byte("drive2:" + text))
	return "b!" + base64.RawURLEncoding.EncodeToString(append(a[:], b[:16]...))
}

func (w *World) siteOf(id string) *Site { return w.sitesByID[strings.ToLower(id)] }

// RootSite gives the root site of the tenant.
func (w *World) RootSite() (Object, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, s := range w.Sites {
		if s.Parent == "" && s.Path == "" {
			return s.object(), true
		}
	}
	return nil, false
}

// GetSite gives a site by its ID (the host, and two GUIDs).
func (w *World) GetSite(id string) (Object, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if s := w.siteOf(id); s != nil {
		return s.object(), true
	}
	return nil, false
}

// SiteByPath gives a site by the host and the server relative path: contoso.sharepoint.com and /sites/intranet.
func (w *World) SiteByPath(host, path string) (Object, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	path = "/" + strings.Trim(path, "/")
	if path == "/" {
		path = ""
	}
	for _, s := range w.Sites {
		if strings.EqualFold(s.host(), host) && strings.EqualFold(s.Path, path) {
			return s.object(), true
		}
	}
	return nil, false
}

// GroupSite gives the team site of a Microsoft 365 group. The first bool is false when there is no such group, and the second when the group has no site.
func (w *World) GroupSite(groupID string) (o Object, groupFound, siteFound bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.groups[strings.ToLower(groupID)] == nil {
		return nil, false, false
	}
	for _, s := range w.Sites {
		if s.Kind == "group" && isID(s.Group, groupID) {
			return s.object(), true, true
		}
	}
	return nil, true, false
}

// SearchSites gives the sites that match a text, in the order of the data. The text is free: a site matches when it is in the name, the display name, the
// description, or the address. "*" and "" match every site.
func (w *World) SearchSites(text string) []Object {
	w.mu.RLock()
	defer w.mu.RUnlock()
	text = strings.ToLower(strings.Trim(strings.TrimSpace(text), `"`))
	var items []Object
	for _, s := range w.Sites {
		o := s.object()
		hay := strings.ToLower(s.Name + " " + s.DisplayName + " " + s.Description + " " + str(o["webUrl"]))
		if text == "" || text == "*" || strings.Contains(hay, text) {
			items = append(items, o)
		}
	}
	return items
}

// Subsites gives the sites below a site. The bool is false when there is no such site.
func (w *World) Subsites(id string) ([]Object, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	site := w.siteOf(id)
	if site == nil {
		return nil, false
	}
	var items []Object
	for _, s := range w.Sites {
		if isID(s.Parent, site.ID) {
			items = append(items, s.object())
		}
	}
	return items, true
}

// SiteDrives gives the drives of a site. The bool is false when there is no such site.
func (w *World) SiteDrives(id string) ([]Object, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	site := w.siteOf(id)
	if site == nil {
		return nil, false
	}
	items := []Object{}
	for _, d := range w.Drives {
		if isID(d.Site, site.ID) {
			items = append(items, d.object(w))
		}
	}
	return items, true
}

// SiteDrive gives one drive of a site. An empty driveID gives the first (the default drive: the library Documents). The first bool is false when there
// is no such site, and the second when the site has no such drive.
func (w *World) SiteDrive(siteID, driveID string) (o Object, siteFound, driveFound bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	site := w.siteOf(siteID)
	if site == nil {
		return nil, false, false
	}
	for _, d := range w.Drives {
		if isID(d.Site, site.ID) && (driveID == "" || isID(d.ID, driveID)) {
			return d.object(w), true, true
		}
	}
	return nil, true, false
}

// AdminSites gives every site for the portal: the object that Graph gives, and what Graph does not show (the kind, the group, the parent, and the drives).
func (w *World) AdminSites() []Object {
	w.mu.RLock()
	defer w.mu.RUnlock()
	items := make([]Object, 0, len(w.Sites))
	for _, s := range w.Sites {
		o := s.object()
		o["kind"], o["groupId"], o["parentId"], o["path"] = s.Kind, s.Group, s.Parent, s.Path
		if g := w.groups[strings.ToLower(s.Group)]; g != nil {
			o["groupName"] = g.Name
		}
		drives := []any{}
		for _, d := range w.Drives {
			if isID(d.Site, s.ID) {
				drives = append(drives, d.object(w))
			}
		}
		o["drives"] = drives
		items = append(items, o)
	}
	return items
}

// SiteGroups gives the Microsoft 365 groups that have no site yet (id and displayName): the groups that can get a team site in the portal.
func (w *World) SiteGroups() []Object {
	w.mu.RLock()
	defer w.mu.RUnlock()
	items := []Object{}
	for _, g := range w.Groups {
		if !g.Unified {
			continue
		}
		has := false
		for _, s := range w.Sites {
			has = has || s.Kind == "group" && isID(s.Group, g.ID)
		}
		if !has {
			items = append(items, Object{"id": g.ID, "displayName": g.Name})
		}
	}
	return items
}

// ---- changes (the portal makes them: Graph has no call to make a site) ----

// CreateSite makes a site. Props: kind ("sharepoint", or "group"), and for a sharepoint site name (the last part of the address: letters, digits,
// and dashes), displayName, description, and parentId (a subsite). A group site needs groupId, the ID of a Microsoft 365 group that has no site: its
// name is the mail nickname of the group. A new site has a drive, Documents. It gives the site as Graph shows it.
func (w *World) CreateSite(props map[string]any, a Actor) (Object, *apiError) {
	for k := range props {
		if !slices.Contains([]string{"kind", "name", "displayName", "description", "parentId", "groupId"}, k) {
			return nil, badRequest("Property '%s' cannot be set on a site.", k)
		}
	}
	kind := str(props["kind"])
	if kind == "" {
		kind = "sharepoint"
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	s := &Site{Kind: kind, Description: str(props["description"])}
	host := w.sharepointHost()
	web := newGUID()
	coll := newGUID()
	switch kind {
	case "group":
		g := w.groups[strings.ToLower(str(props["groupId"]))]
		switch {
		case str(props["groupId"]) == "":
			return nil, missing("groupId")
		case g == nil:
			return nil, notFound("group", str(props["groupId"]))
		case !g.Unified:
			return nil, badRequest("The group '%s' is not a Microsoft 365 group. Only a Microsoft 365 group has a team site.", g.Name)
		case str(props["parentId"]) != "":
			return nil, badRequest("A group site cannot be a subsite.")
		}
		for _, o := range w.Sites {
			if o.Kind == "group" && isID(o.Group, g.ID) {
				return nil, badRequest("The group '%s' has a site already.", g.Name)
			}
		}
		s.Group, s.Name = g.ID, strings.ToLower(firstNonEmpty(g.MailNickname, slug(g.Name)))
		s.DisplayName = firstNonEmpty(str(props["displayName"]), g.Name)
		s.Path = "/sites/" + s.Name
	case "sharepoint":
		name := str(props["name"])
		switch {
		case name == "":
			return nil, missing("name")
		case !siteName.MatchString(name):
			return nil, badRequest("The name '%s' is not valid. Use letters, digits, and dashes, in lowercase.", name)
		case str(props["groupId"]) != "":
			return nil, badRequest("A SharePoint site has no group. Use the kind 'group' for the site of a group.")
		}
		s.Name, s.DisplayName = name, firstNonEmpty(str(props["displayName"]), name)
		s.Path = "/sites/" + name
		if pid := str(props["parentId"]); pid != "" {
			parent := w.siteOf(pid)
			if parent == nil {
				return nil, badRequest("The parent site '%s' does not exist.", pid)
			}
			s.Parent, s.Path, coll = parent.ID, parent.Path+"/"+name, parent.collection()
		}
	default:
		return nil, badRequest("The kind '%s' is not known. Use 'sharepoint' or 'group'.", kind)
	}
	for _, o := range w.Sites {
		if strings.EqualFold(o.Path, s.Path) {
			return nil, badRequest("A site at %s exists already.", s.Path)
		}
	}
	s.ID = host + "," + coll + "," + web
	s.Created = w.now()
	s.Modified = s.Created
	docs := &Drive{ID: driveIDFrom(s.ID + "/Documents"), Name: "Documents", Site: s.ID, Created: s.Created, Modified: s.Created}
	w.addSite(s)
	w.addDrive(docs)
	sc := *s
	w.record(Event{Type: evSiteCreate, Actor: a, Site: &sc, Drives: []Drive{*docs}, SiteID: s.ID, SiteName: s.DisplayName})
	return s.object(), nil
}

// UpdateSite changes the display name and the description of a site. The name, the path, and the kind cannot change.
func (w *World) UpdateSite(id string, props map[string]any, a Actor) (Object, *apiError) {
	w.mu.Lock()
	defer w.mu.Unlock()
	s := w.siteOf(id)
	if s == nil {
		return nil, notFound("site", id)
	}
	set := map[string]any{}
	for k, v := range props {
		switch k {
		case "displayName":
			if strings.TrimSpace(str(v)) == "" {
				return nil, badRequest("Property 'displayName' cannot be empty.")
			}
			set[k] = str(v)
		case "description":
			set[k] = str(v)
		default:
			return nil, badRequest("Property '%s' cannot be changed.", k)
		}
	}
	if len(set) > 0 {
		set["modified"] = w.now().UTC().Format(time.RFC3339)
		w.setSite(s, set)
		w.record(Event{Type: evSiteUpdate, Actor: a, SiteID: s.ID, SiteName: s.DisplayName, Set: set})
	}
	return s.object(), nil
}

// DeleteSite deletes a site and its drives. The root site, and a site that has subsites, cannot be deleted.
func (w *World) DeleteSite(id string, a Actor) *apiError {
	w.mu.Lock()
	defer w.mu.Unlock()
	s := w.siteOf(id)
	if s == nil {
		return notFound("site", id)
	}
	if s.Parent == "" && s.Path == "" {
		return badRequest("The root site of the tenant cannot be deleted.")
	}
	for _, o := range w.Sites {
		if isID(o.Parent, s.ID) {
			return badRequest("The site has subsites. Delete them first.")
		}
	}
	name := s.DisplayName
	w.removeSite(s.ID)
	w.record(Event{Type: evSiteDelete, Actor: a, SiteID: s.ID, SiteName: name})
	return nil
}

// CreateDrive makes a drive on a site. Props: name (required, and not used by another drive of the site) and description.
func (w *World) CreateDrive(siteID string, props map[string]any, a Actor) (Object, *apiError) {
	for k := range props {
		if k != "name" && k != "description" {
			return nil, badRequest("Property '%s' cannot be set on a drive.", k)
		}
	}
	name := strings.TrimSpace(str(props["name"]))
	if name == "" {
		return nil, missing("name")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	s := w.siteOf(siteID)
	if s == nil {
		return nil, notFound("site", siteID)
	}
	if w.driveNamed(s.ID, name) != nil {
		return nil, badRequest("The site has a drive named '%s' already.", name)
	}
	now := w.now()
	d := &Drive{ID: driveIDFrom(newGUID()), Name: name, Description: str(props["description"]), Site: s.ID, Created: now, Modified: now}
	w.addDrive(d)
	dc := *d
	w.record(Event{Type: evDriveCreate, Actor: a, Drive: &dc, SiteID: s.ID, SiteName: s.DisplayName, DriveName: name})
	return d.object(w), nil
}

func (w *World) driveNamed(siteID, name string) *Drive {
	for _, d := range w.Drives {
		if isID(d.Site, siteID) && strings.EqualFold(d.Name, name) {
			return d
		}
	}
	return nil
}

// UpdateDrive changes the name and the description of a drive.
func (w *World) UpdateDrive(id string, props map[string]any, a Actor) (Object, *apiError) {
	w.mu.Lock()
	defer w.mu.Unlock()
	d := w.drivesByID[strings.ToLower(id)]
	if d == nil {
		return nil, notFound("drive", id)
	}
	set := map[string]any{}
	for k, v := range props {
		switch k {
		case "name":
			name := strings.TrimSpace(str(v))
			if name == "" {
				return nil, badRequest("Property 'name' cannot be empty.")
			}
			if o := w.driveNamed(d.Site, name); o != nil && o != d {
				return nil, badRequest("The site has a drive named '%s' already.", name)
			}
			set[k] = name
		case "description":
			set[k] = str(v)
		default:
			return nil, badRequest("Property '%s' cannot be changed.", k)
		}
	}
	if len(set) > 0 {
		set["modified"] = w.now().UTC().Format(time.RFC3339)
		w.setDrive(d, set)
		site := w.siteOf(d.Site)
		w.record(Event{Type: evDriveUpdate, Actor: a, DriveID: d.ID, DriveName: d.Name, SiteName: site.DisplayName, Set: set})
	}
	return d.object(w), nil
}

// DeleteDrive deletes a drive. A site keeps at least one.
func (w *World) DeleteDrive(id string, a Actor) *apiError {
	w.mu.Lock()
	defer w.mu.Unlock()
	d := w.drivesByID[strings.ToLower(id)]
	if d == nil {
		return notFound("drive", id)
	}
	n := 0
	for _, o := range w.Drives {
		if isID(o.Site, d.Site) {
			n++
		}
	}
	if n < 2 {
		return badRequest("A site keeps at least one drive.")
	}
	site := w.siteOf(d.Site)
	w.removeDrive(d.ID)
	w.record(Event{Type: evDriveDelete, Actor: a, DriveID: d.ID, DriveName: d.Name, SiteName: site.DisplayName})
	return nil
}

func (w *World) addSite(s *Site) {
	w.Sites = append(w.Sites, s)
	w.sitesByID[strings.ToLower(s.ID)] = s
}

func (w *World) addDrive(d *Drive) {
	w.Drives = append(w.Drives, d)
	w.drivesByID[strings.ToLower(d.ID)] = d
}

// removeSite takes a site out, with its drives.
func (w *World) removeSite(id string) {
	delete(w.sitesByID, strings.ToLower(id))
	w.Sites = slices.DeleteFunc(w.Sites, func(s *Site) bool { return isID(s.ID, id) })
	for _, d := range slices.Clone(w.Drives) {
		if isID(d.Site, id) {
			w.removeDrive(d.ID)
		}
	}
}

func (w *World) removeDrive(id string) {
	for _, it := range w.childrenOf(id, "") { // the items of the drive go with it
		w.removeItem(it.ID)
	}
	delete(w.drivesByID, strings.ToLower(id))
	w.Drives = slices.DeleteFunc(w.Drives, func(d *Drive) bool { return isID(d.ID, id) })
}

func (w *World) setSite(s *Site, set map[string]any) {
	if v, ok := set["displayName"]; ok {
		s.DisplayName = str(v)
	}
	if v, ok := set["description"]; ok {
		s.Description = str(v)
	}
	s.Modified = parseTime(str(set["modified"]), s.Modified)
}

func (w *World) setDrive(d *Drive, set map[string]any) {
	if v, ok := set["name"]; ok {
		d.Name = str(v)
	}
	if v, ok := set["description"]; ok {
		d.Description = str(v)
	}
	d.Modified = parseTime(str(set["modified"]), d.Modified)
}

func parseTime(text string, fallback time.Time) time.Time {
	if t, err := time.Parse(time.RFC3339, text); err == nil {
		return t
	}
	return fallback
}

// ---- the history of sites ----

func siteEventText(e Event) (string, bool) {
	switch e.Type {
	case evSiteCreate:
		return fmt.Sprintf("Made the site %s", firstNonEmpty(e.SiteName, e.SiteID)), true
	case evSiteUpdate:
		return fmt.Sprintf("Changed the site %s: %s", firstNonEmpty(e.SiteName, e.SiteID), setText(withoutModified(e.Set))), true
	case evSiteDelete:
		return fmt.Sprintf("Deleted the site %s", firstNonEmpty(e.SiteName, e.SiteID)), true
	case evDriveCreate:
		return fmt.Sprintf("Made the drive %s on the site %s", e.DriveName, e.SiteName), true
	case evDriveUpdate:
		return fmt.Sprintf("Changed the drive %s on the site %s: %s", e.DriveName, e.SiteName, setText(withoutModified(e.Set))), true
	case evDriveDelete:
		return fmt.Sprintf("Deleted the drive %s on the site %s", e.DriveName, e.SiteName), true
	}
	return "", false
}

func withoutModified(set map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range set {
		if k != "modified" {
			out[k] = v
		}
	}
	return out
}

func (w *World) applySiteEvent(e Event) bool {
	switch e.Type {
	case evSiteCreate:
		if e.Site != nil && w.siteOf(e.Site.ID) == nil {
			s := *e.Site
			w.addSite(&s)
			for _, d := range e.Drives {
				dc := d
				w.addDrive(&dc)
			}
		}
	case evSiteUpdate:
		if s := w.siteOf(e.SiteID); s != nil {
			w.setSite(s, e.Set)
		}
	case evSiteDelete:
		w.removeSite(e.SiteID)
	case evDriveCreate:
		if e.Drive != nil && w.drivesByID[strings.ToLower(e.Drive.ID)] == nil {
			d := *e.Drive
			w.addDrive(&d)
		}
	case evDriveUpdate:
		if d := w.drivesByID[strings.ToLower(e.DriveID)]; d != nil {
			w.setDrive(d, e.Set)
		}
	case evDriveDelete:
		w.removeDrive(e.DriveID)
	default:
		return false
	}
	return true
}

// ---- the seed ----

// sites makes the sites of a new world: the root site of the tenant, a team site for each Microsoft 365 group, and some sites that have no group (and a
// subsite). Nothing here draws from the random generator: the sites are made from the names, so the data of a seed that had no sites stays as it was.
func (g *gen) sites() {
	w := g.w
	w.indexSites()
	host := w.sharepointHost()
	base := time.Date(2024, 3, 4, 8, 0, 0, 0, time.UTC)
	n := 0
	add := func(kind, name, display, description, path, group string, parent *Site) *Site {
		coll := guidFrom("site:" + host + path)
		if parent != nil {
			coll = parent.collection()
		}
		s := &Site{ID: host + "," + coll + "," + guidFrom("web:"+host+path), Name: name, DisplayName: display, Description: description, Kind: kind, Group: group, Path: path}
		if parent != nil {
			s.Parent = parent.ID
		}
		s.Created = base.Add(time.Duration(n) * 24 * time.Hour)
		s.Modified = s.Created
		n++
		w.addSite(s)
		w.addDrive(&Drive{ID: driveIDFrom(s.ID + "/Documents"), Name: "Documents", Site: s.ID, Created: s.Created, Modified: s.Created})
		return s
	}
	add("sharepoint", "", g.p.Organization.DisplayName, "The home site of the tenant.", "", "", nil)
	for _, grp := range w.Groups {
		if grp.Unified {
			nick := strings.ToLower(firstNonEmpty(grp.MailNickname, slug(grp.Name)))
			add("group", nick, grp.Name, grp.Description, "/sites/"+nick, grp.ID, nil)
		}
	}
	add("sharepoint", "intranet", "Intranet", "News and links for everyone.", "/sites/intranet", "", nil)
	hr := add("sharepoint", "hr-policies", "HR policies", "Policies and forms of the HR department.", "/sites/hr-policies", "", nil)
	add("sharepoint", "onboarding", "Onboarding", "What a new colleague needs in the first weeks.", "/sites/hr-policies/onboarding", "", hr)
}
