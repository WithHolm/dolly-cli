// Package docs reads the documentation of an API from markdown files. One file is one page of the
// reference: the name, the order, and the text of the page, and the text of the calls on it.
// An API embeds a folder of these files (go:embed) and gives it to Load.
//
// A file looks like this:
//
//	---
//	name: Users
//	order: 1
//	resource: users            (the first part of the paths of the page. The default is the file name)
//	covers: me=Signed-in user  (more first parts of paths for this page, and the section that they go in)
//	---
//	The text of the page. It is markdown.
//
//	## GET /v1.0/users/{id}
//	The text of the call: what is special about it.
//
//	### Filtering
//	A part of the call with a title. The portal puts "Filtering" and "Search" next to the
//	documentation that the code makes for $filter and $search. Other titles have a heading of their own.
//
// The text that is made from the code (the forms of $filter, the query options, what Graph lists)
// is not in these files. The files add to it. Check finds a file that does not fit the routes.
package docs

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
)

// Page is one page of the reference.
type Page struct {
	Resource    string  `json:"resource"` // the first part of the paths: users, groups, $batch
	Name        string  `json:"name"`
	Order       int     `json:"order"`
	Covers      []Cover `json:"covers,omitempty"`
	Description string  `json:"description"` // markdown
	Ops         []Op    `json:"-"`
	File        string  `json:"-"`
}

// Cover is a first part of a path that belongs to a page of another resource, such as /me on the
// page of the users. Section is the section that its calls are in.
type Cover struct {
	Segment string `json:"segment"`
	Section string `json:"section"`
}

// Op is the text of one call.
type Op struct {
	Key      string    `json:"key"`  // GET /v1.0/users/{id}
	Body     string    `json:"body"` // markdown, before the first title
	Sections []Section `json:"sections,omitempty"`
}

// Section is a part of a call with a title.
type Section struct {
	Title string `json:"title"`
	Body  string `json:"body"` // markdown
}

var methods = map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}

// Load reads every .md file in dir (README.md is not a page). The pages are in the order of their order,
// then their name.
func Load(fsys fs.FS, dir string) ([]Page, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	var pages []Page
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || strings.EqualFold(e.Name(), "README.md") {
			continue
		}
		b, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		p, err := parse(e.Name(), string(b))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		pages = append(pages, p)
	}
	sort.SliceStable(pages, func(i, j int) bool {
		if pages[i].Order != pages[j].Order {
			return pages[i].Order < pages[j].Order
		}
		return pages[i].Name < pages[j].Name
	})
	return pages, nil
}

func parse(file, text string) (Page, error) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	p := Page{File: file, Resource: strings.TrimSuffix(file, ".md")}
	front, body, ok := strings.Cut(strings.TrimPrefix(text, "---\n"), "\n---\n")
	if !strings.HasPrefix(text, "---\n") || !ok {
		return p, fmt.Errorf("the file must start with a block of --- lines that has at least a name")
	}
	for i, line := range strings.Split(front, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			return p, fmt.Errorf("line %d of the front matter: write key: value", i+2)
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		switch key {
		case "name":
			p.Name = val
		case "resource":
			p.Resource = val
		case "order":
			n, err := strconv.Atoi(val)
			if err != nil {
				return p, fmt.Errorf("order must be a whole number, not %q", val)
			}
			p.Order = n
		case "covers":
			for _, c := range strings.Split(val, ",") {
				seg, section, ok := strings.Cut(c, "=")
				if seg = strings.TrimSpace(seg); !ok || seg == "" || strings.TrimSpace(section) == "" {
					return p, fmt.Errorf("covers must be segment=Section title, for example me=Signed-in user")
				}
				p.Covers = append(p.Covers, Cover{Segment: seg, Section: strings.TrimSpace(section)})
			}
		default:
			return p, fmt.Errorf("the front matter has no %q (it has name, order, resource, covers)", key)
		}
	}
	if p.Name == "" {
		return p, fmt.Errorf("the front matter needs a name")
	}

	var (
		cur      *Op
		sec      *Section
		fenced   bool
		curLines []string
	)
	flush := func() {
		text := trim(curLines)
		switch {
		case sec != nil:
			sec.Body = text
			cur.Sections = append(cur.Sections, *sec)
		case cur != nil:
			cur.Body = text
		default:
			p.Description = text
		}
		curLines, sec = nil, nil
	}
	endOp := func() {
		flush()
		if cur != nil {
			p.Ops = append(p.Ops, *cur)
			cur = nil
		}
	}
	for n, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
		}
		if !fenced {
			switch {
			case strings.HasPrefix(line, "## "):
				endOp()
				method, route, _ := strings.Cut(strings.TrimSpace(line[3:]), " ")
				route = strings.TrimSpace(route)
				if !methods[method] || !strings.HasPrefix(route, "/") {
					return p, fmt.Errorf("line %d: the title of a call is the method and the path, for example '## GET /v1.0/users/{id}', not %q", n+1, line)
				}
				cur = &Op{Key: method + " " + route}
				continue
			case strings.HasPrefix(line, "# "):
				return p, fmt.Errorf("line %d: do not use a title of level 1. The name of the page is in the front matter", n+1)
			case strings.HasPrefix(line, "### "):
				if cur == nil {
					return p, fmt.Errorf("line %d: a title of level 3 is a part of a call. Write a '## GET /path' title first", n+1)
				}
				flush()
				sec = &Section{Title: strings.TrimSpace(line[4:])}
				continue
			}
		}
		curLines = append(curLines, line)
	}
	if fenced {
		return p, fmt.Errorf("a code block is not closed")
	}
	endOp()
	return p, nil
}

// trim takes the empty lines from both ends of the text.
func trim(lines []string) string {
	return strings.Trim(strings.Join(lines, "\n"), "\n ")
}

// Check says what is wrong with the pages for the routes of an API. routes are "METHOD /path", and
// base is the start of every path (/v1.0): the first part after it says which page a call is on. A call
// does not need text, but a text must be for a call that exists, and every call must have a page.
func Check(pages []Page, routes []string, base string) []string {
	var bad []string
	known := map[string]bool{}
	segments := map[string]bool{}
	for _, r := range routes {
		known[r] = true
		_, p, _ := strings.Cut(r, " ")
		segments[strings.SplitN(strings.TrimPrefix(strings.TrimPrefix(p, base), "/"), "/", 2)[0]] = true
	}
	owned := map[string]string{}
	for _, p := range pages {
		for _, seg := range append([]string{p.Resource}, coverSegments(p)...) {
			if prev, dup := owned[seg]; dup {
				bad = append(bad, fmt.Sprintf("%s and %s both cover %q", prev, p.File, seg))
			}
			owned[seg] = p.File
		}
		seen := map[string]bool{}
		for _, op := range p.Ops {
			switch {
			case !known[op.Key]:
				bad = append(bad, fmt.Sprintf("%s: %q is not a call of the API", p.File, op.Key))
			case seen[op.Key]:
				bad = append(bad, fmt.Sprintf("%s: %q is there twice", p.File, op.Key))
			case strings.TrimSpace(op.Body) == "" && len(op.Sections) == 0:
				bad = append(bad, fmt.Sprintf("%s: %q has no text", p.File, op.Key))
			}
			seen[op.Key] = true
		}
	}
	for seg := range segments {
		if _, ok := owned[seg]; !ok {
			bad = append(bad, fmt.Sprintf("the calls of /%s have no page: add a file with resource: %s", seg, seg))
		}
	}
	for seg, file := range owned {
		if !segments[seg] {
			bad = append(bad, fmt.Sprintf("%s covers /%s, and no call starts with it", file, seg))
		}
	}
	sort.Strings(bad)
	return bad
}

func coverSegments(p Page) []string {
	out := make([]string, len(p.Covers))
	for i, c := range p.Covers {
		out[i] = c.Segment
	}
	return out
}
