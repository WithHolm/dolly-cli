# Permissions of your own tenant

dollycli has a built-in list of the permissions of Microsoft Graph: 809 delegated and 763 application ones. A real tenant has a few more (about 870 and 780 at the time of writing), and new ones come often. If a permission that you need is not in the picker, you can add the list of your own tenant.

The built-in list has two sources. Microsoft's own list has the real IDs (598 and 534). A newer list of Microsoft's has more names and texts, and no IDs: for these permissions dollycli makes an ID, the same one every time. It is not Microsoft's ID. The portal never shows IDs, you pick a permission by its name, and a script that assigns a permission by its real ID gets `404 dollycli does not know a permission of Microsoft Graph with the ID …` when that ID is not one that dollycli has. Use the name, add the permission in the portal, or add your tenant's list: it has the real IDs, and replaces the made ones.

dollycli does not ship that list. You download it from your tenant with a read-only call, and you keep the file on your machine. dollycli reads the file when it starts, adds its permissions to the built-in list, and a permission of the file wins over the built-in one with the same name.

## 1. Download

The service principal *Microsoft Graph* of a tenant has the list: `appRoles` (application permissions) and `oauth2PermissionScopes` (delegated permissions). Save the answer of this call to `dolly-data/permissions/graph.json`:

    GET https://graph.microsoft.com/v1.0/servicePrincipals(appId='00000003-0000-0000-c000-000000000000')?$select=appRoles,oauth2PermissionScopes

With the Azure CLI (bash):

    az login
    mkdir -p dolly-data/permissions
    az rest --method get \
      --url "https://graph.microsoft.com/v1.0/servicePrincipals(appId='00000003-0000-0000-c000-000000000000')?\$select=appRoles,oauth2PermissionScopes" \
      > dolly-data/permissions/graph.json

With Microsoft Graph PowerShell:

    Connect-MgGraph -Scopes Application.Read.All
    New-Item -ItemType Directory -Force dolly-data/permissions | Out-Null
    Invoke-MgGraphRequest -Method GET `
      -Uri "https://graph.microsoft.com/v1.0/servicePrincipals(appId='00000003-0000-0000-c000-000000000000')?`$select=appRoles,oauth2PermissionScopes" `
      -OutputFilePath dolly-data/permissions/graph.json

If you used `iwr 'http://127.0.0.1:8650/pwsh' | iex` in this PowerShell session, it points the module at dollycli. Run `Disconnect-MgGraph` first, and start a new session if `Connect-MgGraph` still goes to dollycli.

The call only reads. It answers about 800 KB.

## 2. Start dollycli

    dolly serve

If the file is at `dolly-data/permissions/graph.json`, dollycli finds it and logs `Dolly adds the permissions of your tenant`. For another place, give the file:

    dolly serve --permissions ~/graph-permissions.json

(`DOLLY_PERMISSIONS` does the same.) The picker for a service principal then says where the list is from, and has the new permissions.

dollycli stops with a message that names the file when it cannot read it, or when it is not the answer of the call above. It does not start with a part of the list missing.

## Keep in mind

- The file is data of Microsoft's service. It is yours to keep on your machine. Do not commit it to a repository or publish it: the folder `dolly-data/` is in the `.gitignore` of this repository for that reason.
- Download again when you want the newer permissions. dollycli reads the file at start, so restart it.
- This adds permissions to *Microsoft Graph*. It does not change which permissions a call needs: that is [the map of calls](../graph/permissions/operations/README.md), and it names a permission that is not in your list as one that dollycli cannot grant.
