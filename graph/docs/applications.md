---
name: App registrations
resource: applications
order: 4
---
The app registrations of the tenant. An application is the definition of an app, and it lives in its home tenant only. A service principal (`/servicePrincipals`) is the instance of the app in a tenant: it signs in, gets permissions, and has users assigned. The two have the same `appId` (the client ID) and different `id` values. An app of another tenant, such as Microsoft Graph or a third-party app, has a service principal here and no application.

`requiredResourceAccess` is what the app asks for. What it was given is on the service principal (`/oauth2PermissionGrants`, and the app role assignments).

## GET /v1.0/applications

### Filtering
`appId eq '<client ID>'` finds an application by its client ID. `id` is the object ID.

## GET /v1.0/applications/{id}
`{id}` is the object ID, or `applications(appId='<client ID>')`: `GET /v1.0/applications(appId='…')` is the same call. An ID that is not there is 404 `Request_ResourceNotFound`.

## POST /v1.0/applications
Needs `displayName`. dollycli makes `id`, `appId`, and `createdDateTime`. The application has no service principal until a client signs in with its `appId`: then dollycli makes one with the name of the application.

`requiredResourceAccess` is a list of `{ "resourceAppId": "...", "resourceAccess": [ { "id": "...", "type": "Role" | "Scope" } ] }`. For Microsoft Graph (`00000003-0000-0000-c000-000000000000`) each `id` must be the ID of a permission that dollycli knows. If it is not, dollycli answers **404** with what to do: the permissions of Microsoft Graph in dollycli are a list, and some of the IDs in it are made by dollycli, so a script that carries the ID of another tenant's permission can fail here. Use the permission names in the portal, or add the [permissions of your own tenant](../../docs/permissions.md). Real Graph is not checked for this call: it cannot be tried here.

## PATCH /v1.0/applications/{id}
Changes properties. `requiredResourceAccess` is replaced as a whole (the same check as above), and an object property (`web`, `spa`, `publicClient`) changes only the properties that the call names. `id`, `appId`, and `createdDateTime` cannot change. Gives 204.

## DELETE /v1.0/applications/{id}
Deletes the application. Its service principal stays.
