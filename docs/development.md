# Layout and development

Each API is its own package, with its own data folder, and its own subcommand. The API `graph` does not import
the others. The API `login` works on the data of `graph`, so it imports it.

    graph/        the Graph API (api, repo, rule, spec inside)  -> dolly graph ..., dolly-data/graph/
    login/        the sign-in API: discovery, keys, tokens      -> dolly-data/login/key.pem
    graph/query/  the rules of Graph for $filter and $orderby, from Microsoft's tables (make update-query)
    az/           the Azure API, when it comes                  -> dolly az ...,    dolly-data/az/
    shared/       what the APIs have in common: the API interface, the router, the admin guard
    ui/           the portal: a Svelte app (src/) that is built into dist/, which the Go program embeds
    cmd/dolly/    the CLI (dolly serve runs every API; add an API to the list in serve.go)

To add an API: make its package, implement `shared.API`, and add one line to `apis` in `cmd/dolly/serve.go`.
Its page in the portal is a folder in `ui/src/` (see `ui/src/graph/`) and one line in `ui/src/App.svelte`.

## Add a route to Graph

A route is a call that dollycli answers, for example `GET /users/{id}/manager`. Do these steps in this order: the facts first, then tests that fail, then the code,
then the details. A resource-sized change (a new endpoint, a new set of properties) also needs the plan of [CLAUDE.md](../CLAUDE.md): show it after step 3
and wait for a yes. A small fix can skip the plan, and still uses the steps that fit.

Some steps are for a **stored** resource only (one that can be made, changed, or deleted, and that is in the history). They say so.

1. **The spec.** Add the path prefix (and the schema names of its objects) to [graph/spec/gen/main.go](../graph/spec/gen/main.go), run `make graph-spec`,
   and read the operations and properties that come out. The spec lacks the alternate keys, such as `applications(appId='…')`, and it lists more than the
   service does (it has `$orderby` on `/organization`, which the real Graph refuses). There are two descriptions, see [graph/spec](../graph/spec/README.md): the OpenAPI
   one (routes, summaries, properties) and the **OData** one (CSDL: the types and their base types, the navigation properties between them, containment, the entity sets). Read
   the OData one for step 3: it says what a resource relates to. Add the names of the new types to `metadataRoots`, and of the new entity sets to `metadataSets`, in `gen/main.go`:
   dollycli serves that description at `/v1.0/$metadata`, and a test checks it.
2. **The documentation.** Read the Learn pages of each call (at the commit of [SOURCE.json](../graph/cost/raw/SOURCE.json)), and ask the real Graph through the
   proxy of Graph Explorer (GET only, see [CLAUDE.md](../CLAUDE.md)): the properties, the error texts, the paging, the headers. Write down what each behavior is:
   *checked* (and where), or *guess*. A write cannot be tried on the real Graph, so it is a guess unless a page says it.
3. **The relations.** Which other resources does it link to (users, groups, service principals)? Is it read-only or stored? What does it do to the others when it
   changes, and what does the portal show? **Plan checkpoint:** show the plan, with the guesses, and wait for the answer.
4. **Simple tests first.** Write tests for the behaviors of step 2: list, get, the 404, and for a stored resource make, change, and delete. For a stored resource also
   test the history, a restart, and a rollback. The test server in memory has no history: open a data folder (see `graph/repo/applications_test.go`). They fail now.
5. **The route.** Write it in `graph/api`, with the data in `graph/repo`. Mount it in [graph/mount.go](../graph/mount.go) and add it to `schemaOf` in
   [graph/endpoints.go](../graph/endpoints.go). A stored resource also needs its field in `World`, the lookups in `index()`, `replaceWith`, and its events in the two
   lists in `journal.go` (the text and the apply). An alternate key is a rewrite in `graph/server.go`. If the seed gets new data, it must draw nothing from the
   random generator, or the hashes of `golden_test.go` change.
6. **OData tests.** Add tests for `$select`, `$filter`, `$orderby`, `$top`, `$count` with and without `ConsistencyLevel`, and paging, as the real Graph answers them.
7. **The OData details.** Set `ListOptions` (what is filterable, the resource kind for the rules, the default properties), the rules of `graph/api/query.go`, and the
   headers, until the tests of step 6 pass.
8. **The cost and the permissions.**
   - Cost: see if the table of costs ([graph/cost](../graph/cost/README.md)) has a row for the route. Run `go test ./graph -run TestCost_RoutesWithNoRow -v`: it lists
     the routes with no row, and they cost only the default of their method (1 RU, and 1 write unit for a write), which Microsoft does not publish. If your route is in
     that list, run `make update-cost` and make `cost.json` again (the skill `/update-cost` does it: it takes a while, so do it and do not skip it), then run the test again.
     If the route is still in the list, Microsoft's tables do not name it: **ask the user** for the cost (or for where it is published), and do not guess a number.
     A cost that you are given goes in [overrides.json](../graph/cost/overrides.json) with its source (see [the cost data](../graph/cost/README.md#add-a-cost-that-a-person-reported)).
     The default is fine when nobody knows: the cost is a best guess, and it is not shown as Microsoft's.
   - Permissions: run `make update-operation-permissions` and commit what it changes. It reads the pages of Microsoft Learn about the calls, finds the page that
     is about your route, and keeps the permissions that it needs, so that the portal can say what a client needs. It also makes the map of every call of Graph
     and its permissions, `all.json`. See [graph/permissions/operations](../graph/permissions/operations/README.md). The test
     `TestOperationPermissions_EveryServedRouteHasItsData` fails until you do (it fails from step 5 on, and that is expected).
9. **The words.** Add the page of the API in [graph/docs](../graph/docs/README.md): front matter with `resource:` and an `order` that no other page has. The test
   `TestDocs_FitTheRoutes` fails without it.
10. **The portal**, if it shows the resource: the admin API, the types, the page, then `npm run check`, `npm test`, and `npm run build` in `ui/`, and look at the page.
11. **Run everything:** `go test ./...`, `go vet ./...`, `gofmt -l .`, and the checks of `ui/`. In the report, say what was checked against the real Graph and what is a guess.

## Make targets

| Command | What it does |
|---|---|
| `make test` | Runs the Go tests. |
| `make ui` | Checks, tests, and builds the portal into `ui/dist`. See [the portal](portal.md). |
| `make dev` | A dollycli on port 8651 that builds the portal and the program again at every change ([.air.toml](../.air.toml)). Its data is in memory. |
| `make compat` | Checks that sign-in libraries work with dollycli. See [library checks](compat/README.md). |
| `make update-cost`, `make update-query`, `make update-permissions`, `make update-operation-permissions`, `make graph-spec` | Download the data of Microsoft again. See [dollycli Graph](graph.md). |
| `make release VERSION=1.2.3` | A release build. It has no `--docs` flag. |
