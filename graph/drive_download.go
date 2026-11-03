package graph

import (
	"bytes"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/withholm/dolly-cli/graph/api"
)

// The address in @microsoft.graph.downloadUrl: /dolly-download/{drive}/{item}/{tag}/{name}. It needs no token, as the address that the real Graph gives (it is
// "pre-authenticated"). The tag is made from the cTag of the file, so an address of older content gives 404. It answers Range, as the Learn page "Download the
// contents of a DriveItem" says the real one does.
func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	name, data, ok := s.world.DownloadOf(r.PathValue("drive"), r.PathValue("item"), r.PathValue("tag"))
	if !ok {
		api.WriteError(w, api.ItemNotFound())
		return
	}
	typ := mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
	if typ == "" {
		typ = "application/octet-stream"
	}
	w.Header().Set("Content-Type", typ)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}
