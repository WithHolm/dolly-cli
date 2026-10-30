package graph_test

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/withholm/dolly-cli/graph"
)

// callAs sends a Graph call with a token that has this application ID in it (the token is not checked, as any token is accepted).
func callAs(h http.Handler, appID, path string) *httptest.ResponseRecorder {
	enc := base64.RawURLEncoding.EncodeToString
	token := enc([]byte(`{"alg":"none"}`)) + "." + enc([]byte(`{"appid":"`+appID+`"}`)) + "."
	req := httptest.NewRequest("GET", "http://127.0.0.1"+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// The portal is a client of the Graph API. It is in every world, with the same IDs, and it is left out of the throttle and the list of calls.
func TestPortal_IsInEveryWorld(t *testing.T) {
	var ids []string
	for _, seed := range []int64{1, 2} {
		w := graph.Generate(graph.Options{Users: 10, Seed: seed})
		sp, ok := w.FindServicePrincipal(graph.PortalAppID)
		if !ok || sp.Name != "Dolly portal" || !sp.ExemptFromThrottle || !sp.HideCalls || !sp.System || sp.Builtin != "" || sp.Auto {
			t.Fatalf("seed %d: %+v %v", seed, sp, ok)
		}
		ids = append(ids, sp.ID)
	}
	if ids[0] != ids[1] {
		t.Errorf("the object ID must be the same in every world: %v", ids)
	}
	// the clients of Microsoft are there, with their well-known client IDs, and nothing is hidden or exempt
	w := graph.Generate(graph.Options{Users: 10, Seed: 1})
	for _, c := range []string{"14d82eec-204b-4c2f-b7e8-296a70dab67e", "04b07795-8ddb-461a-bbee-02f9e1bf7b46"} {
		if sp, ok := w.FindServicePrincipal(c); !ok || !sp.System || sp.ExemptFromThrottle || sp.HideCalls {
			t.Errorf("%s: %+v %v", c, sp, ok)
		}
	}
	// the other data does not change because of it: the same seed gives the same users
	a := graph.Generate(graph.Options{Users: 10, Seed: 1}).ListUsers()
	if len(a) != 10 {
		t.Errorf("users: %d", len(a))
	}
}

func TestClientFlags_ExemptFromThrottleAndHideCalls(t *testing.T) {
	const quiet = "11111111-2222-3333-4444-555555555555"
	const plain = "66666666-7777-8888-9999-aaaaaaaaaaaa"
	srv := graph.New(graph.Generate(graph.Options{Users: 10, Seed: 1}), graph.ServerOptions{Throttle: true})
	h := srv.Handler()
	for _, id := range []string{quiet, plain} {
		if _, e := srv.World().EnsureServicePrincipal(id, graph.Actor{}); e != nil {
			t.Fatal(e)
		}
	}

	// 1750 calls of 2 RU is the limit of this small tenant (3500 RU in 10 seconds): the next one is throttled
	for range 1750 {
		callAs(h, plain, "/v1.0/users")
	}
	if rec := callAs(h, plain, "/v1.0/users"); rec.Code != 429 {
		t.Fatalf("a client with no flag: %d, want 429", rec.Code)
	}

	// the flags are set with the admin API, and only the ones in the body change
	if rec := send(h, "PUT", "/_dolly/service-principals/"+quiet+"/flags", `{"exemptFromThrottle":true}`); rec.Code != 204 && rec.Code != 200 {
		t.Fatalf("set the flag: %d %s", rec.Code, rec.Body)
	}
	if rec := send(h, "PUT", "/_dolly/service-principals/"+quiet+"/flags", `{}`); rec.Code != 400 {
		t.Errorf("no flag in the body: %d, want 400", rec.Code)
	}
	if rec := send(h, "PUT", "/_dolly/service-principals/"+plain+"/flags", `{"exemptFromThrottle":"yes"}`); rec.Code != 400 {
		t.Errorf("a flag that is not true or false: %d, want 400", rec.Code)
	}
	if rec := send(h, "PUT", "/_dolly/service-principals/"+graph.FakeTenantID+"/flags", `{"hideCalls":true}`); rec.Code != 404 {
		t.Errorf("a client that is not there: %d, want 404", rec.Code)
	}
	var list struct {
		ServicePrincipals []struct {
			AppID              string
			ExemptFromThrottle bool
			HideCalls          bool
		}
	}
	decode(t, send(h, "GET", "/_dolly/service-principals", "").Body.String(), &list)
	for _, sp := range list.ServicePrincipals {
		if sp.AppID == graph.PortalAppID {
			continue // it has both flags from the start (see TestPortal_IsInEveryWorld)
		}
		if want := sp.AppID == quiet; sp.ExemptFromThrottle != want || sp.HideCalls {
			t.Errorf("%s: %+v", sp.AppID, sp)
		}
	}

	// the same tenant is over its limit, and the exempt client is still let through
	for range 2000 {
		if rec := callAs(h, quiet, "/v1.0/users"); rec.Code != 200 {
			t.Fatalf("an exempt client: %d", rec.Code)
		}
	}
	if rec := callAs(h, plain, "/v1.0/users"); rec.Code != 429 {
		t.Errorf("the other client is still throttled: %d", rec.Code)
	}

	// hidden calls: not in the list, also not inside a $batch. The cost is still counted.
	send(h, "DELETE", "/_dolly/requests", "")
	send(h, "PUT", "/_dolly/service-principals/"+quiet+"/flags", `{"hideCalls":true}`)
	callAs(h, quiet, "/v1.0/organization")
	callAs(h, plain, "/v1.0/groups")
	body := send(h, "GET", "/_dolly/requests", "").Body.String()
	if strings.Contains(body, "/v1.0/organization") || !strings.Contains(body, "/v1.0/groups") {
		t.Errorf("the list of calls: %s", body)
	}
	var usage struct {
		ServicePrincipals []struct {
			AppID string
			Calls int
		}
	}
	decode(t, send(h, "GET", "/_dolly/usage", "").Body.String(), &usage)
	for _, u := range usage.ServicePrincipals {
		if u.AppID == quiet && u.Calls < 2001 {
			t.Errorf("a hidden client is still counted: %+v", u)
		}
	}
}
