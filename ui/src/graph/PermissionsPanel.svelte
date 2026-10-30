<script lang="ts">
  import { graph } from "../lib/api";
  import { succeeded } from "../lib/notice.svelte";
  import { groupBy } from "../lib/format";
  import { poll } from "../lib/poll";
  import Chip from "../lib/Chip.svelte";
  import CopyButton from "../lib/CopyButton.svelte";
  import Icon from "../lib/Icon.svelte";
  import PermissionPicker from "./PermissionPicker.svelte";
  import type { Grants, Kind, ServicePrincipal } from "./types";

  // The permissions of a service principal: what it has, and a picker to add more. It is the Permissions tab of the dialog.
  let { sp }: { sp: ServicePrincipal } = $props();

  let grants = $state.raw<Grants | null>(null);

  async function load() {
    grants = await graph.get<Grants>(`/service-principals/${sp.appId}/grants`);
  }
  $effect(() => poll(load, 2000));

  // what the service principal has: "kind|resource|value", for the picker
  const have = $derived(
    new Set([
      ...(grants?.application ?? []).map((g) => `application|${g.resourceAppId}|${g.value.toLowerCase()}`),
      ...(grants?.delegated ?? []).flatMap((d) => d.permissions.map((p) => `delegated|${d.resourceAppId}|${p.value.toLowerCase()}`)),
    ]),
  );

  async function grant(resourceAppId: string, kind: Kind, value: string) {
    if (await succeeded(() => graph.post(`/service-principals/${sp.appId}/grants`, { resourceAppId, kind, value }), `Added ${value}.`)) await load();
  }

  async function revoke(resourceAppId: string, kind: Kind, value: string) {
    const q = `resourceAppId=${resourceAppId}&kind=${kind}&value=${encodeURIComponent(value)}`;
    if (await succeeded(() => graph.del(`/service-principals/${sp.appId}/grants?${q}`), `Removed ${value}.`)) await load();
  }

  // the application permissions, one group for each resource
  const byResource = $derived(groupBy(grants?.application ?? [], (g) => g.resourceAppId).map(([, items]) => items));
</script>

<section>
    <h3>Delegated permissions</h3>
    <p class="hint">A user signs in. The client asks for these scopes by name, and the token has them in the claim <code>scp</code>.</p>
    {#each grants?.delegated ?? [] as d (d.grantId)}
      <div class="group">
        <div class="head">
          <strong>{d.resourceName}</strong>
          <Chip>consent for {d.consentType === "AllPrincipals" ? "all users" : (d.principalName ?? "one user")}</Chip>
          <span class="spacer"></span>
          <CopyButton text={d.scope} label="Copy all scopes" title={d.scope} />
        </div>
        <ul>
          {#each d.permissions as p (p.id)}
            <li>
              <code>{p.value}</code>
              <span class="muted truncate">{p.displayName}</span>
              <span class="actions">
                <CopyButton text={p.scope} title="Copy the scope {p.scope}" />
                <button type="button" class="btn icon ghost danger hover" title="Remove {p.value}" onclick={() => revoke(d.resourceAppId, "delegated", p.value)}><Icon name="trash" /></button>
              </span>
            </li>
          {/each}
        </ul>
      </div>
    {:else}
      {#if grants}<p class="none">None.</p>{/if}
    {/each}
  </section>

  <section>
    <h3>Application permissions</h3>
    <p class="hint">
      No user signs in. The client asks for the scope <code>.default</code> of the resource, and the token has the roles that are granted here, in
      the claim <code>roles</code>.
    </p>
    {#each byResource as items (items[0].resourceAppId)}
      <div class="group">
        <div class="head">
          <strong>{items[0].resourceName}</strong>
          <span class="muted">Scope to ask for</span> <code class="truncate">{items[0].scope}</code>
          <span class="spacer"></span>
          <CopyButton text={items[0].scope} label="Copy scope" />
        </div>
        <ul>
          {#each items as g (g.assignmentId)}
            <li>
              <code>{g.value}</code>
              <span class="muted truncate">{g.displayName}</span>
              <span class="actions">
                <button type="button" class="btn icon ghost danger hover" title="Remove {g.value}" onclick={() => revoke(g.resourceAppId, "application", g.value)}><Icon name="trash" /></button>
              </span>
            </li>
          {/each}
        </ul>
      </div>
    {:else}
      {#if grants}<p class="none">None.</p>{/if}
    {/each}
  </section>

  <section>
    <h3>Add a permission</h3>
    <PermissionPicker {have} ongrant={grant} />
  </section>

<style>
  section { margin-top: 20px; }
  section:first-child { margin-top: 4px; }
  .hint { margin: 2px 0 8px; color: var(--muted); }
  .none { color: var(--faint); }
  .group { margin-top: 8px; border: 1px solid var(--line); border-radius: var(--radius); }
  .head { display: flex; align-items: center; gap: 8px; padding: 8px 12px; background: var(--sunken); border-bottom: 1px solid var(--line); border-radius: var(--radius) var(--radius) 0 0; min-width: 0; }
  .spacer { flex: 1; }
  ul { margin: 0; padding: 0; list-style: none; }
  li { display: grid; grid-template-columns: minmax(0, auto) minmax(0, 1fr) auto; align-items: center; gap: 12px; padding: 2px 8px 2px 12px; min-height: 32px; }
  li + li { border-top: 1px solid var(--line); }
  li:hover { background: var(--sunken); }
  .actions { display: flex; align-items: center; }
  /* the action that is used less is shown when the row is looked at */
  .hover { opacity: 0; }
  li:hover .hover, li:focus-within .hover, .hover:focus-visible { opacity: 1; }
</style>
