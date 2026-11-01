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

## Adding or changing a Graph resource (plan first)

A resource-sized change (a new endpoint, a new set of properties, a new page) starts with a plan. A small fix or a text change does not.

**1. Probe. Change nothing yet.** Look in this order, from the most general to the most true. When two disagree, the later one wins.

1. **The spec.** [graph/spec/graph-v1.0.json](graph/spec/graph-v1.0.json) is an extract of Microsoft's OpenAPI description. `operations` lists every route with its summary, the Learn page, and the query options. `schemas` has the properties of the types that dollycli serves. It lists more than the service does (it has `$orderby` on `/organization`, and the real Graph refuses it), and it lacks some properties that the real Graph gives. To make it again, run `make graph-spec` (see the [makefile](makefile)).
2. **The Learn page** of the resource and of each call, at the commit that dollycli pins (see `commit` in [graph/cost/raw/SOURCE.json](graph/cost/raw/SOURCE.json)). The spec has the link for each route. The text is at `https://raw.githubusercontent.com/microsoftgraph/microsoft-graph-docs-contrib/<commit>/api-reference/v1.0/api/<name>.md`, and the properties are in `.../resources/<name>.md`. It says which properties can change, and what the call answers.
3. **The real Graph**, through the proxy above. It says what the service does: the error texts, the properties it gives, and the options it refuses.

**2. Write a short plan, and show it before you change anything.**

- The behaviors, each marked *checked* (with where: spec, Learn, or the real Graph) or *guess*.
- The files that change, and the tests.
- What is out of scope (for example the relations, or editing in the portal).
- The risks. Ask: what can a tool set through the API, and where does the portal show it? A value shown as a link, as HTML, or in a command must be checked.
- The choices that are the user's. Ask them in one question, with a recommendation.

**3. Implement after the user agrees.** Follow the checklist [Add a route to Graph](docs/development.md#add-a-route-to-graph), in its order: spec, documentation, relations (the plan above is shown here), simple tests that fail, the route, OData tests, OData details, then cost and permissions. Then check, and say what was run:

- A **new route** (a call that dollycli serves that it did not serve before) needs its permission data: run `make update-operation-permissions`, and commit the changes in [graph/permissions/operations](graph/permissions/operations/README.md). The portal reads them to say which permissions the calls of a client need. The test `TestOperationPermissions_EveryServedRouteHasItsData` fails until you do (from the step of the route on, and that is expected). If a page of Learn has a table that the parser cannot read, the tool stops and names it: change [table.go](graph/permissions/table.go).
- `go test ./...`, and `npm run check` and `npm test` in `ui/` after a change in `ui/src`.
- For a change in the portal, open the page with the Playwright tools and look at it.
- Where the real Graph was asked, a test that holds the answer.

**4. Report** what was checked against the real Graph and what is a guess, and what was left out.

## Writing docs

- Documentation goes in [docs/](docs/README.md). The [README](README.md) is a short introduction and a quick start: two minutes to read at most. Do not add sections to it. Add a page to `docs/`, and a link to that page in `docs/README.md`.
- The text of the *API* page of the portal is the exception: it is in `graph/docs/` because the program embeds it.
- A reference to another document or file is a real markdown link, `[text](relative/path)`, not a bare name or backticks. The reader checks the text by following the link. Paths are relative to the file that has the link.
- After you write or edit a doc, check that every link target exists.

## Sign-in libraries (guidance that must stay true)

[libraries docs](docs/libraries.md) says what each library (MSAL, `@azure/identity`, `oidc-client-ts`, `openid-client`) needs to work with dollycli. `make compat` checks it. After a change in `login/`, `shared/` (TLS, CORS), or a new library version, follow [WORKFLOW](docs/compat/WORKFLOW.md): run the checks, compare them with the page, and update the page only with what the run showed.

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
