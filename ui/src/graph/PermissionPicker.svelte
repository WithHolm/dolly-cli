<script lang="ts">
  import { graph } from "../lib/api";
  import Chip from "../lib/Chip.svelte";
  import CopyButton from "../lib/CopyButton.svelte";
  import Icon from "../lib/Icon.svelte";
  import type { Kind, Permission, Resource } from "./types";

  // Pick a resource, pick the kind, search, and add. Microsoft Graph is the first resource;
  // any other service principal that has roles or scopes is one too.
  let { have, ongrant }: { have: Set<string>; ongrant: (resourceAppId: string, kind: Kind, value: string) => void } = $props();

  const LIMIT = 30;

  let resources = $state.raw<Resource[]>([]);
  let resourceId = $state("");
  let kind = $state<Kind>("delegated");
  let query = $state("");
  let found = $state.raw<{ permissions: Permission[]; total: number } | null>(null);

  $effect(() => {
    void graph.get<{ resources: Resource[] }>("/resources").then((r) => {
      resources = r.resources;
      resourceId ||= r.resources[0]?.appId ?? "";
    });
  });

  // search, a moment after the last key
  $effect(() => {
    const [res, k, q] = [resourceId, kind, query];
    if (!res) return;
    let stale = false;
    const timer = setTimeout(async () => {
      const r = await graph.get<{ permissions: Permission[]; total: number }>(
        `/resources/${res}/permissions?kind=${k}&q=${encodeURIComponent(q)}&limit=${LIMIT}`,
      );
      if (!stale) found = r;
    }, 200);
    return () => {
      stale = true;
      clearTimeout(timer);
    };
  });

  const resource = $derived(resources.find((r) => r.appId === resourceId));
</script>

<div class="controls">
  <label>
    <span class="sr-only">Resource</span>
    <select bind:value={resourceId}>
      {#each resources as r (r.appId)}
        <option value={r.appId}>{r.displayName} ({r.namespace})</option>
      {/each}
    </select>
  </label>
  <div class="seg" role="radiogroup" aria-label="Kind of permission">
    <label class={{ on: kind === "delegated" }}><input type="radio" bind:group={kind} value="delegated" />Delegated{resource ? ` · ${resource.delegated}` : ""}</label>
    <label class={{ on: kind === "application" }}><input type="radio" bind:group={kind} value="application" />Application{resource ? ` · ${resource.application}` : ""}</label>
  </div>
  <label class="search">
    <Icon name="search" />
    <span class="sr-only">Search</span>
    <input type="search" placeholder="Search, for example user.read" bind:value={query} />
  </label>
</div>

{#if found}
  {#if !found.permissions.length}
    <p class="none">No permission matches "{query}".</p>
  {:else}
    <ul>
      {#each found.permissions as p (p.id)}
        {@const added = have.has(`${kind}|${resourceId}|${p.value.toLowerCase()}`)}
        <li>
          <div class="main">
            <div><code>{p.value}</code>{#if p.admin && kind === "delegated"}&nbsp;<Chip title="Only an administrator can consent to it">admin consent</Chip>{/if}</div>
            {#if p.displayName}<div class="title">{p.displayName}</div>{/if}
            {#if p.description}<div class="desc muted">{p.description}</div>{/if}
          </div>
          <div class="actions">
            <CopyButton text={p.scope} title="Copy the scope {p.scope}" />
            <button type="button" class="btn" disabled={added} onclick={() => ongrant(resourceId, kind, p.value)}>
              {#if added}<Icon name="check" /> Added{:else}Add{/if}
            </button>
          </div>
        </li>
      {/each}
    </ul>
    <p class="muted foot">{found.permissions.length} of {found.total}.{found.total > found.permissions.length ? " Type to narrow the list." : ""}</p>
  {/if}
{/if}

<style>
  .controls { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; margin-top: 8px; }
  .controls select { max-width: 100%; }
  .seg { display: inline-flex; border: 1px solid var(--line-strong); border-radius: var(--radius); overflow: hidden; }
  .seg label { position: relative; padding: 0 10px; height: 26px; display: flex; align-items: center; font-size: 13px; color: var(--muted); cursor: pointer; }
  .seg label + label { border-left: 1px solid var(--line-strong); }
  .seg label.on { color: var(--primary); background: var(--primary-soft); font-weight: 500; }
  .seg input { position: absolute; opacity: 0; inset: 0; margin: 0; cursor: pointer; }
  .seg label:has(input:focus-visible) { outline: 2px solid var(--primary); outline-offset: -2px; }
  .search { position: relative; flex: 1; min-width: 180px; }
  .search :global(svg) { position: absolute; left: 8px; top: 6px; color: var(--faint); pointer-events: none; }
  .search input { width: 100%; padding-left: 28px; }

  ul { margin: 12px 0 0; padding: 0; list-style: none; border: 1px solid var(--line); border-radius: var(--radius); }
  li { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; padding: 8px 8px 8px 12px; }
  li + li { border-top: 1px solid var(--line); }
  .main { min-width: 0; }
  .title { margin-top: 2px; }
  .desc { display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; margin-top: 2px; font-size: 13px; }
  .actions { display: flex; align-items: center; gap: 2px; flex: none; }
  .none, .foot { margin-top: 12px; color: var(--faint); }
</style>
