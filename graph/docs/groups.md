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
