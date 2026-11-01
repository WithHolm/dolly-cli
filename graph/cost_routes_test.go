package graph_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
	"github.com/withholm/dolly-cli/graph/cost"
)

// The routes that the table of costs (graph/cost/cost.json, from Microsoft's documents) has no row for. They are priced by the default of their method, which
// is not a price that Microsoft publishes. This test never fails: it is the list to look at when you add a route (see docs/development.md, step 8).
// Run: go test ./graph -run TestCost_RoutesWithNoRow -v
func TestCost_RoutesWithNoRow(t *testing.T) {
	data, err := cost.Load()
	if err != nil {
		t.Fatal(err)
	}
	endpoints, _ := graph.New(graph.NewMemory(graph.Options{Users: 5, Seed: 1}), graph.ServerOptions{}).API()
	var other []string
	for _, e := range endpoints {
		price := data.Price(e.Method, strings.TrimPrefix(e.Path, graph.BasePath), url.Values{})
		if price.Route == strings.ToUpper(e.Method)+" (other)" {
			other = append(other, e.Method+" "+e.Path)
		}
	}
	t.Logf("%d of %d routes have no row in the table of costs, and cost the default of their method:", len(other), len(endpoints))
	for _, r := range other {
		t.Log("  " + r)
	}
}
