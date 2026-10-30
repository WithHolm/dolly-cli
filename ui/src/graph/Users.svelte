<script lang="ts">
  import { poll } from "../lib/poll";
  import { route } from "../lib/route.svelte";
  import Avatar from "../lib/Avatar.svelte";
  import Chip from "../lib/Chip.svelte";
  import Icon from "../lib/Icon.svelte";
  import { listUsers } from "./directory";
  import UserDialog from "./UserDialog.svelte";
  import type { UserInfo } from "./types";

  const STEP = 50;
  const MAX = 200; // the most that the admin API gives in one answer

  let query = $state("");
  let limit = $state(STEP);
  let result = $state.raw<{ users: UserInfo[]; total: number } | null>(null);
  let table = $state<HTMLTableElement>();

  // The address says which user is open, and on which tab: #/graph/users/<id>/<tab>. A link from another page opens the dialog.
  const openId = $derived(route.segments[2]);
  const openTab = $derived(route.segments[3]);

  // A click selects a user. A double click, or Enter, opens the dialog. The selected user stays selected after the dialog closes.
  let selected = $state<string | null>(null);

  $effect(() => {
    const [text, size] = [query, limit];
    return poll(async () => {
      const r = await listUsers(text, size);
      // an answer to an earlier text or size must not replace the answer to this one
      if (text === query && size === limit) result = r;
    }, 4000);
  });

  const users = $derived(result?.users ?? []);
  const total = $derived(result?.total ?? 0);

  const open = (id: string) => route.go("graph", "users", id);

  // Only one row is in the order of the Tab key: the selected one, or the first. The arrow keys move the selection from there.
  const tabbable = (u: UserInfo, i: number) => (selected ? selected === u.id : i === 0);

  function onKey(e: KeyboardEvent, i: number) {
    const to = { ArrowDown: i + 1, ArrowUp: i - 1, Home: 0, End: users.length - 1 }[e.key];
    if (to !== undefined) {
      e.preventDefault();
      const next = users[Math.max(0, Math.min(to, users.length - 1))];
      selected = next.id;
      table?.querySelector<HTMLElement>(`tr[data-id="${next.id}"]`)?.focus();
    } else if (e.key === "Enter") {
      e.preventDefault();
      open(users[i].id);
    }
  }
</script>

<div class="bar">
  <label class="search">
    <Icon name="search" />
    <span class="sr-only">Find a user</span>
    <input type="search" placeholder="Find by the start of a name, user principal name, department, title, or office" bind:value={query} oninput={() => (limit = STEP)} />
  </label>
  <button type="button" class="btn" disabled={!selected} onclick={() => selected && open(selected)} title="Or double-click a user, or press Enter">Open details</button>
  {#if result}<span class="muted count">{total} {total === 1 ? "user" : "users"}</span>{/if}
</div>

{#if result && !users.length}
  <div class="empty">
    <strong>{query ? `No user matches "${query}"` : "No users"}</strong>
    {query ? "Try a part of the name, or clear the search." : "The data has no users."}
  </div>
{:else if users.length}
  <table class="t fixed" role="grid" aria-label="Users" bind:this={table}>
    <colgroup><col style="width: 34%" /><col style="width: 18%" /><col style="width: 22%" /><col style="width: 16%" /><col style="width: 90px" /></colgroup>
    <thead>
      <tr><th>User</th><th>Department</th><th>Job title</th><th>Office</th><th></th></tr>
    </thead>
    <tbody>
      {#each users as u, i (u.id)}
        <tr
          data-id={u.id}
          aria-selected={selected === u.id}
          class={["click", selected === u.id && "selected", !u.accountEnabled && "disabled"]}
          tabindex={tabbable(u, i) ? 0 : -1}
          onclick={() => (selected = u.id)}
          ondblclick={() => open(u.id)}
          onkeydown={(e) => onKey(e, i)}
        >
          <td class="cut">
            <div class="who">
              <Avatar name={u.displayName} />
              <div class="names">
                <div class="truncate name">{u.displayName}</div>
                <div class="truncate muted upn">{u.userPrincipalName}</div>
              </div>
            </div>
          </td>
          <td class="cut"><div class="truncate">{u.department}</div></td>
          <td class="cut muted"><div class="truncate">{u.jobTitle}</div></td>
          <td class="cut muted"><div class="truncate">{u.officeLocation}</div></td>
          <td>
            {#if !u.accountEnabled}<Chip tone="amber" title="The account is disabled: the user cannot sign in">Disabled</Chip>{/if}
            {#if u.userType === "Guest"}<Chip>Guest</Chip>{/if}
          </td>
        </tr>
      {/each}
    </tbody>
  </table>

  <div class="more">
    {#if total > users.length && limit < MAX}
      <button type="button" class="btn" onclick={() => (limit = Math.min(limit + STEP, MAX))}>Show {Math.min(STEP, total - users.length)} more</button>
    {/if}
    {#if total > users.length}
      <span class="muted">
        Showing {users.length} of {total}.{limit >= MAX ? " Narrow the search to see the others." : ""}
      </span>
    {/if}
    <span class="muted hint">Click to select. Double-click, or press Enter, to open.</span>
  </div>
{/if}

{#if openId}
  <UserDialog
    id={openId}
    tab={openTab}
    onclose={() => ((selected = openId), route.go("graph", "users"))}
    navigate={(id, tab) => ((selected = id), route.go("graph", "users", id, tab === "properties" ? "" : tab))}
  />
{/if}

<style>
  .bar { display: flex; align-items: center; gap: 12px; margin-bottom: 12px; }
  .search { position: relative; display: block; width: min(520px, 100%); }
  .search :global(svg) { position: absolute; left: 8px; top: 6px; color: var(--faint); pointer-events: none; }
  .search input { width: 100%; padding-left: 28px; }
  .count { margin-left: auto; font-variant-numeric: tabular-nums; }

  .who { display: flex; align-items: center; gap: 10px; min-width: 0; }
  .names { min-width: 0; }
  .name { font-weight: 500; }
  .upn { font-size: 12.5px; }
  tbody tr { user-select: none; } /* a double click must not select the text */
  tbody tr:focus-visible { outline: 2px solid var(--primary); outline-offset: -2px; }
  tbody tr.selected, tbody tr.selected:hover { background: color-mix(in srgb, var(--primary) 14%, transparent); }
  tr.disabled .name, tr.disabled :global(.avatar) { opacity: 0.55; }
  .more { display: flex; align-items: center; gap: 12px; margin-top: 12px; }
  .more .hint { margin-left: auto; font-size: 12.5px; }
</style>
