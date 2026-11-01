---
name: Groups
order: 2
---
Security and Microsoft 365 groups, with their members, owners, and app role assignments. A member can be a user, a group, or a service principal.

## GET /v1.0/groups

### Filtering
`displayName eq 'text'`, `startsWith(displayName,'grp.app')`, and `securityEnabled eq true` work by default. Filter on a list of values with `any`: `groupTypes/any(c:c eq 'Unified')` finds Microsoft 365 groups, and `groupTypes/any(c:c eq 'DynamicMembership')` finds dynamic groups.

`ne`, `not`, `eq null`, and a few properties (for example `description`) need an advanced query: the header `ConsistencyLevel: eventual` and `$count=true`. `createdDateTime` does not support `eq`.

### Search
`"displayName:acme"` finds groups with a word that starts with `acme`, so it finds `grp.app.acme.main`. `"description:text"` works the same way. The call needs the header `ConsistencyLevel: eventual`.

### Sorting
`$orderby=displayName` works by default. `createdDateTime` needs an advanced query. No other property sorts.

## GET /v1.0/groups/delta
Delta query for groups. It works as users delta: pages with a `nextLink`, an empty last page with a `deltaLink`, `@removed` for a deleted group, `$select` to choose what is tracked, `$filter` with `id eq '…'` only, and `410 Gone` for a token from before a reset or a rollback. `/groups/delta()` is the same call.

Without `$select`, delta tracks `createdDateTime`, `description`, `displayName`, `mailEnabled`, `mailNickname`, `renewedDateTime`, `securityEnabled`, `securityIdentifier`, `visibility`, and `id`. A property that dollycli does not keep for a group (for example `visibility`) is left out.

Each group has `members@delta`. In the first round it lists the members of the group, each with its `@odata.type` and `id`. In a later round it lists the members that went in, and the ones that went out with `@removed`. A group shows when a member changes, even when no property changed. A dynamic group takes and loses members when users change, and those show too. A user that is deleted goes out of its groups. The member that went out of a group because it was deleted has no `@odata.type`, because the object is not there to ask (a guess: the real Graph may give one). `$select` without `members` leaves `members@delta` out, and `$select=displayName,members` brings it back (checked).

The owners of a group are not tracked. Checked against the real Graph: the default properties, `members@delta` in the first round, and `$select` with and without `members`. Not checked, because the proxy reads only: the shape of a member that went out.
