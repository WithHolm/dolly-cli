---
name: Users
order: 1
covers: me=Signed-in user
---
The people of the tenant, with their managers and direct reports. `/me` is the user of a delegated token.

## GET /v1.0/users

### Filtering
What works depends on the property. `eq`, `in`, and `startsWith` work by default on most of them. `ne`, `not`, `endsWith`, `eq null`, `/$count`, and a few properties (for example `officeLocation`) need an advanced query: the header `ConsistencyLevel: eventual` and `$count=true`. The table below shows each property.

`endsWith` works only on `mail`, `otherMails`, `userPrincipalName`, and `proxyAddresses`.

### Search
`"displayName:berg"` finds users that have a word that starts with `berg`. Other properties that `$filter` works on, such as `"mail:berg"`, are a `startsWith`. The call needs the header `ConsistencyLevel: eventual`.

### Sorting
`$orderby=displayName` and `$orderby=userPrincipalName` work by default. `createdDateTime` needs an advanced query. No other property sorts. Using `$orderby` with `$filter` always needs an advanced query.

## GET /v1.0/me
The user of the token. Get a delegated token from the Sign-in tab: the authorization code flow, or the password grant.

An application token (client credentials) gets `400`, as it does in Graph: an application has no user.

## PATCH /v1.0/me
Changes the properties of the user of the token. It follows the same rules as `PATCH /users/{id}`.

## GET /v1.0/users/delta
Delta query: read every user once, and after that only what changed. See [delta query](https://learn.microsoft.com/graph/delta-query-overview). `/users/delta()` is the same call.

A round is a chain of pages. A page with users has an `@odata.nextLink`. The last page is empty and has an `@odata.deltaLink`. Call the `deltaLink` later to get the users that were made, changed, or deleted since. A round with no changes still takes two calls: an empty page with a `nextLink`, then an empty page with the new `deltaLink`. A page is 200 users. `$top`, `$orderby`, `$count`, and `Prefer: odata.maxpagesize` change nothing.

Without `$select`, delta tracks four properties: `displayName`, `mail`, `userPrincipalName`, and `id`. `$select` in the first call chooses the properties that are tracked, and the links carry it. A change to a property that is not tracked does not show the user. A property that a user never had is left out of the answer, and not `null`.

A deleted user is its `id` with `@removed: {"reason": "deleted"}`. `Prefer: return=minimal` on a `deltaLink` gives only the properties that changed. `$filter` takes only `id eq '…'`, and several of them with `or` (up to 50). Any other filter is `400 Request_UnsupportedQuery` (*Unsupported query.*). A token that is not a token is `400 BadRequest` (*Badly formed token.*). `$deltatoken=latest` skips the users, and gives a `deltaLink` for what changes from now.

A token is good until the data goes back or is made again: after a reset or a rollback, and after a restart when there is no data folder, it is `410 Gone` with a `Location` that starts again (the code `syncStateNotFound` and its text are a guess). The tokens do not expire after 7 days, as they do in Graph. `$expand` is accepted and not done: the answer has no `manager`.

Checked against the real Graph: the pages, the links, the default properties, `latest`, `$filter`, the token errors, and the options that change nothing. Not checked, because the proxy reads only: what a changed or deleted user looks like (from the Learn page), `Prefer: return=minimal`, and the 410.
