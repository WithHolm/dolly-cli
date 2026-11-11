<script lang="ts">
  import { graph } from "../lib/api";
  import { succeeded } from "../lib/notice.svelte";
  import { poll } from "../lib/poll";
  import { route } from "../lib/route.svelte";
  import Chip from "../lib/Chip.svelte";
  import Icon from "../lib/Icon.svelte";
  import ObjectPicker from "./ObjectPicker.svelte";
  import { assignableRoles, roleLabel } from "./roles";
  import type { RoleAssignment, RoleAssignments, ServicePrincipal } from "./types";

  // Who has the app roles of a service principal: users, groups, and other apps. It is the tab Users and groups. When someone signs in to the app, the values of
  // the roles they have (also through a group they are a direct member of) are in the claim roles of the token. The roles are defined on the app registration.
  let { sp }: { sp: ServicePrincipal } = $props();

  type Kind = RoleAssignment["principalType"];
  const KINDS: { id: Kind; label: string }[] = [
    { id: "User", label: "User" },
    { id: "Group", label: "Group" },
    { id: "ServicePrincipal", label: "Service principal" },
  ];

  let data = $state.raw<RoleAssignments | null>(null);
  let kind = $state<Kind>("User");
  let principal = $state("");
  let roleId = $state("");
  let fresh = $state(0); // a new number makes a new picker, with nobody chosen

  async function load() {
    data = await graph.get<RoleAssignments>(`/service-principals/${sp.appId}/role-assignments`);
  }
  $effect(() => poll(load, 2000));

  const roles = $derived(data?.roles ?? []);
  const options = $derived(assignableRoles(roles, kind));
  // the chosen role must be one of the options for this kind of principal
  const chosen = $derived(options.some((r) => r.id === roleId) ? roleId : (options[0]?.id ?? ""));

  async function assign(e: SubmitEvent) {
    e.preventDefault();
    if (await succeeded(() => graph.post(`/service-principals/${sp.appId}/role-assignments`, { principalId: principal, appRoleId: chosen }), "Assigned the role.")) {
      principal = "";
      fresh++;
      await load();
    }
  }
  async function unassign(a: RoleAssignment) {
    if (await succeeded(() => graph.del(`/service-principals/${sp.appId}/role-assignments/${encodeURIComponent(a.id)}`), `Removed the role of ${a.principalDisplayName}.`)) await load();
  }
  const typeLabel = (t: Kind) => KINDS.find((k) => k.id === t)?.label ?? t;
</script>

{#if sp.builtin}
  <p class="muted">This is a built-in service principal. Its roles are the application permissions of Microsoft Graph, and they are given on the tab Permissions of the app that asks for them.</p>
{:else}
  <p class="hint">
    A user, a group, or another app that has a role of this app gets the <code>value</code> of the role in the claim <code>roles</code> when it signs in to the app.
    The members of a group have the roles of the group (a group in a group does not count).
    {#if sp.application}
      The roles are defined on <a href={route.href("graph", "app-registrations", sp.appId)}>the app registration</a>.
    {:else}
      The app registration is in another tenant, and it defines the roles there.
    {/if}
  </p>

  {#if data && !roles.length}
    <div class="empty"><strong>This app has no roles</strong>{#if sp.application}Add app roles on its app registration, and then assign them here.{:else}A role that only this tenant needs can be set on the service principal with the Graph API.{/if}</div>
  {:else if data}
    <section>
      <h3>Assigned</h3>
      {#if data.assignments.length}
        <table class="t" aria-label="Who has a role of {sp.displayName}">
          <thead><tr><th>Who</th><th>Role</th><th></th></tr></thead>
          <tbody>
            {#each data.assignments as a (a.id)}
              <tr>
                <td>
                  <strong>{a.principalDisplayName || a.principalId}</strong>
                  <Chip>{typeLabel(a.principalType)}</Chip>
                </td>
                <td>{roleLabel(roles, a.appRoleId)}</td>
                <td class="actions">
                  <button type="button" class="btn icon ghost danger" title="Take the role away from {a.principalDisplayName}" onclick={() => unassign(a)}><Icon name="trash" /></button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      {:else}
        <p class="none">Nobody has a role of this app yet.</p>
      {/if}
    </section>

    <section>
      <h3>Assign a role</h3>
      <form class="form" onsubmit={assign} aria-label="Assign a role">
        <fieldset class="kinds">
          <legend>Who</legend>
          {#each KINDS as k (k.id)}
            <label><input type="radio" name="kind" value={k.id} bind:group={kind} onchange={() => (principal = "")} /> {k.label}</label>
          {/each}
        </fieldset>
        {#key `${kind}:${fresh}`}
          <ObjectPicker kind={kind === "User" ? "user" : kind === "Group" ? "group" : "servicePrincipal"} bind:id={principal} label={typeLabel(kind)} />
        {/key}
        <label class="role">
          Role
          <select bind:value={roleId} disabled={!options.length} aria-label="Role">
            {#each options as r (r.id)}<option value={r.id}>{roleLabel(roles, r.id)}</option>{/each}
          </select>
          {#if !options.length}<span class="hint">No enabled role is for this kind of principal ({kind === "ServicePrincipal" ? "it needs Applications" : "it needs Users and groups"}).</span>{/if}
        </label>
        <div><button type="submit" class="btn primary" disabled={!principal || !chosen}>Assign</button></div>
      </form>
    </section>
  {/if}
{/if}

<style>
  .hint { margin: 2px 0 12px; color: var(--muted); max-width: 72ch; }
  .none { color: var(--faint); }
  section { margin-top: 20px; }
  section:first-of-type { margin-top: 4px; }
  .actions { text-align: right; white-space: nowrap; }
  .form { display: grid; gap: 10px; max-width: 560px; padding: 12px; border: 1px solid var(--line); border-radius: var(--radius); background: var(--surface); }
  .kinds { display: flex; gap: 16px; margin: 0; padding: 0; border: 0; }
  .kinds legend { padding: 0; margin-bottom: 4px; font-weight: 500; }
  .role { display: grid; gap: 2px; font-weight: 500; }
  .role .hint { margin: 2px 0 0; font-size: 12.5px; font-weight: 400; }
</style>
