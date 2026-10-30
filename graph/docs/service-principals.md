---
name: Service principals
resource: servicePrincipals
order: 3
---
The clients and the resources of the tenant: their roles, and who has them. A permission is an app role assignment.

## GET /v1.0/servicePrincipals

### Filtering
`appId eq '<client ID>'` finds a service principal by its client ID. `id` is the object ID, and `appId` is the client ID.

`tags/any(t:t eq 'WindowsAzureActiveDirectoryIntegratedApp')` finds the enterprise applications. Capitals matter in a tag.

`ne`, `not`, `eq null`, and some properties need an advanced query: the header `ConsistencyLevel: eventual` and `$count=true`.

### Sorting
`$orderby=displayName` needs an advanced query for a service principal.
