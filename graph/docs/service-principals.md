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

## GET /v1.0/servicePrincipals/{id}
`{id}` is the object ID, or `servicePrincipals(appId='<client ID>')`. The object has the properties that the real Graph gives (checked with the sample tenant): `accountEnabled`, `appOwnerOrganizationId`, `appRoleAssignmentRequired`, `loginUrl`, `notes`, `replyUrls`, `servicePrincipalType`, and the rest. `appOwnerOrganizationId` is this tenant for an app that is registered here (`/applications`), and another tenant for the other apps.

## POST /v1.0/servicePrincipals
Makes the instance of an app. Needs `appId`, and the app must be an app registration of this tenant (`/applications`). The service principal takes its name, `appRoles`, scopes, identifier URIs (as `servicePrincipalNames`), and redirect URIs (as `replyUrls`) from the application. Gives 201. An `appId` that is not a registered app is 400 with the text of the real Graph: "The appId '…' of the service principal does not reference a valid application object." One that has a service principal already is 400.

## PATCH /v1.0/servicePrincipals/{id}
Changes the properties that Learn lists: `accountEnabled`, `appRoleAssignmentRequired`, `displayName`, `homepage`, `loginUrl`, `logoutUrl`, `notes`, `description`, `preferredSingleSignOnMode` (`password`, `saml`, `notSupported`, or `oidc`), `replyUrls`, `servicePrincipalNames`, `tags`, `notificationEmailAddresses`, `appRoles`, `oauth2PermissionScopes`, and more. The tag `WindowsAzureActiveDirectoryIntegratedApp` in `tags` makes it an enterprise application. A property that is read-only (`id`, `appId`, `servicePrincipalType`, …) is 400 "cannot be changed". Gives 204. Microsoft Graph and the service principals that dollycli needs cannot be changed.

## DELETE /v1.0/servicePrincipals/{id}
Deletes it, with its role assignments and its consents. Its app registration stays. Gives 204. Real Graph is not checked for these writes: they cannot be tried there. The status codes follow the Learn pages, and the texts of the errors are guesses, except the one above.

## GET /v1.0/servicePrincipals/delta
Delta query for service principals. It works as users delta: pages with a `nextLink`, an empty last page with a `deltaLink`, `$select` to choose what is tracked, `$filter` with `id eq '…'` only, and `410 Gone` for a token from before a reset or a rollback. `/servicePrincipals/delta()` is the same call.

Without `$select`, delta tracks 16 properties (checked with the real Graph), and the objects have the roles and the scopes that the list does not have: `accountEnabled`, `appDisplayName`, `appId`, `appOwnerOrganizationId`, `createdDateTime`, `displayName`, `logoutUrl`, `replyUrls`, `servicePrincipalNames`, `servicePrincipalType`, `id`, `appRoles`, `keyCredentials`, `oauth2PermissionScopes`, `passwordCredentials`, and `resourceSpecificApplicationPermissions`. A property that a service principal does not have is left out.

A service principal is made by the first sign-in of a client, or by `POST /servicePrincipals`. A change of a property, a delete, and a new one are all in the delta. The permission mode, the flags, and the credential state are settings of dollycli and not properties of Graph, so they are not changes.
