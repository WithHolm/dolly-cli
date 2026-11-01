# Graph permissions

The permissions of Microsoft Graph, as they are in a real tenant: the delegated permissions (scopes)
and the application permissions (app roles), each with its ID, its value (`User.Read`), and its
descriptions. Dolly shows them as the service principal Microsoft Graph
(application ID `00000003-0000-0000-c000-000000000000`, namespace `https://graph.microsoft.com`).

```
raw/              Microsoft's file, as it is. Made by 'make update-permissions'. Do not edit.
raw/SOURCE.json   the commit, the date, the counts, and the hash of the file
permissions.go    the types, the loader (it refuses a field it does not know), and the lookups
raw/names.json    the newer list of Microsoft (permissions/new/permissions.json) with only the texts of each permission: more names, and no IDs. Made by 'make update-permissions'.
                  A permission that only this list has gets an ID that dollycli makes (permissions.MadeID): not Microsoft's ID.
supplement.json   permissions that the permissions reference of Microsoft Learn has, and the file of Microsoft does not have yet (written by hand)
implies.json      what a permission is assumed to include (written by hand; see operations/README.md)
```

## Update the data

```
make update-permissions
```

This downloads `permissions/permissions-descriptions.json` of the repo
`microsoftgraph/microsoft-graph-devx-content`, from one commit, and runs the tests. It needs the network.
GitHub may limit calls with no token: set `GITHUB_TOKEN` if it says so.

There is nothing to make by hand or by Claude: Dolly reads the file as Microsoft has it. The one exception is [supplement.json](supplement.json):
`User.ReadUpdate.All` is in the permissions reference of Microsoft Learn but not in the file, so it is added from the reference. When the file has
a permission of the supplement, the file wins.
If the test says that the file has a field the types do not have, change `permissions.go` first.

## What it does not have

- The permissions of other resources (Exchange, SharePoint, and so on). Only Microsoft Graph is here.
- Which Graph call needs which permission. That is [operations](operations/README.md), made from the pages of Microsoft Learn.
- Grants. A grant is data of a tenant, and it is in the data of Dolly (the change log), not here.

## Credit

The data is from <https://github.com/microsoftgraph/microsoft-graph-devx-content>, MIT license,
Copyright (c) 2020 Microsoft Graph.
