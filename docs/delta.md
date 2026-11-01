# Delta query

A tool can read a collection once, and after that read only what changed. This is *delta query*, or change tracking. See [Use delta query to track changes](https://learn.microsoft.com/graph/delta-query-overview) in Microsoft's documentation.

dollycli has it for three collections:

| Call | What it tracks |
|---|---|
| `GET /v1.0/users/delta` | users: `displayName`, `mail`, `userPrincipalName`, and `id` by default |
| `GET /v1.0/groups/delta` | groups, and `members@delta` |
| `GET /v1.0/servicePrincipals/delta` | service principals, with their roles and scopes |

`/users/delta()` (with the brackets) is the same call. Delta for other collections (for example `oauth2PermissionGrants` and `organization`) and for the services that dollycli does not have (calendar, mail, Teams) is not built.

## How a client uses it

1. Call `GET /users/delta`. Follow `@odata.nextLink` until a page has `@odata.deltaLink`. That page is empty. The pages before it have every user as it is now.
2. Keep the `deltaLink`.
3. Later, call the `deltaLink`. It gives the users that were made, changed, or deleted since. Follow its `nextLink` until a page has a new `deltaLink`. Keep that one.

    curl -s 'http://127.0.0.1:8650/v1.0/users/delta?$select=displayName,department' -H 'Authorization: Bearer any'
    # ... follow @odata.nextLink, then keep the @odata.deltaLink of the last page ...
    curl -s '<the deltaLink>' -H 'Authorization: Bearer any'

- A deleted object is its `id` and `@removed`: `{"id": "...", "@removed": {"reason": "deleted"}}`.
- `$select` in the first call chooses the properties that are tracked. The links carry it, so later calls repeat nothing. A change to a property that is not tracked does not show the object.
- `Prefer: return=minimal` on a `deltaLink` gives only the properties that changed.
- `$filter` takes only `id eq '…'`, and several of them with `or` (up to 50).
- `$deltatoken=latest` skips the objects, and gives a `deltaLink` for what changes from now.
- A group has `members@delta`: its members in the first round, and later the members that went in, and the ones that went out with `@removed`.

## What a token is

A token holds the position in the log of changes, the options of the first call, and the *epoch* of the data. dollycli keeps the log of changes in memory, also when there is no data folder. When the log is made again, the epoch changes, and an old token is `410 Gone` with a `Location` that starts again. That happens:

- after a reset (`dolly graph reset`) or a rollback,
- after a restart when there is no data folder (the data is new),
- after a new seed in the folder.

With a data folder, a restart keeps the epoch, and a `deltaLink` from before the restart is still good. The log is made again from the history. Tokens do not expire after 7 days, as they do in Graph.

## What was checked

Checked against the real Graph (the sample tenant of Graph Explorer): the pages and links, the empty last page, a round with no changes (two calls), the default properties of the three collections, `members@delta`, `$select` with and without `members`, `$deltatoken=latest`, `$filter`, the errors for a token that is not a token, and the options that change nothing (`$top`, `$orderby`, `$count`).

Not checked, because the proxy of Graph Explorer reads only and the sample tenant cannot be changed: what a changed, deleted, or moved object looks like (this comes from Microsoft's pages), `Prefer: return=minimal`, a member that went out of a group, and the `410 Gone` (the code `syncStateNotFound` and its text are a guess). The page size of 200 is checked for service principals only. `$expand` is accepted and not done.

The API page of the portal has a section for each call: [graph/docs/users.md](../graph/docs/users.md), [groups.md](../graph/docs/groups.md), and [service-principals.md](../graph/docs/service-principals.md).
