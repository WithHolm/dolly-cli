package graph_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

// The help of a query option is a promise. Every example in it is sent to a server, so a help that is
// not true is a failing test.
func TestQueryOptionUsage_EveryExampleWorks(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	get := func(path string) (int, string) {
		req, _ := http.NewRequest("GET", ts.URL+path, nil)
		req.Header.Set("Authorization", "Bearer x")
		req.Header.Set("ConsistencyLevel", "eventual") // the notes in the help say when it is needed
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}

	// a user that has a manager, and a user that has direct reports: an example runs on a user that has what it expands
	var list struct {
		Value []struct {
			ID            string           `json:"id"`
			Manager       map[string]any   `json:"manager"`
			DirectReports []map[string]any `json:"directReports"`
		}
	}
	userWith := map[string]string{}
	for _, relation := range []string{"manager", "directReports"} {
		_, body := get("/v1.0/users?$expand=" + relation + "&$top=999")
		if err := json.Unmarshal([]byte(body), &list); err != nil {
			t.Fatal(err)
		}
		for _, u := range list.Value {
			if (relation == "manager" && u.Manager != nil) || (relation == "directReports" && len(u.DirectReports) > 0) {
				userWith[relation] = u.ID
				break
			}
		}
		if userWith[relation] == "" {
			t.Fatalf("no user with %s in the data", relation)
		}
	}

	tested := 0
	for _, o := range graph.QueryOptions {
		for _, u := range o.Usage {
			if !strings.HasPrefix(u.Syntax, o.Name+"=") || u.Note == "" || u.Example == "" {
				t.Errorf("%s: the form %+v needs a syntax that starts with the option, a note, and an example", o.Name, u)
				continue
			}
			relation, _, _ := strings.Cut(u.Example, "(")
			code, body := get("/v1.0/users/" + userWith[relation] + "?" + url.PathEscape(o.Name) + "=" + u.Example)
			if code != 200 || !strings.Contains(body, `"`+relation+`"`) {
				t.Errorf("%s: the example %q gives %d %s", o.Name, u.Example, code, body)
			}
			tested++
		}
	}
	if tested < 6 {
		t.Errorf("only %d examples were tested", tested)
	}

	// what the help says is refused is refused
	for _, bad := range []string{"memberOf", "manager($filter=a)", "directReports($filter=a)", "directReports($levels=2)", "manager($levels=0)", "manager($levels=1001)"} {
		if code, _ := get("/v1.0/users/" + userWith["manager"] + "?$expand=" + bad); code != 400 {
			t.Errorf("$expand=%s must be 400, got %d", bad, code)
		}
	}
	// and $levels is for one user, not for the list
	if code, _ := get("/v1.0/users?$expand=manager($levels=2)&$top=1"); code != 400 {
		t.Errorf("$levels on the list must be 400, got %d", code)
	}
}
