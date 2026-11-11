package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/withholm/dolly-cli/graph"
)

// mcpCmd runs dolly as an MCP server on stdin and stdout, for a coding tool. See docs/mcp.md.
func mcpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Run as an MCP server on stdin and stdout",
		Long: "Run as an MCP server (Model Context Protocol) on stdin and stdout, for a coding tool. It has three tools:\n" +
			"graph_route tells what dollycli knows about a call of Graph, and start_instance and stop_instance run a\n" +
			"throwaway dollycli (in memory, on 127.0.0.1) that code can be tested against.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return serveMCP(cmd.InOrStdin(), os.Stdout) },
	}
}

type rpcRequest struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

var mcpTools = []mcpTool{
	{"graph_route",
		"What dollycli knows about one call of Microsoft Graph v1.0: whether dollycli serves it, the summary and Learn link, the query options, the least-privilege permissions (delegated and application), and the cost in resource units. The summary and options are from Microsoft's OpenAPI description, not from the real Graph.",
		schema(map[string]any{"route": map[string]any{"type": "string", "description": `Method and path, for example "GET /users/{id}". The names in {} do not matter.`}}, "route")},
	{"start_instance",
		"Start a throwaway dollycli Graph server with fake data, in memory, on 127.0.0.1. Any token works. Returns the base URL (ending in /v1.0). If one runs, returns its URL.",
		schema(map[string]any{
			"users":     map[string]any{"type": "integer", "description": "How many users the fake tenant has (1 to 100000, default 500, as dolly serve). Seed 1, so the same number gives the same people."},
			"cloneFrom": map[string]any{"type": "string", "description": "Instead of new fake data, start from a copy of this data folder (a data root such as dolly-data, or the folder with seed.json), for example to reproduce a problem in someone's data. The folder is only read, and the copy is in memory. Not together with users."},
		})},
	{"stop_instance", "Stop the instance that start_instance made. The data is lost.", schema(map[string]any{})},
}

func schema(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

// instance is the one throwaway server of start_instance.
type instance struct {
	mu  sync.Mutex
	srv *http.Server
	url string
}

func (i *instance) start(users int, cloneFrom string) (string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.srv != nil {
		return i.url, nil
	}
	var world *graph.World
	if cloneFrom != "" {
		if users != 0 {
			return "", errors.New("cloneFrom and users do not go together: the copy has the data of its source")
		}
		var err error
		if world, err = graph.OpenClone(cloneFrom); err != nil {
			return "", err
		}
	} else {
		if users == 0 {
			users = 500 // as dolly serve
		}
		if users < 1 || users > 100000 {
			return "", errors.New("users must be a whole number from 1 to 100000")
		}
		world = graph.NewMemory(graph.Options{Users: users, Seed: 1})
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0") // never a LAN address: any token works
	if err != nil {
		return "", err
	}
	g := graph.New(world, graph.ServerOptions{})
	i.srv = &http.Server{Handler: g.Handler(), ReadHeaderTimeout: 10 * time.Second}
	i.url = "http://" + ln.Addr().String() + graph.BasePath
	g.SetGraphURL(i.url)
	go func() { _ = i.srv.Serve(ln) }()
	return i.url, nil
}

func (i *instance) stop() {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.srv != nil {
		_ = i.srv.Shutdown(context.Background())
		i.srv = nil
	}
}

// serveMCP answers JSON-RPC lines from in on out until in ends. Only the part of the protocol that three tools need.
func serveMCP(in io.Reader, out io.Writer) error {
	var inst instance
	defer inst.stop()
	enc := json.NewEncoder(out)
	sc := bufio.NewScanner(in)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var req rpcRequest
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			continue
		}
		if len(req.ID) == 0 { // a notification gets no answer
			continue
		}
		reply := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		if res, err := mcpHandle(&inst, req); err != nil {
			reply["error"] = map[string]any{"code": -32601, "message": err.Error()}
		} else {
			reply["result"] = res
		}
		if err := enc.Encode(reply); err != nil {
			return err
		}
	}
	return sc.Err()
}

func mcpHandle(inst *instance, req rpcRequest) (any, error) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		if p.ProtocolVersion == "" {
			p.ProtocolVersion = "2025-03-26"
		}
		return map[string]any{
			"protocolVersion": p.ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "dolly", "version": version},
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": mcpTools}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, err
		}
		text, err := mcpCall(inst, p.Name, p.Arguments)
		res := map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}}
		if err != nil {
			res["content"] = []any{map[string]any{"type": "text", "text": err.Error()}}
			res["isError"] = true
		}
		return res, nil
	}
	return nil, fmt.Errorf("method not found: %s", req.Method)
}

func mcpCall(inst *instance, name string, raw json.RawMessage) (string, error) {
	switch name {
	case "graph_route":
		var a struct {
			Route string `json:"route"`
		}
		_ = json.Unmarshal(raw, &a)
		method, path, ok := strings.Cut(strings.TrimSpace(a.Route), " ")
		if !ok || strings.TrimSpace(path) == "" {
			return "", errors.New(`route must be a method and a path, for example "GET /users/{id}"`)
		}
		b, _ := json.MarshalIndent(graph.Describe(method, path), "", "  ")
		return string(b), nil
	case "start_instance":
		var a struct {
			Users     int    `json:"users"`
			CloneFrom string `json:"cloneFrom"`
		}
		_ = json.Unmarshal(raw, &a)
		return inst.start(a.Users, a.CloneFrom)
	case "stop_instance":
		inst.stop()
		return "stopped", nil
	}
	return "", fmt.Errorf("no such tool: %s", name)
}
