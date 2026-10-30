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
