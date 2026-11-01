# Sign-in libraries and dollycli

You can use dollycli as the Entra of a web app in development. This page says what each library needs, and what was checked.

All checks were made on 2026-10-30 against a running dollycli, with a real browser (Chromium) for the browser libraries. A library that is not in the first table was not tested. Do not read a missing library as "works" or as "does not work".

## What was tested

| Library | Version | Where it runs | Needs https | Other settings | Result |
|---|---|---|---|---|---|
| MSAL.js (`@azure/msal-browser`) | 5.27.0 | browser | yes | `knownAuthorities` | Works. Full sign-in with the code flow and PKCE. |
| MSAL Node (`@azure/msal-node`) | 7.1.2 | server | yes | `knownAuthorities` | Works. Code flow with a client secret. |
| `@azure/identity` | 4.14.1 | server | yes | `disableInstanceDiscovery: true` | Works with `ClientSecretCredential` and `UsernamePasswordCredential`. |
| `oidc-client-ts` | 3.5.0 | browser | no | none | Works. Sign-in, refresh token, silent sign-in (needs https), sign-out. |
| `openid-client` | 6.8.8 | server | no | `allowInsecureRequests` | Works. Code flow with PKCE, ID token checks (nonce, issuer, signature), refresh. |

"Needs https" means that the library refuses an `http` address. It stops before it sends any request.

## Start dollycli for the library

For a library that needs http only, nothing special:

    dolly serve

For a library that needs https:

    dolly serve --tls

dollycli makes a local certificate authority (CA) the first time. It keeps the CA in `dolly-data/tls/ca.pem`, and it signs a certificate for `localhost`, `127.0.0.1`, `--host`, and the names in `--allow-host`. Trust the CA once:

- **Node.** Run `NODE_EXTRA_CA_CERTS=dolly-data/tls/ca.pem node app.js`.
- **Browser.** Add `ca.pem` as an authority in the browser or in the system. The file is at `/dolly-ca.pem` on the server too.

Without a data folder (`--dir=`) the CA is new at each start, so nothing can trust it. To use a certificate of your own, give `--tls-cert` and `--tls-key`. More is in [TLS](tls.md).

The tenant ID is on the page *Sign-in* of the portal. In these examples it is `<tenant>`. The address is `https://127.0.0.1:8650` with `--tls`, and `http://127.0.0.1:8650` without it. Use any client ID that is a GUID and any client secret. dollycli makes a service principal for a client ID that it has not seen.

## MSAL.js and MSAL Node

Both refuse an `http` authority (`authority_uri_insecure`) in every mode, including the OIDC mode. Start dollycli with `--tls`.

Without `knownAuthorities`, MSAL asks Microsoft about the host and stops with `endpoints_resolution_error`.

    const msalConfig = {
      auth: {
        clientId: "<client ID>",
        authority: "https://127.0.0.1:8650/<tenant>",
        knownAuthorities: ["127.0.0.1:8650"],
        redirectUri: "http://localhost:3000/",
      },
    };

For MSAL Node, add `clientSecret: "anything"` to `auth`, and use `ConfidentialClientApplication`. A scope such as `User.Read` is read as a scope of Microsoft Graph. The checks used `loginRedirect` and `handleRedirectPromise` in the browser, and `getAuthCodeUrl` and `acquireTokenByCode` on the server. The token works on `/v1.0/me`, and the ID token has `preferred_username`.

Not tested: `acquireTokenSilent` with a hidden frame in MSAL.js, popups, and the broker.

## `@azure/identity`

It refuses an `http` authority host (`The authorityHost address must use the 'https' protocol.`). With https it needs `disableInstanceDiscovery: true`. Without it, it stops with `endpoints_resolution_error`.

    import { ClientSecretCredential } from "@azure/identity";

    const credential = new ClientSecretCredential("<tenant>", "<client ID>", "anything", {
      authorityHost: "https://127.0.0.1:8650",
      disableInstanceDiscovery: true,
    });
    const token = await credential.getToken("https://graph.microsoft.com/.default");

`UsernamePasswordCredential` works the same way, with any password. dollycli accepts any password.

## `oidc-client-ts`

It accepts `http`. It needs no other setting, because it reads the discovery document and the keys. A browser app calls dollycli from another origin, so it needs CORS. dollycli allows it for pages on `localhost` (see [what dollycli does for a web app](#what-dollycli-does-for-a-web-app)).

    import { UserManager } from "oidc-client-ts";

    const manager = new UserManager({
      authority: "http://127.0.0.1:8650/<tenant>/v2.0",
      client_id: "<client ID>",
      redirect_uri: "http://localhost:3000/callback",
      post_logout_redirect_uri: "http://localhost:3000/",
      scope: "openid profile offline_access User.Read",
      response_type: "code",
    });

The checks used `signinRedirect` and `signinCallback` (PKCE, with an ID token and a refresh token), a call to `/v1.0/me` from the page, and `signoutRedirect`, which goes back to `post_logout_redirect_uri`. Silent sign-in in a hidden frame (`signinSilent` without a refresh token) works with `--tls`. Over `http` it fails with `login_required`, because the browser does not send the session cookie (see [what dollycli does for a web app](#what-dollycli-does-for-a-web-app)).

## `openid-client`

It refuses `http` (`ClientError: only requests to HTTPS are allowed`) unless you allow it for the call of `discovery`.

    import * as client from "openid-client";

    const config = await client.discovery(
      new URL("http://127.0.0.1:8650/<tenant>/v2.0"),
      "<client ID>",
      { client_secret: "anything" },
      undefined,
      { execute: [client.allowInsecureRequests] },
    );

The checks used `buildAuthorizationUrl` with PKCE, `state`, and `nonce`, then `authorizationCodeGrant` (the library checks the ID token) and `refreshTokenGrant`.

## What dollycli does for a web app

**CORS.** A page on `localhost`, on a loopback address, or on a name in `--allow-host` may call the Graph API and the discovery, keys, and token paths. A page from another origin gets no CORS headers, so a web site cannot read or change the data by way of your browser. The admin API, the authorize page, `/pwsh`, and `/jwtToken` have no CORS headers.

**Sessions.** When a person picks a user, dollycli sets the cookie `dolly_session` (24 hours).

- A request with no `prompt`, or with `prompt=none`, gets a code at once (single sign-on).
- `prompt=login`, `select_account`, and `consent` show the page with the users again.
- With no session, `prompt=none` gives the error `login_required` (`AADSTS50058`).
- Over `https` the cookie is `SameSite=None; Secure`, so a hidden frame of your app sends it. Over `http` it is `SameSite=Lax`, and a browser does not send it from a frame of another site. Silent sign-in in a hidden frame therefore needs `--tls`.

**Sign-out.** The discovery document has `end_session_endpoint`. It ends the session and goes to `post_logout_redirect_uri` (with `state`, if you sent it). Entra wants that address to be registered. dollycli has no app registrations, so it takes any address with the scheme `http` or `https`.

**Response modes.** `query`, `fragment`, and `form_post`. The `state` comes back as it was sent. MSAL.js sends a `state` with `=` in it, and an older version of dollycli encoded it twice in the fragment, so MSAL.js refused the answer (`invalid_state`). This is fixed.

## What is different from Entra

- dollycli has no app registrations. It does not check `redirect_uri` against a list, so a redirect address that Entra would refuse works here. Check your redirect addresses in the real tenant.
- There are no consent pages, no multi-factor pages, and no password. The person picks a user.
- There is no device code flow, and no implicit or hybrid flow (`response_type` is `code` only).
- A library that checks the issuer with Microsoft's own list of hosts (instance discovery) stops. That is why MSAL needs `knownAuthorities`, and `@azure/identity` needs `disableInstanceDiscovery`.
- The claims in a token are not compared with a real Entra token one by one. The Graph Explorer proxy does not reach the sign-in endpoints, so there was no real answer to compare with.

## Errors and what they mean

| Error | Library | Cause and fix |
|---|---|---|
| `authority_uri_insecure` | MSAL | The authority is `http`. Start dollycli with `--tls`. |
| `The authorityHost address must use the 'https' protocol.` | `@azure/identity` | Same. Use `--tls`. |
| `endpoints_resolution_error` | MSAL, `@azure/identity` | The library asked Microsoft about the host. Set `knownAuthorities` (MSAL) or `disableInstanceDiscovery: true` (`@azure/identity`). |
| `invalid_state` | MSAL.js | Fixed in dollycli. Update. |
| "blocked by CORS policy" in the console | any browser library | The page is not on `localhost`, a loopback address, or a name in `--allow-host`. |
| `login_required` for `prompt=none` | any | The browser has no session: nobody picked a user yet, the session is over 24 hours old, or the browser did not send the cookie (a hidden frame over `http`). |
| `AADSTS70000` | any | The code was used before, it is older than 10 minutes, or `redirect_uri` or the client ID is not the one of the sign-in request. |
| A certificate error (not tested for the wording) | Node, browser | The CA is not trusted yet. See "Start dollycli for the library". |

## How this was checked

Run `make compat` to check again. The scripts are in [compat/](compat/). They build dollycli, start it on http and on https, and run 16 cases: each library with and without the setting that it needs, plus the CORS rules. Each case says what it expects (it works, or it stops with a given error), and a row that does something else is reported as failed. The browser libraries run in Chromium (driven by `puppeteer-core`), and the script picks the user on the sign-in page like a person does. Chromium trusts the key of the test server and no other certificate. The Node libraries ran in Node 24.

To test another library, or to update this page after a new version, follow [compat/WORKFLOW.md](compat/WORKFLOW.md).
