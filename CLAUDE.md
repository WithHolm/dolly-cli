# Dolly

## Finding and changing code (graft)

This repo is indexed by graft. Ask graft first. Do not search with `sed`, `awk`, or a loop of `cat` and `grep`.

- Where is it, how does it work: `graft ask "<task>" --source`.
- Every occurrence of a text: `graft grep "<literal>"`. `ask` is ranked and misses some.
- The API of one file: `graft skeleton <file>`. It costs far less than reading the file.
- Who calls a symbol, and what a change breaks: `graft callers <symbol>`. Use `--depth all` before a rename or a move, and change all the files it lists.
- An unknown area: `graft map`. One call is the answer.
- Know the file already? Go to it: `graft grep "<symbol>"`, read that span, edit.
- Make the change with the Edit tool only. No `sed` or scripts to write files. `gofmt -w` and `gopls` are fine.
- Graft shows the saved tokens on top of its output. Do not pipe it through `head` or `tail`.

## Checking how the real Graph answers

Dolly must be 1:1 with Graph. When a behavior is a guess (error texts, response shape, headers, paging), ask the real Graph first through the proxy of Graph Explorer, then fix Dolly to match.

```
curl -s -i \
  -H 'Authorization: Bearer {token:https://graph.microsoft.com/}' \
  -H 'SdkVersion: GraphExplorer/4.0' \
  -H 'Content-Type: application/json' \
  'https://graphexplorer.microsoft.com/api/proxy?url=https%3A%2F%2Fgraph.microsoft.com%2Fv1.0%2Fme'
```

- The `Authorization` value is a literal placeholder. The proxy uses its own sample token. Never send a token or secret of the user.
- Read-only GET only. The data is the sample tenant of Microsoft (user Alex Wilber).
- Put the Graph URL in `url=`, percent-encoded (`$` is `%24`). Add `ConsistencyLevel: eventual` as a header when the call needs it.
- Without the headers above the proxy answers 403. Do not spoof `Origin`, `Referer`, or a browser User-Agent, and do not get around a 403 or a bot check.
- In the answer, say which behaviors were checked against the real Graph and which were not.
- The tables of Microsoft Learn are not complete. Trust the real answer over the table.

## A running Dolly for checks (air)

Many checks need a running server. `.air.toml` gives one that builds the portal (`ui/dist`) and the program again at every change.

- Address: `http://127.0.0.1:8651`. Not 8650, so it does not meet the Dolly of the user.
- Data: in memory (`--dir=`). Every start has the same fresh seed. `dolly-data` is not touched.
- The build runs `npm run build` and `go build`. It skips `npm run check` and the tests: run `make ui` and `make test` for those.
- The user starts it with `make dev`. Claude starts its own, with a name of its own, so other air processes (other projects) are not hit:

```
ln -sf "$(command -v air)" tmp/dolly-air
setsid tmp/dolly-air > tmp/air.log 2>&1 &
```

- Read `tmp/air.log` to see if a build failed. Wait for `Dolly is running` in it, then use the server.
- Stop it with `pkill -x dolly-air`. Never `pkill -x air`: that stops every air on the machine.
- A touch of a file does not start a build. A write does.
