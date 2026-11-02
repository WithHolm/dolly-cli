---
name: Subscriptions
resource: subscriptions
order: 10
---
Change notifications: a client subscribes to a resource, and dollycli sends a `POST` to its URL when an object changes. The notification only says *what* changed. Read the object, or use delta query, for the rest. Microsoft describes the whole feature in [Change notifications](https://learn.microsoft.com/graph/change-notifications-overview). This page is only what dollycli does.

In the portal, **Graph > Subscriptions** makes a subscription, and sends a test notification to it without changing any data.

## GET /v1.0/subscriptions
The subscriptions. In the sample tenant of the real Graph this is an empty list (checked).

## GET /v1.0/subscriptions/{id}
One subscription. An ID that is not there is `404`.

## POST /v1.0/subscriptions
Gives `201` with the subscription.

### Body
```json
{
  "changeType": "created,updated,deleted",
  "notificationUrl": "https://localhost:5000/notify",
  "resource": "users",
  "expirationDateTime": "2026-11-05T12:00:00Z",
  "clientState": "any secret, sent back in each notification"
}
```

All but `clientState` are required.

### What you can subscribe to
| `resource` | Notifies when |
|---|---|
| `users` | any user is created, changed, or deleted |
| `users/{id}` | that user is |
| `groups` | any group is |
| `groups/{id}` | that group is |

Nothing else yet: the members of a group, service principals, applications, and the resources of Outlook or Teams are `400`. A notification has no resource data (`includeResourceData` is `false`), as Learn says for users and groups.

### Before it is made
dollycli sends a `POST` to your `notificationUrl`, with `?validationToken=<token>` in the query. Answer **200**, `Content-Type: text/plain`, with the token as the body, in 10 seconds. If not, the subscription is not made (`400`).

### Rules
- `expirationDateTime` is at most 41,760 minutes (under 29 days) from now. Renew with `PATCH`.
- The same `changeType` and `resource` as a subscription of the same client is `409`.
- Any `http` or `https` URL works, also `localhost`. dollycli does not call a link-local address, follow a redirect, or use a proxy. A certificate that the system does not trust is accepted for an address on this machine (a dev certificate), and checked for any other.
- With no receiver of your own, use dollycli: `POST /dolly-receiver/notifications` on this server answers the validation request and takes each notification with `202`. It needs no token. The portal offers it as *dollycli (this server)*.

### The notification
A change is a `POST` to your URL with `Content-Type: application/json`:

```json
{
  "value": [{
    "id": "7f1c…",
    "subscriptionId": "3864bd68-5095-44b8-b423-58012f62d508",
    "subscriptionExpirationDateTime": "2026-11-05T12:00:00.0000000Z",
    "clientState": "any secret, sent back in each notification",
    "changeType": "updated",
    "resource": "Users/554a3377-3c03-44dd-936a-40b5b0c21f30",
    "resourceData": {
      "@odata.type": "#Microsoft.Graph.User",
      "@odata.id": "Users/554a3377-3c03-44dd-936a-40b5b0c21f30",
      "id": "554a3377-3c03-44dd-936a-40b5b0c21f30"
    },
    "tenantId": "…"
  }]
}
```

Answer with any `2xx`. dollycli tries **once**: a failure is in the *Sent* log of the portal, and is not repeated. A new or deleted user or group also counts as `updated`, so a subscription to `updated` gets those too.

To test the *duplicates* that Graph sometimes sends, set `duplicatePercent` in the behavior (`PUT /_dolly/graph/behavior`, 0 to 100). That share is sent 2 to 4 times with the same `id`.

### Kept and dropped
Subscriptions are in `subscriptions.json` next to the data, so a restart keeps them. A new seed (a new generation) drops them, as another tenant. A reset or rollback keeps them and sends nothing.

### Checked and not checked
Checked against the real Graph: only `GET /subscriptions`. The rest follows Learn. The `code` and text of the errors, `Users/{id}` in the notification, and the one-client rule of the `409` are guesses.

## PATCH /v1.0/subscriptions/{id}
Renews it (`expirationDateTime`), or gives it another `notificationUrl`, which is validated. Nothing else can change. Gives `200` with the subscription.

## DELETE /v1.0/subscriptions/{id}
Deletes it. Gives `204`.
