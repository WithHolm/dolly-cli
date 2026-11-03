---
name: Sites
resource: sites
order: 11
---
The SharePoint sites of the tenant, and the drives (document libraries) on them. In dollycli a call can only **read** a site: Graph has no call to make one. Make and change sites and their drives on the Sites page of the portal.

A site has one of two kinds. The portal shows the kind. Graph does not, and neither does dollycli in an answer.

- **A Microsoft 365 group site** is the team site of a Microsoft 365 group. A call reaches it through the group: `GET /v1.0/groups/{id}/sites/root`. It is also found by its link and by search.
- **A SharePoint site** has no group. A call finds it only by its link (`/sites/{hostname}:/{path}`) or by search.

A **subsite** is a site below another site. `GET /v1.0/sites/{id}/sites` lists them. Only the root site of a site collection has `siteCollection` and `root` in its answer.

The ID of a site is the host name and two GUIDs, with commas between: `contoso.sharepoint.com,2c712604-…,2d2244c3-…`. `root` is the root site of the tenant. The host name is the first part of the initial domain of the tenant, and `.sharepoint.com`.

A site, a drive, or a group site that is not there gives `404` with the code `itemNotFound` and the text `Item not found`. The real Graph answers so for a drive that it cannot find (checked with the proxy of Graph Explorer). A group that is not there gives the `Request_ResourceNotFound` of the directory.

## GET /v1.0/sites/{host}/{path...}
A site by its link, as `contoso.sharepoint.com:/sites/intranet` (Learn page "Get a site"). The host ends with a colon. After the path, `:/drive`, `:/drives`, and `:/sites` give the drive, the drives, and the subsites of that site. `/sites/{hostname}:` alone is the root site of the host.

## GET /v1.0/sites

### Search
`?search=text` finds a site when the text is in its name, its title, its description, or its address. The case does not matter. `search=*` and no search list every site.

The Learn page says that only `$orderby=createdDateTime` works with a search. dollycli sorts by any property.

### What was checked against the real Graph, and what was not
**Checked** (the proxy of Graph Explorer, 2026-11-03): `GET /me/drive` and `GET /me/drive/root/children` of the sample user give `404 itemNotFound`, `Item not found`. `GET /sites/root` gives `403 accessDenied`, `Request Doesn't have the required Permission scopes to access a site.` and `GET /sites?search=*` gives `403 accessDenied`, `Access denied.` The sample token cannot read a site, so no answer of a real site was seen.

**From the Learn pages** (not tried): the properties of a site, `/sites/{host}:/{path}`, `/groups/{id}/sites/root`, `GET /sites?search=`, the subsites, and the list of drives.

**A guess:** the properties `root` and `siteCollection` on the root site of a collection, the quota and the owner of a drive, the error of a group with no site, `$orderby` on other properties than the date of creation, and `GET /sites` with no search (the Learn page says that a delegated token cannot list every site: dollycli does not check the kind of the token, and lists them).
