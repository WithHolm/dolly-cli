# dollycli
dollycli spins up a fake tenant on localhost. It speaks the OAuth/OIDC of Entra ID and the endpoints of Microsoft Graph, with a stateful directory behind them. Add a user to a group, and memberOf, members and token claims all update.

Make a tenant (`dolly serve --users 50`), test against it, then go back to the seed with `dolly graph reset`. Entra first, Azure data plane next.

Warning: dollycli is a development tool. It issues tokens no real service will trust, binds to localhost by default, and must never be used as an identity provider.

dollycli is not affiliated with or endorsed by Microsoft. Entra, Azure and Microsoft Graph are trademarks of Microsoft Corporation. Its just a product i really needed, and might think you do too :)

## dollycli Graph

A fake Microsoft Graph with fake data. It is a Go package and a server with a CLI.

As a server:

    go install github.com/withholm/dolly-cli/cmd/dolly@latest
    dolly serve --dir dolly-data --port 8650 --users 500 --seed 1

`dolly serve` runs every API on one address. `dolly graph serve` runs Graph alone.

The Graph address is `http://127.0.0.1:8650/v1.0`. Any token works. The portal is at
`http://127.0.0.1:8650/`, with a page for each API. The admin API of Graph is at `/_dolly/graph/`. Settings are flags or env (`DOLLY_DIR`, `DOLLY_PORT`, `DOLLY_USERS`,
`DOLLY_SEED`, `DOLLY_PAGE_SIZE`). The Graph data is in `dolly-data/graph/`: `seed.json`, `state.jsonl`, and `.lock`.
In a second terminal: `dolly graph show | rename | move | reset | history | rollback | generate`.

As a package, for tests:

    import "github.com/withholm/dolly-cli/graph"

    srv := graph.New(graph.NewMemory(graph.Options{Users: 50, Seed: 1}), graph.ServerOptions{})
    ts := httptest.NewServer(srv.Handler())
    // use ts.URL + graph.BasePath as the Graph address

## Permissions

The service principal Microsoft Graph is in every dollycli, with the real permissions (598 delegated and 534
application, from Microsoft's own list: `make update-permissions`). A service principal gets a permission as
it does in Entra:

- An **application permission** is an app role assignment (`POST /servicePrincipals/{id}/appRoleAssignments`).
  The client asks for the scope `https://graph.microsoft.com/.default`, and the token has the roles.
- A **delegated permission** is a consent (`POST /oauth2PermissionGrants`). The client asks for
  `https://graph.microsoft.com/User.Read`, and the token has it in the claim `scp`.

Any other service principal with roles or scopes is a resource too, so one app can use the API of another.

In the portal, the page *Service principals* has a panel for each one (the button *Permissions*): pick a resource,
pick the kind, search, and add. *Copy scope* copies the string to put in your app.

Granted permissions are saved and shown in Graph. They do not decide a call yet.

## Run in a container

    docker build -t dolly .
    docker run --rm -p 127.0.0.1:8650:8650 -v dolly-data:/data dolly

Or `docker compose up -d` (see `compose.yaml`). The image is one static program with the portal built in, and it runs as a
user that is not root. The data is in the volume `/data`.

- **Listen address.** dollycli listens on `127.0.0.1` as it comes. In the image it is `0.0.0.0` (`--host`, or `DOLLY_HOST`): inside a container,
  loopback cannot be reached from outside. **Publish the port to `127.0.0.1` only**, as above. dollycli has no sign-in of its own:
  anyone who can reach the port can read and change the data, and dollycli says so in the log when it listens on more than loopback.
- **Other containers** reach it by the name of the service, for example `http://dolly:8650/v1.0/users`. The Graph and sign-in APIs take any
  name. The admin API (and so the portal) takes `localhost` and `127.0.0.1` only, unless you allow a name: `--allow-host dolly`, or
  `DOLLY_ALLOW_HOSTS=dolly`. The addresses in the sign-in documents follow the name that the client used.
- **Stop and start.** `docker stop` is a clean stop (SIGTERM closes the data). A container that was killed leaves a lock in the volume,
  and the next one takes it over. In a container dollycli is always process 1, so it must know that a lock with its own process ID is old.
- **Health check.** `dolly health` asks the running dollycli for its list of APIs. The image runs it, as it has no shell and no curl.
- **Settings** are the same as the flags, as environment variables: `DOLLY_USERS`, `DOLLY_SEED`, `DOLLY_PAGE_SIZE`, `DOLLY_THROTTLE=1`,
  `DOLLY_PORT`, `DOLLY_HOST`, `DOLLY_ALLOW_HOSTS`. The first start makes the seed, so `DOLLY_USERS` counts then.

## The data

`--users N` makes N fake people. One of them is always **Philip Meholm** (`philip.meholm@acme.example`), a *Tech Lead* in
Engineering: the last user of the seed. The examples of the portal use this user, and a tool that signs in as a user can
use them too. The address is fake. The other users of a seed are the same with or without this user. Data in a folder
that was made before keeps its old seed: use a new `--dir`, or delete `dolly-data/graph`, to get the new one.

The user before the last is the admin account of Philip Meholm: `a.philip@acme.example` (*Philip (admin)*, Administrator, IT). The quick
connect for PowerShell signs in as this user. A seed of one user has the owner only.

## Query rules

dollycli follows the rules of Graph for `$filter`, `$orderby`, `$search`, and `$count`, and it answers with the error that Graph gives.
For a directory object (users, groups, service principals), Graph decides by property and operator what works, and what needs an
*advanced query*: the header `ConsistencyLevel: eventual` and `$count=true`. The rules are Microsoft's own tables
(*Advanced query capabilities on Microsoft Entra ID objects*). `make update-query` downloads them into `graph/query/raw/` and
reads them into `graph/query/matrix.json`. A test fails if the two differ, and the parser fails on a table that has changed its form.

- `$filter` is OData: `eq ne gt ge lt le in`, `and or not`, `startsWith endsWith`, `any` and `all`, `/$count`, `null`, dates, GUIDs.
- `ne` needs an advanced query (the header and `$count=true`). `not` and `endsWith` need only the header. Some properties
  need an advanced query too. Without it the answer is `Request_UnsupportedQuery`, with the text of Graph.
- `$search` needs the header. A clause is `"property:text"`, joined with `AND`, `OR`, and parentheses.
- `/$count` without the header is `Request_UnsupportedQuery`. `?$count=true` without it is left out, with no error.
- `$orderby` works on the properties in the table of sorting. With `$filter` it always needs an advanced query.
- `$top` must be a whole number from 1 to 999 on a directory object.
- `$expand` works on a user for `manager` and `directReports`.
- An answer has the headers of Graph (`request-id`, `client-request-id`, `OData-Version`, the OData `Content-Type`), `@odata.context`,
  and the default properties with `null` for a property with no value. An error has an `innerError`. `$select` returns only
  what it names (no `id` unless you ask for it).

**Where the texts come from.** Microsoft's tables say which operators work on which property, and the real Graph answers
the rest. The texts of the errors and the shape of the answers were checked against the real Graph (the sample tenant
of Graph Explorer). The tables are not complete: Graph answers some filters on properties that they do not list, so dollycli
allows a property that Graph declares and the tables do not list.

## Sign-in

`dolly serve` also runs a fake Entra sign-in API, on the same port. A client gets a real, signed token (RS256) and
Graph checks it. The paths are the ones of Entra, with the tenant first. The tenant is the tenant ID, one of its
domains, or `common`:

    http://127.0.0.1:8650/<tenant>/v2.0/.well-known/openid-configuration
    http://127.0.0.1:8650/<tenant>/discovery/v2.0/keys
    http://127.0.0.1:8650/<tenant>/oauth2/v2.0/token        (and /oauth2/token, with resource=)

    curl -s http://127.0.0.1:8650/<tenant>/oauth2/v2.0/token -d grant_type=client_credentials \
      -d client_id=$(uuidgen) -d client_secret=anything -d scope=https://graph.microsoft.com/.default

- **Grants:**
  - `client_credentials`: an application token. `roles` are the application permissions of the service principal.
  - `authorization_code`: the browser flow, with PKCE (S256 or plain). The client sends the browser to
    `/<tenant>/oauth2/v2.0/authorize`. dollycli shows a page with the users and **no password**: pick one, and the
    browser goes back to the `redirect_uri` with a code (`response_mode` query, fragment, or form_post). A code works
    once, for ten minutes. A client ID or a redirect address that is wrong gives an error page, not a redirect.
  - `password`: a delegated token for a user. dollycli accepts any password.
  - `refresh_token`: with `offline_access`. A refresh token is a signed JWT, so it is good for 90 days and after a restart.
  - A delegated token has `scp`, the user (`oid`, `upn`), and an `id_token` with `openid` (with the `nonce`).
  - Not built: device code, `response_type` with a token or id_token, consent pages.
- **`/me`:** with a delegated token, `/me` and `/me/...` (also in `$batch`) are the user of the token. With an
  application token, or a token that dollycli did not make, `/me` is 400, as in Graph.
- **New clients:** a client ID that dollycli has not seen gets a service principal (open mode). Use any secret, or a
  certificate in `client_assertion`. In the portal, *Capture next secret* keeps the next one that a client shows
  (only a hash is saved), and dollycli refuses all others from then on (`AADSTS7000215`).
- **Graph:** a token that dollycli made is checked. A changed, expired, or wrong-audience token is a 401. The call is counted for
  the client ID in the token. Any other token still works, as before.
- **The key** is `dolly-data/login/key.pem`, so a token is good after a restart. It is a fake key.
- **Microsoft Graph PowerShell:** one line connects it, as the admin account `a.philip@acme.example`: `iwr 'http://127.0.0.1:8650/pwsh' | iex`.
  dollycli gives three lines at `/pwsh`. They run `Add-MgEnvironment -Name Dolly -AzureADEndpoint <address> -GraphEndpoint <address>`, get a token from
  `/jwtToken?hint=<user principal name or object ID>`, and run `Connect-MgGraph -Environment Dolly -AccessToken ...` with it. `/pwsh?user=` sets another user.
  The token is good for ten years, and it has a message in its claims. The page *Sign-in* has the lines to copy.
  The module's own sign-in (secret, certificate, browser, device code) needs an https address, so it does not work: dollycli has no https.

A call that fails answers as Entra does: `error`, `error_codes`, and `AADSTS` numbers in the text. The portal page
*Sign-in* has the addresses to copy, and a list of every try.

## Layout

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

## The portal

The portal is Svelte 5 with TypeScript and Vite. `ui/dist` is in the repo, so `go install` needs no Node. After a
change in `ui/src`:

    make ui        # npm ci, a type check, the tests of the logic, and the build into ui/dist. Commit the new ui/dist.
    make ui-dev    # live reload on http://localhost:5173, against a 'dolly serve' on port 8650

The portal has no state of its own. The pages Users, Groups, and Organization (and the sample user on the Sign-in page) are a client of the
Graph API: the portal signs in as its own service principal, *Dolly portal*, and calls `/v1.0` like any tool. That service principal is not
throttled, and its calls are not in the list of calls. The other pages (Calls, Service principals, Data, History, API) use the admin API of
each API (`/_dolly/<name>/`), because they show what Graph does not have: usage, behavior, the history, and the permissions of a client.

On the page *Users*, a click selects a user, and a double click (or Enter, or the button *Open details*) opens a dialog with four tabs:
*Properties* (and the job, which you can change), *Organisation* (the managers above, the user, and the reports, as a tree: a click on a name
opens that person), *Groups*, and *Enterprise applications* (the app roles that the user has, also through a group, and the consents that apply). The address has
the user and the tab (`#/graph/users/<id>/organisation`), so a link opens the dialog on a tab.

The page *Service principals* works the same way. The dialog has three tabs: *Overview*, *Sign-in* (permission mode and credential), and
*Permissions*. On the *Overview* tab, two switches set how dollycli treats the calls of that client. *Never throttle this client* lets it
pass the limits of Graph (its cost is still counted). *Keep its calls out of the list of calls* hides its calls from the page *Calls*.
The same switches are in the admin API: `PUT /_dolly/graph/service-principals/<client ID>/flags` with `exemptFromThrottle` or `hideCalls`.

The page *API* of Graph is a reference with one page for each resource (users, groups, and so on). A call is closed
until you open it, and the open call is in the address, so a link points to it. It shows what dollycli answers for the
call (the options, the forms of `$filter` and `$search`), what Graph says about it (the page of Microsoft Learn,
the properties that `$filter` works on), and the difference between the two. The forms of `$filter` are read from the
table in `graph/api/query.go` that answers them, and a test sends the example of each form to a server.

### Write the documentation

The text of the *API* page is in markdown files, `graph/docs/*.md`, and not in code. They are built into the program.
One file is one page. To add a page, add a file: the name, the order, and the text of the page are in the file, and
the portal needs no change. A `## GET /v1.0/users` title adds text to that call. Its parts (`### Filtering`,
`### Search`) show next to the text that the code makes, with the label *For this call*, so a call can say what is
special about it (a filter that Graph has and dollycli refuses, for example). `graph/docs/README.md` has the format.

A test fails for a title that is not a call, and for a call with no page. A call with no text is fine. Its
one-line summary is in the code, with the route, so the two cannot differ.

While you write, use a `dev` build with the folder: `dolly serve --docs graph/docs` (or `DOLLY_DOCS`). It reads the files
at each reload of the page, and a mistake in a file keeps the last good text. A release build
(`make release VERSION=1.2.3`) has no `--docs` flag: the flag exists only when `version` is `dev`.
