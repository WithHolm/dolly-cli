# Plan: synchronization jobs

Status: plan only. No code is written. Nothing changes until the open question at the end is answered.

## Goal

Serve the Graph synchronization jobs of a service principal, so a tool that sets up provisioning (for example a SCIM job) can create and run a job against dollycli.

## What was checked

| Source | What it said |
| --- | --- |
| Spec ([graph-v1.0.json](graph/spec/graph-v1.0.json)) | About 56 routes under `/servicePrincipals/{id}/synchronization`. `/applications/{id}/synchronization` mirrors them. The spec has **no schemas** for these types, so the property lists come from Learn. |
| Learn (commit pinned in [SOURCE.json](graph/cost/raw/SOURCE.json)) | `POST jobs` needs only `templateId`. It must match a template of the service principal. The text says `201 Created`, the example says `200 OK`. The job starts disabled. `start`, `pause` and `restart` answer `204`. A job has `id`, `templateId`, `schedule {expiration, interval, state}` and `status {code, ...}`. A new job has status code `NotConfigured`, a paused job `Paused`. |
| Real Graph (Graph Explorer proxy) | **Blocked.** The sample tenant has no sync permission. `GET /servicePrincipals/{id}/synchronization/jobs`, `templates`, `secrets` and `jobs/{id}` all answer `401` with an empty message. `GET .../synchronization` answers `404` from `syncfabric`. No response shape and no error text can be checked. |

## Behaviors

All of these are *guess* (from Learn only), except where noted.

| Behavior | Mark |
| --- | --- |
| `GET /servicePrincipals/{id}/synchronization/jobs` lists the jobs of the service principal | guess |
| `POST .../jobs` with `{"templateId": "..."}` makes a job, status `NotConfigured`, schedule state `Disabled`, answers `201` | guess (Learn text and example disagree on 201 or 200) |
| `POST .../jobs` with an unknown `templateId` is refused | guess (the error text is unknown) |
| `GET .../jobs/{jobId}` and `DELETE .../jobs/{jobId}` | guess |
| `POST .../jobs/{jobId}/start`, `pause`, `restart` answer `204` and change `status.code` and `schedule.state` | checked in Learn (the codes); the state changes are a guess |
| `GET .../synchronization/templates` lists a few fixed templates, one of them `scim` | guess. The id `scim` for the custom app is from memory and is not checked. |
| An id of a service principal that does not exist gives the same `404` as the other service principal routes | guess |
| Nothing really synchronizes. A started job never calls an outside endpoint. | decision |

## Scope

In:

- `GET`, `POST` on `/servicePrincipals/{id}/synchronization/jobs`
- `GET`, `DELETE` on `.../jobs/{jobId}`
- `POST .../jobs/{jobId}/start`, `pause`, `restart`
- `GET /servicePrincipals/{id}/synchronization/templates`
- Jobs live in the world, so they survive a restart and show in the journal like other objects.

Out:

- The job **schema**: `jobs/{id}/schema`, `schema/directories`, `discover`, `filterOperators()`, `functions()`, `parseExpression`. These hold the attribute mappings and the rules of a job. Without them a `scim` job is created but has no mappings, so a tool that does `GET schema`, edits the mappings and does `PATCH schema` fails with `404`. See the open question.
- `provisionOnDemand`, `validateCredentials`, `secrets`, `bulkUpload`, `acquireAccessToken`
- `GET`, `PUT`, `DELETE` on `.../synchronization` itself
- The whole `/applications/{id}/synchronization` mirror
- A portal page
- The SCIM protocol itself (`/scim/v2`). It is not a Graph resource.

## Files that change

Follows the order of [Add a route to Graph](docs/development.md#add-a-route-to-graph).

1. Spec: add the synchronization types to [graph-v1.0.json](graph/spec/graph-v1.0.json) (by hand, because `make graph-spec` has no schemas for them).
2. Docs: a page in [docs/](docs/README.md), linked from [docs/README.md](docs/README.md). It says which parts are guesses, and why (the real Graph cannot be asked).
3. Relations: a new job store in `graph/repo/`, next to the service principals.
4. Tests that fail first, in `graph/` (the routes) and `graph/repo/` (the store).
5. The routes, in a new file in `graph/`.
6. OData tests and details (`$select`, `$count`, paging of the list).
7. Cost and permissions: run `make update-operation-permissions` and commit [graph/permissions/operations](graph/permissions/operations/README.md). The test `TestOperationPermissions_EveryServedRouteHasItsData` fails until then, as expected.
8. Check: `go test ./...`. No change in `ui/`, so no `npm` run.

## Risks

- **Wrong shape.** All shapes are guesses from Learn. A client that reads a field that Learn does not show will get nothing. The docs page must say so.
- **No secrets.** `secrets` is out of scope, so a job holds no credential.
- **Portal.** No value of a job is shown in the portal, so no link or HTML risk.
- **Looks real, does nothing.** A job with status `Running` pushes nothing. The docs page must say so, so nobody trusts it as a test of a real SCIM endpoint.

## Open question

Do the schema routes go in the first step?

- **Recommended: no.** Build the job store, the routes above and the `scim` template first. Add the schema as a second step once the store exists. The schema shape is a pure guess without a real answer.
- **Yes:** also `GET` and `PATCH` of `jobs/{id}/schema`, with a default `scim` schema (users and groups mappings). Then a SCIM provisioning setup works end to end. It is a bigger step.

## Report at the end

Will say what was checked against the real Graph (nothing, because of the 401), what is a guess, and what was left out.
