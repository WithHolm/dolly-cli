---
name: Places
order: 7
---
Buildings, floors, rooms, desks, and the other places of the tenant.

Calls on places take `$skip` and `$top`. Calls on users and groups page with a `$skiptoken` in `@odata.nextLink` instead.

A place is not a directory object, so `$count=true` and `$orderby` need no `ConsistencyLevel` header.

## GET /v1.0/places/microsoft.graph.room

### Filtering
`startsWith(displayName,'A')` and `displayName eq 'text'` work. Only rooms, workspaces, and room lists take `$filter`. Buildings, floors, sections, and desks do not.
