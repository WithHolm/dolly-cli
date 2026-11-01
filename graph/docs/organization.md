---
name: Organization
order: 6
---
The tenant: its name, its ID, its address, its service plans, its notification lists, and its verified domains. The ID is the tenant ID that the sign-in API puts in its tokens. Change it on the Organization page of the portal.

There is one organization. It has every property of the [organization resource](https://learn.microsoft.com/graph/api/resources/organization): the same names that the real Graph gives for a tenant, in the same kinds. A property with no value is `null`, and a list with no items is `[]`. The service plans are real plans of Microsoft 365, with their real IDs.

## GET /v1.0/organization
Without `$select` the call gives the 28 properties that Graph gives. `$select` can also name `mobileDeviceManagementAuthority` and the relations, which are not in the answer by default.

`directorySizeQuota` is worked out when you ask. `used` is the number of users, groups, and service principals. `total` is 50000. Whether the real Graph counts the same objects was not checked.

Graph refuses some options on this call, and so does dollycli. `$filter` is `Request_UnsupportedQuery` (*Filtered searches against this resource are not supported.*). `$orderby` is `Request_UnsupportedQuery` (*Sorting is not supported for tenant queries.*). `$skip` is `Request_BadRequest`. `$count=true` is left out of the answer, with the header `ConsistencyLevel` or without it. `$top` must be from 1 to 999.

## GET /v1.0/organization/$count
Graph does not count the organization. The answer is `Request_UnsupportedQuery`, with the header `ConsistencyLevel` or without it.

## GET /v1.0/organization/{id}
Another tenant ID is `400` with `Request_BadRequest` (*Invalid tenant identifier; it must match that of the requested tenant.*), and not `404`.

## PATCH /v1.0/organization/{id}
Only five properties can change, as in the page of Microsoft Learn: `marketingNotificationEmails`, `privacyProfile`, `securityComplianceNotificationMails`, `securityComplianceNotificationPhones`, and `technicalNotificationMails`. `privacyProfile` is merged, so you can set `contactEmail` and keep `statementUrl`. A list cannot be `null`.

The answer is `204`. The change is in the history of the portal, and it is still there after a restart.

Another property is `400`. The text of that error is a guess: the proxy of Graph Explorer reads only, so it was not checked against the real Graph.
