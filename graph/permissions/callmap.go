package permissions

import (
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/withholm/dolly-cli/graph/spec"
)

// Which page of Microsoft Learn is about which call. The description of Graph (graph/spec) has a link to a page for some calls, and the link is not
// always right: GET /me/messages points to the page about open extensions. A page says itself which calls it is about, in its section
// "HTTP request". So a call is matched to the pages that list it, and the link of the description is used to choose between them.

var (
	requestRe = regexp.MustCompile(`^(GET|POST|PATCH|PUT|DELETE)\s+(/.*)$`)
	headingRe = regexp.MustCompile(`^#{1,6}\s`)
)

// CallKey writes a call in a form that a call of the description and a call of a page have in common: the method, then the path with the names of
// the parameters left out and capitals gone: "GET /users/{}/manager".
func CallKey(method, path string) string {
	if i := strings.IndexAny(path, "?"); i >= 0 {
		path = path[:i]
	}
	return strings.ToUpper(method) + " " + strings.TrimRight(spec.Normal(path), "/")
}

// pathOf cuts a path at the first space that is not inside braces: "/users/{id | userPrincipalName} some words" gives the path and not the words.
func pathOf(s string) string {
	depth := 0
	for i, r := range s {
		switch r {
		case '{':
			depth++
		case '}':
			depth = max(0, depth-1)
		case ' ', '\t':
			if depth == 0 {
				return s[:i]
			}
		}
	}
	return s
}

// RequestsOf gives the calls that a page of Learn says it is about: the lines of its section "HTTP request" that start with a method and a path
// (not a whole address: the examples have those). They are in the form of CallKey, and each one is once.
func RequestsOf(md []byte) []string {
	var out []string
	in := false
	for _, line := range strings.Split(string(md), "\n") {
		trimmed := strings.TrimSpace(line)
		if headingRe.MatchString(trimmed) {
			// "## HTTP request" starts the section, and the next heading of the same level (##) ends it. Smaller headings are inside it.
			if strings.HasPrefix(strings.ToLower(trimmed), "## http request") {
				in = true
				continue
			}
			if in && strings.HasPrefix(trimmed, "## ") {
				break
			}
		}
		if !in {
			continue
		}
		if m := requestRe.FindStringSubmatch(trimmed); m != nil {
			if key := CallKey(m[1], pathOf(m[2])); !slices.Contains(out, key) {
				out = append(out, key)
			}
		}
	}
	return out
}

var typedSuffix = regexp.MustCompile(`/microsoft\.graph\.[A-Za-z]+$`)

// ParentRoute finds the route that a route with no page of its own is a form of, and that has a page (in routes): a $count route takes the page of
// the list that it counts, a route with a type at the end (".../microsoft.graph.group") the page of the route without it, and a /me route the page of
// the same route for a user. The routes are written as the server writes them: "GET /v1.0/users/$count".
func ParentRoute(route string, routes map[string]string) (string, bool) {
	method, path, ok := strings.Cut(route, " ")
	if !ok {
		return "", false
	}
	candidates := func(p string) []string {
		var c []string
		if s := strings.TrimSuffix(p, "/$count"); s != p {
			c = append(c, s)
		}
		if s := typedSuffix.ReplaceAllString(p, ""); s != p {
			c = append(c, s)
		}
		if s, found := strings.CutPrefix(p, "/v1.0/me"); found && (s == "" || strings.HasPrefix(s, "/")) {
			c = append(c, "/v1.0/users/{id}"+s)
		}
		return c
	}
	seen := map[string]bool{path: true}
	queue := []string{path}
	for len(queue) > 0 && len(seen) < 20 {
		next := queue[0]
		queue = queue[1:]
		for _, c := range candidates(next) {
			if seen[c] {
				continue
			}
			seen[c] = true
			if page := routes[method+" "+c]; page != "" {
				return method + " " + c, true
			}
			queue = append(queue, c)
		}
	}
	return "", false
}

// CallRef is the page of Learn that a call is matched to, and how sure that is.
type CallRef struct {
	Page string `json:"page"`
	// Via says how it was matched:
	//   verified   the link of the description, and the page lists the call, and no other page that lists it fits the call better
	//   matched    the page lists the call (the description has no link, or the link is to a page that does not list it, or another page fits better)
	//   spec-only  the link of the description, and no page lists the call: it may be wrong
	//   ""         no page
	Via string `json:"via,omitempty"`
	// Others are the other pages that list the call too, when there is more than one.
	Others []string `json:"others,omitempty"`
}

// verbs are the words in the name of a page that say what the call does, and not what it is about.
var verbs = map[string]bool{
	"list": true, "get": true, "post": true, "update": true, "delete": true, "add": true, "remove": true, "create": true, "upsert": true,
	"put": true, "patch": true, "set": true, "assign": true, "invoke": true,
}

// callWords are the words of the path of a call, in the form of CallKey: "GET /users/{}/memberof" gives users, user, memberof. A word is in its
// form with and without a final s. The parameters and the words of the namespace (graph.group gives group) are left out.
func callWords(call string) map[string]bool {
	_, path, _ := strings.Cut(call, " ")
	words := map[string]bool{}
	for _, seg := range strings.Split(path, "/") {
		seg = strings.TrimSuffix(strings.TrimPrefix(seg, "$"), "()")
		if seg == "" || strings.HasPrefix(seg, "{") || seg == "ref" || seg == "count" || seg == "value" {
			continue
		}
		for _, w := range strings.Split(seg, ".") {
			if w == "" || w == "graph" {
				continue
			}
			words[w] = true
			if s := strings.TrimSuffix(w, "s"); s != w && !strings.HasSuffix(w, "ss") {
				words[s] = true
			}
		}
	}
	if words["me"] { // /me is the signed-in user
		words["user"] = true
	}
	return words
}

// nameScore says how well the name of a page fits a call: the words of the name that are words of the path count for it, and the words that are
// neither verbs nor in the path count against it. user-list-memberof fits GET /users/{}/memberof, and agentuser-list-memberof does not (agentuser is
// not in the path).
func nameScore(page, call string) int {
	words, score := callWords(call), 0
	for _, w := range strings.Split(page, "-") {
		switch {
		case words[w]:
			score += 3
		case !verbs[w]:
			score -= 2
		}
	}
	return score
}

// ResolveCall chooses the page for a call. call is the call in the form of CallKey, specPage is the page that the description links to ("" if
// none), and listing are the pages that list the call, with how many calls each lists. Between several pages the one with the name that fits the call
// best is chosen. If that is not decided, the page of the description is, and then the page that lists the fewest calls (it says more about this one).
func ResolveCall(call, specPage string, listing map[string]int) CallRef {
	pages := make([]string, 0, len(listing))
	for p := range listing {
		pages = append(pages, p)
	}
	sort.Slice(pages, func(i, j int) bool {
		if a, b := nameScore(pages[i], call), nameScore(pages[j], call); a != b {
			return a > b
		}
		if (pages[i] == specPage) != (pages[j] == specPage) {
			return pages[i] == specPage
		}
		if listing[pages[i]] != listing[pages[j]] {
			return listing[pages[i]] < listing[pages[j]]
		}
		return pages[i] < pages[j]
	})
	others := func(chosen string) []string {
		var o []string
		for _, p := range pages {
			if p != chosen {
				o = append(o, p)
			}
		}
		return o
	}
	switch {
	case len(pages) > 0 && pages[0] == specPage:
		return CallRef{Page: specPage, Via: "verified", Others: others(specPage)}
	case len(pages) > 0:
		return CallRef{Page: pages[0], Via: "matched", Others: others(pages[0])}
	case specPage != "":
		return CallRef{Page: specPage, Via: "spec-only"}
	}
	return CallRef{}
}
