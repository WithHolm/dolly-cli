# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Developers and testers. They run dollycli on localhost or in CI, point an SDK, a script or an app at it, and use the portal to see the calls, the data, and the sign-ins.

IT admins and consultants. They try Microsoft Graph PowerShell or Graph scripts against fake data before they run them against a real tenant.

Course instructors and students. dollycli runs on a local network, one container per student, and the instructor uses the portal on localhost. The portal and the admin API accept `localhost` and `127.0.0.1` only, unless `--allow-host` names more.

The portal is for a desktop browser.

## Product Purpose

dollycli is a fake Microsoft Graph and a fake Entra sign-in, with fake data, on the machine of the user. Code that works against dollycli works against the real thing, and nothing touches a real tenant. Success is a developer who finds a bug, or a student who learns a flow, with no real tenant, real user, or real secret in reach.

The portal shows what the APIs do and lets a person change it: users, groups, organization, service principals and their permissions, the list of calls, the sign-ins, the history of changes, and a reference of the Graph calls that dollycli answers.

## Positioning

dollycli is 1:1 with the real Graph. Error texts, headers, query rules ($filter, $orderby, $search, $count) and the shape of answers are checked against the real Graph, and where a behavior is a guess, the docs say so. A neighbour that is a generic mock server cannot claim that.

## Operating Context

- A single Go program with the portal built in (`dolly serve`). Data is a folder (`dolly-data`) with a seed and a log of changes, or it stays in memory.
- Runs in a container for courses and CI. dollycli has no sign-in of its own: anyone who can reach the port can read and change the data, so the port is published to localhost.
- Users meet dollycli in a terminal first (curl, PowerShell, an SDK), and open the portal to see what happened. The sign-in page gives lines to copy.
- The portal is a client of the Graph API (it signs in as its own service principal, *Dolly portal*) and of the admin API.

## Capabilities and Constraints

- Microsoft Graph: users, groups, organization, service principals, permission grants, places, `$batch`, `/me`. Throttling and request cost are counted, and dollycli can answer 429.
- Entra sign-in: discovery, keys, token endpoint (client credentials, password, authorization code with PKCE, refresh token), and a sign-in page with no password. Tokens are real signed JWTs with a fake key. No device code, no consent pages, no https.
- Quick connect for Microsoft Graph PowerShell: `iwr '<address>/pwsh' | iex`. It signs in as the admin account `a.philip@acme.example`.
- Terminology is binding. Say service principal, app registration, client ID, or application ID. Never "app" for an Entra object. A user is assigned to an enterprise application, and the service principal is behind it. Graph's own names stay (`appRoleAssignments`, `appId`).
- The name is binding too. The product is **dollycli** (also written dolly-cli), never just "Dolly". In text, titles, and the portal, write dollycli. The program and its command are `dolly`, the admin API is `/_dolly`, the data folder is `dolly-data`, and the portal client is *Dolly portal*. Those stay.
- User-facing text is in short, simple sentences (ASD-STE100) in the voice of the owner. dollycli must not claim what it does not do: a guess is marked as a guess.
- dollycli is a development tool. It issues tokens no real service will trust and must never be used as an identity provider. It is not affiliated with or endorsed by Microsoft.
- Undecided: the Azure data plane (planned after Entra).

## Brand Commitments

The name is dollycli, also written dolly-cli. dollycli is the form for marketing. Never "Dolly" alone, in text, titles, headings, or help. The command stays `dolly`, the admin API stays `/_dolly`, the folder stays `dolly-data`, and the service principal stays *Dolly portal*.

All data is fake. The owner user is Philip Meholm (`philip.meholm@acme.example`, Tech Lead), and the examples use this user. Never put a real address in fake data.

## Evidence on Hand

- A running portal with pages for Users, Groups, Organization, Service principals, Calls, Sign-in, Data, History, and API (`ui/src`).
- Checks against the real Graph through the proxy of Graph Explorer (see `CLAUDE.md`). Not covered yet: the opaque `$skiptoken`, `nextLink`, other resources, and `dependsOn` in `$batch`.
- There are no customers, testimonials, benchmarks, or pricing. Do not invent any.

## Product Principles

1. Be 1:1 with Graph. Fix what dollycli returns before documenting it. Say when something is a guess.
2. Nothing real. Fake data, fake keys, and no real tenant, user, or secret.
3. Say what dollycli does, in short plain sentences, with the exact address or command to copy.
4. Show what happened. Every call, sign-in, and change is visible and can be undone.
5. Use the right word for the Entra object. The wrong word hides which object a setting belongs to.

## Accessibility & Inclusion

WCAG 2.1 AA. Everything works by keyboard, focus is visible, contrast meets AA, and controls have labels for screen readers.
