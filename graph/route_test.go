package graph_test

import (
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

func TestDescribe_ServedRoute(t *testing.T) {
	// the same route in three forms: with /v1.0, with other {names}, and with a lower-case method
	for _, in := range [][2]string{
		{"GET", "/users/{id}"},
		{"get", "/v1.0/users/{user-id}"},
		{"GET", "users/{x}"},
	} {
		r := graph.Describe(in[0], in[1])
		if !r.Served || r.Route != "GET /v1.0/users/{id}" {
			t.Fatalf("%v: served=%v route=%q", in, r.Served, r.Route)
		}
		if r.Summary == "" || r.Learn == "" {
			t.Errorf("%v: no summary or Learn link: %+v", in, r)
		}
		if !r.Permissions.Table || len(r.Permissions.Delegated.Least) == 0 {
			t.Errorf("%v: no permissions: %+v", in, r.Permissions)
		}
		if r.RU < 1 {
			t.Errorf("%v: RU %d", in, r.RU)
		}
	}
}

func TestDescribe_InSpecNotServed(t *testing.T) {
	// Graph has it, dollycli does not
	r := graph.Describe("GET", "/me/messages")
	if r.Served || r.Summary == "" {
		t.Fatalf("want in the spec and not served: %+v", r)
	}
}

func TestDescribe_Unknown(t *testing.T) {
	r := graph.Describe("GET", "/users/{id}/nothing")
	if r.Served || r.Summary != "" {
		t.Fatalf("%+v", r)
	}
	if len(r.Nearest) == 0 {
		t.Error("an unknown route under /users should name routes that are near")
	}
}
