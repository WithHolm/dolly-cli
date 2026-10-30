<script lang="ts">
  import { graph } from "../lib/api";
  import { succeeded } from "../lib/notice.svelte";
  import { poll } from "../lib/poll";
  import { route } from "../lib/route.svelte";
  import Chip from "../lib/Chip.svelte";
  import CopyButton from "../lib/CopyButton.svelte";
  import Icon from "../lib/Icon.svelte";
  import ServicePrincipalDialog from "./ServicePrincipalDialog.svelte";
  import TenantId from "./TenantId.svelte";
  import type { Behavior, ServicePrincipal, Usage } from "./types";

  let list = $state.raw<ServicePrincipal[]>([]);
  let usage = $state.raw<Record<string, Usage>>({});
  let behavior = $state.raw<Behavior | null>(null);
  let loaded = $state(false);
  let table = $state<HTMLTableElement>();

  // The address says which service principal is open (its client ID), and on which tab:
  // #/graph/service-principals/<client ID>/<tab>.
  const openId = $derived(route.segments[2]);
  const openTab = $derived(route.segments[3]);
  const open = $derived(openId ? list.find((s) => s.appId.toLowerCase() === openId.toLowerCase()) : undefined);

  // A click selects a service principal. A double click, or Enter, opens the dialog.
  let selected = $state<string | null>(null);

  async function load() {
    const [l, u, b] = await Promise.all([
      graph.get<{ servicePrincipals: ServicePrincipal[] }>("/service-principals"),
      graph.get<{ servicePrincipals: Usage[] }>("/usage"),
      graph.get<Behavior>("/behavior"),
    ]);
    list = l.servicePrincipals;
    usage = Object.fromEntries(u.servicePrincipals.map((x) => [x.appId.toLowerCase(), x]));
    behavior = b;
    loaded = true;
  }
  $effect(() => poll(load, 1500));

  const setThrottle = async (throttle: boolean) => {
    if (await succeeded(() => graph.put("/behavior", { ...behavior, throttle }))) await load();
  };

  const calls = (sp: ServicePrincipal) => usage[sp.appId.toLowerCase()];
  const go = (appId: string, tab?: string) => route.go("graph", "service-principals", appId, tab && tab !== "overview" ? tab : "");
  const tabbable = (sp: ServicePrincipal, i: number) => (selected ? selected === sp.appId : i === 0);

  function onKey(e: KeyboardEvent, i: number) {
    const to = { ArrowDown: i + 1, ArrowUp: i - 1, Home: 0, End: list.length - 1 }[e.key];
    if (to !== undefined) {
      e.preventDefault();
      const next = list[Math.max(0, Math.min(to, list.length - 1))];
      selected = next.appId;
      table?.querySelector<HTMLElement>(`tr[data-id="${next.appId}"]`)?.focus();
    } else if (e.key === "Enter") {
      e.preventDefault();
      go(list[i].appId);
    }
  }
</script>

<div class="settings">
  <TenantId />
  {#if behavior}
    <label class="switch" title="When on, a client that passes the limits of Graph gets 429, as it does in Entra. The cost of each call is counted in any case.">
      <input type="checkbox" checked={behavior.throttle} onchange={(e) => setThrottle(e.currentTarget.checked)} />
      Throttle at the limits of Graph
    </label>
  {/if}
</div>

<div class="bar">
  <button type="button" class="btn" disabled={!selected} onclick={() => selected && go(selected)} title="Or double-click a service principal, or press Enter">Open details</button>
</div>

{#if loaded && !list.length}
  <div class="empty"><strong>No service principals</strong>A client that signs in gets one here.</div>
{:else if list.length}
  <table class="t" role="grid" aria-label="Service principals" bind:this={table}>
    <thead>
      <tr>
        <th>Service principal</th>
        <th title="Open: every call is allowed, and dollycli records what it needs. Enforce: only granted permissions are allowed (not active yet).">Permission mode</th>
        <th title="How the sign-in API checks the secret or the certificate of the client">Credential</th>
        <th title="How dollycli treats the calls of this client">Calls are</th>
        <th class="num">Calls</th>
        <th class="num" title="Resource units">RU</th>
        <th class="num">Throttled</th>
      </tr>
    </thead>
    <tbody>
      {#each list as sp, i (sp.id)}
        {@const u = calls(sp)}
        <tr
          data-id={sp.appId}
          aria-selected={selected === sp.appId}
          class={["click", selected === sp.appId && "selected"]}
          tabindex={tabbable(sp, i) ? 0 : -1}
          onclick={() => (selected = sp.appId)}
          ondblclick={() => go(sp.appId)}
          onkeydown={(e) => onKey(e, i)}
        >
          <td>
            <div class="name">
              <strong>{sp.displayName}</strong>
              {#if sp.builtin || sp.system}<Chip tone="primary">built in</Chip>{:else if sp.auto}<Chip>auto-created</Chip>{/if}
            </div>
            <div class="id muted"><code>{sp.appId}</code><CopyButton text={sp.appId} title="Copy the client ID" /></div>
          </td>
          <td>
            {#if sp.builtin}<span class="faint">–</span>{:else}<span class={sp.mode === "open" ? "muted" : ""}>{sp.mode === "open" ? "Open" : "Enforce"}</span>{/if}
          </td>
          <td>
            {#if sp.builtin}
              <span class="faint">–</span>
            {:else if sp.credential.state === "armed"}
              <Chip tone="amber">Waiting for a secret</Chip>
            {:else if sp.credential.state === "captured"}
              <Chip tone="primary"><Icon name="lock" size={12} /> {sp.credential.kind}</Chip>
            {:else}
              <span class="muted">Accepts any</span>
            {/if}
          </td>
          <td>
            {#if sp.exemptFromThrottle}<Chip title="The limits of Graph never throttle this client">Not throttled</Chip>{/if}
            {#if sp.hideCalls}<Chip title="Its calls are not in the list of calls">Hidden</Chip>{/if}
          </td>
          <td class={["num", !u?.calls && "faint"]}>{u?.calls ?? 0}</td>
          <td class={["num", !u?.ru && "faint"]}>{u?.ru ?? 0}</td>
          <td class={["num", u?.throttled ? "red" : "faint"]}>{u?.throttled ?? 0}</td>
        </tr>
      {/each}
    </tbody>
  </table>
  <p class="muted hint">Click to select. Double-click, or press Enter, to open.</p>
  {@const anon = usage["anonymous"]}
  {#if anon}
    <p class="muted note">
      {anon.calls} calls ({anon.ru} RU) came with no application ID in the token, so they belong to no service principal.
    </p>
  {/if}
{/if}

{#if open}
  <ServicePrincipalDialog
    sp={open}
    usage={calls(open)}
    throttle={behavior?.throttle ?? false}
    tab={openTab}
    onclose={() => ((selected = open.appId), route.go("graph", "service-principals"))}
    navigate={(tab) => go(open.appId, tab)}
    changed={load}
  />
{/if}

<style>
  .settings { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px 24px; margin-bottom: 12px; }
  .bar { display: flex; margin-bottom: 8px; }
  .name { display: flex; align-items: center; gap: 8px; }
  .id { display: flex; align-items: center; gap: 2px; margin-top: 2px; }
  .id :global(.btn.icon) { width: 22px; height: 22px; }
  .red { color: var(--red); }
  .note, .hint { margin-top: 12px; }
  .hint { font-size: 12.5px; }
  tbody tr { user-select: none; } /* a double click must not select the text */
  tbody tr:focus-visible { outline: 2px solid var(--primary); outline-offset: -2px; }
  tbody tr.selected, tbody tr.selected:hover { background: color-mix(in srgb, var(--primary) 14%, transparent); }
</style>
