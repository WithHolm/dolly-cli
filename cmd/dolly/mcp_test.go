package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// mcpClient talks to serveMCP over a pipe, one JSON line each way.
type mcpClient struct {
	t   *testing.T
	in  io.WriteCloser
	out *bufio.Reader
	id  int
}

func newMCPClient(t *testing.T) *mcpClient {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	go func() { _ = serveMCP(inR, outW); _ = outW.Close() }()
	c := &mcpClient{t: t, in: inW, out: bufio.NewReader(outR)}
	t.Cleanup(func() { _ = inW.Close() })
	return c
}

func (c *mcpClient) call(method string, params any) map[string]any {
	c.t.Helper()
	c.id++
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": c.id, "method": method, "params": params})
	if _, err := c.in.Write(append(b, '\n')); err != nil {
		c.t.Fatal(err)
	}
	line, err := c.out.ReadBytes('\n')
	if err != nil {
		c.t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(line, &m); err != nil {
		c.t.Fatalf("%s: %v", line, err)
	}
	return m
}

// tool calls a tool and gives the text of the answer.
func (c *mcpClient) tool(name string, args map[string]any) (string, bool) {
	c.t.Helper()
	m := c.call("tools/call", map[string]any{"name": name, "arguments": args})
	res, ok := m["result"].(map[string]any)
	if !ok {
		c.t.Fatalf("no result: %v", m)
	}
	text := res["content"].([]any)[0].(map[string]any)["text"].(string)
	isErr, _ := res["isError"].(bool)
	return text, isErr
}

func TestMCP_InitializeAndList(t *testing.T) {
	c := newMCPClient(t)
	m := c.call("initialize", map[string]any{"protocolVersion": "2025-03-26"})
	res := m["result"].(map[string]any)
	if res["protocolVersion"] != "2025-03-26" || res["serverInfo"].(map[string]any)["name"] != "dolly" {
		t.Fatalf("%v", res)
	}
	var names []string
	for _, x := range c.call("tools/list", nil)["result"].(map[string]any)["tools"].([]any) {
		names = append(names, x.(map[string]any)["name"].(string))
	}
	if strings.Join(names, ",") != "graph_route,start_instance,stop_instance" {
		t.Errorf("tools: %v", names)
	}
	if _, bad := c.call("nothing/here", nil)["error"]; !bad {
		t.Error("an unknown method must be an error")
	}
}

func TestMCP_GraphRoute(t *testing.T) {
	c := newMCPClient(t)
	text, isErr := c.tool("graph_route", map[string]any{"route": "GET /users/{id}"})
	if isErr || !strings.Contains(text, `"served": true`) || !strings.Contains(text, "User.Read") {
		t.Errorf("%v %s", isErr, text)
	}
	if _, isErr := c.tool("graph_route", map[string]any{"route": "users"}); !isErr {
		t.Error("a route with no method is an error")
	}
}

func TestMCP_Instance(t *testing.T) {
	c := newMCPClient(t)
	if _, isErr := c.tool("stop_instance", nil); isErr {
		t.Error("stop with no instance is not an error")
	}
	url, isErr := c.tool("start_instance", map[string]any{"users": 5})
	if isErr || !strings.HasPrefix(url, "http://127.0.0.1:") || !strings.HasSuffix(url, "/v1.0") {
		t.Fatalf("%v %q", isErr, url)
	}
	if again, _ := c.tool("start_instance", nil); again != url {
		t.Errorf("a second start gives the same instance: %q vs %q", again, url)
	}
	req, _ := http.NewRequest("GET", url+"/users?$top=1", nil)
	req.Header.Set("Authorization", "Bearer x")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("%v %v", err, resp)
	}
	_ = resp.Body.Close()
	c.tool("stop_instance", nil)
	if _, err := http.Get(url + "/users"); err == nil {
		t.Error("the instance still answers after stop")
	}
}

func TestMCP_StartInstanceCloneFrom(t *testing.T) {
	base := baseline(t)
	c := newMCPClient(t)
	url, isErr := c.tool("start_instance", map[string]any{"cloneFrom": base})
	if isErr {
		t.Fatal(url)
	}
	req, _ := http.NewRequest("GET", url+"/users/$count", nil)
	req.Header.Set("Authorization", "Bearer x")
	req.Header.Set("ConsistencyLevel", "eventual")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if strings.TrimSpace(string(b)) != "7" {
		t.Errorf("the baseline has 7 users, the instance answers %q", b)
	}
	c.tool("stop_instance", nil)
	if text, isErr := c.tool("start_instance", map[string]any{"cloneFrom": t.TempDir()}); !isErr || !strings.Contains(text, "seed.json") {
		t.Errorf("a folder with no data is an error: %v %s", isErr, text)
	}
	if _, isErr := c.tool("start_instance", map[string]any{"cloneFrom": base, "users": 3}); !isErr {
		// the instance runs? it was stopped above, so this is a new start and the two arguments conflict
		t.Error("cloneFrom with users must be an error")
	}
}
