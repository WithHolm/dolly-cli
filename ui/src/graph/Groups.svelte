<script lang="ts">
  import { poll } from "../lib/poll";
  import { route } from "../lib/route.svelte";
  import Avatar from "../lib/Avatar.svelte";
  import Chip from "../lib/Chip.svelte";
  import Icon from "../lib/Icon.svelte";
  import { listGroups } from "./directory";
  import GroupDrawer from "./GroupDrawer.svelte";
  import type { GroupInfo, GroupKind } from "./types";

  let all = $state.raw<GroupInfo[]>([]);
  let loaded = $state(false);
  let query = $state("");
  let kind = $state<"" | GroupKind>("");

  // The address says which group is open: #/graph/groups/<id>.
  const openId = $derived(route.segments[2]);

  $effect(() =>
    poll(async () => {
      all = await listGroups();
      loaded = true;
    }, 4000),
  );

  const shown = $derived.by(() => {
    const q = query.trim().toLowerCase();
    return all.filter((g) => (!kind || g.kind === kind) && (!q || g.name.toLowerCase().includes(q)));
  });

  const kinds: Record<GroupKind, string> = { assigned: "Assigned", dynamic: "Dynamic", microsoft365: "Microsoft 365" };
  const none = (n: number) => (n ? n : "");
</script>

<div class="bar">
  <label class="search">
    <Icon name="search" />
    <span class="sr-only">Find a group</span>
    <input type="search" placeholder="Find a group by name" bind:value={query} />
  </label>
  <label>
    <span class="sr-only">Kind of group</span>
    <select bind:value={kind}>
      <option value="">All kinds</option>
      <option value="assigned">Assigned</option>
      <option value="dynamic">Dynamic</option>
      <option value="microsoft365">Microsoft 365</option>
    </select>
  </label>
  {#if loaded}<span class="muted count">{shown.length}{shown.length !== all.length ? ` of ${all.length}` : ""} groups</span>{/if}
</div>

{#if loaded && !shown.length}
  <div class="empty">
    <strong>{all.length ? "No group matches" : "No groups"}</strong>
    {all.length ? "Clear the search or the kind to see all groups." : "The data has no groups."}
  </div>
{:else if shown.length}
  <table class="t fixed">
    <colgroup><col /><col style="width: 140px" /><col style="width: 80px" /><col style="width: 80px" /><col style="width: 150px" /></colgroup>
    <thead>
      <tr><th>Group</th><th>Kind</th><th class="num">Users</th><th class="num">Groups</th><th class="num" title="Service principals that are members">Service principals</th></tr>
    </thead>
    <tbody>
      {#each shown as g (g.id)}
        <tr class="click" onclick={() => route.go("graph", "groups", g.id)} onkeydown={(e) => e.key === "Enter" && route.go("graph", "groups", g.id)} tabindex="0">
          <td class="cut">
            <div class="who">
              <Avatar name={g.name} square />
              <div class="names">
                <div class="truncate name">{g.name}</div>
                {#if g.rule}<div class="truncate muted rule mono" title={g.rule}>{g.rule}</div>{/if}
              </div>
            </div>
          </td>
          <td>
            <Chip tone={g.kind === "dynamic" ? "primary" : ""}>{kinds[g.kind]}</Chip>
            {#if g.state === "Paused"}<Chip tone="amber" title="The rule is paused: the group keeps its members">Paused</Chip>{/if}
          </td>
          <td class="num">{none(g.users)}</td>
          <td class="num">{none(g.groups)}</td>
          <td class="num">{none(g.others)}</td>
        </tr>
      {/each}
    </tbody>
  </table>
{/if}

{#if openId}
  {#key openId}
    <GroupDrawer id={openId} onclose={() => route.go("graph", "groups")} />
  {/key}
{/if}

<style>
  .bar { display: flex; align-items: center; gap: 12px; margin-bottom: 12px; }
  .count { margin-left: auto; font-variant-numeric: tabular-nums; }
  .search { position: relative; display: block; width: min(360px, 100%); }
  .search :global(svg) { position: absolute; left: 8px; top: 6px; color: var(--faint); pointer-events: none; }
  .search input { width: 100%; padding-left: 28px; }
  .who { display: flex; align-items: center; gap: 10px; min-width: 0; }
  .names { min-width: 0; }
  .name { font-weight: 500; }
  .rule { font-size: 12px; }
</style>
