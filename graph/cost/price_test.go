package cost_test

import (
	"net/url"
	"testing"

	"github.com/withholm/dolly-cli/graph/cost"
)

func TestPrice(t *testing.T) {
	d, err := cost.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		method, path, query string
		ru, write           int
	}{
		{"GET", "/users", "", 2, 0},
		{"GET", "users", "", 2, 0},
		{"GET", "/users", "$select=id", 1, 0},
		{"GET", "/users", "$top=5", 1, 0},
		{"GET", "/users", "$top=50", 2, 0},
		{"GET", "/users", "$top=5&$select=id", 1, 0}, // 2 - 1 - 1 is below the minimum
		{"GET", "/groups/abc/members", "", 3, 0},
		{"GET", "/groups/abc/members", "$expand=x", 4, 0},
		{"GET", "/groups/abc/transitiveMembers", "", 5, 0},
		{"POST", "/directoryObjects/getByIds", "", 5, 0},
		{"POST", "/directoryObjects/getByIds", "$select=id", 2, 0},
		{"GET", "/me/memberOf", "", 2, 0},
		{"GET", "/users/abc/memberOf", "", 2, 0}, // the cost of me/ is the cost of users/{id}/
		{"GET", "/users/abc@x.com/transitiveMemberOf", "", 2, 0},
		{"GET", "/users/abc", "", 1, 0}, // users/{id} is not in the table
		{"GET", "/organization", "", 1, 0},
		{"POST", "/groups", "", 1, 1},
		{"PATCH", "/users/abc", "", 1, 1},
		{"DELETE", "/groups/abc", "", 1, 1},
		{"GET", "/servicePrincipals/abc/appRoleAssignments", "", 2, 0},
	} {
		q, _ := url.ParseQuery(c.query)
		if p := d.Price(c.method, c.path, q); p.RU != c.ru || p.Write != c.write {
			t.Errorf("%s %s?%s: cost %d and write %d, want %d and %d (%s)", c.method, c.path, c.query, p.RU, p.Write, c.ru, c.write, p.Route)
		}
	}
}

func TestPrice_NamesTheRow(t *testing.T) {
	d, _ := cost.Load()
	if p := d.Price("GET", "/users", nil); p.Route != "GET users" {
		t.Errorf("route: %q", p.Route)
	}
	if p := d.Price("GET", "/users/abc/memberOf", nil); p.Route != "GET me/memberOf" {
		t.Errorf("route: %q", p.Route)
	}
	if p := d.Price("GET", "/organization", nil); p.Route != "GET (other)" {
		t.Errorf("route: %q", p.Route)
	}
}
