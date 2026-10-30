---
name: Permission grants
resource: oauth2PermissionGrants
order: 4
---
Consents to delegated permissions: what a client may do for a signed-in user. The property `scope` has the permissions, separated by spaces.

In a grant, `clientId` and `resourceId` are **object IDs** of service principals. They are not client IDs.

## GET /v1.0/oauth2PermissionGrants

### Filtering
Dolly filters on text properties: `consentType eq 'AllPrincipals'`, or `clientId eq '<object ID>'`. Use the object ID of the service principal.
