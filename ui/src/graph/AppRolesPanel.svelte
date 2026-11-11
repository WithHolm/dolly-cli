<script lang="ts">
  import { graph } from "../lib/api";
  import { succeeded } from "../lib/notice.svelte";
  import { poll } from "../lib/poll";
  import Chip from "../lib/Chip.svelte";
  import ConfirmButton from "../lib/ConfirmButton.svelte";
  import CopyButton from "../lib/CopyButton.svelte";
  import type { AppRegistration, AppRoleDef } from "./types";

  // The app roles that an app registration defines. A role is what a user, a group, or another app can be given in this app: the value goes in the claim
  // roles of the token. The roles are on the service principal of the app too (the tab Users and groups assigns them). `changed` tells the page to read again.
  let { app, changed }: { app: AppRegistration; changed: () => Promise<void> } = $props();

  type Form = { id?: string; displayName: string; value: string; description: string; user: boolean; application: boolean };

  let roles = $state.raw<AppRoleDef[]>([]);
  let loaded = $state(false);
  let form = $state<Form | null>(null);

  async function load() {
    roles = (await graph.get<{ roles: AppRoleDef[] }>(`/applications/${app.appId}/app-roles`)).roles;
    loaded = true;
  }
  $effect(() => poll(load, 2000));

  const base = $derived(`/applications/${app.appId}/app-roles`);
  const newRole = (): Form => ({ displayName: "", value: "", description: "", user: true, application: false });
  const editRole = (r: AppRoleDef): Form => ({
    id: r.id,
    displayName: r.displayName,
    value: r.value ?? "",
    description: r.description,
    user: r.allowedMemberTypes.includes("User"),
    application: r.allowedMemberTypes.includes("Application"),
  });

  async function after(ok: boolean) {
    if (ok) {
      await load();
      await changed();
    }
    return ok;
  }

  async function save(e: SubmitEvent) {
    e.preventDefault();
    const f = form;
    if (!f) return;
    const body = {
      displayName: f.displayName.trim(),
      value: f.value.trim() || null,
      description: f.description.trim(),
      allowedMemberTypes: [...(f.user ? ["User"] : []), ...(f.application ? ["Application"] : [])],
    };
    const done = f.id ? `Changed ${body.displayName}.` : `Added ${body.displayName}.`;
    if (await after(await succeeded(() => (f.id ? graph.patch(`${base}/${f.id}`, body) : graph.post(base, body)), done))) form = null;
  }

  const setEnabled = async (r: AppRoleDef, on: boolean) =>
    after(await succeeded(() => graph.patch(`${base}/${r.id}`, { isEnabled: on }), on ? `Enabled ${r.displayName}.` : `Disabled ${r.displayName}.`));
  const remove = async (r: AppRoleDef) => {
    await after(await succeeded(() => graph.del(`${base}/${r.id}`), `Removed ${r.displayName}.`));
  };
</script>

<p class="hint">
  An app role is what a user, a group, or another app can be given in this app. When someone signs in to the app, the <code>value</code> of each role they have is in the
  claim <code>roles</code> of the token. A role is disabled first, and then it can be removed. Assign the roles on the service principal of the app.
</p>

<div class="top"><button type="button" class="btn" disabled={!!form} onclick={() => (form = newRole())}>+ App role</button></div>

{#if loaded && !roles.length && !form}
  <div class="empty"><strong>No app roles</strong>Add one to give users, groups, or apps something to be assigned.</div>
{:else if roles.length}
  <table class="t" aria-label="App roles of {app.displayName}">
    <thead>
      <tr><th>Role</th><th>Value in the token</th><th>For</th><th>State</th><th></th></tr>
    </thead>
    <tbody>
      {#each roles as r (r.id)}
        <tr>
          <td>
            <strong>{r.displayName}</strong>
            {#if r.description}<div class="muted desc">{r.description}</div>{/if}
            <div class="id muted"><code>{r.id}</code><CopyButton text={r.id} title="Copy the role ID" /></div>
          </td>
          <td>{#if r.value}<code>{r.value}</code>{:else}<span class="faint" title="A role with no value is never in the claim roles">none</span>{/if}</td>
          <td>{r.allowedMemberTypes.map((t) => (t === "User" ? "Users and groups" : "Applications")).join(", ")}</td>
          <td>{#if r.isEnabled}<Chip tone="green">enabled</Chip>{:else}<Chip tone="amber" title="Disabled: it cannot be assigned. It can be removed.">disabled</Chip>{/if}</td>
          <td class="actions">
            <button type="button" class="btn ghost" onclick={() => (form = editRole(r))}>Edit</button>
            {#if r.isEnabled}
              <button type="button" class="btn ghost" onclick={() => setEnabled(r, false)} title="Disable the role first. Then it can be removed.">Disable</button>
            {:else}
              <button type="button" class="btn ghost" onclick={() => setEnabled(r, true)}>Enable</button>
              <ConfirmButton onconfirm={() => remove(r)} ask="Remove?" title="Remove {r.displayName}. The people and apps that have it lose it.">Remove</ConfirmButton>
            {/if}
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
{/if}

{#if form}
  <form class="form" onsubmit={save} aria-label={form.id ? "Edit the app role" : "Add an app role"}>
    <h3>{form.id ? "Edit the app role" : "Add an app role"}</h3>
    <label>Display name<input type="text" bind:value={form.displayName} required autocomplete="off" /></label>
    <label>
      Value
      <input type="text" class="mono" bind:value={form.value} autocomplete="off" placeholder="Timesheet.Approve" />
      <span class="hint">The text in the claim <code>roles</code>. No spaces, at most 120 characters. Leave it empty for a role that is never in the claim.</span>
    </label>
    <label>Description<input type="text" bind:value={form.description} autocomplete="off" /></label>
    <fieldset>
      <legend>Who can have it</legend>
      <label class="switch"><input type="checkbox" bind:checked={form.user} /> Users and groups</label>
      <label class="switch"><input type="checkbox" bind:checked={form.application} /> Applications (an application permission that another app asks for)</label>
    </fieldset>
    <div class="buttons">
      <button type="submit" class="btn primary" disabled={!form.displayName.trim() || (!form.user && !form.application)}>{form.id ? "Save" : "Add"}</button>
      <button type="button" class="btn ghost" onclick={() => (form = null)}>Cancel</button>
    </div>
  </form>
{/if}

<style>
  .hint { margin: 2px 0 12px; color: var(--muted); max-width: 72ch; }
  label > .hint { display: block; margin: 2px 0 0; font-size: 12.5px; font-weight: 400; }
  .top { display: flex; justify-content: flex-end; margin-bottom: 8px; }
  .desc { max-width: 48ch; }
  .id { display: flex; align-items: center; gap: 2px; font-size: 12px; }
  .actions { white-space: nowrap; text-align: right; }
  .form { display: grid; gap: 10px; max-width: 560px; margin-top: 20px; padding: 12px; border: 1px solid var(--line); border-radius: var(--radius); background: var(--surface); }
  .form h3 { margin: 0; }
  .form label { display: grid; gap: 2px; font-weight: 500; }
  .form input[type="text"] { width: 100%; }
  fieldset { display: grid; gap: 4px; margin: 0; padding: 0; border: 0; }
  legend { padding: 0; font-weight: 500; }
  .buttons { display: flex; gap: 8px; }
</style>
