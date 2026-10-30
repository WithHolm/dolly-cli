<script lang="ts">
  import { login } from "../lib/api";
  import { succeeded } from "../lib/notice.svelte";
  import { poll } from "../lib/poll";
  import { timeOfDay } from "../lib/format";
  import Chip from "../lib/Chip.svelte";
  import CopyButton from "../lib/CopyButton.svelte";
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

  $effect(() =>
    poll(async () => {
      const [i, s] = await Promise.all([login.get<Info>("/info"), login.get<{ signins: SignIn[] }>("/signins")]);
      info = i;
      signins = s.signins;
      loaded = true;
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
    authorize: "Sign-in page",
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
  // The admin account of the owner (a. and the name). Data that was made before it has no such user, and then the example user is used.
  let adminUser = $state("a.philip@acme.example");
  $effect(() => {
    listUsers("a.philip@", 1)
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

  const example = $derived(
    info
      ? `curl -s ${info.token} \\\n  -d grant_type=client_credentials \\\n  -d client_id=$(uuidgen) \\\n  -d client_secret=anything \\\n  -d scope=https://graph.microsoft.com/.default`
      : "",
  );
</script>

<p class="intro">
  A client signs in here and gets a signed token. Graph checks the token and counts the call for the client ID in it. A client ID that
  dollycli has not seen gets a service principal. Use any secret: dollycli can keep the first one for you (Service principals, "Capture next secret").
</p>

{#if info}
  <section>
    <h3>Addresses</h3>
    <dl>
      {#each rows as r (r.label)}
        <dt title={r.hint}>{r.label}</dt>
        <dd><code>{r.value}</code><CopyButton text={r.value} title="Copy the {r.label.toLowerCase()} address" /></dd>
      {/each}
    </dl>
    <details>
      <summary>Try it from a terminal</summary>
      <div class="example">
        <pre>{example}</pre>
        <CopyButton text={example} title="Copy the command" />
      </div>
    </details>
  </section>
{/if}

{#if info}
  <section>
    <h3>Sign in as a user</h3>
    <p class="muted">
      A token for a user is a <em>delegated</em> token. It has the user in it, and <code>/v1.0/me</code> is that user. dollycli asks for no
      password: any password works. The examples use <code>{sampleUser}</code>, a user of your data.
    </p>

    <h4>In a browser</h4>
    <p class="muted">
      Send the browser to the authorize address. dollycli shows the users, and you pick one. The browser then goes back to the
      <code>redirect_uri</code> with a <code>code</code>. The client trades the code at the token address, with PKCE if it uses it.
    </p>
    <div class="example">
      <pre>{authorize}</pre>
      <CopyButton text={authorize} title="Copy the address" />
    </div>
    <p><a class="btn" href={authorize} target="_blank" rel="noopener noreferrer">Open the sign-in page</a></p>

    <h4>From a script, with a password</h4>
    <p class="muted">The password grant gives the token at once. Use it in tests, where no browser is there.</p>
    <div class="example">
      <pre>{passwordCall}</pre>
      <CopyButton text={passwordCall} title="Copy the command" />
    </div>
    <p class="muted">
      The answer has <code>access_token</code>, an <code>id_token</code> (with <code>openid</code>), and a <code>refresh_token</code> (with
      <code>offline_access</code>). Use the access token:
    </p>
    <div class="example">
      <pre>{meCall}</pre>
      <CopyButton text={meCall} title="Copy the command" />
    </div>
    <p class="muted">The scope says what the token may ask of Graph. A scope that Graph does not have gets an error, as in Entra.</p>
  </section>

  <section>
    <h3>Microsoft Graph PowerShell</h3>
    <p class="muted">
      Paste these lines in PowerShell with the module <code>Microsoft.Graph.Authentication</code>. The first line points the module at dollycli. The
      module keeps this setting between sessions, and the line writes over an old one with the same name, so you can paste it again when
      the port changes. The other Microsoft.Graph modules, such as <code>Get-MgUser</code>, use the same connection.
    </p>

    <h4>Quick connect</h4>
    <p class="muted">
      One line. dollycli gives the first three lines of the next example as text at <code>/pwsh</code>, and <code>iex</code> runs them. Read the text first
      if you like: <a href="{location.origin}/pwsh">{location.origin}/pwsh</a>. To connect as another user, add <code>?user=</code> and a user
      principal name.
    </p>
    <div class="example">
      <pre>{pwshQuick}</pre>
      <CopyButton text={pwshQuick} title="Copy the line" />
    </div>

    <h4>The same, in full</h4>
    <p class="muted">
      The module gets an access token for <code>{adminUser}</code> from <code>/jwtToken</code>, so <code>/me</code> is that user. The token is good for
      ten years, so you can keep it in a script. It is not in the list of sign-ins.
    </p>
    <div class="example">
      <pre>{pwshScript}</pre>
      <CopyButton text={pwshScript} title="Copy the lines" />
    </div>

    <h4>Only the token</h4>
    <p class="muted">
      <code>/jwtToken</code> gives the token as text, for any tool. <code>?hint=</code> is a user principal name or an object ID. Open the
      token in a JWT viewer: there is a message in it.
    </p>
    <div class="example">
      <pre>{pwshToken}</pre>
      <CopyButton text={pwshToken} title="Copy the line" />
    </div>

    <p class="muted">
      The sign-in of the module itself (client secret, certificate, device code, browser) does not work with dollycli. The module needs an https
      address for it, and stops with <em>Authority host must be a TLS protected (https) endpoint</em>. Get the token from dollycli, as above.
    </p>
  </section>
{/if}

<section>
  <div class="head">
    <h3>Sign-ins <span class="faint">{signins.length}</span></h3>
    {#if signins.length}<button type="button" class="btn ghost" onclick={clear}>Clear</button>{/if}
  </div>
  {#if loaded && !signins.length}
    <div class="empty"><strong>No sign-ins yet</strong>Ask for a token at the token address. Each try shows here, with its result.</div>
  {:else if signins.length}
    <table class="t fixed">
      <colgroup><col style="width: 90px" /><col /><col style="width: 150px" /><col style="width: 130px" /><col style="width: 130px" /></colgroup>
      <thead><tr><th>Time</th><th>Client</th><th>Flow</th><th>Credential</th><th>Result</th></tr></thead>
      <tbody>
        {#each signins as s (s.id)}
          <tr>
            <td class="mono muted">{timeOfDay(s.time)}</td>
            <td class="cut">
              <div class="truncate">
                {s.app ?? s.appId}
                {#if s.registered}<Chip tone="primary" title="dollycli made the service principal at this sign-in">new</Chip>{/if}
              </div>
              <div class="truncate muted small mono">{s.appId}{s.user ? ` · ${s.user}` : ""}{s.resource ? ` → ${s.resource}` : ""}</div>
            </td>
            <td>{flows[s.grant] ?? (s.grant || "–")}</td>
            <td class="muted">{s.credential === "none" ? "–" : s.credential}</td>
            <td><Chip tone={s.result === "issued" ? "green" : "red"} title={s.message}>{s.result === "issued" ? "Token issued" : s.result}</Chip></td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</section>

<style>
  .intro { max-width: 70ch; margin: 0 0 20px; color: var(--muted); }
  section { margin-bottom: 28px; }
  .head { display: flex; align-items: center; justify-content: space-between; }
  h3 .faint { font-weight: 400; }
  dl { display: grid; grid-template-columns: 100px minmax(0, 1fr); gap: 6px 16px; margin: 8px 0 12px; align-items: center; }
  dt { color: var(--muted); }
  dd { margin: 0; display: flex; align-items: center; gap: 4px; min-width: 0; }
  dd code { overflow-wrap: anywhere; }
  summary { cursor: pointer; color: var(--muted); }
  .example { display: flex; align-items: flex-start; gap: 4px; margin-top: 8px; }
  pre { margin: 0; padding: 10px 12px; flex: 1; overflow-x: auto; font-size: 12.5px; background: var(--sunken); border-radius: var(--radius); }
  .small { font-size: 12px; }
  h4 { margin: 16px 0 2px; font-size: 13px; font-weight: 600; }
  section p { max-width: 80ch; }
  section a.btn { text-decoration: none; }
</style>
