// Command opfetch reads the pages of Microsoft Learn about the calls of Graph, and keeps the permissions that each call needs. It reads all the
// pages about calls at one commit (about four thousand), and matches each call to the page that lists it in its section "HTTP request". The link
// of the description of Graph is only used to choose between pages: it is not always right. It writes:
//
//	operations/all.json         every call of the description of Graph (graph/spec): its page, the permissions of the pages, and for each permission
//	                            the calls that it allows. Not built into the program. What the tool could not read is written into the file.
//	operations/operations.json  the calls that dollycli serves: the route, the page, and the permissions. Built into the program. The tool stops
//	                            if a page of these calls is missing or has a table that it cannot read.
//	operations/raw/, SOURCE.json  the pages of the calls that dollycli serves, as they are, with a hash of each.
//
// Run it with 'make update-operation-permissions'.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/withholm/dolly-cli/graph"
	"github.com/withholm/dolly-cli/graph/permissions"
	"github.com/withholm/dolly-cli/graph/spec"
	"github.com/withholm/dolly-cli/internal/ghraw"
)

const (
	repo    = "microsoftgraph/microsoft-graph-docs-contrib"
	apiDir  = "api-reference/v1.0/api" // the pages about the calls
	license = "Documentation: CC BY 4.0 (the file LICENSE in that repo). Give credit to Microsoft when you use the data."
)

type file struct {
	Path   string `json:"path"` // in the repo of Microsoft
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

func main() {
	out := flag.String("out", "graph/permissions/operations", "The folder to write to.")
	commit := flag.String("commit", "", "Read the pages from this commit. The default is the latest one.")
	flag.Parse()
	if err := run(*out, *commit); err != nil {
		fmt.Fprintln(os.Stderr, "opfetch:", err)
		os.Exit(1)
	}
}

// fetch reads a file of the repo. A file that is not there is reported as missing, and a failure of the network is tried again twice.
func fetch(sha, p string) (body []byte, missing bool, err error) {
	for try := range 3 {
		body, err = ghraw.Raw(repo, sha, p)
		if err == nil {
			return body, false, nil
		}
		if strings.Contains(err.Error(), "status 404") {
			return nil, true, nil
		}
		time.Sleep(time.Duration(try+1) * time.Second)
	}
	return nil, false, err
}

// result is what was read for one page.
type result struct {
	page     string
	bodies   map[string][]byte // the include files that were read, by path in the repo
	perms    permissions.CallPermissions
	dropped  []string // parts of cells that the parser could not read
	notFound []string // files that are not in the repo at the commit
	err      error    // another failure: the network
}

// parallel runs f for each item, sixteen at a time.
func parallel[T any](items []T, f func(int, T)) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)
	for i, it := range items {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			f(i, it)
		}()
	}
	wg.Wait()
}

// permissionsOf reads the tables of permissions of a page whose text is body: in the page, and in its include files. The include files that were read
// are kept in r.bodies, by their path in the repo.
func permissionsOf(page, sha string, body []byte, r *result) {
	perms, dropped, notFound, err := permissions.ParsePage(body, func(rel string) ([]byte, bool, error) {
		inc := path.Join(apiDir, rel)
		b, missing, err := fetch(sha, inc)
		if err == nil && !missing {
			r.bodies[inc] = b
		}
		return b, missing, err
	})
	for _, rel := range notFound {
		r.notFound = append(r.notFound, path.Join(apiDir, rel))
	}
	r.perms, r.dropped, r.err = perms, dropped, err
}

func writeJSON(dir, name string, v any, indent string) error {
	b, err := json.MarshalIndent(v, "", indent)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), append(b, '\n'), 0o644)
}

// listPages gives the keys of all the pages about calls at a commit (the name of the file without .md). It asks GitHub for the folder, which is
// not cut short (about four thousand files).
func listPages(sha string) ([]string, error) {
	h := map[string]string{"Accept": "application/vnd.github+json"}
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		h["Authorization"] = "Bearer " + t
	}
	data, err := ghraw.Get("https://api.github.com/repos/"+repo+"/git/trees/"+sha+":"+apiDir, h)
	if err != nil {
		return nil, fmt.Errorf("cannot list the pages (GitHub may limit calls without GITHUB_TOKEN): %w", err)
	}
	var tree struct {
		Truncated bool
		Tree      []struct{ Path, Type string }
	}
	if err := json.Unmarshal(data, &tree); err != nil {
		return nil, err
	}
	if tree.Truncated {
		return nil, fmt.Errorf("GitHub cut the list of pages short: the tool must read the folder in parts")
	}
	var pages []string
	for _, e := range tree.Tree {
		if e.Type == "blob" && strings.HasSuffix(e.Path, ".md") {
			pages = append(pages, strings.TrimSuffix(e.Path, ".md"))
		}
	}
	sort.Strings(pages)
	return pages, nil
}

func run(out, commit string) error {
	ref := commit
	if ref == "" {
		ref = "main"
	}
	sha, date, err := ghraw.Head(repo, ref) // the commit and its date, for the latest or for the one that was asked for
	if err != nil {
		return err
	}
	slugs, err := listPages(sha)
	if err != nil {
		return err
	}
	fmt.Printf("Reading %d pages from commit %s ...\n", len(slugs), sha[:10])

	// 1. every page: its text, and the calls that it lists
	bodies := make([][]byte, len(slugs))
	errs := make([]error, len(slugs))
	missing := make([]bool, len(slugs))
	parallel(slugs, func(i int, slug string) {
		bodies[i], missing[i], errs[i] = fetch(sha, path.Join(apiDir, slug+".md"))
	})
	index := map[string]int{}
	lists := map[string][]string{}  // the calls that a page lists
	byCall := map[string][]string{} // the pages that list a call
	var notFound []string
	for i, slug := range slugs {
		index[slug] = i
		switch {
		case errs[i] != nil:
			return errs[i] // the network failed: do not write files that look whole
		case missing[i]:
			notFound = append(notFound, path.Join(apiDir, slug+".md"))
			continue
		}
		lists[slug] = permissions.RequestsOf(bodies[i])
		for _, key := range lists[slug] {
			byCall[key] = append(byCall[key], slug)
		}
	}
	listing := func(key string) map[string]int {
		m := map[string]int{}
		for _, p := range byCall[key] {
			m[p] = len(lists[p])
		}
		return m
	}

	// 2. every call of the description: the page that lists it, and the link of the description to choose between pages
	desc := spec.Load()
	calls := map[string]permissions.CallRef{}
	callsOf := map[string][]string{}
	summary := map[string]string{}
	counts := map[string]int{}
	for _, op := range desc.Operations {
		specPage := ""
		if op.Doc != "" {
			p, ok := permissions.PageOf(op.Doc)
			if !ok {
				return fmt.Errorf("%s %s has the page %q, and it is not a page about a call of Graph", op.Method, op.Path, op.Doc)
			}
			specPage = p
			if _, has := summary[p]; !has {
				summary[p] = op.Summary
			}
		}
		callKey := permissions.CallKey(op.Method, op.Path)
		ref := permissions.ResolveCall(callKey, specPage, listing(callKey))
		key := op.Method + " " + op.Path
		calls[key] = ref
		counts[orNone(ref.Via)]++
		if ref.Page != "" {
			callsOf[ref.Page] = append(callsOf[ref.Page], key)
		}
	}

	// 3. the routes that dollycli serves: the same match, for the call of the description that the route is
	endpoints, _ := graph.New(graph.NewMemory(graph.Options{Users: 5, Seed: 1}), graph.ServerOptions{}).API()
	routes := map[string]string{}
	var noPage []string
	servedPages := map[string]bool{}
	for _, e := range endpoints {
		var op *spec.Operation
		for _, p := range []string{e.Path, e.Path + "()"} { // a function of Graph has () in its path, and dollycli does not
			if op = desc.ForDolly(e.Method, p); op != nil {
				break
			}
		}
		key, specPage := permissions.CallKey(e.Method, e.Path), ""
		if op != nil {
			key = permissions.CallKey(op.Method, op.Path)
			if op.Doc != "" {
				specPage, _ = permissions.PageOf(op.Doc)
			}
		}
		route := e.Method + " " + e.Path
		if ref := permissions.ResolveCall(key, specPage, listing(key)); ref.Page != "" {
			routes[route] = ref.Page
			servedPages[ref.Page] = true
		} else {
			noPage = append(noPage, route)
		}
	}
	// a route with no page of its own takes the page of the route that it is a form of
	inherited := map[string]string{}
	var left []string
	for _, route := range noPage {
		if parent, ok := permissions.ParentRoute(route, routes); ok {
			routes[route], inherited[route] = routes[parent], parent
			continue
		}
		left = append(left, route)
	}
	noPage = left
	sort.Strings(noPage)

	// 4. the permissions of the pages that were chosen
	chosen := make([]string, 0, len(callsOf)+len(servedPages))
	for p := range callsOf {
		chosen = append(chosen, p)
	}
	for p := range servedPages {
		if _, has := callsOf[p]; !has {
			chosen = append(chosen, p)
		}
	}
	sort.Strings(chosen)
	results := make([]result, len(chosen))
	parallel(chosen, func(i int, page string) {
		r := result{page: page, bodies: map[string][]byte{}}
		if j, ok := index[page]; ok && bodies[j] != nil {
			permissionsOf(page, sha, bodies[j], &r)
		} else {
			r.notFound = append(r.notFound, path.Join(apiDir, page+".md")) // the description links to a page that is not in the repo
		}
		results[i] = r
	})

	// the pages of the calls that dollycli serves must be read in full: a gap here is a call with no permissions in the portal
	for _, r := range results {
		if !servedPages[r.page] {
			continue
		}
		switch {
		case r.err != nil:
			return r.err
		case len(r.notFound) > 0:
			return fmt.Errorf("%s: %v is not in the repo at commit %s", r.page, r.notFound, sha[:10])
		case len(r.dropped) > 0:
			// a part of a cell that the parser cannot read is not left out silently: the form of the table has changed
			return fmt.Errorf("%s: cannot read %q in the table of permissions. Update graph/permissions/table.go", r.page, r.dropped)
		}
	}

	all := permissions.AllFile{
		Source:      permissions.OperationsSource{Repo: repo, Commit: sha, CommitDate: date, License: license},
		Pages:       map[string]permissions.AllPage{},
		Calls:       calls,
		Permissions: map[string]permissions.PermissionUse{},
		Problems:    permissions.AllProblems{Unreadable: map[string][]string{}, NotFound: notFound, Counts: counts},
	}
	served := permissions.Operations{
		Source: all.Source, Pages: map[string]permissions.CallPermissions{}, Routes: routes, Inherited: inherited, NoPage: noPage,
	}
	raw := map[string][]byte{}
	for _, r := range results {
		if r.err != nil {
			return r.err
		}
		all.Problems.NotFound = append(all.Problems.NotFound, r.notFound...)
		if len(r.dropped) > 0 {
			all.Problems.Unreadable[r.page] = r.dropped
		}
		if len(r.notFound) == 0 && !r.perms.Table {
			all.Problems.NoTable = append(all.Problems.NoTable, r.page)
		}
		slices.Sort(callsOf[r.page])
		all.Pages[r.page] = permissions.AllPage{Summary: summary[r.page], Calls: callsOf[r.page], Lists: lists[r.page], CallPermissions: r.perms}
		if servedPages[r.page] {
			served.Pages[r.page] = r.perms
			raw[path.Join(apiDir, r.page+".md")] = bodies[index[r.page]]
			for p, b := range r.bodies {
				raw[p] = b
			}
		}
	}
	slices.Sort(all.Problems.NotFound)
	slices.Sort(all.Problems.NoTable)
	if len(all.Problems.Unreadable) == 0 {
		all.Problems.Unreadable = nil
	}
	indexPermissions(&all, chosen)

	// write: the old files are replaced only now, when everything was read
	if err := os.RemoveAll(filepath.Join(out, "raw")); err != nil {
		return err
	}
	var files []file
	for p, b := range raw {
		dst := filepath.Join(out, "raw", filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		files = append(files, file{Path: p, Bytes: len(b), SHA256: hex.EncodeToString(sum[:])})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	if err := writeJSON(out, "SOURCE.json", map[string]any{
		"source": "https://github.com/" + repo, "license": license, "commit": sha, "commitDate": date,
		"fetchedDate": time.Now().UTC().Format("2006-01-02"), "files": files,
	}, "  "); err != nil {
		return err
	}
	if err := writeJSON(out, "operations.json", served, "  "); err != nil {
		return err
	}
	if err := writeJSON(out, "all.json", all, " "); err != nil {
		return err
	}
	fmt.Printf("all.json: %d calls (matched: %v), %d pages, %d permissions.\n", len(calls), counts, len(all.Pages), len(all.Permissions))
	fmt.Printf("          problems: %d files not found, %d pages with a part of a table that was not read, %d pages with no table.\n",
		len(all.Problems.NotFound), len(all.Problems.Unreadable), len(all.Problems.NoTable))
	fmt.Printf("operations.json: %d routes of dollycli, %d pages (%d files in raw/), %d routes that use the page of another route, %d with no page.\n",
		len(routes)+len(noPage), len(served.Pages), len(files), len(inherited), len(noPage))
	return nil
}

// indexPermissions makes the other way: for each permission, the calls that it allows.
func indexPermissions(f *permissions.AllFile, pages []string) {
	catalog, _ := permissions.Load()
	add := func(name, kind string, least bool, page string, with []string, resource, scenario string) {
		u := f.Permissions[name]
		if u.InCatalog == nil {
			u.InCatalog = []string{}
			if catalog != nil {
				for _, k := range []permissions.Kind{permissions.Delegated, permissions.Application} {
					if _, ok := catalog.Find(k, name); ok {
						u.InCatalog = append(u.InCatalog, string(k))
					}
				}
			}
		}
		target := &u.Delegated
		if kind == "application" {
			target = &u.Application
		}
		use := permissions.Use{Page: page, With: with, Resource: resource, Scenario: scenario}
		if least {
			target.Least = append(target.Least, use)
		} else {
			target.Higher = append(target.Higher, use)
		}
		f.Permissions[name] = u
	}
	for _, page := range pages {
		p := f.Pages[page]
		indexNeed := func(need permissions.Need, kind, resource, scenario string) {
			for _, group := range []struct {
				least bool
				alts  []string
			}{{true, need.Least}, {false, need.Higher}} {
				for _, alt := range group.alts {
					parts := permissions.Alternatives(alt)
					for _, name := range parts {
						with := slices.DeleteFunc(slices.Clone(parts), func(o string) bool { return o == name })
						add(name, kind, group.least, page, with, resource, scenario)
					}
				}
			}
		}
		indexNeed(p.Delegated, "delegated", "", "")
		indexNeed(p.Application, "application", "", "")
		for _, v := range p.Variants {
			indexNeed(v.Delegated, "delegated", v.Resource, v.Scenario)
			indexNeed(v.Application, "application", v.Resource, v.Scenario)
		}
	}
	for _, u := range f.Permissions { // a stable file
		for _, uses := range []*permissions.Uses{&u.Delegated, &u.Application} {
			for _, list := range [][]permissions.Use{uses.Least, uses.Higher} {
				sort.Slice(list, func(i, j int) bool {
					if list[i].Page != list[j].Page {
						return list[i].Page < list[j].Page
					}
					a := list[i].Scenario + "|" + list[i].Resource + "|" + strings.Join(list[i].With, ",")
					b := list[j].Scenario + "|" + list[j].Resource + "|" + strings.Join(list[j].With, ",")
					return a < b
				})
			}
		}
	}
}

func orNone(via string) string {
	if via == "" {
		return "none"
	}
	return via
}
