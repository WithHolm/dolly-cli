<script lang="ts">
  import { graph } from "../lib/api";
  import { poll } from "../lib/poll";
  import type { State, ValueCount } from "./types";

  // not called "state": Svelte would read $state as a store of that name
  let info = $state.raw<State | null>(null);
  $effect(() =>
    poll(async () => {
      info = await graph.get<State>("/state");
    }, 5000),
  );

  const lists = $derived<[string, ValueCount[]][]>(
    info
      ? [
          ["Departments", info.departments ?? []],
          ["Job titles", info.jobTitles ?? []],
          ["Offices", info.offices ?? []],
        ]
      : [],
  );
</script>

{#if info}
  <div class="stats">
    {#each [["Users", info.users], ["Groups", info.groups], ["Service principals", info.servicePrincipals]] as [label, n] (label)}
      <div><span class="n">{n}</span><span class="muted">{label}</span></div>
    {/each}
    <div class="muted seed">Seed {info.seed}</div>
  </div>

  <div class="lists">
    {#each lists as [title, items] (title)}
      {@const max = Math.max(1, ...items.map((i) => i.count))}
      <section>
        <h3>{title} <span class="faint">{items.length}</span></h3>
        <ul>
          {#each items as v (v.value)}
            <li style:--w="{(v.count / max) * 100}%">
              <span class="truncate" title={v.value}>{v.value}</span>
              <span class="num">{v.count}</span>
            </li>
          {/each}
        </ul>
      </section>
    {/each}
  </div>
{/if}

<style>
  .stats { display: flex; flex-wrap: wrap; align-items: baseline; gap: 8px 32px; margin-bottom: 24px; }
  .stats > div { display: flex; align-items: baseline; gap: 8px; }
  .n { font-size: 24px; font-weight: 600; letter-spacing: -0.02em; font-variant-numeric: tabular-nums; }
  .seed { margin-left: auto; }
  .lists { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 24px; }
  h3 { margin-bottom: 8px; }
  ul { margin: 0; padding: 0; list-style: none; }
  li {
    display: flex; justify-content: space-between; gap: 12px; padding: 3px 8px; border-radius: 4px;
    /* the bar shows how large the count is next to the others */
    background: linear-gradient(to right, var(--primary-soft) var(--w), transparent var(--w));
  }
  li .num { color: var(--muted); }
</style>
