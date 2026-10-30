// Command fetch downloads the list of the permissions of Microsoft Graph: the file
// permissions/permissions-descriptions.json of the repo microsoft-graph-devx-content. It pins
// it to one commit and writes it to -out, with SOURCE.json (the commit, the time, and a hash of
// the file). The list is read as it is, so no one has to make a file from it.
//
// Run it with 'make update-permissions'.
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

	"github.com/withholm/dolly-cli/internal/ghraw"
)

const (
	repo   = "microsoftgraph/microsoft-graph-devx-content"
	branch = "master"
	file   = "permissions/permissions-descriptions.json"
)

func main() {
	out := flag.String("out", "graph/permissions/raw", "The folder to write to. Its old files are replaced.")
	flag.Parse()
	if err := run(*out); err != nil {
		fmt.Fprintln(os.Stderr, "fetch:", err)
		os.Exit(1)
	}
}

func run(out string) error {
	sha, date, err := ghraw.Head(repo, branch)
	if err != nil {
		return err
	}
	data, err := ghraw.Raw(repo, sha, file)
	if err != nil {
		return err
	}
	var check struct {
		Delegated   []json.RawMessage `json:"delegatedScopesList"`
		Application []json.RawMessage `json:"applicationScopesList"`
	}
	if err := json.Unmarshal(data, &check); err != nil || len(check.Delegated) == 0 || len(check.Application) == 0 {
		return fmt.Errorf("%s has changed its form (no delegatedScopesList or applicationScopesList): update this tool", file)
	}

	// replace the old files only when the download worked
	if err := os.RemoveAll(out); err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	name := filepath.Base(file)
	if err := os.WriteFile(filepath.Join(out, name), data, 0o644); err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	meta, _ := json.MarshalIndent(map[string]any{
		"source":      "https://github.com/" + repo,
		"license":     "MIT (the file LICENSE in that repo). Copyright (c) 2020 Microsoft Graph.",
		"commit":      sha,
		"commitDate":  date,
		"fetchedDate": time.Now().UTC().Format("2006-01-02"),
		"file":        map[string]any{"path": file, "bytes": len(data), "sha256": hex.EncodeToString(sum[:])},
		"delegated":   len(check.Delegated),
		"application": len(check.Application),
	}, "", "  ")
	if err := os.WriteFile(filepath.Join(out, "SOURCE.json"), append(meta, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("Fetched %d delegated and %d application permissions from commit %s (%s) into %s\n",
		len(check.Delegated), len(check.Application), sha[:10], date, out)
	return nil
}
