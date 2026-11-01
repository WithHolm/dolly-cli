<script lang="ts">
  import { login } from "../lib/api";
  import { succeeded } from "../lib/notice.svelte";
  import { poll } from "../lib/poll";
  import { timeOfDay } from "../lib/format";
  import Chip from "../lib/Chip.svelte";
  import CopyButton from "../lib/CopyButton.svelte";
  import ConfirmButton from "../lib/ConfirmButton.svelte";
  import { listUsers } from "../graph/directory";

  // A client ID for the examples. The first sign-in makes a service principal for it, as for any new client ID.
  const DEMO_CLIENT = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee";
  const SCOPE = "https://graph.microsoft.com/User.Read openid offline_access";

  // A user from the data, so the examples work as they are. Every new Dolly has Philip Meholm, a tech lead. Data that was
  // made before that has no such user, and then the first user is the example.
  let sampleUser = $state("user@example.com");
  $effect(() => {
    const first = async (q: string) => (await listUsers(q, 1)).users[0]?.userPrincipalName;
    first("Philip Meholm")
      .then((upn) => upn ?? first(""))
      .then((upn) => (sampleUser = upn ?? sampleUser))
      .catch(() => {});
  });

  interface Info {
    tenantId: string;
    authority: string;
    issuer: string;
    discovery: string;
    jwks: string;
    token: string;
    adminUser?: string;
  }
  interface SignIn {
    id: number;
    time: string;
    appId: string;
    app?: string;
    registered: boolean;
    grant: string;
    credential: "secret" | "certificate" | "none";
    resource?: string;
    user?: string;
    result: string;
    message?: string;
  }

  let info = $state.raw<Info | null>(null);
  let signins = $state.raw<SignIn[]>([]);
  let loaded = $state(false);
  let failed = $state(false); // the last poll got no answer

  $effect(() =>
    poll(async () => {
      try {
        const [i, s] = await Promise.all([login.get<Info>("/info"), login.get<{ signins: SignIn[] }>("/signins")]);
        info = i;
        signins = s.signins;
        loaded = true;
        failed = false;
      } catch (e) {
        failed = true;
        throw e; // poll tries again at the next run
      }
    }, 1500),
  );

  async function clear() {
    if (await succeeded(() => login.del("/signins"))) signins = [];
  }

  const rows = $derived(
    info
      ? [
          { label: "Authority", value: info.authority, hint: "The authority of MSAL, and the authority host of azure-identity, with the tenant." },
          { label: "Discovery", value: info.discovery, hint: "The OpenID Connect configuration." },
          { label: "Authorize", value: info.token.replace(/token$/, "authorize"), hint: "Where a client sends the browser. The person picks a user, with no password." },
          { label: "Token", value: info.token, hint: "Where a client asks for a token." },
          { label: "Keys", value: info.jwks, hint: "The public key that checks a token (JWKS)." },
        ]
      : [],
  );

  const flows: Record<string, string> = {
    client_credentials: "Client credentials",
    password: "Password",
    authorize: "Browser sign-in",
    jwtToken: "Ready-made token",
    authorization_code: "Authorization code",
    refresh_token: "Refresh token",
  };
  // the address of the sign-in page for a browser, and the same call for a script
  const authorize = $derived(
    info
      ? `${info.token.replace(/token$/, "authorize")}?${new URLSearchParams({
          client_id: DEMO_CLIENT,
          response_type: "code",
          redirect_uri: `${location.origin}/`,
          scope: SCOPE,
          state: "demo",
          prompt: "select_account", // a browser that has a session would sign in at once without this
        })}`
      : "",
  );
  const passwordCall = $derived(
    info
      ? `curl -s ${info.token} \\\n  -d grant_type=password \\\n  -d client_id=${DEMO_CLIENT} \\\n  -d username=${sampleUser} \\\n  -d password=anything \\\n  -d 'scope=${SCOPE}'`
      : "",
  );
  const meCall = `curl -s ${location.origin}/v1.0/me \\\n  -H "Authorization: Bearer $ACCESS_TOKEN"`;

  // Microsoft Graph PowerShell. The SDK keeps a custom environment between sessions, and Add-MgEnvironment writes over an old one
  // of the same name (the port can change). It signs in with an access token: its own sign-in needs an https authority, and
  // Dolly has no https.
  const pwshSetup = [
    `Add-MgEnvironment -Name Dolly -AzureADEndpoint '${location.origin}' -GraphEndpoint '${location.origin}' | Out-Null`,
  ];
  // The admin account of the owner. The profile of the data says how it is named (a.philip, adm.philip, and so on), and the server gives
  // the name. Data that was made before it has no such user, and then the example user is used.
  let adminUser = $state("a.philip@acme.example");
  $effect(() => {
    if (!info?.adminUser) return;
    const upn = info.adminUser;
    adminUser = upn;
    listUsers(upn, 1)
      .then((r) => {
        if (!r.users[0]) adminUser = sampleUser;
      })
      .catch(() => {});
  });
  // The quick connect: the server gives the three lines of the example below at /pwsh, and iex runs them.
  const pwshQuick = `iwr '${location.origin}/pwsh' | iex`;
  const pwshToken = $derived(`iwr '${location.origin}/jwtToken?hint=${adminUser}'`);
  const pwshScript = $derived(
    [
      ...pwshSetup,
      `$token = (iwr '${location.origin}/jwtToken?hint=${adminUser}').Content`,
      `Connect-MgGraph -Environment Dolly -AccessToken (ConvertTo-SecureString $token -AsPlainText -Force)`,
      `Invoke-MgGraphRequest -Uri '/v1.0/me'`,
    ].join("\n"),
  );

  // MSAL.js and MSAL Node take a custom authority only with https and knownAuthorities (checked against dollycli).
  const msalConfig = $derived(
    info
      ? `const msalConfig = {\n  auth: {\n    clientId: "${DEMO_CLIENT}",\n    authority: "${info.authority}",\n    knownAuthorities: ["${location.host}"],\n    redirectUri: "http://localhost:3000/",\n  },\n};`
      : "",
  );

  const clientCall = $derived(
    info
      ? `curl -s ${info.token} \\\n  -d grant_type=client_credentials \\\n  -d client_id=$(uuidgen) \\\n  -d client_secret=anything \\\n  -d scope=https://graph.microsoft.com/.default`
      : "",
  );
</script>

{#snippet example(text: string, copyTitle: string, label: string)}
  <div class="example">
    <!-- a block that scrolls sideways takes the focus, so a keyboard can scroll it -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
    <pre tabindex="0" role="region" aria-label={label}>{text}</pre>
    <CopyButton {text} title={copyTitle} />
  </div>
{/snippet}

<h2 class="sr-only">Sign-in</h2>
<p class="intro">
  A client signs in here and gets a signed token. Graph counts each call for the client ID in the token. A client ID that dollycli has not
  seen gets a service principal. Any secret works. To keep the first one, use <em>Capture next secret</em> on the page
  <a href="#/graph/service-principals">Service principals</a>.
</p>

<section>
  <h3>Quick connect</h3>
  <p class="muted">Microsoft Graph PowerShell, signed in as <code>{adminUser}</code>. Paste this line:</p>
  {@render example(pwshQuick, "Copy the quick connect line", "Quick connect line")}
  <p class="muted">
    The sign-in shows in the list below, and the calls that you make after it show on the page <a href="#/graph/calls">Calls</a>. You can read the
    script first: <a href="{location.origin}/pwsh">{location.origin}/pwsh</a>. For another user, add <code>?user=</code> and a user principal name.
  </p>
</section>

<section aria-labelledby="signins-title">
  <div class="head">
    <h3 id="signins-title">Sign-ins <span class="count">{signins.length}</span></h3>
    <span class="status">
      {#if failed}
        <span role="alert" class="bad">dollycli does not answer. Trying again.</span>
      {:else}
        <span class="muted small">Live: updates every 1.5 seconds</span>
      {/if}
      {#if signins.length}
        <ConfirmButton onconfirm={clear} ask="Clear the list?" title="Remove every sign-in from this list">Clear</ConfirmButton>
      {/if}
    </span>
  </div>
  <span class="sr-only" role="status">{signins.length} {signins.length === 1 ? "sign-in" : "sign-ins"} in the list</span>
  {#if loaded && !signins.length}
    <div class="empty"><strong>No sign-ins yet</strong>Ask for a token at the token address. Each try shows here, with its result.</div>
  {:else if signins.length}
    <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
    <div class="log" tabindex="0" role="region" aria-label="List of sign-ins, newest first">
      <table class="t fixed">
        <colgroup><col style="width: 90px" /><col /><col style="width: 150px" /><col style="width: 110px" /><col style="width: 130px" /></colgroup>
        <thead><tr><th>Time</th><th>Client</th><th>Flow</th><th>Credential</th><th>Result</th></tr></thead>
        <tbody>
          {#each signins as s (s.id)}
            <tr class:reasoned={s.result !== "issued" && s.message}>
              <td class="mono muted"><time datetime={s.time}>{timeOfDay(s.time)}</time></td>
              <td class="cut">
                <div class="truncate">
                  {s.app ?? s.appId}
                  {#if s.registered}<Chip tone="primary" title="dollycli made the service principal at this sign-in">new</Chip>{/if}
                </div>
                <div class="truncate muted small mono">{s.appId}{s.user ? ` · ${s.user}` : ""}{s.resource ? ` → ${s.resource}` : ""}</div>
              </td>
              <td>{flows[s.grant] ?? (s.grant || "–")}</td>
              <td class="muted">{s.credential === "none" ? "–" : s.credential}</td>
              <td><Chip tone={s.result === "issued" ? "green" : "red"}>{s.result === "issued" ? "Token issued" : s.result}</Chip></td>
            </tr>
            {#if s.result !== "issued" && s.message}
              <tr class="reason"><td></td><td colspan="4">{s.message}</td></tr>
            {/if}
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</section>

{#if info}
  <section>
    <h3>Addresses</h3>
    <dl>
      {#each rows as r (r.label)}
        <dt>{r.label}</dt>
        <dd>
          <div class="line"><code>{r.value}</code><CopyButton text={r.value} title="Copy the {r.label.toLowerCase()} address" /></div>
          <div class="muted small">{r.hint}</div>
        </dd>
      {/each}
    </dl>
  </section>

  <section>
    <h3>Other ways to sign in</h3>
    <p class="muted">
      A token for a user is <em>delegated</em>. It has the user in it, and <code>/v1.0/me</code> is that user. dollycli asks for no password: any
      password works. The examples use <code>{sampleUser}</code>, a user of your data.
    </p>

    <details>
      <summary>With a client secret (curl)</summary>
      <p class="muted">A token for the client itself, with no user. Use any secret.</p>
      {@render example(clientCall, "Copy the client secret command", "Client secret command")}
    </details>

    <details>
      <summary>In a browser</summary>
      <p class="muted">
        Send the browser to this address. dollycli shows the users, and you pick one. The browser then goes back to the
        <code>redirect_uri</code> with a <code>code</code>. Trade the code at the token address, with PKCE if the client uses it.
      </p>
      {@render example(authorize, "Copy the sign-in address", "Sign-in address")}
      <p><a class="btn" href={authorize} target="_blank" rel="noopener noreferrer">Open the sign-in page (new tab)</a></p>
    </details>

    <details>
      <summary>With a password (curl)</summary>
      <p class="muted">
        The password grant gives the token at once, so use it in tests. The answer has <code>access_token</code>, <code>id_token</code> (with
        <code>openid</code>), and <code>refresh_token</code> (with <code>offline_access</code>). A scope that Graph does not have gets an error, as in Entra.
      </p>
      {@render example(passwordCall, "Copy the password command", "Password command")}
      <p class="muted">Then use the access token:</p>
      {@render example(meCall, "Copy the /me command", "Command for /me")}
    </details>

    <details>
      <summary>PowerShell in full, and the token alone</summary>
      <p class="muted">
        The quick connect, in full. The module gets a token for <code>{adminUser}</code> from <code>/jwtToken</code>, so <code>/me</code> is that
        user. The module keeps the environment <code>Dolly</code> between sessions, and the first line writes over an old one, so paste it again
        when the port changes.
      </p>
      {@render example(pwshScript, "Copy the PowerShell script", "PowerShell script")}
      <p class="muted">
        <code>/jwtToken</code> gives the token as text, for any tool. <code>?hint=</code> is a user principal name or an object ID. The token is good for ten
        years, and it has a message in its claims.
      </p>
      {@render example(pwshToken, "Copy the token command", "Token command")}
      <p class="muted">
        The sign-in of the module itself (secret, certificate, device code, browser) needs an https address, so it does not work with dollycli. It stops with
        <em>Authority host must be a TLS protected (https) endpoint</em>.
      </p>
    </details>

    <details>
      <summary>A web app with a library (MSAL, OpenID Connect)</summary>
      <p class="muted">
        MSAL.js and MSAL Node accept an <code>https</code> authority only, and they need <code>knownAuthorities</code> for a custom host.
        {#if location.protocol === "http:"}
          This dollycli runs on http. Start it with <code>dolly serve --tls</code>, and trust the CA file <code>dolly-data/tls/ca.pem</code>
          (the browser or the system, and <code>NODE_EXTRA_CA_CERTS</code> for Node). It is at <code>/dolly-ca.pem</code> too.
        {:else}
          This dollycli runs on https. Trust the CA file <code>dolly-data/tls/ca.pem</code> (the browser or the system, and
          <code>NODE_EXTRA_CA_CERTS</code> for Node). <a href="{location.origin}/dolly-ca.pem">Get it here</a>.
        {/if}
        Libraries that allow http, such as <code>openid-client</code> with <code>allowInsecureRequests</code>, need none of this.
      </p>
      {@render example(msalConfig, "Copy the MSAL settings", "MSAL settings")}
      <p class="muted">
        A web app in the browser may call dollycli from <code>localhost</code> (CORS). Sign out with <code>end_session_endpoint</code> from the
        discovery document, and <code>post_logout_redirect_uri</code>. A browser that signed in once has a session: a request with no
        <code>prompt</code> signs in at once, and <code>prompt=select_account</code> shows the users again.
      </p>
    </details>
  </section>
{:else}
  <p class="muted" role={failed ? "alert" : "status"}>{failed ? "dollycli does not answer. Trying again." : "Loading…"}</p>
{/if}

<style>
  .intro { max-width: 70ch; margin: 0 0 20px; color: var(--muted); }
  section { margin-bottom: 28px; }
  .head { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 8px; }
  .status { display: flex; align-items: center; gap: 12px; }
  .count { font-weight: 400; color: var(--muted); }
  .bad { color: var(--red); font-size: 12px; }
  .log { max-height: 320px; overflow: auto; border: 1px solid var(--line); border-radius: var(--radius); }
  .log :global(th) { position: sticky; top: 0; background: var(--surface); }
  tr.reasoned td { border-bottom-color: transparent; }
  tr.reason td { padding-top: 0; font-size: 12px; color: var(--text); overflow-wrap: anywhere; }
  dl { display: grid; grid-template-columns: 100px minmax(0, 1fr); gap: 8px 16px; margin: 8px 0 12px; }
  dt { color: var(--muted); padding-top: 4px; }
  dd { margin: 0; min-width: 0; }
  dd code { overflow-wrap: anywhere; }
  .line { display: flex; align-items: center; gap: 4px; }
  details { border-top: 1px solid var(--line); padding: 10px 0; }
  summary { cursor: pointer; font-weight: 500; }
  .example { display: flex; align-items: flex-start; gap: 4px; margin-top: 8px; min-width: 0; }
  pre { margin: 0; padding: 10px 12px; flex: 1; min-width: 0; overflow-x: auto; font-size: 12.5px; background: var(--sunken); border-radius: var(--radius); }
  .small { font-size: 12px; }
  section p, details p { max-width: 80ch; margin-top: 8px; }
  section a.btn, details a.btn { text-decoration: none; }
</style>
