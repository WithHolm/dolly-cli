package graph_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

// source makes a data folder with a seed and one change, and leaves it unlocked.
func source(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "graph")
	w, err := graph.Open(dir, graph.OpenOptions{Users: 120, Seed: 9, Profile: graph.AcmeProfile()})
	if err != nil {
		t.Fatal(err)
	}
	w.Rename("department", "Sales", "Sales-Global", person)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return dir
}

func read(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestOpenClone_HasTheDataAndLeavesTheSourceAlone(t *testing.T) {
	src := source(t)
	seed, state := read(t, src, "seed.json"), read(t, src, "state.jsonl")

	c, err := graph.OpenClone(src)
	if err != nil {
		t.Fatal(err)
	}
	if c.Dir() != "" {
		t.Errorf("a clone in memory has no folder: %q", c.Dir())
	}
	if c.Snapshot().Users != 120 || count(c, "Sales-Global") == 0 || count(c, "Sales") != 0 {
		t.Fatalf("the clone must have the seed and the change: %+v", c.Snapshot())
	}
	c.Rename("department", "Sales-Global", "Sales-Clone", person)
	if read(t, src, "seed.json") != seed || read(t, src, "state.jsonl") != state {
		t.Error("a change in the clone must not touch the source")
	}
}

func TestOpenClone_WorksWhileADollyHasTheSource(t *testing.T) {
	src := filepath.Join(t.TempDir(), "graph")
	live := openWorld(t, src, nil) // holds the lock
	live.Rename("department", "Sales", "Sales-Live", person)
	c, err := graph.OpenClone(src)
	if err != nil {
		t.Fatal(err)
	}
	if count(c, "Sales-Live") == 0 {
		t.Error("the clone must have the change of the running dolly")
	}
}

func TestOpenClone_TakesTheRootToo(t *testing.T) {
	src := source(t) // .../graph
	c, err := graph.OpenClone(filepath.Dir(src))
	if err != nil {
		t.Fatal(err)
	}
	if c.Snapshot().Users != 120 {
		t.Errorf("users = %d", c.Snapshot().Users)
	}
}

func TestOpenClone_NoSeedIsAnError(t *testing.T) {
	_, err := graph.OpenClone(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "seed.json") {
		t.Errorf("want an error that names seed.json: %v", err)
	}
}

func TestCopyData_MakesAFolderWithTheHistory(t *testing.T) {
	src := source(t)
	dst := filepath.Join(t.TempDir(), "copy", "graph")
	if err := graph.CopyData(src, dst); err != nil {
		t.Fatal(err)
	}
	w, err := graph.Open(dst, graph.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if count(w, "Sales-Global") == 0 || len(w.History()) != 1 {
		t.Errorf("the copy must have the change in its history: %d", len(w.History()))
	}
}

func TestCopyData_RefusesAFolderWithData(t *testing.T) {
	src := source(t)
	other := source(t)
	before := read(t, other, "seed.json")
	err := graph.CopyData(src, other)
	if err == nil || !strings.Contains(err.Error(), "already") {
		t.Errorf("want a refusal: %v", err)
	}
	if read(t, other, "seed.json") != before {
		t.Error("the folder with data must not change")
	}
}

func TestCopyData_CutLastLineDoesNotBreakTheNextChange(t *testing.T) {
	// a clone of a dolly that runs can catch a line that is half written
	src := source(t)
	f, err := os.OpenFile(filepath.Join(src, "state.jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(`{"seq":2,"type":"ren`)
	_ = f.Close()

	dst := filepath.Join(t.TempDir(), "graph")
	if err := graph.CopyData(src, dst); err != nil {
		t.Fatal(err)
	}
	w, err := graph.Open(dst, graph.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	w.Rename("department", "Sales-Global", "Sales-Again", person)
	_ = w.Close()
	w, err = graph.Open(dst, graph.OpenOptions{})
	if err != nil {
		t.Fatalf("the copy must open again after a change: %v", err)
	}
	defer w.Close()
	if count(w, "Sales-Again") == 0 {
		t.Error("the change after the copy was lost")
	}
}
