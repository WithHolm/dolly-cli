package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

// baseline makes a data root (root/graph) the way 'dolly graph generate' does.
func baseline(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "base")
	w, err := graph.Open(filepath.Join(root, "graph"), graph.OpenOptions{Users: 7, Seed: 3})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestOpenGraph_CloneInMemory(t *testing.T) {
	base := baseline(t)
	s := settings{cloneFrom: base, users: 500, seed: 1}
	api, closeData, err := openGraph(&s, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer closeData()
	w := api.(*graph.Server).World()
	if w.Snapshot().Users != 7 || w.Dir() != "" {
		t.Errorf("want the 7 users of the baseline, in memory: users=%d dir=%q", w.Snapshot().Users, w.Dir())
	}
}

func TestOpenGraph_CloneToAFolder(t *testing.T) {
	base := baseline(t)
	dir := filepath.Join(t.TempDir(), "mine", "graph")
	s := settings{cloneFrom: base, dir: dir, users: 500, seed: 1}
	api, closeData, err := openGraph(&s, nil)
	if err != nil {
		t.Fatal(err)
	}
	w := api.(*graph.Server).World()
	if w.Snapshot().Users != 7 || w.Dir() != dir {
		t.Errorf("users=%d dir=%q", w.Snapshot().Users, w.Dir())
	}
	closeData()

	// a folder that has data is never replaced
	if _, _, err := openGraph(&s, nil); err == nil || !strings.Contains(err.Error(), "already") {
		t.Errorf("the second clone into the same folder must be refused: %v", err)
	}
}

func TestOpenGraph_CloneOfNothingIsAnError(t *testing.T) {
	s := settings{cloneFrom: t.TempDir(), users: 5, seed: 1}
	if _, _, err := openGraph(&s, nil); err == nil || !strings.Contains(err.Error(), "seed.json") {
		t.Errorf("the error: %v", err)
	}
}

// --clone-from with a flag that says how to make new data is an error, and with no --dir the clone is in memory.
func TestServe_CloneFlags(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "dolly")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	base := baseline(t)
	cwd := t.TempDir() // the default folder dolly-data would be made here
	for _, flag := range []string{"--users=5", "--seed=2", "--profile=x.json"} {
		cmd := exec.Command(bin, "serve", "--clone-from", base, flag)
		cmd.Dir = cwd
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), "--clone-from") || !strings.Contains(string(out), strings.SplitN(flag, "=", 2)[0]) {
			t.Errorf("%s: want a refusal that names both flags: %v\n%s", flag, err, out)
		}
	}
	// a port that is taken stops dolly after the checks, and still must not make dolly-data
	cmd := exec.Command(bin, "serve", "--clone-from", base, "--port", "1")
	cmd.Dir = cwd
	_, _ = cmd.CombinedOutput()
	if _, err := os.Stat(filepath.Join(cwd, "dolly-data")); !os.IsNotExist(err) {
		t.Error("with --clone-from and no --dir the data is in memory: dolly-data must not be made")
	}
}
