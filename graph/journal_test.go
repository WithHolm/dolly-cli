package graph_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/withholm/dolly-cli/graph"
)

// clock is a time that a test moves by hand.
type clock struct{ t time.Time }

func (c *clock) now() time.Time      { return c.t }
func (c *clock) add(d time.Duration) { c.t = c.t.Add(d) }

func newClock() *clock { return &clock{t: time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)} }

func openWorld(t *testing.T, dir string, c *clock) *graph.World {
	t.Helper()
	opt := graph.OpenOptions{Users: 120, Seed: 9}
	if c != nil {
		opt.Now = c.now
	}
	w, err := graph.Open(dir, opt)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

var person = graph.Actor{Source: "admin"}

// departmentOf finds the department of the first user with this name.
func count(w *graph.World, dept string) int {
	for _, d := range w.Snapshot().Departments {
		if d.Value == dept {
			return d.Count
		}
	}
	return 0
}

func TestOpen_MakesTheSeedAndTheStateFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "dolly")
	w := openWorld(t, dir, nil)
	if w.Dir() != dir {
		t.Errorf("dir = %q", w.Dir())
	}
	for _, f := range []string{"seed.json", "state.jsonl"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s must be there: %v", f, err)
		}
	}
	if w.Snapshot().Users != 120 {
		t.Errorf("users = %d", w.Snapshot().Users)
	}
	seed, _ := os.ReadFile(filepath.Join(dir, "seed.json"))
	w.Rename("department", "Sales", "Sales-Global", person)
	if after, _ := os.ReadFile(filepath.Join(dir, "seed.json")); string(after) != string(seed) {
		t.Error("the seed file never changes")
	}
}

func TestChanges_AreSavedAndComeBackAfterARestart(t *testing.T) {
	dir := t.TempDir()
	w := openWorld(t, dir, nil)
	n := w.Rename("department", "Sales", "Sales-Global", person)
	if n == 0 {
		t.Fatal("nobody in Sales")
	}
	if !w.SetUser(firstUPN(t, w), map[string]any{"jobTitle": "Chief of Fun"}, person) {
		t.Fatal("set user")
	}
	before := w.Snapshot()
	hist := w.History()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	again, err := graph.Open(dir, graph.OpenOptions{Users: 5, Seed: 5}) // the options do not matter: the seed file is there
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	after := again.Snapshot()
	if after.Users != before.Users || count(again, "Sales-Global") != n || count(again, "Sales") != 0 {
		t.Errorf("the world after a restart is not the one before: %+v vs %+v", after.Users, before.Users)
	}
	if got := again.History(); len(got) != len(hist) || got[0].Type != "rename" || got[1].Type != "user" {
		t.Errorf("history: %+v", got)
	}
}

func firstUPN(t *testing.T, w *graph.World) string {
	t.Helper()
	users, _ := w.SearchUsers("", 1)
	return users[0].UPN
}

func TestHistory_NamesWhoChangedWhat(t *testing.T) {
	dir := t.TempDir()
	w := openWorld(t, dir, nil)
	srv := graph.New(w, graph.ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	c := &client{BaseURL: ts.URL + graph.BasePath}
	ctx := context.Background()

	var groups struct{ Value []struct{ ID string } }
	if err := c.Get(ctx, "/groups?$filter=displayName%20eq%20'grp.app.acme.main'&$select=id", &groups); err != nil || len(groups.Value) == 0 {
		t.Fatalf("group: %+v %v", groups, err)
	}
	groupID := groups.Value[0].ID
	var members struct {
		Value []struct{ ID, DisplayName string }
	}
	if err := c.Get(ctx, "/groups/"+groupID+"/members?$select=id,displayName&$top=1", &members); err != nil || len(members.Value) == 0 {
		t.Fatalf("members: %+v %v", members, err)
	}
	user := members.Value[0]
	// a tool removes a user, through $batch (the way a tool does)
	parts, err := c.Batch(ctx, []batchRequest{{ID: "1", Method: "DELETE", URL: "/groups/" + groupID + "/members/" + user.ID + "/$ref"}})
	if err != nil || parts[0].Status >= 300 {
		t.Fatalf("remove: %+v %v", parts, err)
	}
	w.Rename("department", "Sales", "Sales-Global", person)

	var h struct {
		Events []struct {
			Seq   int
			Type  string
			Text  string
			Actor graph.Actor
		}
		Total int
		Kept  bool
	}
	getJSON(t, ts.URL+"/_dolly/history", &h)
	if !h.Kept || h.Total < 2 {
		t.Fatalf("history: %+v", h)
	}
	last := h.Events[0] // newest first
	if last.Type != "rename" || last.Actor.Source != "admin" || !strings.Contains(last.Text, "Sales-Global") {
		t.Errorf("newest: %+v", last)
	}
	var tool *struct {
		Seq   int
		Type  string
		Text  string
		Actor graph.Actor
	}
	for i := range h.Events {
		if h.Events[i].Type == "member.remove" {
			tool = &h.Events[i]
		}
	}
	if tool == nil || tool.Actor.Source != "graph" || !strings.HasPrefix(tool.Actor.Agent, "Go-http-client") || tool.Actor.Call == 0 {
		t.Fatalf("a change by a tool names the tool and the call: %+v", tool)
	}
	if !strings.Contains(tool.Text, "grp.app.acme.main") || !strings.Contains(tool.Text, user.DisplayName) {
		t.Errorf("text: %q", tool.Text)
	}
}

func getJSON(t *testing.T, url string, out any) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		t.Fatal(err)
	}
}

func TestRollback_ToATime_AndBackAgain(t *testing.T) {
	dir := t.TempDir()
	clk := newClock()
	w := openWorld(t, dir, clk)

	clk.add(time.Minute) // 08:01
	w.Rename("department", "Sales", "S1", person)
	clk.add(time.Hour) // 09:01
	w.Rename("department", "S1", "S2", person)
	clk.add(time.Hour) // 10:01
	w.Rename("department", "S2", "S3", person)
	if count(w, "S3") == 0 {
		t.Fatal("setup")
	}

	// "one hour ago", seen from 10:30, is 09:30: the state after the change at 09:01
	target := w.RollbackTarget(time.Date(2026, 10, 10, 9, 30, 0, 0, time.UTC))
	if target != 2 {
		t.Fatalf("target = %d, want 2", target)
	}
	clk.add(30 * time.Minute)
	if err := w.Rollback(target, person); err != nil {
		t.Fatal(err)
	}
	if count(w, "S2") == 0 || count(w, "S3") != 0 || count(w, "S1") != 0 {
		t.Errorf("after the rollback the department must be S2: %+v", w.Snapshot().Departments)
	}
	h := w.History()
	if len(h) != 4 || h[3].Type != "rollback" || h[3].Target != 2 {
		t.Fatalf("the rollback is a change in the history: %+v", h)
	}

	// it survives a restart
	_ = w.Close()
	again := openWorld(t, dir, clk)
	if count(again, "S2") == 0 || count(again, "S3") != 0 {
		t.Error("the rollback must stay after a restart")
	}

	// and it can be undone: go back to the state after change 3 (before the rollback)
	if err := again.Rollback(3, person); err != nil {
		t.Fatal(err)
	}
	if count(again, "S3") == 0 || count(again, "S2") != 0 {
		t.Errorf("going back past the rollback must bring S3 back: %+v", again.Snapshot().Departments)
	}
	// and to the seed
	if err := again.Rollback(0, person); err != nil {
		t.Fatal(err)
	}
	if count(again, "Sales") == 0 {
		t.Error("rollback 0 is the seed")
	}
	if err := again.Rollback(99, person); err == nil {
		t.Error("a number that is not in the history must fail")
	}
}

func TestReset_GoesBackToTheSeed_AndCanBeUndone(t *testing.T) {
	w := openWorld(t, t.TempDir(), nil)
	w.Rename("department", "Sales", "Sales-Global", person)
	if err := w.Reset(person); err != nil {
		t.Fatal(err)
	}
	if count(w, "Sales-Global") != 0 || count(w, "Sales") == 0 {
		t.Error("reset must go back to the seed")
	}
	if err := w.Rollback(1, person); err != nil {
		t.Fatal(err)
	}
	if count(w, "Sales-Global") == 0 {
		t.Error("a rollback to before the reset brings the change back")
	}
}

func TestMemoryWorld_HasNoHistory(t *testing.T) {
	w := graph.NewMemory(graph.Options{Users: 30, Seed: 2})
	w.Rename("department", "Sales", "X", person)
	if len(w.History()) != 0 || w.Dir() != "" {
		t.Error("a world in memory keeps no history")
	}
	if err := w.Rollback(0, person); err == nil {
		t.Error("a world in memory cannot go back")
	}
}

func TestOpen_OneDollyForAFolder(t *testing.T) {
	dir := t.TempDir()
	w := openWorld(t, dir, nil)
	w.SetAddress("http://127.0.0.1:8650")
	_, err := graph.Open(dir, graph.OpenOptions{Users: 10})
	if err == nil || !strings.Contains(err.Error(), "http://127.0.0.1:8650") || !strings.Contains(err.Error(), "Stop it") {
		t.Fatalf("a second open must fail and say where the first one listens: %v", err)
	}
	if url, ok := graph.Running(dir); !ok || url != "http://127.0.0.1:8650" {
		t.Errorf("Running must give the address of the dolly that holds the folder: %q %v", url, ok)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok := graph.Running(dir); ok {
		t.Error("after Close no dolly runs")
	}
	again, err := graph.Open(dir, graph.OpenOptions{Users: 10})
	if err != nil {
		t.Fatalf("after Close the folder is free: %v", err)
	}
	_ = again.Close()
}

// A dolly that crashed leaves its lock file. The next one must not be stuck.
func TestOpen_TakesOverALockThatNoProcessHolds(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows cannot tell if a process runs")
	}
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Skip("no true command")
	}
	dead := cmd.Process.Pid // the process has ended
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".lock"), []byte(`{"pid":`+strconv.Itoa(dead)+`}`), 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := graph.Open(dir, graph.OpenOptions{Users: 10})
	if err != nil {
		t.Fatalf("a lock of a process that is gone must be taken over: %v", err)
	}
	_ = w.Close()

	// a lock of a process that runs (this one) is respected
	w2 := openWorld(t, t.TempDir(), nil)
	if _, err := graph.Open(w2.Dir(), graph.OpenOptions{Users: 10}); err == nil {
		t.Error("a lock of a live process must stop a second open")
	}
}

func TestOpen_StateFileProblems(t *testing.T) {
	dir := t.TempDir()
	w := openWorld(t, dir, nil)
	w.Rename("department", "Sales", "A", person)
	w.Rename("department", "A", "B", person)
	_ = w.Close()
	path := filepath.Join(dir, "state.jsonl")
	good, _ := os.ReadFile(path)

	// a last line that is cut (a crash while it was written) is left out
	if err := os.WriteFile(path, append(good, []byte(`{"seq":3,"time":"2026-10-10T08:00:00Z","type":"ren`)...), 0o644); err != nil {
		t.Fatal(err)
	}
	again, err := graph.Open(dir, graph.OpenOptions{})
	if err != nil {
		t.Fatalf("a cut last line must be left out: %v", err)
	}
	if len(again.History()) != 2 || count(again, "B") == 0 {
		t.Errorf("history %d", len(again.History()))
	}
	_ = again.Close()

	// a bad line in the middle is an error: a person must look at it
	lines := strings.Split(strings.TrimSpace(string(good)), "\n")
	bad := lines[0] + "\nnot json\n" + lines[1] + "\n"
	_ = os.WriteFile(path, []byte(bad), 0o644)
	if _, err := graph.Open(dir, graph.OpenOptions{}); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("a bad line must fail and name the line: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".lock")); err == nil {
		t.Error("a failed open must release the folder")
	}
}

func TestAdminAPI_RollbackWays(t *testing.T) {
	w := openWorld(t, t.TempDir(), nil)
	srv := graph.New(w, graph.ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	w.Rename("department", "Sales", "A", person)
	w.Rename("department", "A", "B", person)
	post := func(body string) int {
		res, err := http.Post(ts.URL+"/_dolly/rollback", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if post(`{"before":2}`) != 200 || count(w, "A") == 0 || count(w, "B") != 0 {
		t.Errorf("before 2 is the state after change 1: %+v", w.Snapshot().Departments)
	}
	if post(`{"ago":"1h"}`) != 200 || count(w, "Sales") == 0 {
		t.Error("one hour ago is before every change: the seed")
	}
	if post(`{"seed":true}`) != 200 {
		t.Error("seed")
	}
	for _, bad := range []string{`{}`, `{"ago":"soon"}`, `{"to":"yesterday"}`, `{"before":99}`} {
		if s := post(bad); s != 400 {
			t.Errorf("%s gave %d, want 400", bad, s)
		}
	}
}
