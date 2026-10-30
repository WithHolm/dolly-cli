package graph

import (
	"embed"
	"io/fs"
	"log/slog"
	"strings"
	"sync"

	"github.com/withholm/dolly-cli/shared/docs"
)

// The text of the API page of the portal is in markdown files (docs/*.md, see docs/README.md). They are
// embedded in the program. The code adds what it knows: the summary of a call, the forms of $filter,
// the query options. A test checks that the files fit the routes.

//go:embed docs/*.md
var embeddedDocs embed.FS

// docSource keeps the pages. A folder that a person gave is read again at each request.
type docSource struct {
	mu    sync.Mutex
	fsys  fs.FS
	dir   string
	live  bool
	pages []docs.Page
}

func (d *docSource) init(custom fs.FS) {
	if custom != nil {
		d.fsys, d.dir, d.live = custom, ".", true
	} else {
		d.fsys, d.dir = embeddedDocs, "docs"
	}
	d.reload()
}

// reload reads the files. When they cannot be read, the last good pages stay, and the error is logged.
func (d *docSource) reload() {
	pages, err := docs.Load(d.fsys, d.dir)
	if err != nil {
		slog.Error("Dolly cannot read the documentation files. It keeps what it had.", "error", err)
		return
	}
	d.pages = pages
}

// DocPages gives the pages of the reference, in order.
func (s *Server) DocPages() []docs.Page {
	s.docs.mu.Lock()
	defer s.docs.mu.Unlock()
	if s.docs.live {
		s.docs.reload()
	}
	return s.docs.pages
}

// attachDocs adds to each operation the page that it is on, the section that it is in when its page
// only covers it, and its text.
func attachDocs(eps []EndpointInfo, pages []docs.Page) {
	type home struct {
		page    *docs.Page
		section string
	}
	byPart := map[string]home{}
	notes := map[string]*docs.Op{}
	for i := range pages {
		p := &pages[i]
		byPart[p.Resource] = home{page: p}
		for _, c := range p.Covers {
			byPart[c.Segment] = home{page: p, section: c.Section}
		}
		for j := range p.Ops {
			notes[p.Ops[j].Key] = &p.Ops[j]
		}
	}
	for i := range eps {
		e := &eps[i]
		part, _, _ := strings.Cut(strings.TrimPrefix(e.Path, BasePath+"/"), "/")
		if h, ok := byPart[part]; ok {
			e.Resource, e.Section = h.page.Resource, h.section
		} else {
			e.Resource = part // no page: the portal still shows it
		}
		e.Notes = notes[e.Method+" "+e.Path]
	}
}
