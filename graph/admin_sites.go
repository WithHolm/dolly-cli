package graph

import (
	"encoding/json"
	"net/http"
)

// The admin API of the sites page of the portal. Graph has no call to make or change a site or a drive, so the portal does it here. The checks and the
// history are those of the repo package (repo/sites.go). The Graph API reads what these calls change.
func (s *Server) adminSiteRoutes() {
	a := s.admin
	// every site, with its kind, its group, and its drives, and the groups that can still get a site
	a.HandleFunc("GET /sites", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"sites": s.world.AdminSites(), "groups": s.world.SiteGroups()})
	})
	a.HandleFunc("POST /sites", func(w http.ResponseWriter, r *http.Request) {
		s.adminMake(w, r, func(props map[string]any) (map[string]any, *apiError) { return s.world.CreateSite(props, adminActor) })
	})
	a.HandleFunc("PATCH /sites/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.adminMake(w, r, func(props map[string]any) (map[string]any, *apiError) {
			return s.world.UpdateSite(r.PathValue("id"), props, adminActor)
		})
	})
	a.HandleFunc("DELETE /sites/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.adminChanged(w, s.world.DeleteSite(r.PathValue("id"), adminActor))
	})
	a.HandleFunc("POST /sites/{id}/drives", func(w http.ResponseWriter, r *http.Request) {
		s.adminMake(w, r, func(props map[string]any) (map[string]any, *apiError) {
			return s.world.CreateDrive(r.PathValue("id"), props, adminActor)
		})
	})
	a.HandleFunc("PATCH /drives/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.adminMake(w, r, func(props map[string]any) (map[string]any, *apiError) {
			return s.world.UpdateDrive(r.PathValue("id"), props, adminActor)
		})
	})
	a.HandleFunc("DELETE /drives/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.adminChanged(w, s.world.DeleteDrive(r.PathValue("id"), adminActor))
	})
	// the folders and files of a drive, as a flat list (the portal makes the tree)
	a.HandleFunc("GET /drives/{id}/items", func(w http.ResponseWriter, r *http.Request) {
		items, e := s.world.DriveItems(r.PathValue("id"))
		if e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	})
	// a folder, or a file: parentId ("" is the root), name, and for a file text, or a size in bytes for a file that has no content of its own
	a.HandleFunc("POST /drives/{id}/items", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			ParentID string  `json:"parentId"`
			Name     string  `json:"name"`
			Folder   bool    `json:"folder"`
			Text     string  `json:"text"`
			Size     float64 `json:"size"`
		}
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			writeError(w, badRequest("The body must be a JSON object."))
			return
		}
		parent := b.ParentID
		if parent == "" {
			parent = "root"
		}
		var o map[string]any
		var e *apiError
		if b.Folder {
			o, e = s.world.CreateChild(r.PathValue("id"), parent, map[string]any{"name": b.Name, "folder": map[string]any{}}, adminActor)
		} else {
			o, e = s.world.AddFile(r.PathValue("id"), parent, b.Name, b.Text, int64(b.Size), adminActor)
		}
		if e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, http.StatusOK, o)
	})
	a.HandleFunc("PATCH /drives/{id}/items/{item}", func(w http.ResponseWriter, r *http.Request) {
		s.adminMake(w, r, func(props map[string]any) (map[string]any, *apiError) {
			return s.world.UpdateItem(r.PathValue("id"), r.PathValue("item"), props, adminActor)
		})
	})
	a.HandleFunc("DELETE /drives/{id}/items/{item}", func(w http.ResponseWriter, r *http.Request) {
		s.adminChanged(w, s.world.DeleteItem(r.PathValue("id"), r.PathValue("item"), adminActor))
	})
}

// adminMake reads a JSON object from the body, runs a change on it, and answers with the object that it gives (200).
func (s *Server) adminMake(w http.ResponseWriter, r *http.Request, change func(map[string]any) (map[string]any, *apiError)) {
	var props map[string]any
	if err := json.NewDecoder(r.Body).Decode(&props); err != nil || props == nil {
		writeError(w, badRequest("The body must be a JSON object."))
		return
	}
	o, e := change(props)
	if e != nil {
		writeError(w, e)
		return
	}
	writeJSON(w, http.StatusOK, o)
}
