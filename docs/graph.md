# dollycli Graph

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

## App registrations and service principals

`/applications` is the **app registration**: the definition of an app, in its home tenant only. `/servicePrincipals` is the **service principal**: the instance of the
app in a tenant, which signs in, gets permissions, and has users assigned. They have the same `appId` (the client ID) and different `id` values, and nothing else
links them. An app of another tenant (a third-party app, Microsoft Graph) has a service principal here and no app registration. Only a service principal
with the tag `WindowsAzureActiveDirectoryIntegratedApp` is an enterprise application.

- `GET`, `POST`, `PATCH`, and `DELETE /v1.0/applications`, and `applications(appId='…')` for a call by client ID. The seed registers two apps: the first
  enterprise application, and the backend service (whose service principal has no enterprise tag). `POST /applications` makes only the registration: a
  service principal appears when a client signs in with its `appId`, and it has the name of the registration.
- `requiredResourceAccess` is what the app asks for. For Microsoft Graph each permission ID must be one that dollycli knows, else the answer is
  404 with what to do (use the permission names, or add [your tenant's permissions](permissions.md)).

Not in this part: the app roles and scopes of a registration are not copied to its service principal (the service principal keeps its own), and there is
no `addPassword`, owners, or federated credentials.

Checked against the real Graph (the sample tenant, read-only): the properties of an application, `GET` by ID, by `appId`, `$filter=appId eq`, the 404 for an
unknown ID, and the owners list. Not checked, because a write cannot be tried there: what `PATCH` and `DELETE` answer, and what the Azure CLI sends.

## Permissions

The service principal Microsoft Graph is in every dollycli, with the permissions of Microsoft's own lists (809 delegated and 763
application: `make update-permissions`). Some of them have an ID that dollycli made, because the list has none: use the
permission names, or add the permissions of [your own tenant](permissions.md), which have the real IDs. A service principal gets a permission as
it does in Entra:

- An **application permission** is an app role assignment (`POST /servicePrincipals/{id}/appRoleAssignments`).
  The client asks for the scope `https://graph.microsoft.com/.default`, and the token has the roles.
- A **delegated permission** is a consent (`POST /oauth2PermissionGrants`). The client asks for
  `https://graph.microsoft.com/User.Read`, and the token has it in the claim `scp`.

Any other service principal with roles or scopes is a resource too, so one app can use the API of another.

In the portal, the page *Service principals* has a panel for each one (the button *Permissions*): pick a resource,
pick the kind, search, and add. *Copy scope* copies the string to put in your app.

Granted permissions are saved and shown in Graph. They do not decide a call yet.

## The data

`--users N` makes N fake people. One of them is always **Philip Meholm** (`philip.meholm@acme.example`), a *Tech Lead* in
Engineering: the last user of the seed. The examples of the portal use this user, and a tool that signs in as a user can
use them too. The address is fake. The other users of a seed are the same with or without this user. Data in a folder
that was made before keeps its old seed: use a new `--dir`, or delete `dolly-data/graph`, to get the new one.

The user before the last is the admin account of Philip Meholm: `a.philip@acme.example` (*Philip (admin)*, Administrator, IT). The quick
connect for PowerShell signs in as this user. A seed of one user has the owner only.

## Where the data of Microsoft comes from

dollycli reads committed copies of Microsoft's own data. It fetches nothing while it runs. Each folder with copies has a `SOURCE.json`
with the commit, the date, and a hash of each file.

| Data | Repository | Update with |
|---|---|---|
| Throttling cost and limits ([graph/cost](../graph/cost/README.md)) | `microsoftgraph/microsoft-graph-docs-contrib` | `make update-cost` |
| Query rules (see [query rules](query-rules.md)) | `microsoftgraph/microsoft-graph-docs-contrib` | `make update-query` |
| The list of permissions ([graph/permissions](../graph/permissions/README.md)) | `microsoftgraph/microsoft-graph-devx-content` | `make update-permissions` |
| The permissions that each call needs ([graph/permissions/operations](../graph/permissions/operations/README.md)) | `microsoftgraph/microsoft-graph-docs-contrib` | `make update-operation-permissions` |
| Routes and properties of Graph (`graph/spec`) | `microsoftgraph/msgraph-metadata` | `make graph-spec` |

The data of `docs-contrib` is CC BY 4.0. The data of the other two repositories is MIT licensed.
