package graph_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/withholm/dolly-cli/graph"
)

// A subscriber: it answers the validation request, and keeps the notifications that it gets.
type subscriber struct {
	*httptest.Server
	mu    sync.Mutex
	items []map[string]any
}

func newSubscriber(t *testing.T) *subscriber {
	s := &subscriber{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tok := r.URL.Query().Get("validationToken"); tok != "" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = io.WriteString(w, tok)
			return
		}
		var body struct {
			Value []map[string]any `json:"value"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		s.mu.Lock()
		s.items = append(s.items, body.Value...)
		s.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *subscriber) got() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]map[string]any(nil), s.items...)
}

func (s *subscriber) types() []string {
	var out []string
	for _, n := range s.got() {
		out = append(out, n["changeType"].(string))
	}
	return out
}

func subscribe(t *testing.T, ts *httptest.Server, url, resource, changeType string) (int, map[string]any) {
	t.Helper()
	exp := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	return graphCall(t, ts, "POST", "/subscriptions", `{"changeType":"`+changeType+`","notificationUrl":"`+url+`/hook","resource":"`+resource+`","expirationDateTime":"`+exp+`","clientState":"cs-1"}`)
}

const newUserBody = `{"accountEnabled":true,"displayName":"Sub Test","mailNickname":"sub.test","userPrincipalName":"sub.test@acme.example","passwordProfile":{"password":"xWwvJ]6NMw+bWH-d","forceChangePasswordNextSignIn":false}}`

// The whole way: a client subscribes to users, and a user that is made, changed, and deleted through the API is a notification for each.
func TestSubscriptions_AChangeThroughTheAPIIsANotification(t *testing.T) {
	ts, _, srv := startServer(t, graph.ServerOptions{})
	sub := newSubscriber(t)
	status, made := subscribe(t, ts, sub.URL, "users", "created,updated,deleted")
	if status != 201 || made["id"] == nil || made["resource"] != "users" || made["clientState"] != "cs-1" || made["@odata.context"] == nil {
		t.Fatalf("create: %d %v", status, made)
	}
	if s, got := graphCall(t, ts, "GET", "/subscriptions/"+made["id"].(string), ""); s != 200 || got["id"] != made["id"] {
		t.Errorf("get: %d %v", s, got)
	}
	if s, list := graphCall(t, ts, "GET", "/subscriptions", ""); s != 200 || len(listOf(list)) != 1 {
		t.Errorf("list: %d %v", s, list)
	}

	_, user := graphCall(t, ts, "POST", "/users", newUserBody)
	id := user["id"].(string)
	graphCall(t, ts, "PATCH", "/users/"+id, `{"jobTitle":"Tester"}`)
	graphCall(t, ts, "DELETE", "/users/"+id, "")
	srv.FlushNotifications()

	got := sub.got()
	if want := []string{"created", "updated", "deleted"}; strings.Join(sub.types(), ",") != strings.Join(want, ",") {
		t.Fatalf("the change types: %v, want %v", sub.types(), want)
	}
	n := got[0]
	data := n["resourceData"].(map[string]any)
	if n["subscriptionId"] != made["id"] || n["clientState"] != "cs-1" || n["resource"] != "Users/"+id || data["id"] != id || data["@odata.type"] != "#Microsoft.Graph.User" || n["tenantId"] != srv.World().TenantID() {
		t.Errorf("the notification: %v", n)
	}

	// the portal reads the subscriptions and the deliveries from the admin API
	var admin struct {
		Subscriptions []map[string]any `json:"subscriptions"`
		Deliveries    []map[string]any `json:"deliveries"`
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "http://127.0.0.1/_dolly/subscriptions", nil)) // the admin API takes the host of this machine only
	if err := json.Unmarshal(rec.Body.Bytes(), &admin); err != nil || len(admin.Subscriptions) != 1 || len(admin.Deliveries) != 3 || admin.Deliveries[0]["status"] != float64(202) {
		t.Errorf("admin: %v %s", err, rec.Body)
	}
}

func TestSubscriptions_OneUserOneGroupAndTheLimits(t *testing.T) {
	ts, _, srv := startServer(t, graph.ServerOptions{})
	_, user := graphCall(t, ts, "POST", "/users", newUserBody)
	id := user["id"].(string)
	forUser, forGroups, other := newSubscriber(t), newSubscriber(t), newSubscriber(t)
	subscribe(t, ts, forUser.URL, "users/"+id, "updated")
	subscribe(t, ts, forGroups.URL, "groups", "created,updated,deleted")
	subscribe(t, ts, other.URL, "users/11111111-2222-3333-4444-555555555555", "updated")

	graphCall(t, ts, "PATCH", "/users/"+id, `{"jobTitle":"A"}`)
	_, group := graphCall(t, ts, "POST", "/groups", `{"displayName":"Sub group","mailNickname":"subgroup","mailEnabled":false,"securityEnabled":true}`)
	graphCall(t, ts, "PATCH", "/groups/"+group["id"].(string), `{"description":"d"}`)
	graphCall(t, ts, "DELETE", "/groups/"+group["id"].(string), "")
	srv.FlushNotifications()
	if len(forUser.got()) != 1 || strings.Join(forGroups.types(), ",") != "created,updated,deleted" || len(other.got()) != 0 {
		t.Errorf("user %d, groups %v, another user %d", len(forUser.got()), forGroups.types(), len(other.got()))
	}
	if n := forGroups.got()[0]; n["resource"] != "Groups/"+group["id"].(string) || n["resourceData"].(map[string]any)["@odata.type"] != "#Microsoft.Graph.Group" {
		t.Errorf("a group: %v", n)
	}

	// what dollycli does not send a notification for: a resource that is not supported, and the members of a group (a later step)
	for _, res := range []string{"servicePrincipals", "applications", "groups/1/members", "me/messages"} {
		if s, out := subscribe(t, ts, forUser.URL, res, "updated"); s != 400 || out["error"].(map[string]any)["code"] != "ValidationError" {
			t.Errorf("%s: %d %v", res, s, out)
		}
	}
	if s, _ := graphCall(t, ts, "POST", "/subscriptions", `{"changeType":"updated"}`); s != 400 {
		t.Errorf("an incomplete request: %d", s)
	}
}

// With duplicatePercent at 100, every change is sent 2 to 4 times with the same message. At 0 (as it comes) it is sent once.
func TestSubscriptions_DuplicatesFollowTheBehavior(t *testing.T) {
	ts, _, srv := startServer(t, graph.ServerOptions{})
	sub := newSubscriber(t)
	subscribe(t, ts, sub.URL, "users", "created,updated,deleted")
	admin(t, ts, "PUT", "/behavior", `{"duplicatePercent":100}`, nil)
	_, user := graphCall(t, ts, "POST", "/users", newUserBody)
	srv.FlushNotifications()
	got := sub.got()
	if len(got) < 2 || len(got) > 4 || got[0]["id"] != got[len(got)-1]["id"] || got[0]["resource"] != "Users/"+user["id"].(string) {
		t.Fatalf("2 to 4 copies of one message, got %d: %v", len(got), got)
	}
	admin(t, ts, "PUT", "/behavior", `{}`, nil)
	graphCall(t, ts, "DELETE", "/users/"+user["id"].(string), "")
	srv.FlushNotifications()
	if len(sub.got()) != len(got)+1 {
		t.Errorf("with the behavior off, one post: %d after %d", len(sub.got()), len(got))
	}
	if s := admin(t, ts, "PUT", "/behavior", `{"duplicatePercent":101}`, nil); s != 400 {
		t.Errorf("101 percent is not a share: %d", s)
	}
}

// The portal makes a subscription with the admin API, sees what can be subscribed to, and emulates a change for it (no data changes).
func TestSubscriptions_ThePortalCreatesAndEmulates(t *testing.T) {
	ts, _, srv := startServer(t, graph.ServerOptions{})
	sub := newSubscriber(t)
	var list struct {
		Resources []struct {
			Pattern    string `json:"pattern"`
			Kind       string `json:"kind"`
			MaxMinutes int    `json:"maxMinutes"`
		} `json:"resources"`
	}
	if s := admin(t, ts, "GET", "/subscriptions", "", &list); s != 200 || len(list.Resources) != 4 || list.Resources[0].Pattern != "users" || list.Resources[0].MaxMinutes != 41760 {
		t.Fatalf("what can be subscribed to: %d %+v", s, list)
	}
	exp := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	var made map[string]any
	body := `{"changeType":"created,updated,deleted","notificationUrl":"` + sub.URL + `/hook","resource":"users","expirationDateTime":"` + exp + `"}`
	if s := admin(t, ts, "POST", "/subscriptions", body, &made); s != 201 && s != 200 || made["id"] == nil {
		t.Fatalf("create: %d %v", s, made)
	}
	// a failure is the error of Graph, for the dialog to show
	req, _ := http.NewRequest("POST", ts.URL+"/_dolly/subscriptions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	text, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 409 || !strings.Contains(string(text), "already exists") {
		t.Errorf("a duplicate: %d %s", res.StatusCode, text)
	}

	users := len(srv.World().ListUsers())
	if s := admin(t, ts, "POST", "/subscriptions/"+made["id"].(string)+"/emulate", `{"changeType":"updated"}`, nil); s != 204 {
		t.Fatalf("emulate: %d", s)
	}
	srv.FlushNotifications()
	got := sub.got()
	if len(got) != 1 || got[0]["changeType"] != "updated" || !strings.HasPrefix(got[0]["resource"].(string), "Users/") {
		t.Errorf("the notification: %v", got)
	}
	if len(srv.World().ListUsers()) != users {
		t.Error("an emulated change does not change the data")
	}
	if s := admin(t, ts, "POST", "/subscriptions/nope/emulate", `{"changeType":"updated"}`, nil); s != 404 {
		t.Errorf("unknown: %d", s)
	}
}

// dollycli can be the receiver of its own notifications: it answers the validation request, and takes a notification with 202. It needs no token (a receiver does not have one).
func TestSubscriptions_DollyIsAReceiver(t *testing.T) {
	ts, _, srv := startServer(t, graph.ServerOptions{})
	post := func(path, contentType string) (int, string, string) {
		res, err := http.Post(ts.URL+path, contentType, strings.NewReader(`{"value":[]}`))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, res.Header.Get("Content-Type"), string(b)
	}
	if s, ct, body := post(graph.ReceiverPath+"?validationToken=a%20b%2Bc", "text/plain"); s != 200 || !strings.HasPrefix(ct, "text/plain") || body != "a b+c" {
		t.Errorf("validation: %d %q %q", s, ct, body)
	}
	if s, _, _ := post(graph.ReceiverPath, "application/json"); s != 202 {
		t.Errorf("a notification: %d", s)
	}
	if res, err := http.Get(ts.URL + graph.ReceiverPath); err != nil || res.StatusCode != 405 {
		t.Errorf("only POST: %v", err)
	}
	// and a subscription to it works end to end
	_, made := graphCall(t, ts, "POST", "/subscriptions", `{"changeType":"updated","notificationUrl":"`+ts.URL+graph.ReceiverPath+`","resource":"users","expirationDateTime":"`+time.Now().Add(time.Hour).UTC().Format(time.RFC3339)+`"}`)
	if made["id"] == nil {
		t.Fatalf("subscribe to itself: %v", made)
	}
	if s := admin(t, ts, "POST", "/subscriptions/"+made["id"].(string)+"/emulate", `{"changeType":"updated"}`, nil); s != 204 {
		t.Fatal(s)
	}
	srv.FlushNotifications()
	var out struct{ Deliveries []map[string]any }
	admin(t, ts, "GET", "/subscriptions", "", &out)
	if len(out.Deliveries) != 1 || out.Deliveries[0]["status"] != float64(202) {
		t.Errorf("the delivery: %v", out.Deliveries)
	}
}

func TestSubscriptions_RenewAndDelete(t *testing.T) {
	ts, _, _ := startServer(t, graph.ServerOptions{})
	sub := newSubscriber(t)
	_, made := subscribe(t, ts, sub.URL, "users", "updated")
	id := made["id"].(string)
	later := time.Now().Add(72 * time.Hour).UTC().Format(time.RFC3339)
	if s, got := graphCall(t, ts, "PATCH", "/subscriptions/"+id, `{"expirationDateTime":"`+later+`"}`); s != 200 || got["id"] != id {
		t.Errorf("renew: %d %v", s, got)
	}
	if s, _ := graphCall(t, ts, "PATCH", "/subscriptions/"+id, `{"changeType":"created"}`); s != 400 {
		t.Errorf("only the expiration and the URL can change: %d", s)
	}
	if s, _ := graphCall(t, ts, "DELETE", "/subscriptions/"+id, ""); s != 204 {
		t.Errorf("delete: %d", s)
	}
	if s, out := graphCall(t, ts, "GET", "/subscriptions/"+id, ""); s != 404 {
		t.Errorf("after the delete: %d %v", s, out)
	}
}

// A restart keeps the subscriptions (they are in a file next to the data). A new seed is another tenant: they are gone. A reset and a rollback keep them and send nothing.
func TestSubscriptions_RestartRegenerateAndRollback(t *testing.T) {
	dir := t.TempDir()
	world, err := graph.Open(dir, graph.OpenOptions{Users: 10, Seed: 3, Profile: graph.AcmeProfile()})
	if err != nil {
		t.Fatal(err)
	}
	srv := graph.New(world, graph.ServerOptions{})
	ts := httptest.NewServer(srv.Handler())
	sub := newSubscriber(t)
	_, made := subscribe(t, ts, sub.URL, "users", "created,updated,deleted")
	id := made["id"].(string)
	before := len(world.History())
	graphCall(t, ts, "POST", "/users", newUserBody)
	srv.FlushNotifications()
	if len(sub.got()) != 1 {
		t.Fatalf("a notification: %d", len(sub.got()))
	}
	ts.Close()
	srv.Close()
	_ = world.Close()

	// a restart
	world, err = graph.Open(dir, graph.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	srv = graph.New(world, graph.ServerOptions{})
	ts = httptest.NewServer(srv.Handler())
	defer func() { ts.Close(); srv.Close(); _ = world.Close() }()
	if s, got := graphCall(t, ts, "GET", "/subscriptions/"+id, ""); s != 200 || got["clientState"] != "cs-1" {
		t.Fatalf("after a restart: %d %v", s, got)
	}
	graphCall(t, ts, "PATCH", "/users/"+world.ListUsers()[0]["id"].(string), `{"jobTitle":"After the restart"}`)
	srv.FlushNotifications()
	if len(sub.got()) != 2 {
		t.Errorf("after a restart the subscription delivers: %d", len(sub.got()))
	}

	// a rollback and a reset: the subscription stays, and nobody is told
	if err := world.Rollback(before, graph.Actor{Source: "admin"}); err != nil {
		t.Fatal(err)
	}
	srv.FlushNotifications()
	if err := world.Reset(graph.Actor{Source: "admin"}); err != nil {
		t.Fatal(err)
	}
	srv.FlushNotifications()
	if len(sub.got()) != 2 {
		t.Errorf("a rollback and a reset send nothing: %d", len(sub.got()))
	}
	if s, _ := graphCall(t, ts, "GET", "/subscriptions/"+id, ""); s != 200 {
		t.Errorf("a rollback and a reset keep the subscription: %d", s)
	}

	// the data is made again from a new seed: another tenant
	if _, err := world.Regenerate(graph.Options{Users: 10, Seed: 99, Profile: graph.AcmeProfile()}); err != nil {
		t.Fatal(err)
	}
	if s, list := graphCall(t, ts, "GET", "/subscriptions", ""); s != 200 || len(listOf(list)) != 0 {
		t.Errorf("a new seed: %d %v", s, list)
	}
	graphCall(t, ts, "POST", "/users", newUserBody)
	srv.FlushNotifications()
	if len(sub.got()) != 2 {
		t.Errorf("a dropped subscription gets nothing: %d", len(sub.got()))
	}
}
