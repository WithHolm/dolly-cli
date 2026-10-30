# Graph cost data

Microsoft publishes how Graph throttles calls. Dolly uses it to give each call a cost
(`x-ms-resource-unit`) and to throttle like the real Graph. This folder has the data.

```
raw/          Microsoft's documents, as they are. Made by 'make update-cost'. Do not edit.
raw/SOURCE.json   the commit they come from, the date, and a hash of each file
cost.json     the data that Dolly reads, made from raw/ (see "Make cost.json")
cost_test.go  checks cost.json against raw/
fetch/        the tool that makes raw/
```

## Update the data

```
make update-cost
```

This downloads the article `concepts/throttling-limits.md` and the 60 or so include files
that hold its tables, all from one commit. It changes nothing but `raw/`.
It needs the network. GitHub may limit calls with no token: set `GITHUB_TOKEN` if it says so.

After that, `go test ./graph/cost` fails until `cost.json` is made again from the new `raw/`.
That is on purpose: it tells you that the data is old.

Then make `cost.json`. In Claude Code, run **`/update-cost`**: the skill in
`.claude/skills/update-cost` runs `make update-cost`, makes `cost.json` from this file, and runs
the test. Or ask Claude: "Make graph/cost/cost.json from graph/cost/raw. Follow graph/cost/README.md."

## Make cost.json

Read `raw/concepts/throttling-limits.md` and the files in `raw/includes/`. Write `cost.json`.
Rules:

1. Copy numbers as they are. Never guess a number. If a table is not clear, put the text in
   `notes` with the file name, and leave the number out.
2. Every file in `raw/includes/` must be in `services` once (field `include`). A service that has
   no usable limit (for example, only text) is in `services` with an empty `limits` and a `notes`.
3. `source.commit` and `source.commitDate` are copied from `raw/SOURCE.json`.
4. Keep the order of the source. Use the units of the source: `10s`, `20s`, `2m30s`, `5m`, `1m`.
5. Do not change the cost table of the identity service. `cost_test.go` compares it row by row
   with `raw/includes/throttling-identity-and-access.md`.

The form of `cost.json` is defined by the Go types in `cost.go`. `cost.Load` refuses a field that
the types do not have, and `go test ./graph/cost` runs it. If the source has something that the types
cannot hold, change the types first (and the price or limiter code if Dolly should use it), then
the file. The example below is only a picture; the types are the authority.

Dolly reads the global limit and the `identity` part (see `price.go` and `limiter.go`). The other
services are kept as the source says them, so that the next API can use them: they are not an
omission to clean up.

Form of `cost.json`:

```json
{
  "source": { "commit": "…", "commitDate": "…", "license": "CC BY 4.0, Microsoft" },
  "global": { "requests": 130000, "per": "10s", "scope": "app" },

  "identity": {
    "unit": "ru",
    "limits": [
      { "scope": "appTenant", "ru": { "S": 3500, "M": 5000, "L": 8000 }, "per": "10s",
        "writes": 3000, "writesPer": "2m30s" },
      { "scope": "app", "ru": 150000, "per": "20s", "writes": 35000, "writesPer": "5m" },
      { "scope": "tenant", "writes": 18000, "writesPer": "5m" }
    ],
    "tenantSize": { "S": "under 50 users", "M": "50 to 500 users", "L": "over 500 users" },
    "baseCosts": [ { "method": "GET", "path": "users", "ru": 2, "write": 0 } ],
    "defaults": [ { "method": "GET", "ru": 1, "write": 0 }, { "method": "POST", "ru": 1, "write": 1 } ],
    "modifiers": [ { "when": "$select", "ru": -1 }, { "when": "$expand", "ru": 1 },
                   { "when": "$top<20", "ru": -1 }, { "when": "b2cUserCreate", "ru": 4 } ],
    "minimumRu": 1,
    "notes": [ "A path that starts with me/ costs the same as users/{id|userPrincipalName}/." ]
  },

  "services": [
    { "name": "outlook", "include": "throttling-outlook.md",
      "limits": [ { "scope": "appMailbox", "requests": 10000, "per": "10m", "methods": "any", "notes": "" } ],
      "notes": "" }
  ]
}
```

`scope` is one of `app`, `tenant`, `appTenant`, or a name from the source when none of these fit
(for example `appMailbox`, `appUser`, `resource`). Use `unspecified` when the source gives no
scope. `methods` is `any` or a list like `["POST","PATCH"]`.

Forms that a limit can take, besides `requests` and `per`:

- `concurrent`: a number of requests at the same time (Outlook, Bookings).
- `uploadMB` with `per`: an upload size (Outlook).
- `minRequests`, `minConcurrent`, `minRecords`: the source says "at least" (security audit log query).
- `path` and `resource`: what the limit is for, as the source says it.
- `from` and `until` (a date): the source says when a limit starts or stops (Windows 365).

Identity (`identity`) also has:

- `specialLimits`: limits on one path that depend on a property (the `applications` path and `signInAudience`).
- `headers`: the throttle headers that the source describes, in `request`, `response`, and `throttled`.

A service whose limits depend on the request (Microsoft Teams) has `routes` in place of a long
`limits` list. Each route has `group`, `methods`, `path`, and the numbers `app`, `appTenant`,
`user` (requests per `per` of the service), and `resource` (`requests`, `per`, `of`). A missing
number means that the source has a dash. `routeDefaults` are the limits for a request that no
route names. `dimensions` says what each number counts.

Write what the source does not say as a `notes` text, for example when you had to assume
something. Do not hide an assumption in a number.

## Check it

```
go test ./graph/cost
```

The test checks that `cost.json` is valid JSON, that it comes from the commit in `raw/SOURCE.json`,
that its base costs are the rows of the identity table (no more, no less), and that every file
in `raw/includes/` is in `services`.

The test cannot check the limits of the other services. Look at a few by hand: open the
include file and compare.

## Credit

The data is from Microsoft's documentation of Microsoft Graph
(<https://github.com/microsoftgraph/microsoft-graph-docs-contrib>), licensed
CC BY 4.0. Say so where you use it.
