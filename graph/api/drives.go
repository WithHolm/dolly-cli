package api

import (
	"io"
	"net/http"
	"path"
	"strings"
)

// MaxUpload is the size of the largest file that one call can put: the 4 MB that the Learn page "Upload or replace the contents of a DriveItem" gives for a
// simple upload. A larger file needs an upload session, and dollycli has none.
const MaxUpload = 4 << 20

// DrivesBackend is what the drive resource needs from the data: a drive, and the folders and files on it. An item is an object as Graph shows it (id, name,
// size, folder or file, parentReference). A ref names an item: an item ID, "root", or a path from the root that starts with a slash. The first return
// value of ResolveDrive is the ID of a drive that the other calls take. The lists are copies.
type DrivesBackend interface {
	// ResolveDrive finds the drive of a call. kind is "drive" (ref is its ID), "user" (an ID or a user principal name), "group", or "site" (an ID, or "root").
	ResolveDrive(kind, ref string) (string, *Error)
	GetDrive(id string) (Object, *Error)
	GetItem(drive, ref string) (Object, *Error)
	Children(drive, ref string) ([]Object, *Error)
	// CreateChild makes a folder or an empty file in a folder.
	CreateChild(drive, parentRef string, props Object, a Actor) (Object, *Error)
	// PutFile puts a file in a folder by name. The bool says that the file is new. A parentRef that is a path makes the folders that are not there.
	PutFile(drive, parentRef, name string, data []byte, behavior string, a Actor) (Object, bool, *Error)
	// PutContent replaces the content of a file that is there.
	PutContent(drive, ref string, data []byte, a Actor) (Object, *Error)
	// Content gives the item and the bytes of a file.
	Content(drive, ref string) (Object, []byte, *Error)
	// DownloadOf gives a file for the address of its download (see DownloadPath).
	DownloadOf(drive, id, tag string) (name string, data []byte, ok bool)
	// UpdateItem renames and moves an item.
	UpdateItem(drive, ref string, props Object, a Actor) (Object, *Error)
	DeleteItem(drive, ref string, a Actor) *Error
}

// Drives is the resource for the drive of a user, a group, and a site, and for /drives: the same calls on each.
func Drives(b DrivesBackend) Resource { return drives{b} }

type drives struct{ b DrivesBackend }

// DownloadPrefix is where the download addresses are (see @microsoft.graph.downloadUrl). The server answers there with no token, as the real Graph does for the
// address that it gives.
const DownloadPrefix = "/dolly-download/"

func (d drives) Register(r *Router) {
	d.mount(r.Scope("/drives", "Drives"), "/{drive}", true, func(req *http.Request) (string, *Error) {
		return d.b.ResolveDrive("drive", req.PathValue("drive"))
	})
	d.mount(r.Scope("/users", "Drives"), "/{id}/drive", true, func(req *http.Request) (string, *Error) {
		return d.b.ResolveDrive("user", req.PathValue("id"))
	})
	d.mount(r.Scope("/groups", "Drives"), "/{id}/drive", true, func(req *http.Request) (string, *Error) {
		return d.b.ResolveDrive("group", req.PathValue("id"))
	})
	// GET /sites/{id}/drive is in sites.go
	d.mount(r.Scope("/sites", "Drives"), "/{id}/drive", false, func(req *http.Request) (string, *Error) {
		return d.b.ResolveDrive("site", req.PathValue("id"))
	})
}

// pathForm splits what follows root: in /drive/root:/Reports/2026.docx:/content into the path and what follows the colon.
func pathForm(rest string) (itemPath, suffix string) {
	itemPath, suffix, _ = strings.Cut(rest, ":/")
	return "/" + strings.Trim(strings.TrimSuffix(itemPath, ":"), "/"), suffix
}

// absolute turns the path of a download address into a full address, from the request.
func absolute(r *http.Request, o Object) Object {
	if p, ok := o["@microsoft.graph.downloadUrl"].(string); ok && strings.HasPrefix(p, "/") {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		o["@microsoft.graph.downloadUrl"] = scheme + "://" + r.Host + p
	}
	return o
}

func readUpload(r *http.Request) ([]byte, *Error) {
	data, err := io.ReadAll(io.LimitReader(r.Body, MaxUpload+1))
	switch {
	case err != nil:
		return nil, BadRequest("The content could not be read.")
	case len(data) > MaxUpload:
		// guess: the real Graph answers so for a simple upload that is too large
		return nil, &Error{http.StatusRequestEntityTooLarge, "requestEntityTooLarge", "A call that puts content takes a file of up to 4 MB. Use an upload session for a larger file (dollycli has none)."}
	}
	return data, nil
}

// mount registers the calls of one drive below base. drive finds the ID of the drive of the request.
func (d drives) mount(s *Scope, base string, withDrive bool, drive func(*http.Request) (string, *Error)) {
	env := s.r.Env()
	list := ListOptions{NoConsistency: true, PageSize: 200}
	opts := []string{"$select"}

	// with finds the drive first, and answers the error when there is none
	with := func(f func(w http.ResponseWriter, req *http.Request, id string)) http.HandlerFunc {
		return func(w http.ResponseWriter, req *http.Request) {
			id, e := drive(req)
			if e != nil {
				WriteError(w, e)
				return
			}
			f(w, req, id)
		}
	}
	one := func(w http.ResponseWriter, req *http.Request, o Object, e *Error, status int) {
		if e != nil {
			WriteError(w, e)
			return
		}
		// an item always has its id, with $select too (unlike a directory object)
		WriteJSON(w, status, SelectFields(absolute(req, o), SplitList(req.URL.Query().Get("$select")), []string{"id"}))
	}
	children := func(w http.ResponseWriter, req *http.Request, id, ref string) {
		items, e := d.b.Children(id, ref)
		if e != nil {
			WriteError(w, e)
			return
		}
		for _, it := range items {
			absolute(req, it)
		}
		ServeList(w, req, env, items, list)
	}
	create := func(w http.ResponseWriter, req *http.Request, id, ref string) {
		props, e := ReadJSON(req)
		if e == nil {
			var o Object
			o, e = d.b.CreateChild(id, ref, props, env.Actor(req))
			one(w, req, o, e, http.StatusCreated)
			return
		}
		WriteError(w, e)
	}
	update := func(w http.ResponseWriter, req *http.Request, id, ref string) {
		props, e := ReadJSON(req)
		if e == nil {
			var o Object
			o, e = d.b.UpdateItem(id, ref, props, env.Actor(req))
			one(w, req, o, e, http.StatusOK)
			return
		}
		WriteError(w, e)
	}
	// content answers with a redirect to the address of the download, as the Learn page says (302).
	content := func(w http.ResponseWriter, req *http.Request, id, ref string) {
		o, _, e := d.b.Content(id, ref)
		if e != nil {
			WriteError(w, e)
			return
		}
		http.Redirect(w, req, absolute(req, o)["@microsoft.graph.downloadUrl"].(string), http.StatusFound)
	}
	replace := func(w http.ResponseWriter, req *http.Request, id, ref string) {
		data, e := readUpload(req)
		if e == nil {
			var o Object
			o, e = d.b.PutContent(id, ref, data, env.Actor(req))
			one(w, req, o, e, http.StatusOK)
			return
		}
		WriteError(w, e)
	}
	// put makes a file with a name in a folder, or puts new content in the file that has the name. 201 for a new file, and 200 for one that was there.
	put := func(w http.ResponseWriter, req *http.Request, id, parent, name string) {
		data, e := readUpload(req)
		if e != nil {
			WriteError(w, e)
			return
		}
		o, created, e := d.b.PutFile(id, parent, name, data, req.URL.Query().Get("@microsoft.graph.conflictBehavior"), env.Actor(req))
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		one(w, req, o, e, status)
	}
	remove := func(w http.ResponseWriter, req *http.Request, id, ref string) {
		respond(w, d.b.DeleteItem(id, ref, env.Actor(req)))
	}

	if withDrive {
		s.Handle("GET", base, "Get the drive: its type, owner, and quota.", opts, with(func(w http.ResponseWriter, req *http.Request, id string) {
			o, e := d.b.GetDrive(id)
			one(w, req, o, e, http.StatusOK)
		}))
	}
	s.Handle("GET", base+"/root", "Get the root folder of the drive.", opts, with(func(w http.ResponseWriter, req *http.Request, id string) {
		o, e := d.b.GetItem(id, "root")
		one(w, req, o, e, http.StatusOK)
	}))
	s.Handle("GET", base+"/root/children", "List the files and folders in the root folder.", []string{"$select", "$top", "$orderby"},
		with(func(w http.ResponseWriter, req *http.Request, id string) { children(w, req, id, "root") }))
	s.Handle("POST", base+"/root/children", "Make a folder (name and folder: {}) or an empty file (name and file: {}) in the root folder. @microsoft.graph.conflictBehavior is fail (the default), rename, or replace. Gives 201.", nil,
		with(func(w http.ResponseWriter, req *http.Request, id string) { create(w, req, id, "root") }))

	s.Handle("GET", base+"/items/{item}", "Get a file or a folder by its ID.", opts, with(func(w http.ResponseWriter, req *http.Request, id string) {
		o, e := d.b.GetItem(id, req.PathValue("item"))
		one(w, req, o, e, http.StatusOK)
	}))
	s.Handle("PATCH", base+"/items/{item}", "Rename (name) or move (parentReference: {id}) a file or a folder. A name that the folder has fails with 409 nameAlreadyExists.", nil,
		with(func(w http.ResponseWriter, req *http.Request, id string) { update(w, req, id, req.PathValue("item")) }))
	s.Handle("DELETE", base+"/items/{item}", "Delete a file, or a folder and what is in it. Gives 204.", nil,
		with(func(w http.ResponseWriter, req *http.Request, id string) { remove(w, req, id, req.PathValue("item")) }))
	s.Handle("GET", base+"/items/{item}/children", "List the files and folders in a folder.", []string{"$select", "$top", "$orderby"},
		with(func(w http.ResponseWriter, req *http.Request, id string) { children(w, req, id, req.PathValue("item")) }))
	s.Handle("POST", base+"/items/{item}/children", "Make a folder (name and folder: {}) or an empty file (name and file: {}) in a folder. @microsoft.graph.conflictBehavior is fail (the default), rename, or replace. Gives 201.", nil,
		with(func(w http.ResponseWriter, req *http.Request, id string) { create(w, req, id, req.PathValue("item")) }))
	s.Handle("GET", base+"/items/{item}/content", "Download a file: a redirect (302) to the address of the download.", nil,
		with(func(w http.ResponseWriter, req *http.Request, id string) { content(w, req, id, req.PathValue("item")) }))
	s.Handle("PUT", base+"/items/{item}/content", "Put new content in a file that is there. The body is the content, up to 4 MB. Gives 200 with the file.", nil,
		with(func(w http.ResponseWriter, req *http.Request, id string) { replace(w, req, id, req.PathValue("item")) }))

	// An item is also found by its path: /root:/Reports/2026.docx, with :/children and :/content after it. A file is put in a folder by name: /items/{id}:/name:/content,
	// and /root:/folder/name:/content (the folders that are not there are made).
	notFound := func(w http.ResponseWriter) {
		WriteError(w, Unsupported("The path of this call is not one that dollycli answers."))
	}
	s.Handle("GET", base+"/root:/{rest...}", "Get a file or a folder by its path from the root: /root:/Reports/2026.docx. After the path, :/children lists a folder, and :/content downloads a file.", opts,
		with(func(w http.ResponseWriter, req *http.Request, id string) {
			p, suffix := pathForm(req.PathValue("rest"))
			switch suffix {
			case "":
				o, e := d.b.GetItem(id, p)
				one(w, req, o, e, http.StatusOK)
			case "children":
				children(w, req, id, p)
			case "content":
				content(w, req, id, p)
			default:
				notFound(w)
			}
		}))
	s.Handle("PUT", base+"/root:/{rest...}", "Put a file by its path: /root:/Reports/2026.docx:/content. The folders of the path that are not there are made. The body is the content, up to 4 MB. Gives 201 for a new file, and 200 when the file was there.", nil,
		with(func(w http.ResponseWriter, req *http.Request, id string) {
			p, suffix := pathForm(req.PathValue("rest"))
			if suffix != "content" || p == "/" {
				notFound(w)
				return
			}
			put(w, req, id, path.Dir(p), path.Base(p))
		}))
	s.Handle("PATCH", base+"/root:/{rest...}", "Rename or move a file or a folder by its path.", nil,
		with(func(w http.ResponseWriter, req *http.Request, id string) {
			if p, suffix := pathForm(req.PathValue("rest")); suffix == "" {
				update(w, req, id, p)
			} else {
				notFound(w)
			}
		}))
	s.Handle("DELETE", base+"/root:/{rest...}", "Delete a file or a folder by its path. Gives 204.", nil,
		with(func(w http.ResponseWriter, req *http.Request, id string) {
			if p, suffix := pathForm(req.PathValue("rest")); suffix == "" {
				remove(w, req, id, p)
			} else {
				notFound(w)
			}
		}))
	s.Handle("POST", base+"/root:/{rest...}", "Make a folder or an empty file in a folder by its path: /root:/Reports:/children.", nil,
		with(func(w http.ResponseWriter, req *http.Request, id string) {
			if p, suffix := pathForm(req.PathValue("rest")); suffix == "children" {
				create(w, req, id, p)
			} else {
				notFound(w)
			}
		}))
	s.Handle("PUT", base+"/items/{item}/{rest...}", "Put a file by name in a folder: /items/{folder-id}:/name.txt:/content. The body is the content, up to 4 MB. Gives 201 for a new file, and 200 when the file was there.", nil,
		with(func(w http.ResponseWriter, req *http.Request, id string) {
			folder, ok := strings.CutSuffix(req.PathValue("item"), ":")
			name, suffix, _ := strings.Cut(req.PathValue("rest"), ":/")
			if !ok || suffix != "content" || name == "" || strings.Contains(name, "/") {
				notFound(w)
				return
			}
			put(w, req, id, folder, name)
		}))
}
