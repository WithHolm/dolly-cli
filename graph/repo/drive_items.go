package repo

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"github.com/withholm/dolly-cli/graph/api"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// The folders and files on a drive. The data side of api/drives.go. It satisfies api.DrivesBackend.
//
// An item is a folder or a file. The root folder of a drive is not stored: it is made from the drive (rootID). The same items answer for every way to
// reach a drive: /drives/{id}, /users/{id}/drive (and /me/drive), /groups/{id}/drive, and /sites/{id}/drive. The drive of a user is not stored
// either, until the user has an item on it: it is made from the user (userDriveID).
//
// The content of a file is kept by its hash, in the folder content/ of the data folder (or in memory when dollycli has no folder). A file with no hash has
// no content of its own: it gives the same text again and again, as long as its size (filler). The seed and the portal make such files.
//
// The rules are from the Learn pages "Get a DriveItem", "List children", "Create a new folder", "Upload or replace the contents of a DriveItem", "Download
// the contents", "Update a DriveItem", and "Delete a DriveItem". Where a page does not say, the code says "guess" (see graph/docs/drives.md).

// MaxUpload is the size of the largest file that a call can put: the size that the Learn page gives for a simple upload.
const MaxUpload = api.MaxUpload

// Events that the items make.
const (
	evItemCreate = "item.create" // a new item (Item has it)
	evItemChange = "item.change" // an item after a rename, a move, or new content (Item has the whole item)
	evItemDelete = "item.delete" // an item is gone, and the items below it
)

// Item is a folder or a file as dollycli keeps it.
type Item struct {
	ID     string `json:"id"`
	Drive  string `json:"drive"`
	Parent string `json:"parent,omitempty"` // the ID of the folder that holds it. Empty: the root of the drive.
	Name   string `json:"name"`
	Folder bool   `json:"folder,omitempty"`
	Size   int64  `json:"size,omitempty"` // of a file
	// Hash is the SHA-256 of the content of a file, in hex. Empty means filler (see above).
	Hash string `json:"hash,omitempty"`
	// Version counts the changes of the item, and CVersion the changes of its content: they make the eTag and the cTag.
	Version  int       `json:"version"`
	CVersion int       `json:"cVersion"`
	Created  time.Time `json:"created"`
	Modified time.Time `json:"modified"`
}

func itemIDFrom(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "01" + base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:20])
}

func newItemID() string {
	var b [20]byte
	_, _ = rand.Read(b[:])
	return "01" + base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])
}

func rootID(drive string) string     { return itemIDFrom("root:" + drive) }
func userDriveID(user string) string { return driveIDFrom("user:" + strings.ToLower(user)) }
func (it *Item) isFile() bool        { return !it.Folder }
func (it *Item) cTag() string {
	return fmt.Sprintf("\"c:{%s},%d\"", strings.ToUpper(guidFrom("c"+it.ID)), it.CVersion)
}
func (it *Item) eTag() string {
	return fmt.Sprintf("\"{%s},%d\"", strings.ToUpper(guidFrom("e"+it.ID)), it.Version)
}
func (it *Item) mimeType() string       { return mimeOf(it.Name) }
func (w *World) itemOf(id string) *Item { return w.itemsByID[strings.ToLower(id)] }

func mimeOf(name string) string {
	if t := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); t != "" {
		return t
	}
	return "application/octet-stream"
}

func (w *World) indexItems() {
	w.itemsByID = make(map[string]*Item, len(w.Items))
	for _, it := range w.Items {
		w.itemsByID[strings.ToLower(it.ID)] = it
	}
}

// ---- the drives ----

// driveByID finds a stored drive, or the drive of a user (made from the user). Call it with the lock held.
func (w *World) driveByID(id string) *Drive {
	if d := w.drivesByID[strings.ToLower(id)]; d != nil {
		return d
	}
	for _, u := range w.Users {
		if uid := str(u["id"]); strings.EqualFold(userDriveID(uid), id) {
			return &Drive{ID: userDriveID(uid), Name: "OneDrive", User: uid, Created: driveBase, Modified: driveBase}
		}
	}
	return nil
}

func (w *World) driveLabel(d *Drive) string {
	if d.User != "" {
		return "OneDrive of " + str(w.users[strings.ToLower(d.User)]["displayName"])
	}
	if s := w.siteOf(d.Site); s != nil {
		return d.Name + " on " + s.DisplayName
	}
	return d.Name
}

// driveWebURL is the address of a drive: the site and the name of the library, or the personal site of the user.
func (w *World) driveWebURL(d *Drive) string {
	if d.User != "" {
		upn := strings.NewReplacer("@", "_", ".", "_").Replace(str(w.users[strings.ToLower(d.User)]["userPrincipalName"]))
		host := strings.TrimSuffix(w.sharepointHost(), ".sharepoint.com") + "-my.sharepoint.com"
		return "https://" + host + "/personal/" + upn + "/Documents"
	}
	if s := w.siteOf(d.Site); s != nil {
		return "https://" + s.host() + s.Path + "/" + strings.ReplaceAll(d.Name, " ", "%20")
	}
	return ""
}

func itemNotFound() *apiError {
	return &apiError{Status: 404, Code: "itemNotFound", Message: "Item not found"}
}

// ResolveDrive gives the ID of a drive that a call names. kind is "user" (an ID or a user principal name), "group" (the drive of the team site), "site" (an ID
// or "root": the first drive), or "drive" (the ID of the drive).
func (w *World) ResolveDrive(kind, ref string) (string, *apiError) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	switch kind {
	case "user":
		u := w.userByRef(ref)
		if u == nil {
			// the real Graph answers so for a user that is not there (checked with the proxy of Graph Explorer)
			return "", &apiError{Status: 404, Code: "ResourceNotFound", Message: "User not found"}
		}
		return userDriveID(str(u["id"])), nil
	case "group":
		if w.groups[strings.ToLower(ref)] == nil {
			return "", notFound("group", ref)
		}
		for _, s := range w.Sites {
			if s.Kind == "group" && isID(s.Group, ref) {
				return w.defaultDrive(s), nil
			}
		}
		return "", itemNotFound()
	case "site":
		var s *Site
		if strings.EqualFold(ref, "root") {
			for _, o := range w.Sites {
				if o.Parent == "" && o.Path == "" {
					s = o
				}
			}
		} else {
			s = w.siteOf(ref)
		}
		if s == nil {
			return "", itemNotFound()
		}
		return w.defaultDrive(s), nil
	}
	if !driveIDForm.MatchString(ref) {
		// the real Graph answers so for b!nothing (checked with the proxy of Graph Explorer). What is a valid form is a guess.
		return "", &apiError{Status: 400, Code: "invalidRequest", Message: "The provided drive id appears to be malformed, or does not represent a valid drive."}
	}
	if w.driveByID(ref) == nil {
		return "", itemNotFound()
	}
	return ref, nil
}

func (w *World) defaultDrive(s *Site) string {
	for _, d := range w.Drives {
		if isID(d.Site, s.ID) {
			return d.ID
		}
	}
	return ""
}

// GetDrive gives a drive by its ID.
func (w *World) GetDrive(id string) (Object, *apiError) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	d := w.driveByID(id)
	if d == nil {
		return nil, itemNotFound()
	}
	return d.object(w), nil
}

// ---- the lookups ----

func (w *World) childrenOf(drive, parent string) []*Item {
	var items []*Item
	for _, it := range w.Items {
		if isID(it.Drive, drive) && isID(it.Parent, parent) {
			items = append(items, it)
		}
	}
	return items
}

func (w *World) child(drive, parent, name string) *Item {
	for _, it := range w.childrenOf(drive, parent) {
		if strings.EqualFold(it.Name, name) {
			return it
		}
	}
	return nil
}

// resolve finds the item that a ref names: an item ID, "root", or a path from the root that starts with a slash (/Reports/2026). The item is nil for the root.
func (w *World) resolve(drive, ref string) (it *Item, e *apiError) {
	switch {
	case strings.EqualFold(ref, "root") || isID(ref, rootID(drive)) || ref == "/":
		return nil, nil
	case strings.HasPrefix(ref, "/"):
		parent := ""
		var found *Item
		for _, name := range strings.Split(strings.Trim(ref, "/"), "/") {
			if found = w.child(drive, parent, name); found == nil {
				return nil, itemNotFound()
			}
			parent = found.ID
		}
		return found, nil
	}
	if found := w.itemOf(ref); found != nil && isID(found.Drive, drive) {
		return found, nil
	}
	return nil, itemNotFound()
}

// pathOf gives the path of an item from the root: /Reports/2026.
func (w *World) pathOf(it *Item) string {
	var parts []string
	for it != nil {
		parts = append(parts, it.Name)
		it = w.itemOf(it.Parent)
	}
	slices.Reverse(parts)
	return "/" + strings.Join(parts, "/")
}

func (w *World) sizeOf(it *Item) int64 {
	if !it.Folder {
		return it.Size
	}
	return w.sizeIn(it.Drive, it.ID)
}

func (w *World) sizeIn(drive, parent string) int64 {
	var n int64
	for _, c := range w.childrenOf(drive, parent) {
		n += w.sizeOf(c)
	}
	return n
}

func (w *World) identity(d *Drive) Object {
	if d.User != "" {
		u := w.users[strings.ToLower(d.User)]
		return Object{"user": Object{"id": d.User, "displayName": str(u["displayName"]), "email": str(u["userPrincipalName"])}}
	}
	return Object{"user": Object{"displayName": "System Account"}}
}

func (w *World) parentRef(d *Drive, it *Item) Object {
	ref := Object{"driveId": d.ID, "driveType": driveType(d)}
	if it == nil {
		return ref
	}
	if it.Parent == "" {
		ref["id"], ref["path"] = rootID(d.ID), "/drives/"+d.ID+"/root:"
		return ref
	}
	parent := w.itemOf(it.Parent)
	ref["id"], ref["name"] = parent.ID, parent.Name
	ref["path"] = "/drives/" + d.ID + "/root:" + w.pathOf(parent)
	return ref
}

func driveType(d *Drive) string {
	if d.User != "" {
		return "business"
	}
	return "documentLibrary"
}

func (w *World) rootObject(d *Drive) Object {
	count := len(w.childrenOf(d.ID, ""))
	return Object{
		"id": rootID(d.ID), "name": "root", "size": float64(w.sizeIn(d.ID, "")), "webUrl": w.driveWebURL(d),
		"createdDateTime": d.Created.UTC().Format(time.RFC3339), "lastModifiedDateTime": d.Modified.UTC().Format(time.RFC3339),
		"createdBy": w.identity(d), "lastModifiedBy": w.identity(d), "parentReference": Object{"driveId": d.ID, "driveType": driveType(d)},
		"folder": Object{"childCount": float64(count)}, "root": Object{},
	}
}

func (w *World) itemObject(d *Drive, it *Item) Object {
	o := Object{
		"id": it.ID, "name": it.Name, "size": float64(w.sizeOf(it)), "eTag": it.eTag(), "cTag": it.cTag(),
		"webUrl":          w.driveWebURL(d) + escapePath(w.pathOf(it)),
		"createdDateTime": it.Created.UTC().Format(time.RFC3339), "lastModifiedDateTime": it.Modified.UTC().Format(time.RFC3339),
		"createdBy": w.identity(d), "lastModifiedBy": w.identity(d), "parentReference": w.parentRef(d, it),
		"fileSystemInfo": Object{"createdDateTime": it.Created.UTC().Format(time.RFC3339), "lastModifiedDateTime": it.Modified.UTC().Format(time.RFC3339)},
	}
	if it.Folder {
		o["folder"] = Object{"childCount": float64(len(w.childrenOf(d.ID, it.ID)))}
	} else {
		o["file"] = Object{"mimeType": it.mimeType()}
		// the address of the download, without the host: the call adds it (api/drives.go). The cTag is in it, so an address of older content stops working.
		o["@microsoft.graph.downloadUrl"] = api.DownloadPrefix + d.ID + "/" + it.ID + "/" + guidFrom(it.cTag()) + "/" + url.PathEscape(it.Name)
	}
	return o
}

func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = strings.ReplaceAll(url.PathEscape(s), "+", "%2B")
	}
	return strings.Join(parts, "/")
}

// object gives the object of what a ref names (the root, or an item) on a drive.
func (w *World) object(drive, ref string) (Object, *apiError) {
	d := w.driveByID(drive)
	if d == nil {
		return nil, itemNotFound()
	}
	it, e := w.resolve(drive, ref)
	if e != nil {
		return nil, e
	}
	if it == nil {
		return w.rootObject(d), nil
	}
	return w.itemObject(d, it), nil
}

// GetItem gives an item. The ref is an item ID, "root", or a path that starts with a slash.
func (w *World) GetItem(drive, ref string) (Object, *apiError) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.object(drive, ref)
}

// Children gives the items in a folder: the folders first is not a rule of Graph, so they are in the order of the data.
func (w *World) Children(drive, ref string) ([]Object, *apiError) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	d := w.driveByID(drive)
	if d == nil {
		return nil, itemNotFound()
	}
	it, e := w.resolve(drive, ref)
	if e != nil {
		return nil, e
	}
	parent := ""
	if it != nil {
		if !it.Folder {
			// guess: the real Graph answers a file here with an error of the same kind as a call that does not fit the item
			return nil, badRequest("The item is a file. Only a folder has children.")
		}
		parent = it.ID
	}
	items := []Object{}
	for _, c := range w.childrenOf(drive, parent) {
		items = append(items, w.itemObject(d, c))
	}
	return items, nil
}

// ---- the content ----

func (w *World) blobPath(hash string) string { return filepath.Join(w.j.dir, "content", hash) }

// storeBlob keeps the content of a file and gives its hash. A call with the lock of the world held can use it.
func (w *World) storeBlob(data []byte) (string, error) {
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	if w.j == nil {
		if w.blobs == nil {
			w.blobs = map[string][]byte{}
		}
		w.blobs[hash] = slices.Clone(data)
		return hash, nil
	}
	if _, err := os.Stat(w.blobPath(hash)); err == nil {
		return hash, nil
	}
	if err := os.MkdirAll(filepath.Join(w.j.dir, "content"), 0o755); err != nil {
		return "", err
	}
	return hash, os.WriteFile(w.blobPath(hash), data, 0o644)
}

func (w *World) loadBlob(hash string) ([]byte, bool) {
	if w.j == nil {
		data, ok := w.blobs[hash]
		return data, ok
	}
	data, err := os.ReadFile(w.blobPath(hash))
	return data, err == nil
}

// content gives the bytes of a file.
func (w *World) content(it *Item) []byte {
	if it.Hash != "" {
		if data, ok := w.loadBlob(it.Hash); ok {
			return data
		}
	}
	return filler(it.Name, it.Size)
}

// filler is the content of a file that has none of its own: a sentence, again and again, as long as the size.
func filler(name string, size int64) []byte {
	if size <= 0 {
		return []byte{}
	}
	line := []byte("dollycli sample content for " + name + "\n")
	out := make([]byte, 0, size)
	for int64(len(out)) < size {
		out = append(out, line...)
	}
	return out[:size]
}

// Content gives the item and the bytes of a file. A folder has no content.
func (w *World) Content(drive, ref string) (Object, []byte, *apiError) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	d := w.driveByID(drive)
	if d == nil {
		return nil, nil, itemNotFound()
	}
	it, e := w.resolve(drive, ref)
	switch {
	case e != nil:
		return nil, nil, e
	case it == nil || it.Folder:
		return nil, nil, badRequest("The item is a folder. Only a file has content.") // guess: the text is ours
	}
	return w.itemObject(d, it), w.content(it), nil
}

// DownloadOf gives the file and the bytes for a download address (the address of @microsoft.graph.downloadUrl). The cTag in the address must be the one of
// the file now: an address of an older content does not work.
func (w *World) DownloadOf(drive, id, ctag string) (name string, data []byte, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	it := w.itemOf(id)
	if it == nil || it.Folder || !isID(it.Drive, drive) || guidFrom(it.cTag()) != ctag {
		return "", nil, false
	}
	return it.Name, w.content(it), true
}

// ---- the changes ----

func validName(name string) *apiError {
	switch {
	case strings.TrimSpace(name) == "":
		return missing("name")
	case len(name) > 255 || name != strings.TrimSpace(name) || strings.HasSuffix(name, ".") || strings.ContainsAny(name, "\"*:<>?/\\|") || strings.ContainsFunc(name, func(r rune) bool { return r < 32 }):
		// guess: the real Graph refuses these names too (they are the characters that SharePoint does not allow), and its words are not known
		return &apiError{Status: 400, Code: "invalidRequest", Message: fmt.Sprintf("The name '%s' is not valid. A name has no \" * : < > ? / \\ | and no space or dot at the end.", name)}
	}
	return nil
}

func nameExists() *apiError {
	return &apiError{Status: 409, Code: "nameAlreadyExists", Message: "The specified item name already exists."}
}

// freeName gives a name that no item of the folder has: "Report 1.docx" for "Report.docx".
func (w *World) freeName(drive, parent, name string) string {
	ext := path.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		if try := fmt.Sprintf("%s %d%s", stem, i, ext); w.child(drive, parent, try) == nil {
			return try
		}
	}
}

// spot decides where a new name goes in a folder. behavior is "fail", "rename", or "replace" (the value of @microsoft.graph.conflictBehavior). It gives the name to
// use, and the item that has the name now when the call replaces it.
func (w *World) spot(drive, parent, name, behavior string, self *Item) (string, *Item, *apiError) {
	other := w.child(drive, parent, name)
	if other == nil || other == self {
		return name, nil, nil
	}
	switch behavior {
	case "replace":
		return name, other, nil
	case "rename":
		return w.freeName(drive, parent, name), nil, nil
	}
	return "", nil, nameExists()
}

// ensureFolders makes the folders of a path that are not there, and gives the ID of the last ("" is the root). The lock is held.
func (w *World) ensureFolders(d *Drive, folderPath string, a Actor) (string, *apiError) {
	parent := ""
	for _, name := range strings.Split(strings.Trim(folderPath, "/"), "/") {
		if name == "" {
			continue
		}
		if e := validName(name); e != nil {
			return "", e
		}
		if f := w.child(d.ID, parent, name); f != nil {
			if !f.Folder {
				return "", nameExists()
			}
			parent = f.ID
			continue
		}
		f := w.newItem(d, parent, name, true)
		w.addItem(f)
		w.recordItem(evItemCreate, d, f, a)
		parent = f.ID
	}
	return parent, nil
}

func (w *World) newItem(d *Drive, parent, name string, folder bool) *Item {
	now := w.now()
	return &Item{ID: newItemID(), Drive: d.ID, Parent: parent, Name: name, Folder: folder, Created: now, Modified: now}
}

func (w *World) addItem(it *Item) {
	w.Items = append(w.Items, it)
	w.itemsByID[strings.ToLower(it.ID)] = it
}

func (w *World) recordItem(typ string, d *Drive, it *Item, a Actor) {
	c := *it
	w.record(Event{Type: typ, Actor: a, Item: &c, ItemID: it.ID, ItemName: it.Name, DriveID: d.ID, DriveName: w.driveLabel(d)})
}

// parentOf finds the folder that a ref names, for a new item. A ref that is a path makes the folders that are not there (as a call that puts a file in a
// path does). The ID is "" for the root.
func (w *World) parentOf(d *Drive, ref string, a Actor) (string, *apiError) {
	if strings.HasPrefix(ref, "/") && ref != "/" {
		return w.ensureFolders(d, ref, a)
	}
	p, e := w.resolve(d.ID, ref)
	switch {
	case e != nil:
		return "", e
	case p == nil:
		return "", nil
	case !p.Folder:
		return "", badRequest("The item is a file. Only a folder can hold items.")
	}
	return p.ID, nil
}

// CreateChild makes a folder in a folder (props: name, folder: {}, and @microsoft.graph.conflictBehavior), or an empty file (props: name, file: {}).
func (w *World) CreateChild(drive, parentRef string, props map[string]any, a Actor) (Object, *apiError) {
	name := str(props["name"])
	if e := validName(name); e != nil {
		return nil, e
	}
	_, isFolder := props["folder"]
	_, isFile := props["file"]
	if isFolder == isFile {
		return nil, badRequest("Give the type of the new item: folder, or file.") // guess: the text is ours
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	d := w.driveByID(drive)
	if d == nil {
		return nil, itemNotFound()
	}
	parent, e := w.parentOf(d, parentRef, a)
	if e != nil {
		return nil, e
	}
	name, replace, e := w.spot(d.ID, parent, name, str(props["@microsoft.graph.conflictBehavior"]), nil)
	if e != nil {
		return nil, e
	}
	if replace != nil {
		w.removeTree(d, replace, a)
	}
	it := w.newItem(d, parent, name, isFolder)
	w.addItem(it)
	w.recordItem(evItemCreate, d, it, a)
	return w.itemObject(d, it), nil
}

// PutFile puts the content of a file in a folder, by name: it makes the file, or replaces the content of the file that has the name (behavior "replace", the
// default of an upload), or renames the new one ("rename"), or fails ("fail"). The ref of the folder can be a path: the folders that are not there are made.
// The bool says that the file is new.
func (w *World) PutFile(drive, parentRef, name string, data []byte, behavior string, a Actor) (Object, bool, *apiError) {
	if e := validName(name); e != nil {
		return nil, false, e
	}
	if len(data) > MaxUpload {
		return nil, false, tooLarge()
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	d := w.driveByID(drive)
	if d == nil {
		return nil, false, itemNotFound()
	}
	parent, e := w.parentOf(d, parentRef, a)
	if e != nil {
		return nil, false, e
	}
	if behavior == "" {
		behavior = "replace"
	}
	name, replace, e := w.spot(d.ID, parent, name, behavior, nil)
	if e != nil {
		return nil, false, e
	}
	if replace != nil && replace.Folder {
		return nil, false, nameExists()
	}
	if replace != nil {
		return w.setContent(d, replace, data, a)
	}
	it := w.newItem(d, parent, name, false)
	w.addItem(it)
	if _, _, e := w.setContent(d, it, data, a); e != nil {
		w.removeItem(it.ID)
		return nil, false, e
	}
	return w.itemObject(d, it), true, nil
}

// PutContent replaces the content of a file that is there.
func (w *World) PutContent(drive, ref string, data []byte, a Actor) (Object, *apiError) {
	if len(data) > MaxUpload {
		return nil, tooLarge()
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	d := w.driveByID(drive)
	if d == nil {
		return nil, itemNotFound()
	}
	it, e := w.resolve(drive, ref)
	switch {
	case e != nil:
		return nil, e
	case it == nil || it.Folder:
		return nil, badRequest("The item is a folder. Only a file has content.")
	}
	o, _, e := w.setContent(d, it, data, a)
	return o, e
}

func tooLarge() *apiError {
	// guess: the real Graph says so for a simple upload that is too large. The limit is the one of the Learn page (4 MB).
	return &apiError{Status: 413, Code: "requestEntityTooLarge", Message: fmt.Sprintf("A call that puts content takes a file of up to %d MB. Use an upload session for a larger file (dollycli has none).", MaxUpload>>20)}
}

// setContent stores the content of a file and records it. An item that was just made has no event yet: it gets the create event.
func (w *World) setContent(d *Drive, it *Item, data []byte, a Actor) (Object, bool, *apiError) {
	created := it.Version == 0
	hash := ""
	if len(data) > 0 {
		var err error
		if hash, err = w.storeBlob(data); err != nil {
			return nil, false, &apiError{Status: 500, Code: "generalException", Message: "dollycli cannot keep the content: " + err.Error()}
		}
	}
	it.Hash, it.Size = hash, int64(len(data))
	it.Version++
	it.CVersion++
	it.Modified = w.now()
	if created {
		w.recordItem(evItemCreate, d, it, a)
	} else {
		w.recordItem(evItemChange, d, it, a)
	}
	return w.itemObject(d, it), created, nil
}

// UpdateItem renames an item (name) or moves it (parentReference: {id}), or both. A name that is taken fails, as a new item does (the default of a move).
func (w *World) UpdateItem(drive, ref string, props map[string]any, a Actor) (Object, *apiError) {
	w.mu.Lock()
	defer w.mu.Unlock()
	d := w.driveByID(drive)
	if d == nil {
		return nil, itemNotFound()
	}
	it, e := w.resolve(drive, ref)
	switch {
	case e != nil:
		return nil, e
	case it == nil:
		return nil, badRequest("The root folder of a drive cannot be changed.") // guess: the text is ours
	}
	name, parent := it.Name, it.Parent
	for k, v := range props {
		switch k {
		case "name":
			name = str(v)
			if e := validName(name); e != nil {
				return nil, e
			}
		case "parentReference":
			ref, _ := v.(map[string]any)
			if ref == nil {
				return nil, badRequest("The property 'parentReference' must be an object with the id of the new folder.")
			}
			id := str(ref["id"])
			if id == "" {
				return nil, badRequest("The property 'parentReference' needs the id of the new folder.")
			}
			if d := str(ref["driveId"]); d != "" && !isID(d, drive) {
				return nil, badRequest("An item cannot move to another drive.") // guess: the text is ours
			}
			p, e := w.resolve(drive, id)
			switch {
			case e != nil:
				return nil, e
			case p != nil && !p.Folder:
				return nil, badRequest("The item is a file. Only a folder can hold items.")
			}
			parent = ""
			if p != nil {
				parent = p.ID
			}
			for up := w.itemOf(parent); up != nil; up = w.itemOf(up.Parent) {
				if up == it {
					return nil, badRequest("A folder cannot move into itself.")
				}
			}
		default:
			return nil, badRequest("Property '%s' cannot be changed.", k)
		}
	}
	if name == it.Name && parent == it.Parent {
		return w.itemObject(d, it), nil
	}
	if _, _, e := w.spot(d.ID, parent, name, "fail", it); e != nil {
		return nil, e
	}
	it.Name, it.Parent = name, parent
	it.Version++
	it.Modified = w.now()
	w.recordItem(evItemChange, d, it, a)
	return w.itemObject(d, it), nil
}

// DeleteItem deletes a file, or a folder and what is in it. The root folder cannot be deleted.
func (w *World) DeleteItem(drive, ref string, a Actor) *apiError {
	w.mu.Lock()
	defer w.mu.Unlock()
	d := w.driveByID(drive)
	if d == nil {
		return itemNotFound()
	}
	it, e := w.resolve(drive, ref)
	switch {
	case e != nil:
		return e
	case it == nil:
		return badRequest("The root folder of a drive cannot be deleted.") // guess: the text is ours
	}
	w.removeTree(d, it, a)
	return nil
}

// removeTree deletes an item and the items below it, and records it.
func (w *World) removeTree(d *Drive, it *Item, a Actor) {
	c := *it
	w.removeItem(it.ID)
	w.record(Event{Type: evItemDelete, Actor: a, ItemID: c.ID, ItemName: c.Name, DriveID: d.ID, DriveName: w.driveLabel(d)})
}

// removeItem takes an item out of the data, and the items below it.
func (w *World) removeItem(id string) {
	it := w.itemOf(id)
	if it == nil {
		return
	}
	for _, c := range w.childrenOf(it.Drive, it.ID) {
		w.removeItem(c.ID)
	}
	delete(w.itemsByID, strings.ToLower(id))
	w.Items = slices.DeleteFunc(w.Items, func(x *Item) bool { return isID(x.ID, id) })
}

// ---- for the portal ----

// DriveItems gives the items of a drive as a flat list for the portal (the portal builds the tree): id, parentId ("" for the root), name, folder, size, and
// mimeType.
func (w *World) DriveItems(drive string) ([]Object, *apiError) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.driveByID(drive) == nil {
		return nil, itemNotFound()
	}
	items := []Object{}
	var walk func(parent string)
	walk = func(parent string) {
		for _, it := range w.childrenOf(drive, parent) {
			o := Object{"id": it.ID, "parentId": it.Parent, "name": it.Name, "folder": it.Folder, "size": float64(w.sizeOf(it)), "modified": it.Modified.UTC().Format(time.RFC3339)}
			if !it.Folder {
				o["mimeType"] = it.mimeType()
			}
			items = append(items, o)
			if it.Folder {
				walk(it.ID)
			}
		}
	}
	walk("")
	return items, nil
}

// AddFile makes a file for the portal: with the text as its content, or (with no text) filler of the size.
func (w *World) AddFile(drive, parentRef, name, text string, size int64, a Actor) (Object, *apiError) {
	if text != "" {
		o, _, e := w.PutFile(drive, parentRef, name, []byte(text), "fail", a)
		return o, e
	}
	if size < 0 || size > MaxUpload {
		return nil, badRequest("The size must be from 0 to %d bytes.", MaxUpload)
	}
	if e := validName(name); e != nil {
		return nil, e
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	d := w.driveByID(drive)
	if d == nil {
		return nil, itemNotFound()
	}
	parent, e := w.parentOf(d, parentRef, a)
	if e != nil {
		return nil, e
	}
	if _, _, e := w.spot(d.ID, parent, name, "fail", nil); e != nil {
		return nil, e
	}
	it := w.newItem(d, parent, name, false)
	it.Size, it.Version, it.CVersion = size, 1, 1
	w.addItem(it)
	w.recordItem(evItemCreate, d, it, a)
	return w.itemObject(d, it), nil
}

// ---- the history of items ----

func itemEventText(e Event) (string, bool) {
	switch e.Type {
	case evItemCreate:
		kind := "file"
		if e.Item != nil && e.Item.Folder {
			kind = "folder"
		}
		return fmt.Sprintf("Made the %s %s on %s", kind, e.ItemName, e.DriveName), true
	case evItemChange:
		return fmt.Sprintf("Changed the item %s on %s", e.ItemName, e.DriveName), true
	case evItemDelete:
		return fmt.Sprintf("Deleted the item %s on %s", e.ItemName, e.DriveName), true
	}
	return "", false
}

func (w *World) applyItemEvent(e Event) bool {
	switch e.Type {
	case evItemCreate, evItemChange:
		if e.Item == nil {
			return true
		}
		c := *e.Item
		if old := w.itemOf(c.ID); old != nil {
			*old = c
		} else {
			w.addItem(&c)
		}
	case evItemDelete:
		w.removeItem(e.ItemID)
	default:
		return false
	}
	return true
}

// ---- the seed ----

// driveBase is the time of the seed: the sites, the drives, and the items of a new world are made at it.
var driveBase = time.Date(2024, 3, 4, 8, 0, 0, 0, time.UTC)

// driveItems makes the folders and files of a new world: some on the first drive of each site, and some on the drive of the owner. Nothing here draws from the
// random generator. The files have no content of their own (filler).
func (g *gen) driveItems() {
	w := g.w
	type file struct {
		path string
		size int64
	}
	put := func(d *Drive, files ...file) {
		made := map[string]string{} // the folders made so far, by their path: "Documents/Projects"
		for i, f := range files {
			dir, name := path.Split(f.path)
			parent, soFar := "", ""
			for _, part := range strings.FieldsFunc(dir, func(r rune) bool { return r == '/' }) {
				soFar += part + "/"
				id, ok := made[soFar]
				if !ok {
					id = itemIDFrom("item:" + d.ID + "/" + soFar)
					w.addItem(&Item{ID: id, Drive: d.ID, Parent: parent, Name: part, Folder: true, Version: 1, CVersion: 1, Created: driveBase, Modified: driveBase})
					made[soFar] = id
				}
				parent = id
			}
			t := driveBase.Add(time.Duration(i) * time.Hour)
			it := &Item{ID: itemIDFrom("item:" + d.ID + "/" + f.path), Drive: d.ID, Parent: parent, Name: name, Size: f.size, Version: 1, CVersion: 1, Created: t, Modified: t}
			w.addItem(it)
		}
	}
	w.indexSites()
	for _, s := range w.Sites {
		d := w.driveByID(w.defaultDrive(s))
		if d == nil {
			continue
		}
		put(d, file{"Welcome.docx", 18_432}, file{"General/Readme.txt", 612}, file{"General/Plan 2026.xlsx", 52_224}, file{"Archive/Minutes 2025.pdf", 245_760})
	}
	for _, u := range w.Users {
		if str(u["userPrincipalName"]) == w.ownerUPN() {
			d := &Drive{ID: userDriveID(str(u["id"])), Name: "OneDrive", User: str(u["id"]), Created: driveBase, Modified: driveBase}
			put(d, file{"Notes.txt", 1_204}, file{"Documents/CV.docx", 36_864}, file{"Documents/Projects/Ideas.md", 2_048}, file{"Pictures/Team photo.jpg", 1_572_864})
		}
	}
}

// ownerUPN is the user principal name of the owner of this world.
func (w *World) ownerUPN() string {
	for _, u := range w.Users {
		if str(u["displayName"]) == OwnerName {
			return str(u["userPrincipalName"])
		}
	}
	return ""
}
