// Command fetch downloads Microsoft's documents about Graph throttling: the
// article concepts/throttling-limits.md, and the include files that hold its
// tables. It pins them to one commit and writes them to -out, with SOURCE.json
// (the commit, the time, and a hash of each file). It does not read the tables.
// Reading them into cost.json is the job described in ../README.md.
//
// Run it with 'make update-cost'.
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
	"regexp"
	"sort"
	"time"

	"github.com/withholm/dolly-cli/internal/ghraw"
)

const (
	repo    = "microsoftgraph/microsoft-graph-docs-contrib"
	article = "concepts/throttling-limits.md"
)

// includeRe finds the links to the include files: [!INCLUDE [title](../includes/x.md)]
var includeRe = regexp.MustCompile(`\[!INCLUDE \[[^\]]*\]\(([^)]+\.md)\)\]`)

type file struct {
	Path   string `json:"path"` // in the repo of Microsoft
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

func main() {
	out := flag.String("out", "graph/cost/raw", "The folder to write to. Its old files are replaced.")
	flag.Parse()
	if err := run(*out); err != nil {
		fmt.Fprintln(os.Stderr, "fetch:", err)
		os.Exit(1)
	}
}

func run(out string) error {
	sha, date, err := ghraw.Head(repo, "main")
	if err != nil {
		return err
	}
	raw := func(p string) ([]byte, error) { return ghraw.Raw(repo, sha, p) }

	art, err := raw(article)
	if err != nil {
		return err
	}
	// the links are relative to the folder of the article: ../includes/x.md
	var paths []string
	seen := map[string]bool{}
	for _, m := range includeRe.FindAllSubmatch(art, -1) {
		p := path.Join(path.Dir(article), string(m[1]))
		if !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	if len(paths) == 0 {
		return fmt.Errorf("%s has no include links. The article has changed its form: update this tool", article)
	}
	sort.Strings(paths)

	bodies := map[string][]byte{article: art}
	for _, p := range paths {
		if bodies[p], err = raw(p); err != nil {
			return err
		}
	}

	// replace the old files only when every download worked
	if err := os.RemoveAll(out); err != nil {
		return err
	}
	var files []file
	for p, b := range bodies {
		dst := filepath.Join(out, filepath.FromSlash(p))
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

	meta, _ := json.MarshalIndent(map[string]any{
		"source":      "https://github.com/" + repo,
		"license":     "Documentation: CC BY 4.0 (the file LICENSE in that repo). Give credit to Microsoft when you use the data.",
		"commit":      sha,
		"commitDate":  date,
		"fetchedDate": time.Now().UTC().Format("2006-01-02"),
		"files":       files,
	}, "", "  ")
	if err := os.WriteFile(filepath.Join(out, "SOURCE.json"), append(meta, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("Fetched %d files from commit %s (%s) into %s\n", len(files), sha[:10], date, out)
	return nil
}
