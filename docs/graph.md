# dollycli Graph

A fake Microsoft Graph with fake data. It is a Go package and a server with a CLI.

As a server:

    go install github.com/withholm/dolly-cli/cmd/dolly@latest
    dolly serve --dir dolly-data --port 8650 --users 500 --seed 1

`dolly serve` runs every API on one address. `dolly graph serve` runs Graph alone.

The Graph address is `http://127.0.0.1:8650/v1.0`. Any token works. The portal is at
`http://127.0.0.1:8650/`, with a page for each API. The admin API of Graph is at `/_dolly/graph/`. Settings are flags or env (`DOLLY_DIR`, `DOLLY_PORT`, `DOLLY_USERS`,
`DOLLY_SEED`, `DOLLY_PAGE_SIZE`). The Graph data is in `dolly-data/graph/`: `seed.json`, `state.jsonl`, and `.lock`.

**If port 8650 is busy**, dollycli stops with `address already in use` and exit code 1. It does not pick another port, because the tools that you point at it have a fixed address. Use `--port 8651` (or `DOLLY_PORT`). The port is tried first, so a failed start leaves no data folder behind. If another dollycli runs on the same data folder, the message names its process and its address, and says to stop it or point your tool at that address (delete `dolly-data/graph/.lock` only if no dollycli runs).
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

Not in this part: there is no `addPassword`, owners, or federated credentials.

### Exposing an API (scopes)

An app registration exposes an API with an **Application ID URI** (`identifierUris`), **scopes** (`api.oauth2PermissionScopes`, the delegated permissions that a client can
ask for), and **authorized clients** (`api.preAuthorizedApplications`). All of it is written with `PATCH /applications`: Graph has no route for a scope, so there is
no new route here.

- A client asks for a scope as the URI, a slash, and the value (`api://files-api/Files.Read`), and the token has the value in `scp`. A scope has `id`, `value`,
  `type` (`User`: a user can consent for himself or herself, `Admin`: only an administrator), `isEnabled`, and the consent texts
  (`adminConsentDisplayName`, `adminConsentDescription`, `userConsentDisplayName`, `userConsentDescription`).
- The rules are from the Learn pages *permissionScope* and *apiApplication*: the `value` follows the rules of the value of an app role, a scope is disabled in one call and
  removed in a later one, and `id` is a GUID. The checks that are dollycli's own guesses: the two admin consent texts are required, an Application ID URI is `api://…`
  or `https://…`, and no other registration has it (the real Graph also asks for a verified domain in an `https` URI).
- `knownClientApplications` is a list of client IDs. It is kept, and it changes nothing yet.
- **The version of the access tokens.** `requestedAccessTokenVersion` (null, 1, or 2; null is 1) says which version of access token the API gets, whatever endpoint (v1.0 or
  v2.0) the client used: the endpoint only decides the version of an ID token (Learn: *apiApplication*). Version 1 gives `ver` `1.0`, the issuer
  `https://sts.windows.net/<tenant>/`, and `aud` the Application ID URI of the API (its client ID when it has none). Version 2 gives `ver` `2.0`, the issuer of dollycli
  (`<address>/<tenant>/v2.0`), and `aud` the client ID. A library that validates the tokens of an API must expect the audience and the issuer of the version. When
  `signInAudience` lets personal Microsoft accounts in (`AzureADandPersonalMicrosoftAccount` or `PersonalMicrosoftAccount`) the version must be 2 (a rule of Learn). A call that makes
  an app personal and says nothing about the version sets it to 2 (a guess). Microsoft Graph keeps the tokens that it had.
- The service principal of the app follows the registration: its scopes and its `servicePrincipalNames` change when the registration changes, and a scope that is
  removed (or gets a new value) is taken out of the consents that have it. A consent with no scope left is deleted.
- A disabled scope cannot be consented to or asked for (`AADSTS70011`, as for an unknown scope).
- An **authorized client** needs no consent for the scopes that it is listed with: when it asks for `.default`, the token has them. Each scope must be a scope of the app, and a
  scope that is removed goes out of the list. In the *Open* permission mode a client can ask for any scope by name, with or without consent: the mode *Enforce* is not active yet.

Checked against the real Graph (the sample tenant, read-only): the shape of a scope (`user_impersonation`, `type: User`, with the four consent texts). From Learn, and not
tried: the rules, and that the version of an access token is the one that the API asks for. Guesses: the texts of the errors, that pre-authorized scopes count as consented for
`.default`, and what the real Graph does with the consents of a scope that is removed. From the docs of Entra as remembered, not from Learn: that the audience of a version 1
token is the Application ID URI. The claims of a token are the same for both versions apart from `iss`, `ver`, and `aud`: a real version 2 token has `azp` and no `appid`, and
dollycli's tokens have both.

### App roles

An **app role** is what a user, a group, or another app can be given in an app. The app registration defines the roles (`appRoles`), and the service principal of
the app has them, with `origin` `Application`. A role that only this tenant needs is set on the service principal (`origin` `ServicePrincipal`).

- A role has `id`, `displayName`, `description`, `value`, `allowedMemberTypes` (`User`, `Application`, or both), and `isEnabled`. The rules are from the Learn page
  *appRole resource type*: a new role needs an `id` that is a GUID, a `value` has at most 120 characters from a list of characters (no space, and it does not
  start with a dot), `origin` is read-only, and `Application` is a member type of the roles of an application only. A role is disabled in one call (`isEnabled`
  false), and removed in a later one. A role that is removed takes its assignments with it.
- A change to `appRoles` on the registration reaches its service principal, and a service principal made from a registration starts with its roles.
- `POST /servicePrincipals/{id}/appRoleAssignedTo` gives a user, a group, or a service principal a role of the app. The role must exist, be enabled, and allow
  the type: a role for `Application` only is not for a user. `GET` of one assignment, and `/$count` (it needs `ConsistencyLevel: eventual`), work on
  `appRoleAssignedTo` and on `appRoleAssignments`.
- **The token has the roles.** When a user signs in to the app, the ID token has `roles` with the `value` of each role of the user, and so has the access token for
  the app. The roles are those that are assigned to the user, and to a group that the user is a direct member of (a group in a group does not count). A role with no
  value, a disabled role, and *Default access* are not in the claim. See [sign-in](sign-in.md).
- The [portal](portal.md) edits the roles on the page *App registrations*, and assigns them on the tab *Users and groups* of a service principal.

Checked against the real Graph (the sample tenant, read-only): the shape of an app role and of an assignment, `GET` of one assignment (200, with
`@odata.context` `#appRoleAssignments/$entity`), and the 400 `Request_UnsupportedQuery` for a raw `/$count` with no `ConsistencyLevel`. From Learn, and not
tried: the rules above. Guesses: the texts of the errors, that a removed role takes its assignments, and the claim `roles` for a group (Entra's own
rules for nested groups come from memory). The IDs of assignments are `<principal ID>.<role ID>`, and the real Graph writes a 43-character string: a client
treats the ID as opaque, and it is not made the same yet.

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

The default tenant is **Fjordvik**, a made-up Norwegian energy company (`fjordvik.example`): see [profiles](profile.md). A data folder that was made before keeps its
old seed, which was Acme (`acme.example`), and dollycli reads it as Acme.

`--users N` makes N fake people. One of them is always **Philip Meholm** (`philip.meholm@fjordvik.example`), an *Identitetsarkitekt* in
*IT og digitalisering*: the last user of the seed. (In Acme he is a *Tech Lead* in Engineering: the profile says.) The examples of the portal use this user, and a tool that signs in as a user can
use them too. The address is fake. The other users of a seed are the same with or without this user. Data in a folder
that was made before keeps its old seed: use a new `--dir`, or delete `dolly-data/graph`, to get the new one.

The user before the last is the admin account of Philip Meholm: `adm.philip@fjordvik.example` (*Philip (adm)*, Administrator, IT og digitalisering). The quick
connect for PowerShell signs in as this user. A seed of one user has the owner only.

The user before the admin account is **Nora Helmer** (`nora.helmer@fjordvik.example`), a member of the tenant, from Ibsen's *A Doll's House*. The address uses the domain of the profile. She keeps the
department and the job title that the seed gave that user. A seed of fewer than three users has no room for her. Like the owner, she draws nothing from the random numbers.

Three more people from the play are there when there is room, as the users four, five, and six from the end (a seed of four users has Torvald, five has Nils too, six has Dr. Rank too):

| Person | Address | What they are |
|---|---|---|
| **Torvald Helmer** | `torvald.helmer@<domain>` | Nora's boss, in her department and office. He has the title of that department that says that someone leads (*Salgsleder* at Fjordvik, *Sales Manager* at Acme), when it has one. He keeps the manager he got from the tree. |
| **Nils Krogstad** | `nils.krogstad@<domain>` | A colleague of Nora: the same department and office. His account is **disabled**. Torvald is his manager too. |
| **Dr. Rank** | `dr.rank_<guest domain>#EXT#@<initial domain>` | A **guest**, invited and accepted, as a generated guest is. He has no manager. |

They are made after the users and before the manager tree and the groups, so the department and office groups agree with them. The links to Torvald are set after the tree is made, and a loop is checked for: if Torvald's own manager chain reaches Nora or Nils, Torvald has no manager. Like the owner, they draw nothing from the random numbers.

## Copy the data of another folder (`--clone-from`)

`dolly serve --clone-from SOURCE` serves a copy of the data in another data folder. SOURCE is a data root, as `--dir` is (dollycli reads `SOURCE/graph`), or the folder with `seed.json`. The seed and every change in `state.jsonl` are in the copy. The source is only read: it is never changed, it is not locked, and a dollycli can run on it while you copy. `--users`, `--seed`, and `--profile` do not go with it, because the copy has the data of its source.

- Without `--dir` the copy is in memory, and gone when dollycli stops. It has no history of its own.
- With `--dir DIR` the copy is written to `DIR/graph` and has the history. That folder must have no data: a copy never replaces data. It is a second copy of the source data on disk, so treat it as you treat the source.

One baseline, many copies. Make the baseline once, and give each test run or each agent a copy of its own:

```
dolly --dir ./base graph generate --users 200
dolly serve --clone-from ./base --port 8661
dolly serve --clone-from ./base --port 8662
```

A change in one copy is not in another. To see a problem in someone's data, ask for their data folder (or a copy of it) and serve it with `--clone-from`. [The MCP server](mcp.md) does the same with `start_instance`.

## Where the data of Microsoft comes from

dollycli reads committed copies of Microsoft's own data. It fetches nothing while it runs. Each folder with copies has a `SOURCE.json`
with the commit, the date, and a hash of each file.

| Data | Repository | Update with |
|---|---|---|
| Throttling cost and limits ([graph/cost](../graph/cost/README.md)) | `microsoftgraph/microsoft-graph-docs-contrib` | `make update-cost` |
| Query rules (see [query rules](query-rules.md)) | `microsoftgraph/microsoft-graph-docs-contrib` | `make update-query` |
| The list of permissions ([graph/permissions](../graph/permissions/README.md)) | `microsoftgraph/microsoft-graph-devx-content` | `make update-permissions` |
| The permissions that each call needs ([graph/permissions/operations](../graph/permissions/operations/README.md)) | `microsoftgraph/microsoft-graph-docs-contrib` | `make update-operation-permissions` |
| Routes and properties of Graph (OpenAPI), and the OData description of the types ([graph/spec](../graph/spec/README.md)). The second is served at `/v1.0/$metadata`. | `microsoftgraph/msgraph-metadata` | `make graph-spec` |

The data of `docs-contrib` is CC BY 4.0. The data of the other two repositories is MIT licensed.
