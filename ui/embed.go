// Package ui is the portal of Dolly: a Svelte app (the folder src) that is built into the folder
// dist and embedded here. dist is kept in the repo, so 'go install' needs no Node. After a change
// in src, run 'make ui'.
package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed dist
var files embed.FS

// Handler serves the app. The pages are in the hash of the address (#/graph/calls), so every
// path that is not a file is a 404, and the server needs no fallback to index.html.
func Handler() http.Handler {
	dist, err := fs.Sub(files, "dist")
	if err != nil {
		panic(err) // dist is embedded: this does not happen
	}
	server := http.FileServerFS(dist)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			// the name of a file in assets has a hash of its content: it never changes
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		server.ServeHTTP(w, r)
	})
}
