# Sign-in

`dolly serve` also runs a fake Entra sign-in API, on the same port. A client gets a real, signed token (RS256) and
Graph checks it. The paths are the ones of Entra, with the tenant first. The tenant is the tenant ID, one of its
domains, or `common`:

    http://127.0.0.1:8650/<tenant>/v2.0/.well-known/openid-configuration
    http://127.0.0.1:8650/<tenant>/discovery/v2.0/keys
    http://127.0.0.1:8650/<tenant>/oauth2/v2.0/token        (and /oauth2/token, with resource=)

    curl -s http://127.0.0.1:8650/<tenant>/oauth2/v2.0/token -d grant_type=client_credentials \
      -d client_id=$(uuidgen) -d client_secret=anything -d scope=https://graph.microsoft.com/.default

- **Grants.**
  - `client_credentials`: an application token. `roles` are the application permissions of the service principal.
  - `authorization_code`: the browser flow, with PKCE (S256 or plain). The client sends the browser to
    `/<tenant>/oauth2/v2.0/authorize`. dollycli shows a page with the users and **no password**: pick one, and the
    browser goes back to the `redirect_uri` with a code (`response_mode` query, fragment, or form_post). A code works
    once, for ten minutes. A client ID or a redirect address that is wrong gives an error page, not a redirect.
  - `password`: a delegated token for a user. dollycli accepts any password.
  - `refresh_token`: with `offline_access`. A refresh token is a signed JWT, so it is good for 90 days and after a restart.
  - A delegated token has `scp`, the user (`oid`, `upn`), and an `id_token` with `openid` (with the `nonce`).
  - A browser that picked a user has a session (the cookie `dolly_session`, 24 hours). A request with no `prompt`, or with `prompt=none`, gets a code at once.
    `prompt=login`, `select_account`, and `consent` show the page again. Without a session, `prompt=none` is `login_required`.
  - Sign-out: `end_session_endpoint` is in the discovery document (`/<tenant>/oauth2/v2.0/logout`). It ends the session and goes to
    `post_logout_redirect_uri` (http or https, with `state`). Entra wants that address registered. dollycli has no app registrations, so it takes any.
  - Not built: device code, `response_type` with a token or id_token, consent pages, checks of the redirect address against an app registration.
- **`/me`.** With a delegated token, `/me` and `/me/...` (also in `$batch`) are the user of the token. With an
  application token, or a token that dollycli did not make, `/me` is 400, as in Graph.
- **New clients.** A client ID that dollycli has not seen gets a service principal (open mode). Use any secret, or a
  certificate in `client_assertion`. In the portal, *Capture next secret* keeps the next one that a client shows
  (only a hash is saved), and dollycli refuses all others from then on (`AADSTS7000215`).
- **Graph.** A token that dollycli made is checked. A changed, expired, or wrong-audience token is a 401. The call is counted for
  the client ID in the token. Any other token still works, as before.
- **The key** is `dolly-data/login/key.pem`, so a token is good after a restart. It is a fake key.

A call that fails answers as Entra does: `error`, `error_codes`, and `AADSTS` numbers in the text. The portal page
*Sign-in* has the addresses to copy, and a list of every try.

## Microsoft Graph PowerShell

One line connects it, as the admin account `adm.philip@fjordvik.example` (`a.philip@acme.example` in data that was made with Acme): `iwr 'http://127.0.0.1:8650/pwsh' | iex`.
dollycli gives three lines at `/pwsh`. They run `Add-MgEnvironment -Name Dolly -AzureADEndpoint <address> -GraphEndpoint <address>`, get a token from
`/jwtToken?hint=<user principal name or object ID>`, and run `Connect-MgGraph -Environment Dolly -AccessToken ...` with it. `/pwsh?user=` sets another user.
The token is good for ten years, and it has a message in its claims. The page *Sign-in* has the lines to copy.
The module's own sign-in (secret, certificate, browser, device code) needs an https address, so it does not work: dollycli has no https.

## A web app with a library

A web app in the browser works against dollycli. MSAL.js, MSAL Node, `@azure/identity`, `oidc-client-ts`, and `openid-client` were
checked. MSAL and `@azure/identity` need https (`dolly serve --tls`). A web app on `localhost` may call dollycli from the browser (CORS).
The settings of each library, and what was checked, are in [sign-in libraries](libraries.md).

## Azure CLI

`az` works in part. See [Azure CLI](azure-cli.md).
