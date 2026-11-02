package notify_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/withholm/dolly-cli/graph/notify"
)

// The change notifications of Graph, as a subscriber meets them: a subscription is made with a notification URL that must answer a validation request, and then the
// subscriber gets a POST for each change of what it subscribed to. The rules are from the pages of Microsoft Learn about change notifications (checked against the real Graph:
// only GET /subscriptions, which is an empty list in the sample tenant; a write cannot be tried there).

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// receiver is a subscriber: it answers the validation request as it is told to, and keeps what it is sent.
type receiver struct {
	*httptest.Server
	mu          sync.Mutex
	validations []validation
	posts       []post
	// how it behaves
	answer func(w http.ResponseWriter, token string) // for a validation request
	status int                                       // for a notification
}

type validation struct{ token, contentType, method string }
type post struct {
	body        string
	contentType string
}

func newReceiver(t *testing.T) *receiver {
	r := &receiver{status: 202}
	r.answer = func(w http.ResponseWriter, token string) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, token)
	}
	r.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if tok, ok := req.URL.Query()["validationToken"]; ok {
			r.mu.Lock()
			r.validations = append(r.validations, validation{tok[0], req.Header.Get("Content-Type"), req.Method})
			answer := r.answer
			r.mu.Unlock()
			answer(w, tok[0])
			return
		}
		b, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.posts = append(r.posts, post{string(b), req.Header.Get("Content-Type")})
		status := r.status
		r.mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(r.Close)
	return r
}

func (r *receiver) got() []post {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]post(nil), r.posts...)
}

func supported() []notify.Resource {
	return []notify.Resource{
		{Pattern: "users", Kind: "user", MaxMinutes: 41760}, {Pattern: "users/{id}", Kind: "user", MaxMinutes: 41760},
		{Pattern: "groups", Kind: "group", MaxMinutes: 41760}, {Pattern: "groups/{id}", Kind: "group", MaxMinutes: 41760},
	}
}

type env struct {
	e    *notify.Engine
	clk  *clock
	seed *string
	dir  string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	return reopen(t, &env{clk: &clock{t: time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)}, seed: new(string), dir: t.TempDir()}, "seed-1")
}

func reopen(t *testing.T, e *env, seed string) *env {
	t.Helper()
	if e.e != nil {
		e.e.Close()
	}
	*e.seed = seed
	var err error
	e.e, err = notify.New(notify.Config{
		Dir: e.dir, SeedID: func() string { return *e.seed }, TenantID: func() string { return "11111111-2222-3333-4444-555555555555" },
		Now: e.clk.now, Resources: supported(), ValidationTimeout: 300 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.e.Close)
	return e
}

func (e *env) create(t *testing.T, r *receiver, extra map[string]any) (map[string]any, *notify.Error) {
	t.Helper()
	body := map[string]any{
		"changeType": "created,updated,deleted", "notificationUrl": r.URL + "/hook?x=1", "resource": "users",
		"expirationDateTime": e.clk.now().Add(24 * time.Hour).Format("2006-01-02T15:04:05.0000000Z"), "clientState": "secret-state",
	}
	for k, v := range extra {
		if v == nil {
			delete(body, k)
		} else {
			body[k] = v
		}
	}
	return e.e.Create(context.Background(), body, notify.Who{AppID: "99999999-0000-0000-0000-000000000001"})
}

func TestCreate_ValidatesTheNotificationURLAndGivesTheSubscription(t *testing.T) {
	e, r := newEnv(t), newReceiver(t)
	sub, err := e.create(t, r, nil)
	if err != nil {
		t.Fatalf("%+v", err)
	}
	// the validation request: a POST with the token in the query, as text/plain, and the other query parameters of the URL are kept
	if len(r.validations) != 1 || r.validations[0].method != "POST" || !strings.HasPrefix(r.validations[0].contentType, "text/plain") || len(r.validations[0].token) < 16 {
		t.Fatalf("validation: %+v", r.validations)
	}
	for k, want := range map[string]any{
		"resource": "users", "changeType": "created,updated,deleted", "clientState": "secret-state", "notificationUrl": r.URL + "/hook?x=1",
		"applicationId": "99999999-0000-0000-0000-000000000001", "includeResourceData": false, "latestSupportedTlsVersion": "v1_2",
	} {
		if sub[k] != want {
			t.Errorf("%s = %v, want %v", k, sub[k], want)
		}
	}
	id, _ := sub["id"].(string)
	if len(id) != 36 {
		t.Errorf("the id of a subscription is a GUID: %q", id)
	}
	if exp, _ := sub["expirationDateTime"].(string); !strings.HasSuffix(exp, "Z") || !strings.Contains(exp, ".") {
		t.Errorf("the expiration is written as Graph writes it: %q", exp)
	}
	for _, k := range []string{"notificationQueryOptions", "lifecycleNotificationUrl", "encryptionCertificate", "encryptionCertificateId", "creatorId"} {
		if v, has := sub[k]; !has || v != nil {
			t.Errorf("%s is there and null: %v %v", k, has, v)
		}
	}
	if got, _ := e.e.Get(id); got["id"] != id {
		t.Errorf("get: %v", got)
	}
	if len(e.e.List()) != 1 {
		t.Errorf("list: %v", e.e.List())
	}
}

// The subscription is not made when the endpoint does not answer the validation request as Graph asks.
func TestCreate_RefusesWhenTheEndpointDoesNotValidate(t *testing.T) {
	e := newEnv(t)
	for name, answer := range map[string]func(w http.ResponseWriter, token string){
		"a 500": func(w http.ResponseWriter, _ string) { w.WriteHeader(500) },
		"a 202": func(w http.ResponseWriter, token string) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(202)
			_, _ = io.WriteString(w, token)
		},
		"another body": func(w http.ResponseWriter, _ string) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, "nope")
		},
		"the encoded token": func(w http.ResponseWriter, token string) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, url.QueryEscape(token))
		},
		"json": func(w http.ResponseWriter, token string) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, token)
		},
		"too slow": func(w http.ResponseWriter, token string) {
			time.Sleep(time.Second)
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, token)
		},
	} {
		r := newReceiver(t)
		r.answer = answer
		_, err := e.create(t, r, nil)
		if err == nil || err.Status != 400 || err.Code != "ValidationError" || !strings.Contains(err.Message, "Subscription validation request failed") {
			t.Errorf("%s: %+v", name, err)
		}
	}
	if len(e.e.List()) != 0 {
		t.Error("a subscription was made although the validation failed")
	}
	// a redirect is not followed: the endpoint that it points at is not called
	other := newReceiver(t)
	r := newReceiver(t)
	r.answer = func(w http.ResponseWriter, _ string) {
		w.Header().Set("Location", other.URL+"/?validationToken=x")
		w.WriteHeader(http.StatusFound)
	}
	if _, err := e.create(t, r, nil); err == nil || len(other.validations) != 0 {
		t.Errorf("a redirect: %+v, and the other endpoint was called %d times", err, len(other.validations))
	}
}

func TestCreate_ChecksTheRequest(t *testing.T) {
	e, r := newEnv(t), newReceiver(t)
	future := func(d time.Duration) string { return e.clk.now().Add(d).Format(time.RFC3339Nano) }
	for name, c := range map[string]struct {
		extra map[string]any
		want  string
	}{
		"no resource":                      {map[string]any{"resource": nil}, "resource"},
		"no notificationUrl":               {map[string]any{"notificationUrl": nil}, "notificationUrl"},
		"no changeType":                    {map[string]any{"changeType": nil}, "changeType"},
		"no expirationDateTime":            {map[string]any{"expirationDateTime": nil}, "expirationDateTime"},
		"a change type that is not":        {map[string]any{"changeType": "created,moved"}, "changeType"},
		"a resource that is not supported": {map[string]any{"resource": "groups/1/members"}, "resource"},
		"a resource that does not exist":   {map[string]any{"resource": "widgets"}, "resource"},
		"too long":                         {map[string]any{"expirationDateTime": future(42000 * time.Minute)}, "can only be 41760 minutes in the future"},
		"in the past":                      {map[string]any{"expirationDateTime": future(-time.Hour)}, "expirationDateTime"},
		"not a date":                       {map[string]any{"expirationDateTime": "tomorrow"}, "expirationDateTime"},
		"not a URL":                        {map[string]any{"notificationUrl": "webhook"}, "notificationUrl"},
		"ftp":                              {map[string]any{"notificationUrl": "ftp://example.com/x"}, "notificationUrl"},
		"the metadata address":             {map[string]any{"notificationUrl": "http://169.254.169.254/latest/meta-data"}, "notificationUrl"},
		"a link-local IPv6 address":        {map[string]any{"notificationUrl": "http://[fe80::1]:80/x"}, "notificationUrl"},
	} {
		sub, err := e.create(t, r, c.extra)
		if err == nil || err.Status != 400 || !strings.Contains(err.Message, c.want) {
			t.Errorf("%s: %+v %v", name, err, sub)
		}
	}
	if len(r.validations) != 0 {
		t.Errorf("an endpoint was called for a request that is not valid: %d", len(r.validations))
	}
	// the supported forms of the resource, in any case, with a slash at the start
	for i, res := range []string{"users", "/users", "Users", "users/11111111-2222-3333-4444-555555555555", "groups", "/groups/abc"} {
		// "users" three times is the same combination (409), so each of them has its own change type
		if _, err := e.create(t, r, map[string]any{"resource": res, "changeType": []string{"created", "updated", "deleted", "created,updated", "created,deleted", "updated,deleted"}[i]}); err != nil {
			t.Errorf("%s: %+v", res, err)
		}
	}
}

func decode(t *testing.T, p post) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(p.body), &m); err != nil {
		t.Fatalf("the notification is not JSON: %v\n%s", err, p.body)
	}
	return m
}

// A change of a user is a POST to the notification URL of each subscription that matches: the body has the id of the subscription, the change type, the resource,
// the client state that the subscriber gave, and the tenant.
func TestNotify_DeliversTheChange(t *testing.T) {
	e, r := newEnv(t), newReceiver(t)
	sub, _ := e.create(t, r, nil)
	e.e.Notify(notify.Change{Kind: "user", ID: "aaaaaaaa-1111-2222-3333-444444444444", Type: "created"})
	e.e.Flush()
	posts := r.got()
	if len(posts) != 1 || !strings.HasPrefix(posts[0].contentType, "application/json") {
		t.Fatalf("posts: %+v", posts)
	}
	n := decode(t, posts[0])["value"].([]any)
	if len(n) != 1 {
		t.Fatalf("value: %v", n)
	}
	item := n[0].(map[string]any)
	data := item["resourceData"].(map[string]any)
	for k, want := range map[string]any{
		"subscriptionId": sub["id"], "changeType": "created", "clientState": "secret-state", "tenantId": "11111111-2222-3333-4444-555555555555",
		"resource": "Users/aaaaaaaa-1111-2222-3333-444444444444", "subscriptionExpirationDateTime": sub["expirationDateTime"],
	} {
		if item[k] != want {
			t.Errorf("%s = %v, want %v", k, item[k], want)
		}
	}
	if data["@odata.type"] != "#Microsoft.Graph.User" || data["@odata.id"] != "Users/aaaaaaaa-1111-2222-3333-444444444444" || data["id"] != "aaaaaaaa-1111-2222-3333-444444444444" {
		t.Errorf("resourceData: %v", data)
	}
	if id, _ := item["id"].(string); id == "" {
		t.Error("a notification has an id of its own")
	}
	// the log of deliveries
	log := e.e.Deliveries()
	if len(log) != 1 || log[0].Status != 202 || log[0].SubscriptionID != sub["id"] || log[0].ChangeType != "created" || log[0].Error != "" {
		t.Errorf("deliveries: %+v", log)
	}
}

// Which subscription gets which change.
func TestNotify_MatchesTheSubscription(t *testing.T) {
	e := newEnv(t)
	id := "aaaaaaaa-1111-2222-3333-444444444444"
	all, one, other, groups, onlyCreated, onlyUpdated := newReceiver(t), newReceiver(t), newReceiver(t), newReceiver(t), newReceiver(t), newReceiver(t)
	e.create(t, all, nil)
	e.create(t, one, map[string]any{"resource": "users/" + id})
	e.create(t, other, map[string]any{"resource": "users/bbbbbbbb-1111-2222-3333-444444444444"})
	e.create(t, groups, map[string]any{"resource": "groups"})
	e.create(t, onlyCreated, map[string]any{"changeType": "created"})
	e.create(t, onlyUpdated, map[string]any{"changeType": "updated"})

	for _, ch := range []string{"created", "updated", "deleted"} {
		e.e.Notify(notify.Change{Kind: "user", ID: id, Type: ch})
	}
	e.e.Flush()
	count := func(r *receiver) int { return len(r.got()) }
	if count(all) != 3 || count(one) != 3 {
		t.Errorf("the subscription to users and to this user: %d %d", count(all), count(one))
	}
	if count(other) != 0 || count(groups) != 0 {
		t.Errorf("another user, and groups, get nothing: %d %d", count(other), count(groups))
	}
	if count(onlyCreated) != 1 {
		t.Errorf("created only: %d", count(onlyCreated))
	}
	// Learn: the creation and the deletion of a user or a group also trigger "updated"
	if count(onlyUpdated) != 3 {
		t.Errorf("updated gets the creation, the update, and the deletion: %d", count(onlyUpdated))
	}
	// a change of a group
	e.e.Notify(notify.Change{Kind: "group", ID: "cccccccc-1111-2222-3333-444444444444", Type: "updated"})
	e.e.Flush()
	if count(groups) != 1 || count(all) != 3 {
		t.Errorf("a group: %d %d", count(groups), count(all))
	}
	if item := decode(t, groups.got()[0])["value"].([]any)[0].(map[string]any); item["resource"] != "Groups/cccccccc-1111-2222-3333-444444444444" || item["resourceData"].(map[string]any)["@odata.type"] != "#Microsoft.Graph.Group" {
		t.Errorf("the notification of a group: %v", item)
	}
}

// A subscriber that fails is in the log, and does not stop the others. (Retries come later.)
func TestNotify_ALogOfWhatHappened(t *testing.T) {
	e, bad, good := newEnv(t), newReceiver(t), newReceiver(t)
	e.create(t, bad, nil)
	e.create(t, good, map[string]any{"changeType": "updated"}) // not the same combination as the first: that is a 409
	bad.mu.Lock()
	bad.status = 500
	bad.mu.Unlock()
	e.e.Notify(notify.Change{Kind: "user", ID: "aaaaaaaa-1111-2222-3333-444444444444", Type: "updated"})
	e.e.Flush()
	statuses := map[int]int{}
	for _, d := range e.e.Deliveries() {
		statuses[d.Status]++
	}
	if statuses[500] != 1 || statuses[202] != 1 || len(good.got()) != 1 {
		t.Errorf("statuses %v", statuses)
	}
	// an endpoint that is gone
	gone := newReceiver(t)
	e.create(t, gone, map[string]any{"changeType": "created,updated"})
	gone.Close()
	e.e.Notify(notify.Change{Kind: "user", ID: "aaaaaaaa-1111-2222-3333-444444444444", Type: "updated"})
	e.e.Flush()
	var errs int
	for _, d := range e.e.Deliveries() {
		if d.Error != "" && d.Status == 0 {
			errs++
		}
	}
	if errs != 1 {
		t.Errorf("a delivery to an endpoint that is gone has an error and no status: %+v", e.e.Deliveries())
	}
}

func TestUpdateAndDelete(t *testing.T) {
	e, r := newEnv(t), newReceiver(t)
	sub, _ := e.create(t, r, nil)
	id := sub["id"].(string)

	// a renewal
	later := e.clk.now().Add(48 * time.Hour).Format(time.RFC3339Nano)
	got, err := e.e.Update(context.Background(), id, map[string]any{"expirationDateTime": later})
	if err != nil || got["id"] != id {
		t.Fatalf("renew: %+v %v", err, got)
	}
	if exp, _ := time.Parse(time.RFC3339Nano, got["expirationDateTime"].(string)); !exp.Equal(e.clk.now().Add(48 * time.Hour)) {
		t.Errorf("the new expiration: %v", got["expirationDateTime"])
	}
	// a change of the notification URL is validated again, and a bad one is refused
	nr := newReceiver(t)
	if got, err := e.e.Update(context.Background(), id, map[string]any{"notificationUrl": nr.URL + "/new"}); err != nil || got["notificationUrl"] != nr.URL+"/new" || len(nr.validations) != 1 {
		t.Errorf("a new URL: %+v %v %d", err, got["notificationUrl"], len(nr.validations))
	}
	bad := newReceiver(t)
	bad.answer = func(w http.ResponseWriter, _ string) { w.WriteHeader(500) }
	if _, err := e.e.Update(context.Background(), id, map[string]any{"notificationUrl": bad.URL}); err == nil || err.Status != 400 {
		t.Errorf("a URL that does not validate: %+v", err)
	}
	if got, _ := e.e.Get(id); got["notificationUrl"] != nr.URL+"/new" {
		t.Error("a refused change must change nothing")
	}
	// only the expiration and the URL can change
	for name, body := range map[string]map[string]any{"nothing": {}, "the resource": {"resource": "groups"}, "the state": {"clientState": "x"}, "too long": {"expirationDateTime": e.clk.now().Add(50000 * time.Minute).Format(time.RFC3339Nano)}} {
		if _, err := e.e.Update(context.Background(), id, body); err == nil || err.Status != 400 {
			t.Errorf("%s: %+v", name, err)
		}
	}
	if _, err := e.e.Update(context.Background(), "00000000-0000-0000-0000-000000000000", map[string]any{"expirationDateTime": later}); err == nil || err.Status != 404 {
		t.Errorf("an unknown subscription: %+v", err)
	}

	if err := e.e.Delete(id); err != nil {
		t.Fatal(err)
	}
	if err := e.e.Delete(id); err == nil || err.Status != 404 {
		t.Errorf("twice: %+v", err)
	}
	e.e.Notify(notify.Change{Kind: "user", ID: "aaaaaaaa-1111-2222-3333-444444444444", Type: "created"})
	e.e.Flush()
	if len(r.got()) != 0 || len(nr.got()) != 0 {
		t.Error("a deleted subscription gets nothing")
	}
}

func TestExpiry(t *testing.T) {
	e, r := newEnv(t), newReceiver(t)
	e.create(t, r, nil)
	e.clk.advance(25 * time.Hour)
	if len(e.e.List()) != 0 {
		t.Errorf("an expired subscription is gone: %v", e.e.List())
	}
	e.e.Notify(notify.Change{Kind: "user", ID: "aaaaaaaa-1111-2222-3333-444444444444", Type: "created"})
	e.e.Flush()
	if len(r.got()) != 0 {
		t.Error("an expired subscription gets nothing")
	}
}

// The subscriptions are in a file, so they are there after a restart. A new seed makes a different tenant: they are gone.
func TestPersistence(t *testing.T) {
	e, r := newEnv(t), newReceiver(t)
	sub, _ := e.create(t, r, nil)
	file := filepath.Join(e.dir, "subscriptions.json")
	if st, err := os.Stat(file); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("the file: %v %v", st, err)
	}
	e = reopen(t, e, "seed-1")
	if got, err := e.e.Get(sub["id"].(string)); err != nil || got["clientState"] != "secret-state" {
		t.Errorf("after a restart: %v %v", got, err)
	}
	// it still delivers, with the client state
	e.e.Notify(notify.Change{Kind: "user", ID: "aaaaaaaa-1111-2222-3333-444444444444", Type: "updated"})
	e.e.Flush()
	if len(r.got()) != 1 {
		t.Errorf("after a restart the subscription delivers: %d", len(r.got()))
	}
	// the time passes while dollycli is stopped
	e.clk.advance(30 * time.Hour)
	e = reopen(t, e, "seed-1")
	if len(e.e.List()) != 0 {
		t.Errorf("a subscription that expired while dollycli was stopped: %v", e.e.List())
	}
}

func TestANewSeedDropsTheSubscriptions(t *testing.T) {
	e, r := newEnv(t), newReceiver(t)
	e.create(t, r, nil)
	e.create(t, r, map[string]any{"resource": "groups"})
	// at a restart
	e2 := reopen(t, &env{clk: e.clk, seed: e.seed, dir: e.dir}, "seed-2")
	if len(e2.e.List()) != 0 {
		t.Errorf("a restart with another seed: %v", e2.e.List())
	}
	// while it runs: the data is made again
	e3, r3 := newEnv(t), newReceiver(t)
	e3.create(t, r3, nil)
	*e3.seed = "seed-3"
	if len(e3.e.List()) != 0 {
		t.Error("the data was made again: the subscriptions are gone")
	}
	e3.e.Notify(notify.Change{Kind: "user", ID: "aaaaaaaa-1111-2222-3333-444444444444", Type: "created"})
	e3.e.Flush()
	if len(r3.got()) != 0 {
		t.Error("and they get nothing")
	}
	// the file has no subscriptions either
	if b, _ := os.ReadFile(filepath.Join(e3.dir, "subscriptions.json")); strings.Contains(string(b), "secret-state") {
		t.Errorf("the file still has them: %s", b)
	}
}

// With no folder the subscriptions are in memory only.
func TestInMemory(t *testing.T) {
	clk := &clock{t: time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)}
	seed := "s"
	eng, err := notify.New(notify.Config{SeedID: func() string { return seed }, TenantID: func() string { return "t" }, Now: clk.now, Resources: supported()})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	r := newReceiver(t)
	_, cerr := eng.Create(context.Background(), map[string]any{"changeType": "created", "notificationUrl": r.URL, "resource": "users", "expirationDateTime": clk.now().Add(time.Hour).Format(time.RFC3339)}, notify.Who{})
	if cerr != nil || len(eng.List()) != 1 {
		t.Errorf("%+v", cerr)
	}
}

// Graph sometimes sends the same notification more than once (several services hold the data, and they are not in step). Copies says how many times each is sent.
func TestDuplicates_TheSameNotificationIsSentSeveralTimes(t *testing.T) {
	clk := &clock{t: time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)}
	copies := 3
	eng, err := notify.New(notify.Config{SeedID: func() string { return "s" }, TenantID: func() string { return "t" }, Now: clk.now, Resources: supported(), Copies: func() int { return copies }})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	r := newReceiver(t)
	if _, e := eng.Create(context.Background(), map[string]any{"changeType": "updated", "notificationUrl": r.URL, "resource": "users", "expirationDateTime": clk.now().Add(time.Hour).Format(time.RFC3339)}, notify.Who{}); e != nil {
		t.Fatal(e)
	}
	eng.Notify(notify.Change{Kind: "user", ID: "aaaaaaaa-1111-2222-3333-444444444444", Type: "updated"})
	eng.Flush()
	got := r.got()
	if len(got) != 3 || got[0].body != got[1].body || got[1].body != got[2].body {
		t.Fatalf("three identical posts, got %d: %v", len(got), got)
	}
	if n := len(eng.Deliveries()); n != 3 {
		t.Errorf("each send is in the log: %d", n)
	}
	copies = 1
	eng.Notify(notify.Change{Kind: "user", ID: "aaaaaaaa-1111-2222-3333-444444444444", Type: "updated"})
	eng.Flush()
	if len(r.got()) != 4 {
		t.Errorf("one copy is one post: %d", len(r.got()))
	}
}

// A receiver on this machine with a certificate that nobody trusts (a dev certificate, or the CA of dollycli itself) is fine: the traffic does not leave the machine.
func TestHTTPSOnLoopbackNeedsNoTrust(t *testing.T) {
	x := newEnv(t)
	var posts int
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if tok := req.URL.Query().Get("validationToken"); tok != "" {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, tok)
			return
		}
		posts++
		w.WriteHeader(202)
	}))
	defer srv.Close()
	sub, err := x.e.Create(context.Background(), map[string]any{"changeType": "updated", "notificationUrl": srv.URL, "resource": "users", "expirationDateTime": x.clk.now().Add(time.Hour).Format(time.RFC3339)}, notify.Who{})
	if err != nil {
		t.Fatalf("validation over https with a certificate that is not trusted: %+v", err)
	}
	_ = sub
	x.e.Notify(notify.Change{Kind: "user", ID: "aaaaaaaa-1111-2222-3333-444444444444", Type: "updated"})
	x.e.Flush()
	if posts != 1 {
		t.Errorf("a notification: %d", posts)
	}
}

// Emulate sends a notification to one subscription as if an object had changed. No data changes, and the other subscriptions get nothing.
func TestEmulate(t *testing.T) {
	x := newEnv(t)
	one, other := newReceiver(t), newReceiver(t)
	all, _ := x.create(t, one, map[string]any{"changeType": "updated"})
	x.create(t, other, map[string]any{"changeType": "updated", "resource": "users/aaaaaaaa-1111-2222-3333-444444444444"})
	pick := func(kind string) string { return "bbbbbbbb-1111-2222-3333-444444444444" }

	if err := x.e.Emulate(all["id"].(string), "updated", pick); err != nil {
		t.Fatal(err)
	}
	x.e.Flush()
	got := one.got()
	if len(got) != 1 || !strings.Contains(got[0].body, `"Users/bbbbbbbb-1111-2222-3333-444444444444"`) || len(other.got()) != 0 {
		t.Fatalf("one gets one, the other none: %v / %d", got, len(other.got()))
	}
	// the subscription to one user is for that user
	subs := x.e.List()
	var oneUser string
	for _, s := range subs {
		if s["resource"] != "users" {
			oneUser = s["id"].(string)
		}
	}
	if err := x.e.Emulate(oneUser, "updated", pick); err != nil {
		t.Fatal(err)
	}
	x.e.Flush()
	if len(other.got()) != 1 || !strings.Contains(other.got()[0].body, "Users/aaaaaaaa-1111-2222-3333-444444444444") {
		t.Errorf("one user: %v", other.got())
	}
	// a change that the subscription is not for, and an ID that is not there
	onlyCreated, _ := x.create(t, newReceiver(t), map[string]any{"changeType": "created", "resource": "groups"})
	if err := x.e.Emulate(onlyCreated["id"].(string), "deleted", pick); err == nil || err.Status != 400 {
		t.Errorf("deleted is not in this subscription (only updated takes it too): %+v", err)
	}
	if err := x.e.Emulate(all["id"].(string), "moved", pick); err == nil || err.Status != 400 {
		t.Errorf("not a change type: %+v", err)
	}
	if err := x.e.Emulate("nope", "updated", pick); err == nil || err.Status != 404 {
		t.Errorf("unknown: %+v", err)
	}
	if err := x.e.Emulate(all["id"].(string), "updated", func(string) string { return "" }); err == nil || err.Status != 400 {
		t.Errorf("no object to name: %+v", err)
	}
}

// Learn: the same changeType and resource as a subscription that is there is 409. Another change type, another resource, or another application is not a duplicate.
func TestDuplicateSubscriptionIs409(t *testing.T) {
	x := newEnv(t)
	r := newReceiver(t)
	make := func(res, types, app string) (map[string]any, *notify.Error) {
		return x.e.Create(context.Background(), map[string]any{"changeType": types, "notificationUrl": r.URL, "resource": res, "expirationDateTime": x.clk.now().Add(time.Hour).Format(time.RFC3339)}, notify.Who{AppID: app})
	}
	first, err := make("users", "updated,created", "app-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, res := range []string{"users", "/Users/"} {
		_, err = make(res, "created, updated", "app-1")
		if err == nil || err.Status != 409 || !strings.Contains(err.Message, first["id"].(string)+" already exists for the requested combination") {
			t.Errorf("%q: %+v", res, err)
		}
	}
	if n := len(x.e.List()); n != 1 {
		t.Errorf("the duplicate was stored: %d", n)
	}
	for _, c := range []struct{ res, types, app string }{{"groups", "updated,created", "app-1"}, {"users", "updated", "app-1"}, {"users", "updated,created", "app-2"}} {
		if _, err := make(c.res, c.types, c.app); err != nil {
			t.Errorf("%v is not a duplicate: %+v", c, err)
		}
	}
	// when the first is gone, the combination is free
	_ = x.e.Delete(first["id"].(string))
	if _, err := make("users", "updated,created", "app-1"); err != nil {
		t.Errorf("after the delete: %+v", err)
	}
}
