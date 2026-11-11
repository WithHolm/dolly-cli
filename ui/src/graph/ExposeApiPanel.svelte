<script lang="ts">
  import { graph } from "../lib/api";
  import { succeeded } from "../lib/notice.svelte";
  import { poll } from "../lib/poll";
  import Chip from "../lib/Chip.svelte";
  import ConfirmButton from "../lib/ConfirmButton.svelte";
  import CopyButton from "../lib/CopyButton.svelte";
  import { consentWho, defaultIdentifierUri, scopeFullName } from "./scopes";
  import type { ApiScopeDef, ApiSettings, AppRegistration, ServicePrincipal } from "./types";

  // The API that an app registration exposes: the Application ID URI, the scopes that a client can ask for, and the clients that need no consent for some of them.
  // A client asks for a scope as <Application ID URI>/<value>, and the token has the value in the claim scp. `changed` tells the page to read again.
  let { app, changed }: { app: AppRegistration; changed: () => Promise<void> } = $props();

  type ScopeForm = { id?: string; value: string; type: "User" | "Admin"; adminName: string; adminDesc: string; userName: string; userDesc: string };
  type ClientForm = { appId: string; ids: string[]; fixed: boolean };

  let api = $state.raw<ApiSettings | null>(null);
  let sps = $state.raw<ServicePrincipal[]>([]);
  let newUri = $state("");
  let scopeForm = $state<ScopeForm | null>(null);
  let clientForm = $state<ClientForm | null>(null);

  async function load() {
    const [a, s] = await Promise.all([graph.get<ApiSettings>(`/applications/${app.appId}/api`), graph.get<{ servicePrincipals: ServicePrincipal[] }>("/service-principals")]);
    [api, sps] = [a, s.servicePrincipals];
  }
  $effect(() => poll(load, 2000));

  const base = $derived(`/applications/${app.appId}`);
  const uris = $derived(api?.identifierUris ?? []);
  const scopes = $derived(api?.scopes ?? []);
  const clients = $derived(api?.preAuthorizedApplications ?? []);
  const clientName = (appId: string) => sps.find((s) => s.appId.toLowerCase() === appId.toLowerCase())?.displayName;
  const scopeById = (id: string) => scopes.find((s) => s.id === id);

  async function after(ok: boolean) {
    if (ok) {
      await load();
      await changed();
    }
    return ok;
  }

  // ---- the Application ID URI ----
  const setUris = async (list: string[], done: string) => after(await succeeded(() => graph.put(`${base}/identifier-uris`, { identifierUris: list }), done));
  async function addUri(e: SubmitEvent) {
    e.preventDefault();
    if (await setUris([...uris, newUri.trim()], "Added the Application ID URI.")) newUri = "";
  }

  // ---- the scopes ----
  const newScope = (): ScopeForm => ({ value: "", type: "Admin", adminName: "", adminDesc: "", userName: "", userDesc: "" });
  const editScope = (s: ApiScopeDef): ScopeForm => ({
    id: s.id,
    value: s.value,
    type: s.type,
    adminName: s.adminConsentDisplayName,
    adminDesc: s.adminConsentDescription,
    userName: s.userConsentDisplayName ?? "",
    userDesc: s.userConsentDescription ?? "",
  });

  async function saveScope(e: SubmitEvent) {
    e.preventDefault();
    const f = scopeForm;
    if (!f) return;
    const body = {
      value: f.value.trim(),
      type: f.type,
      adminConsentDisplayName: f.adminName.trim(),
      adminConsentDescription: f.adminDesc.trim(),
      userConsentDisplayName: f.type === "User" ? f.userName.trim() || null : null,
      userConsentDescription: f.type === "User" ? f.userDesc.trim() || null : null,
    };
    const done = f.id ? `Changed ${body.value}.` : `Added ${body.value}.`;
    if (await after(await succeeded(() => (f.id ? graph.patch(`${base}/api-scopes/${f.id}`, body) : graph.post(`${base}/api-scopes`, body)), done))) scopeForm = null;
  }
  const setEnabled = async (s: ApiScopeDef, on: boolean) =>
    after(await succeeded(() => graph.patch(`${base}/api-scopes/${s.id}`, { isEnabled: on }), on ? `Enabled ${s.value}.` : `Disabled ${s.value}.`));
  const removeScope = async (s: ApiScopeDef) => {
    await after(await succeeded(() => graph.del(`${base}/api-scopes/${s.id}`), `Removed ${s.value}.`));
  };

  // ---- the clients that need no consent ----
  async function saveClient(e: SubmitEvent) {
    e.preventDefault();
    const f = clientForm;
    if (!f) return;
    const appId = f.appId.trim();
    if (await after(await succeeded(() => graph.put(`${base}/pre-authorized`, { appId, delegatedPermissionIds: f.ids }), `${clientName(appId) ?? appId} needs no consent for these scopes.`))) clientForm = null;
  }
  const removeClient = async (appId: string) => {
    await after(await succeeded(() => graph.del(`${base}/pre-authorized/${appId}`), "Removed the client."));
  };
</script>

<section>
  <h3>Application ID URI</h3>
  <p class="hint">
    The name of this API. A client asks for a scope as the URI, a slash, and the value: <code>{scopeFullName(uris, app.appId, "Files.Read")}</code>. Entra suggests
    <code>{defaultIdentifierUri(app.appId)}</code>.{#if !uris.length} Until you set one, dollycli takes that one.{/if}
  </p>
  <ul class="uris">
    {#each uris as u (u)}
      <li>
        <code>{u}</code><CopyButton text={u} title="Copy the Application ID URI" />
        <ConfirmButton onconfirm={() => setUris(uris.filter((x) => x !== u), "Removed the Application ID URI.").then(() => undefined)} ask="Remove?" title="Remove {u}">Remove</ConfirmButton>
      </li>
    {/each}
  </ul>
  <form class="inline" onsubmit={addUri} aria-label="Add an Application ID URI">
    <input type="text" bind:value={newUri} placeholder={defaultIdentifierUri(app.appId)} aria-label="Application ID URI" autocomplete="off" />
    <button type="submit" class="btn" disabled={!newUri.trim()}>Add</button>
    {#if !uris.includes(defaultIdentifierUri(app.appId))}
      <button type="button" class="btn ghost" onclick={() => (newUri = defaultIdentifierUri(app.appId))}>Use the suggestion</button>
    {/if}
  </form>
</section>

<section>
  <div class="head"><h3>Scopes</h3><button type="button" class="btn" disabled={!!scopeForm} onclick={() => (scopeForm = newScope())}>+ Scope</button></div>
  <p class="hint">A scope is a delegated permission: a client asks for it on behalf of a signed-in user, and the token has its value in <code>scp</code>. A scope is disabled first, and then it can be removed.</p>
  {#if api && !scopes.length && !scopeForm}
    <div class="empty"><strong>No scopes</strong>Add one for a client to ask for.</div>
  {:else if scopes.length}
    <table class="t" aria-label="Scopes of {app.displayName}">
      <thead><tr><th>Scope</th><th>Who can consent</th><th>State</th><th></th></tr></thead>
      <tbody>
        {#each scopes as s (s.id)}
          <tr>
            <td>
              <div class="full"><code>{scopeFullName(uris, app.appId, s.value)}</code><CopyButton text={scopeFullName(uris, app.appId, s.value)} title="Copy the scope" /></div>
              <div class="muted">{s.adminConsentDisplayName}</div>
            </td>
            <td>{consentWho(s.type)}</td>
            <td>{#if s.isEnabled}<Chip tone="green">enabled</Chip>{:else}<Chip tone="amber" title="Disabled: a client cannot ask for it. It can be removed.">disabled</Chip>{/if}</td>
            <td class="actions">
              <button type="button" class="btn ghost" onclick={() => (scopeForm = editScope(s))}>Edit</button>
              {#if s.isEnabled}
                <button type="button" class="btn ghost" onclick={() => setEnabled(s, false)} title="Disable the scope first. Then it can be removed.">Disable</button>
              {:else}
                <button type="button" class="btn ghost" onclick={() => setEnabled(s, true)}>Enable</button>
                <ConfirmButton onconfirm={() => removeScope(s)} ask="Remove?" title="Remove {s.value}. The consents that have it lose it.">Remove</ConfirmButton>
              {/if}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}

  {#if scopeForm}
    <form class="form" onsubmit={saveScope} aria-label={scopeForm.id ? "Edit the scope" : "Add a scope"}>
      <h4>{scopeForm.id ? "Edit the scope" : "Add a scope"}</h4>
      <label>
        Scope name
        <input type="text" class="mono" bind:value={scopeForm.value} required autocomplete="off" placeholder="Files.Read" />
        <span class="hint">The value in <code>scp</code>. No spaces, at most 120 characters.</span>
      </label>
      <label>
        Who can consent
        <select bind:value={scopeForm.type} aria-label="Who can consent">
          <option value="Admin">Admins only</option>
          <option value="User">Admins and users</option>
        </select>
      </label>
      <label>Admin consent title<input type="text" bind:value={scopeForm.adminName} required autocomplete="off" /></label>
      <label>Admin consent description<input type="text" bind:value={scopeForm.adminDesc} required autocomplete="off" /></label>
      {#if scopeForm.type === "User"}
        <label>User consent title<input type="text" bind:value={scopeForm.userName} autocomplete="off" /></label>
        <label>User consent description<input type="text" bind:value={scopeForm.userDesc} autocomplete="off" /></label>
      {/if}
      <div class="buttons">
        <button type="submit" class="btn primary" disabled={!scopeForm.value.trim() || !scopeForm.adminName.trim() || !scopeForm.adminDesc.trim()}>{scopeForm.id ? "Save" : "Add"}</button>
        <button type="button" class="btn ghost" onclick={() => (scopeForm = null)}>Cancel</button>
      </div>
    </form>
  {/if}
</section>

<section>
  <div class="head">
    <h3>Authorized client applications</h3>
    <button type="button" class="btn" disabled={!!clientForm || !scopes.length} onclick={() => (clientForm = { appId: "", ids: [], fixed: false })}>+ Client</button>
  </div>
  <p class="hint">A client that is listed here needs no consent from a user or an administrator for the scopes that are ticked: it gets them when it asks for <code>.default</code>.</p>
  {#if clients.length}
    <table class="t" aria-label="Authorized client applications">
      <thead><tr><th>Client</th><th>Scopes</th><th></th></tr></thead>
      <tbody>
        {#each clients as c (c.appId)}
          <tr>
            <td>
              <strong>{clientName(c.appId) ?? "Unknown client"}</strong>
              <div class="id muted"><code>{c.appId}</code><CopyButton text={c.appId} title="Copy the client ID" /></div>
            </td>
            <td>{#each c.delegatedPermissionIds as id (id)}<code class="chip-scope">{scopeById(id)?.value ?? id}</code>{/each}</td>
            <td class="actions">
              <button type="button" class="btn ghost" onclick={() => (clientForm = { appId: c.appId, ids: [...c.delegatedPermissionIds], fixed: true })}>Edit</button>
              <ConfirmButton onconfirm={() => removeClient(c.appId)} ask="Remove?" title="Remove {clientName(c.appId) ?? c.appId}">Remove</ConfirmButton>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {:else if api}
    <p class="none">No client is authorized.</p>
  {/if}

  {#if clientForm}
    <form class="form" onsubmit={saveClient} aria-label="Authorize a client">
      <h4>{clientForm.fixed ? "Change the scopes of the client" : "Authorize a client"}</h4>
      <label>
        Client ID
        <input type="text" class="mono" bind:value={clientForm.appId} list="clients-{app.appId}" required readonly={clientForm.fixed} autocomplete="off" placeholder="Pick a client, or type its client ID" />
        <datalist id="clients-{app.appId}">
          {#each sps as s (s.id)}<option value={s.appId}>{s.displayName}</option>{/each}
        </datalist>
      </label>
      <fieldset>
        <legend>Scopes</legend>
        {#each scopes.filter((s) => s.isEnabled || clientForm?.ids.includes(s.id)) as s (s.id)}
          <label class="switch"><input type="checkbox" value={s.id} bind:group={clientForm.ids} /> <code>{s.value}</code> <span class="muted">{s.adminConsentDisplayName}</span></label>
        {/each}
      </fieldset>
      <div class="buttons">
        <button type="submit" class="btn primary" disabled={!clientForm.appId.trim() || !clientForm.ids.length}>{clientForm.fixed ? "Save" : "Authorize"}</button>
        <button type="button" class="btn ghost" onclick={() => (clientForm = null)}>Cancel</button>
      </div>
    </form>
  {/if}
</section>

<style>
  section { margin-top: 24px; }
  section:first-of-type { margin-top: 4px; }
  h3 { margin: 0; }
  .head { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
  .hint { margin: 2px 0 10px; color: var(--muted); max-width: 72ch; }
  label > .hint { display: block; margin: 2px 0 0; font-size: 12.5px; font-weight: 400; }
  .none { color: var(--faint); }
  .uris { margin: 0 0 8px; padding: 0; list-style: none; }
  .uris li { display: flex; align-items: center; gap: 4px; min-height: 32px; }
  .inline { display: flex; gap: 8px; align-items: center; max-width: 640px; }
  .inline input { flex: 1; min-width: 0; }
  .full { display: flex; align-items: center; gap: 2px; }
  .id { display: flex; align-items: center; gap: 2px; font-size: 12px; }
  .actions { white-space: nowrap; text-align: right; }
  .chip-scope { margin-right: 6px; }
  .form { display: grid; gap: 10px; max-width: 560px; margin-top: 16px; padding: 12px; border: 1px solid var(--line); border-radius: var(--radius); background: var(--surface); }
  .form h4 { margin: 0; }
  .form label { display: grid; gap: 2px; font-weight: 500; }
  .form input[type="text"], .form select { width: 100%; }
  fieldset { display: grid; gap: 4px; margin: 0; padding: 0; border: 0; }
  legend { padding: 0; font-weight: 500; }
  fieldset .switch { display: flex; gap: 6px; align-items: baseline; font-weight: 400; }
  .buttons { display: flex; gap: 8px; }
</style>
