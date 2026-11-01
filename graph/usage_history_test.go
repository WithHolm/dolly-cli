package graph_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/withholm/dolly-cli/graph"
)

type historyAnswer struct {
	History struct {
		BucketSeconds    int `json:"bucketSeconds"`
		RetentionSeconds int `json:"retentionSeconds"`
		Points           []struct {
			T         int64
			RU        int
			Calls     int
			Throttled int
		}
	}
	Limit *struct {
		Scope   string
		RU      int
		Seconds int
	}
}

// The admin API gives the use over time of a client, and of all of them, with the limit of the tenant for the graph.
func TestAdmin_UsageHistory(t *testing.T) {
	h := smallServer(true) // a small tenant: 3500 RU in 10 seconds
	for range 1750 {
		send(h, "GET", "/v1.0/users", "")
	}
	for range 3 {
		if send(h, "GET", "/v1.0/users", "").Code != 429 {
			t.Fatal("must be throttled")
		}
	}

	var a historyAnswer
	rec := send(h, "GET", "/_dolly/usage/history", "")
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &a) != nil {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if a.History.BucketSeconds != 10 || a.History.RetentionSeconds != 3600 || len(a.History.Points) != 360 {
		t.Fatalf("%+v", a.History.BucketSeconds)
	}
	ru, calls, throttled := 0, 0, 0
	for _, p := range a.History.Points {
		ru, calls, throttled = ru+p.RU, calls+p.Calls, throttled+p.Throttled
	}
	if ru != 3500 || calls != 1753 || throttled != 3 {
		t.Errorf("%d RU, %d calls, %d throttled", ru, calls, throttled)
	}
	if a.Limit == nil || a.Limit.RU != 3500 || a.Limit.Seconds != 10 || a.Limit.Scope != "Tenant_Application" {
		t.Errorf("limit: %+v", a.Limit)
	}
	if last := a.History.Points[359]; time.Unix(last.T, 0).After(time.Now()) || last.T%10 != 0 {
		t.Errorf("the last point: %+v", last)
	}

	// one client: the calls came with a token that has no application ID, so they belong to "anonymous"
	var one historyAnswer
	rec = send(h, "GET", "/_dolly/usage/history?app=Anonymous", "") // the case does not matter
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &one) != nil {
		t.Fatal(rec.Body)
	}
	sum := 0
	for _, p := range one.History.Points {
		sum += p.RU
	}
	if sum != 3500 {
		t.Errorf("the client: %d RU", sum)
	}
	// a client that has not called
	var none historyAnswer
	_ = json.Unmarshal(send(h, "GET", "/_dolly/usage/history?app=nobody", "").Body.Bytes(), &none)
	for _, p := range none.History.Points {
		if p.RU != 0 || p.Calls != 0 {
			t.Fatal("a client with no calls has zeros")
		}
	}

	// the reset of the use clears it, and the admin API did not count itself
	send(h, "DELETE", "/_dolly/usage", "")
	var after historyAnswer
	_ = json.Unmarshal(send(h, "GET", "/_dolly/usage/history", "").Body.Bytes(), &after)
	for _, p := range after.History.Points {
		if p.Calls != 0 {
			t.Fatalf("after the reset: %+v", p)
		}
	}
}

// The history is kept for the time that the server is given.
func TestAdmin_UsageHistoryRetention(t *testing.T) {
	h := graph.New(graph.Generate(graph.Options{Users: 10, Seed: 1}), graph.ServerOptions{UsageHistory: 5 * time.Minute}).Handler()
	var a historyAnswer
	if err := json.Unmarshal(send(h, "GET", "/_dolly/usage/history", "").Body.Bytes(), &a); err != nil || a.History.RetentionSeconds != 300 || len(a.History.Points) != 30 {
		t.Errorf("%v %d %d", err, a.History.RetentionSeconds, len(a.History.Points))
	}
}
