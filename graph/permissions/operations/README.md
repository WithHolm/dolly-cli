# Permissions of the calls

Which permissions each call of Microsoft Graph needs, from the pages of Microsoft Learn. There are two files, and one command makes both:

```
make update-operation-permissions                 # the latest commit of the repo of Microsoft
make update-operation-permissions COMMIT=<sha>    # an older one
```

| File | What it is |
|---|---|
| `all.json` | **The map**: every call of the description of Graph (5,922 calls), its page of Learn, and for each permission (294) the calls that it allows. For looking things up. About 1.5 MB. It is **not** built into the program. |
| `operations.json` | What the program reads: the routes that dollycli serves, and the permissions of their pages. The portal uses it on the *Permissions* tab of a service principal. |
| `SOURCE.json`, `raw/` | The commit, and the pages of the routes that dollycli serves, as they are, with a hash of each. Do not edit. |

The tool [opfetch](../opfetch/main.go) reads all the pages about calls at one commit (about 3,900), which takes about a minute and needs the network.
GitHub may limit calls with no token: set `GITHUB_TOKEN`. It stops if a page of a route that dollycli serves is missing, or has a table that it cannot
read, and names it. For all the other calls, it writes what it could not read into `all.json` under `problems`, and does not stop. It then runs the
tests. Run it when dollycli serves a new route: a test fails until you do (see [Add a route to Graph](../../../docs/development.md)).

## Find things in `all.json`

With [jq](https://jqlang.github.io/jq/):

```sh
cd graph/permissions/operations

# what does a call need? (the keys are the calls as the description of Graph writes them)
jq '.calls["GET /users/{user-id}/memberOf"]' all.json                       # the page, and how it was matched
jq '.pages["user-list-memberof"] | {summary, calls, delegated, application}' all.json

# which calls does a permission allow? (the pages, and the calls of each page are under .pages)
jq '.permissions["User.Read.All"].application.least[].page' all.json        # as the least privileged way
jq '.permissions["Mail.Read"].delegated | {least, higher} | map(length)' all.json

# every call that a page is about, and every call that has no page
jq '[.calls | to_entries[] | select(.value.page == "")] | length' all.json
jq '.problems' all.json                                                      # what could not be read
```

In Go, `permissions.LoadAllFile("graph/permissions/operations/all.json")` reads it, and two methods look things up: `Lookup(method, path)` finds
a call with any names for the parameters (`GET /users/{id}/memberOf` finds `GET /users/{user-id}/memberOf`), and `Allows(permission, kind)` gives
the calls that a permission allows.

## What the file has

- **`calls`**: every call of the description, `"GET /users/{user-id}"`, to `{page, via, others}`. `page` is the key of the page of Learn: `user-list`
  is `https://learn.microsoft.com/graph/api/user-list`. `others` are other pages that list the same call.
- **`pages`**: each page that a call was matched to: `summary`, `calls` (the calls of the description that are matched to it), `lists` (what the
  page says it is about), and the permissions.
- **`permissions`**: each permission that the pages name, with `inCatalog` (for which kinds the list of the permissions of Graph that dollycli
  has, [graph/permissions](../README.md), has it: a page can name a permission that is newer than that list), and `delegated` and `application`,
  each with `least` and `higher`: the pages (`page`), `with` when the permission alone is not enough (`A and B`), `resource` when it holds only for a type
  of object, and `scenario` when it is from one table of a page that has more than one.
- **`problems`**: `notFound`, `unreadable` (a part of a table that was not read), `noTable`, and `counts` (how many calls were matched in each way).
  There are none at the commit that is in the file.

## How a call is matched to a page

The description of Graph (the spec) has a link to a page for some calls, and **the link is not always right**: it has `GET /me/messages` linked to the
page about open extensions, and `DELETE /me/messages/{id}` linked to the page about event messages. A page says itself which calls it is about, in
its section *HTTP request*. So the tool reads all the pages, and matches each call to the pages that list it. Between several pages, the one with the
name that fits the path best wins (`user-list-memberof` fits `/users/{id}/memberOf`, and `agentuser-list-memberof` has a word that is not in the
path), and then the link of the description, and then the page that lists the fewest calls. `via` says how sure it is:

| `via` | Calls | Meaning |
|---|---|---|
| `verified` | 576 | The link of the description, and the page lists the call, and no page that fits better. |
| `matched` | 531 | A page lists the call. The description has no link, or the link is to a page that does not list it, or another page fits better. |
| `spec-only` | 559 | The link of the description, and **no page lists the call**. It is often right (a page lists the call with another path), but it is not checked. |
| (empty) | 4,256 | No page of Learn is about the call. Most are long paths that Microsoft does not document one by one (`/groups/{id}/sites/{id}/termStore/...`). |

A page can say more than one thing (`variants`):

- **By type of object** (21 pages): one row for a user, a group, a device (`resource`). What the call needs depends on the object.
- **By case** (29 pages): more than one table, each under a heading (`scenario`): the signed-in user and another user, a channel and a chat, tenant-wide
  settings and group settings. When the heading is about the signed-in user or another user, the path of the call decides, and dollycli picks the table
  (`CallPermissions.Resolve`): `GET /me/memberOf` and `GET /users/{id}/memberOf` are on the same page, and they need different permissions. For
  the other cases the call has no single answer, and the portal says that it depends on the case.

Then `delegated` and `application` at the top of the page are empty, and `variants` has the answers.

## What the permissions say

A page has `delegated` and `application`, each with:

- `least`: the alternatives with the least privilege. One of them is enough. **`A and B` needs both permissions** (Microsoft writes it so in some tables, for example for app role assignments).
- `higher`: the alternatives with more privilege, that also allow the call.
- `notSupported`: the page says that this kind of token cannot make the call.
- `conditions`: words that Microsoft puts after a permission, for example `for online meetings`, or `resource-specific consent` (a star in the table).

The tables have three forms: the include files (*Least privileged permissions*, *Higher privileged permissions*), the older table in the page (one
column, *from least to most privileged*, so the first permission is the least), and the table by type of object. The parser is
[table.go](../table.go). The delegated permissions of a personal Microsoft account are not kept: a tenant of dollycli has no such account.

For `operations.json`, `routes` maps a route of dollycli (`"GET /v1.0/groups/{id}"`) to a page. A route with no page of its own (`GET /v1.0/users/$count`,
`.../microsoft.graph.group`, a `/me` route) uses the page of the route that it is a form of, and `inherited` says which. `noPage` has the routes
that have none (`POST /v1.0/$batch`).

## Guesses of our own

[implies.json](../implies.json) is written by hand and is **not** from Microsoft. It says what a permission is assumed to include, so that the
minimum does not name both: `User.ReadUpdate.All` (named only by the page that updates a user) is assumed to include `User.Read.All` and
`User.ReadBasic.All`, so one permission covers a read and an update. Microsoft describes it as "Read and update users". The portal says when the
minimum rests on an assumption.

`User.ReadUpdate.All` is in the permissions reference of Microsoft Learn, and not in the file that dollycli's list of the permissions comes from. So
[supplement.json](../supplement.json) adds it (with its IDs from the reference), and a client can be granted it. A permission that the file gets
later wins over the entry in the supplement.

## What it does not say

- **It is what Microsoft's pages say.** The tables are not complete, and some are odd (the least privileged permission of `GET /groups` is
  `Group-NestingSupport.ReadWrite.All`). It was not checked against the real Graph: the proxy of Graph Explorer has its own consents.
- **The match of a call to a page is a rule, not a fact.** 531 calls were matched by their path, and 559 rest on the link of the description alone.
- A call can need more than a permission: a role in Entra for the signed-in user. The pages say it in text, and it is not kept here. Some pages
  (for example `approleassignment-get`) also name higher privileged permissions in a list outside the table: these are not kept.
- 2 of the 294 permissions are in none of dollycli's lists of the permissions of Graph (`CallRecording.Read.All` and `User.Create`): dollycli cannot grant
  them. Some more are in only one kind (a page names a delegated permission that exists as an application one only), and `inCatalog` says which.
  Many permissions of the list have an ID that dollycli made (see [permissions](../README.md)).

## Credit

The pages are from <https://github.com/microsoftgraph/microsoft-graph-docs-contrib>, documentation under CC BY 4.0 (the file `LICENSE` in that
repo). Only the names of the permissions, and the short words that Microsoft puts after them, are kept in the JSON. Give credit to Microsoft when you
use the data.
