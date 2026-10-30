// Package ghraw downloads files from a public GitHub repo at one commit. The tools that make
// the data of dolly from Microsoft's documents use it (graph/cost/fetch, graph/permissions/fetch).
package ghraw

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

var client = &http.Client{Timeout: 30 * time.Second}

// Get downloads a URL.
func Get(url string, header map[string]string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: status %d", url, res.StatusCode)
	}
	return io.ReadAll(res.Body)
}

// Head gives the latest commit of a branch, so all the files are read from the same one.
// GitHub limits the calls that have no token: GITHUB_TOKEN is used when it is set.
func Head(repo, branch string) (sha, date string, err error) {
	h := map[string]string{"Accept": "application/vnd.github+json"}
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		h["Authorization"] = "Bearer " + t
	}
	data, err := Get("https://api.github.com/repos/"+repo+"/commits/"+branch, h)
	if err != nil {
		return "", "", fmt.Errorf("cannot find the latest commit (GitHub may limit calls without GITHUB_TOKEN): %w", err)
	}
	var c struct {
		SHA    string `json:"sha"`
		Commit struct {
			Committer struct{ Date string }
		}
	}
	if err := json.Unmarshal(data, &c); err != nil || c.SHA == "" {
		return "", "", fmt.Errorf("cannot read the answer about the latest commit: %v", err)
	}
	return c.SHA, c.Commit.Committer.Date, nil
}

// Raw downloads one file of the repo at a commit.
func Raw(repo, sha, path string) ([]byte, error) {
	return Get("https://raw.githubusercontent.com/"+repo+"/"+sha+"/"+path, nil)
}
