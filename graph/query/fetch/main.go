// Command fetch downloads the two include files of Microsoft that have the tables of what $filter and
// $orderby can do on each property of a directory object. It pins them to one commit, writes them to
// -out with SOURCE.json, and reads them into -matrix (matrix.json) with query.Parse. The tables are made
// by a program at Microsoft, so a program can read them. Parse fails on a form that it does not know.
//
// Run it with 'make update-query'.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/withholm/dolly-cli/graph/query"
	"github.com/withholm/dolly-cli/internal/ghraw"
)

const repo = "microsoftgraph/microsoft-graph-docs-contrib"

// the include files of concepts/aad-advanced-queries.md
var files = []string{"concepts/includes/directory-objects-filter.md", "concepts/includes/directory-objects-orderby.md"}

func main() {
	out := flag.String("out", "graph/query/raw", "The folder to write the documents to. Its old files are replaced.")
	matrix := flag.String("matrix", "graph/query/matrix.json", "The file to write the rules to.")
	flag.Parse()
	if err := run(*out, *matrix); err != nil {
		fmt.Fprintln(os.Stderr, "fetch:", err)
		os.Exit(1)
	}
}

func run(out, matrixFile string) error {
	sha, date, err := ghraw.Head(repo, "main")
	if err != nil {
		return err
	}
	bodies := map[string][]byte{}
	for _, p := range files {
		if bodies[p], err = ghraw.Raw(repo, sha, p); err != nil {
			return err
		}
	}
	m, err := query.Parse(string(bodies[files[0]]), string(bodies[files[1]]))
	if err != nil {
		return err
	}
	m.Commit = sha

	// replace the old files only when the download and the reading worked
	if err := os.RemoveAll(out); err != nil {
		return err
	}
	type file struct {
		Path   string `json:"path"`
		Bytes  int    `json:"bytes"`
		SHA256 string `json:"sha256"`
	}
	var list []file
	for _, p := range files {
		dst := filepath.Join(out, filepath.Base(p))
		if err := os.MkdirAll(out, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, bodies[p], 0o644); err != nil {
			return err
		}
		sum := sha256.Sum256(bodies[p])
		list = append(list, file{Path: p, Bytes: len(bodies[p]), SHA256: hex.EncodeToString(sum[:])})
	}
	meta, _ := json.MarshalIndent(map[string]any{
		"source":      "https://github.com/" + repo,
		"article":     "https://learn.microsoft.com/graph/aad-advanced-queries",
		"license":     "Documentation: CC BY 4.0 (the file LICENSE in that repo). Give credit to Microsoft when you use the data.",
		"commit":      sha,
		"commitDate":  date,
		"fetchedDate": time.Now().UTC().Format("2006-01-02"),
		"files":       list,
	}, "", "  ")
	if err := os.WriteFile(filepath.Join(out, "SOURCE.json"), append(meta, '\n'), 0o644); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", " ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(matrixFile, append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("Fetched %d files from commit %s (%s). The rules: %d user properties, %d group, %d servicePrincipal. Written to %s and %s\n",
		len(list), sha[:10], date, len(m.Filter["user"]), len(m.Filter["group"]), len(m.Filter["servicePrincipal"]), out, matrixFile)
	return nil
}
