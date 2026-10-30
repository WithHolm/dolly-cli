package graph_test

import (
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

// Replace makes new data in the folder: a new seed, and no history. The old changes are gone.
func TestReplace_NewSeedAndNoHistory(t *testing.T) {
	dir := t.TempDir()
	w, err := graph.Open(dir, graph.OpenOptions{Users: 20, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	w.Rename("department", "Sales", "Selling", graph.Actor{})
	if len(w.History()) == 0 {
		t.Fatal("the test needs a change in the history")
	}
	_ = w.Close()

	st, err := graph.Replace(dir, graph.OpenOptions{Users: 50, Seed: 9})
	if err != nil || st.Users != 50 || st.Seed != 9 {
		t.Fatalf("Replace: %+v %v", st, err)
	}

	again, err := graph.Open(dir, graph.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if got := again.Snapshot(); got.Users != 50 || got.Seed != 9 || len(again.History()) != 0 {
		t.Errorf("after Replace: %+v, %d changes. Want 50 users, seed 9, no history", got, len(again.History()))
	}
	// the same seed makes the same data
	st2, err := graph.Replace(t.TempDir(), graph.OpenOptions{Users: 50, Seed: 9})
	if err != nil || st2.Users != st.Users || len(st2.Departments) != len(st.Departments) {
		t.Errorf("the same seed must make the same data: %+v %v", st2, err)
	}
}

// A folder that a dolly uses is not replaced.
func TestReplace_RefusesAFolderInUse(t *testing.T) {
	dir := t.TempDir()
	w, err := graph.Open(dir, graph.OpenOptions{Users: 10, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if _, err := graph.Replace(dir, graph.OpenOptions{Users: 5}); err == nil {
		t.Error("Replace must fail while a dolly has the folder")
	}
	if w.Snapshot().Users != 10 {
		t.Error("the world in use changed")
	}
}

// A folder with no data is just made.
func TestReplace_NoDataYet(t *testing.T) {
	st, err := graph.Replace(t.TempDir()+"/new/dolly", graph.OpenOptions{Users: 7, Seed: 2})
	if err != nil || st.Users != 7 {
		t.Errorf("%+v %v", st, err)
	}
}
