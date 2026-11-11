# MCP server

`dolly mcp` makes dollycli an [MCP](https://modelcontextprotocol.io) server on stdin and stdout. A coding tool (Claude Code, for example) starts it, and can then ask about a call of Graph and test its code against a throwaway dollycli.

## Set up

Claude Code:

```
claude mcp add dolly -- dolly mcp
```

Any other client: run the command `dolly mcp` over stdio.

## Tools

| Tool | What it does |
|---|---|
| `graph_route` | Takes a route, for example `GET /users/{id}`. Says whether dollycli serves it, and gives the summary, the Learn link, the query options, the least-privilege permissions (delegated and application), and the cost in resource units. For a route that nothing matches, it lists routes that are near. |
| `start_instance` | Starts a dollycli Graph in memory on `127.0.0.1` with a free port, and gives the base URL (`http://127.0.0.1:PORT/v1.0`). Any token works. Optional `users` (default 500, as `dolly serve`; seed 1, so the same number gives the same people), or `cloneFrom`: a data folder to start from a copy of (see [`--clone-from`](graph.md#copy-the-data-of-another-folder---clone-from)). The two do not go together. The copy is in memory and the folder is only read. A second call gives the same instance. |
| `stop_instance` | Stops it. The data is lost. The instance also stops when the client closes the connection. |

## What to know

- The summary and the query options of `graph_route` come from Microsoft's OpenAPI description, which is not the truth: it lists options that the real Graph refuses (`$orderby` on `/organization`). Check the real Graph for a behavior that matters.
- The permissions come from the Learn page of the call, as in the portal. The cost is the base cost of the call with no query options.
- The instance listens on `127.0.0.1` only, and has no sign-in, so keep it that way. To test with sign-in, the data folder, or a profile, run `dolly serve` yourself.
- Several agents that need the same start data give each the same `cloneFrom`. Each gets its own copy, so one cannot break the tests of another. They do not share live changes. For that, run `dolly serve` yourself and give them its URL.
- Not there yet: a search by words ("user list"), the OData metadata of a type, and more than one instance.
