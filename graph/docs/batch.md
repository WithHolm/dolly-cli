---
name: Batch
resource: $batch
order: 7
---
Run up to 20 calls in one. Microsoft describes it in [JSON batching](https://learn.microsoft.com/graph/json-batching).

## POST /v1.0/$batch
Send one `POST` with a list of `requests`. Each request has an `id` that you choose, a `method`, and a `url` that starts with `/` and is relative to `/v1.0`. It can also have `headers` and a `body`.

```json
POST /v1.0/$batch
{
  "requests": [
    { "id": "1", "method": "GET", "url": "/users?$top=1&$select=displayName" },
    { "id": "2", "method": "GET", "url": "/groups/$count", "headers": { "ConsistencyLevel": "eventual" } },
    {
      "id": "3", "method": "POST", "url": "/groups",
      "headers": { "Content-Type": "application/json" },
      "body": { "displayName": "grp.batch.example", "mailEnabled": false, "mailNickname": "grpbatchexample", "securityEnabled": true }
    }
  ]
}
```

The answer has a `responses` list, in the order of the requests. Each one has the `id` of its request, a `status`, `headers`, and a `body`. The answer to the batch itself is `200`, even when a request in it fails.

```json
{
  "responses": [
    { "id": "1", "status": 200, "headers": { "Content-Type": "application/json" }, "body": { "value": [ { "id": "…", "displayName": "Ada Lovelace" } ] } },
    { "id": "2", "status": 200, "headers": { "Content-Type": "text/plain" }, "body": 38 },
    { "id": "3", "status": 201, "headers": { "Content-Type": "application/json" }, "body": { "id": "…", "displayName": "grp.batch.example" } }
  ]
}
```

Each request is answered as a call of its own, and it can fail on its own: the others still run. The token is passed on to each one, so `/me` works when the token is delegated. A header that a request needs, such as `ConsistencyLevel`, goes in the `headers` of that request. A header on the batch call is not passed on.

Requests run one after the other. `dependsOn` is not supported yet.
