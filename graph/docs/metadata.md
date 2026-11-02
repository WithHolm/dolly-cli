---
name: Metadata
resource: $metadata
order: 9
---
The OData description of the API: the types, their properties and navigation properties, and the entity sets. OData clients and some code generators read it first.

## GET /v1.0/$metadata
Answers `200` with `Content-Type: application/xml; charset=utf-8` and `OData-Version: 4.0`, as the real Graph does (checked with the Graph Explorer proxy). It needs no token: the description of an API is public, and a client reads it before it signs in. That is a guess, from what Graph is known to do, and it is not checked.

The real Graph answers with all of Graph (about 1.75 MB, 1,241 entity types). dollycli answers with the part that it serves: the types that its resources need (`user`, `group`, `servicePrincipal`, `application`, `organization`, `oAuth2PermissionGrant`, `appRoleAssignment`, the places, and the types that these need, as far as they go), and the entity sets `users`, `groups`, `servicePrincipals`, `applications`, `organization`, `oauth2PermissionGrants`, `directoryObjects`, `places`, and the singleton `me`. A navigation property that points at a type that dollycli does not have is left out, and so are the actions, the functions, and the annotations (the descriptions of the properties). The types are written in full (`microsoft.graph.user`), as in the real Graph.

The description is made from Microsoft's [msgraph-metadata](https://github.com/microsoftgraph/msgraph-metadata) (MIT) by `make graph-spec`, see [graph/spec](../../graph/spec/README.md).
