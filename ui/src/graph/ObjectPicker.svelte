<script lang="ts">
  import Avatar from "../lib/Avatar.svelte";
  import Icon from "../lib/Icon.svelte";
  import { listGroups, listUsers } from "./directory";

  // Finds one user or one group by a search, and gives its ID. Used where the person picks one object (a subscription to one user). The text matches as on the Users
  // and Groups pages.
  let { kind, id = $bindable(""), label }: { kind: "user" | "group"; id: string; label: string } = $props();

  type Hit = { id: string; name: string; detail: string };
  const SHOWN = 8;

  let query = $state("");
  let hits = $state.raw<Hit[]>([]);
  let total = $state(0);
  let picked = $state.raw<Hit | null>(null);
  let failed = $state("");

  // an answer to an earlier text must not replace the answer to this one
  $effect(() => {
    const text = query;
    const timer = setTimeout(async () => {
      try {
        if (kind === "user") {
          const r = await listUsers(text, SHOWN);
          if (text === query) [hits, total] = [r.users.map((u) => ({ id: u.id, name: u.displayName, detail: u.userPrincipalName })), r.total];
        } else {
          const t = text.trim().toLowerCase();
          const all = (await listGroups()).filter((g) => !t || g.name.toLowerCase().includes(t));
          if (text === query) [hits, total] = [all.slice(0, SHOWN).map((g) => ({ id: g.id, name: g.name, detail: g.kind })), all.length];
        }
        failed = "";
      } catch (e) {
        failed = e instanceof Error ? e.message : String(e);
      }
    }, 150);
    return () => clearTimeout(timer);
  });

  function pick(h: Hit) {
    picked = h;
    id = h.id;
  }
  function clear() {
    picked = null;
    id = "";
  }
</script>

<div class="picker">
  <span class="label">{label}</span>
  {#if picked}
    <div class="chosen">
      <Avatar name={picked.name} size={24} square={kind === "group"} />
      <span class="name">{picked.name}</span>
      <span class="detail">{picked.detail}</span>
      <button type="button" class="btn ghost" onclick={clear}>Change</button>
    </div>
  {:else}
    <label class="search">
      <Icon name="search" />
      <input type="search" placeholder={kind === "user" ? "Find by name, user principal name, department, title, or office" : "Find a group by name"} bind:value={query} aria-label={label} />
    </label>
    {#if failed}
      <p class="none" role="alert">{failed}</p>
    {:else if hits.length}
      <ul>
        {#each hits as h (h.id)}
          <li>
            <button type="button" onclick={() => pick(h)}>
              <Avatar name={h.name} size={24} square={kind === "group"} />
              <span class="name">{h.name}</span>
              <span class="detail">{h.detail}</span>
            </button>
          </li>
        {/each}
      </ul>
      {#if total > hits.length}<p class="more">Showing {hits.length} of {total}. Type more to narrow it.</p>{/if}
    {:else}
      <p class="none">No {kind} matches.</p>
    {/if}
  {/if}
</div>

<style>
  .picker { display: grid; gap: 4px; }
  .label { font-weight: 500; }
  .search { position: relative; display: block; }
  .search :global(svg) { position: absolute; left: 8px; top: 6px; color: var(--faint); pointer-events: none; }
  .search input { width: 100%; padding-left: 28px; }
  ul { margin: 0; padding: 0; list-style: none; border: 1px solid var(--line); border-radius: var(--radius); max-height: 240px; overflow-y: auto; }
  li + li { border-top: 1px solid var(--line); }
  li button { display: flex; align-items: center; gap: 8px; width: 100%; padding: 6px 10px; border: 0; background: none; font: inherit; color: inherit; text-align: left; cursor: pointer; }
  li button:hover, li button:focus-visible { background: var(--sunken); }
  .chosen { display: flex; align-items: center; gap: 8px; padding: 4px 4px 4px 8px; border: 1px solid var(--line); border-radius: var(--radius); }
  .name { font-weight: 500; }
  .detail { color: var(--muted); font-weight: 400; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; flex: 1; }
  .chosen .btn { margin-left: auto; }
  .more, .none { margin: 2px 0 0; color: var(--muted); font-size: 12px; font-weight: 400; }
</style>
