package cost_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph/cost"
)

// The cost data is made from raw/ by a person or by Claude (see README.md). These tests
// find the mistakes that a machine can find.

func TestCost_FollowsTheTypes(t *testing.T) {
	// Load refuses a field that the types do not have, and a wrong kind of value.
	if _, err := cost.Load(); err != nil {
		t.Fatal(err)
	}
	if _, err := cost.Parse([]byte(`{"nope": 1}`)); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("a field that the types do not know must be refused: %v", err)
	}
}

func TestCost_IsFromTheRawCommit(t *testing.T) {
	c, _ := cost.Load()
	var src struct{ Commit string }
	data, err := os.ReadFile("raw/SOURCE.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &src); err != nil {
		t.Fatal(err)
	}
	if c.Source.Commit != src.Commit {
		t.Errorf("cost.json is from commit %q, and raw/ is from %q: make cost.json again (see README.md)", c.Source.Commit, src.Commit)
	}
}

// | GET | `users` | 2 | 0 |
var row = regexp.MustCompile("(?m)^\\| (GET|POST|PUT|PATCH|DELETE) \\| `([^`]+)` \\| (\\d+) \\| (\\d+) \\|")

func TestCost_BaseCostsAreTheRowsOfTheIdentityTable(t *testing.T) {
	c, _ := cost.Load()
	data, err := os.ReadFile("raw/includes/throttling-identity-and-access.md")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][2]int{}
	for _, m := range row.FindAllStringSubmatch(string(data), -1) {
		ru, _ := strconv.Atoi(m[3])
		w, _ := strconv.Atoi(m[4])
		want[m[1]+" "+m[2]] = [2]int{ru, w}
	}
	if len(want) < 20 {
		t.Fatalf("only %d rows in the identity table: has its form changed?", len(want))
	}
	got := map[string][2]int{}
	for _, b := range c.Identity.BaseCosts {
		k := b.Method + " " + b.Path
		if _, dup := got[k]; dup {
			t.Errorf("%s is twice in baseCosts", k)
		}
		got[k] = [2]int{b.RU, b.Write}
	}
	for k, v := range want {
		if g, ok := got[k]; !ok {
			t.Errorf("%s is in the table and not in cost.json", k)
		} else if g != v {
			t.Errorf("%s: cost.json has cost %d and write %d, the table has %d and %d", k, g[0], g[1], v[0], v[1])
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("%s is in cost.json and not in the table", k)
		}
	}
}

func TestCost_EveryIncludeFileIsAService(t *testing.T) {
	c, _ := cost.Load()
	have := map[string]bool{}
	for _, s := range c.Services {
		have[s.Include] = true
	}
	files, err := filepath.Glob("raw/includes/*.md")
	if err != nil || len(files) == 0 {
		t.Fatalf("no include files in raw/: %v", err)
	}
	for _, f := range files {
		if name := filepath.Base(f); !have[name] && name != "throttling-identity-and-access.md" {
			t.Errorf("%s is in raw/ and not in services", name)
		}
	}
}

func TestCost_EverySpanCanBeRead(t *testing.T) {
	c, _ := cost.Load()
	spans := []string{c.Global.Per}
	for _, l := range c.Identity.Limits {
		spans = append(spans, l.Per, l.WritesPer)
	}
	for _, s := range c.Services {
		for _, l := range s.Limits {
			spans = append(spans, l.Per)
		}
	}
	for _, sp := range spans {
		if sp == "" || sp == "1 month" {
			continue // not a span of time: the limit of meetings for each month
		}
		if _, err := cost.ParseDuration(sp); err != nil {
			t.Error(err)
		}
	}
}
