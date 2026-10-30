<script lang="ts">
  import Chip from "../lib/Chip.svelte";
  import Icon from "../lib/Icon.svelte";
  import { filterOperators } from "./reference";
  import type { FilterProperty, FilterRule, Mode } from "./types";

  // What $filter and $orderby can do on each property of the objects of a call. For a directory object (user, group,
  // service principal) these are the tables of Microsoft, and Dolly applies the same rules. For another object, Graph
  // lists only the operators of each property.
  let { rules, properties, schema }: { rules?: FilterRule[]; properties: FilterProperty[]; schema: string } = $props();

  const columns: Record<string, string> = {
    eq: "eq",
    startsWith: "startsWith",
    "ge/le": "ge/le",
    eqNull: "eq null",
    count0: "$count eq 0",
    count1: "$count eq 1",
  };
  const tone = (m: Mode) => (m === "advanced" ? "amber" : m === "defaultOnly" ? "" : "green");
  const title = (m: Mode) =>
    ({
      default: "Works by default.",
      "default+advanced": "Works by default, and in an advanced query.",
      defaultOnly: "Works only without an advanced query.",
      advanced: "Needs an advanced query: the header ConsistencyLevel: eventual, and $count=true.",
      none: "Not supported.",
    })[m];

  interface Row {
    name: string;
    type?: string;
    chips: { label: string; tone: "" | "green" | "amber"; title?: string }[];
    sort?: Mode;
  }

  const rows = $derived<Row[]>(
    rules
      ? rules.map((r) => ({
          name: r.property,
          chips: Object.entries(r.operators)
            .filter(([, m]) => m !== "none")
            .map(([op, m]) => ({ label: columns[op] ?? op, tone: tone(m) as "" | "green" | "amber", title: title(m) })),
          sort: r.orderby && r.orderby !== "none" ? r.orderby : undefined,
        }))
      : properties.map((p) => ({ name: p.name, type: p.type, chips: filterOperators(p.filter).map((op) => ({ label: op, tone: "" as const })) })),
  );

  const SHOWN = 10;
  let query = $state("");
  let all = $state(false);

  const found = $derived(rows.filter((p) => p.name.toLowerCase().includes(query.trim().toLowerCase())));
  const shown = $derived(all || query ? found : found.slice(0, SHOWN));
</script>

<div class="head">
  <h5>
    {rules ? "What Graph allows" : "What Graph says"} for <code>$filter</code> on a <code>{schema}</code>
    <span class="faint">{rows.length} properties</span>
  </h5>
  <label class="search">
    <Icon name="search" />
    <span class="sr-only">Find a property</span>
    <input type="search" placeholder="Find a property" bind:value={query} />
  </label>
</div>
{#if rules}
  <p class="legend muted">
    <Chip tone="green">eq</Chip> works by default. <Chip tone="amber">ne</Chip> needs an advanced query: the header
    <code>ConsistencyLevel: eventual</code> and <code>$count=true</code>. A property that is not here does not support <code>$filter</code>.
    <code>ne</code>, <code>not</code>, and <code>in</code> follow <code>eq</code>.
  </p>
{/if}

{#if found.length}
  <table class="t">
    <thead>
      <tr><th>Property</th>{#if !rules}<th>Type</th>{/if}<th>Filter</th>{#if rules}<th>Sorts</th>{/if}</tr>
    </thead>
    <tbody>
      {#each shown as p (p.name)}
        <tr>
          <td class="mono name">{p.name}</td>
          {#if !rules}<td class="muted">{p.type}</td>{/if}
          <td>
            <div class="ops">
              {#each p.chips as c (c.label)}
                <Chip tone={c.tone} title={c.title}>{c.label}</Chip>
              {/each}
            </div>
          </td>
          {#if rules}
            <td>{#if p.sort}<Chip tone={tone(p.sort)} title={title(p.sort)}>$orderby</Chip>{/if}</td>
          {/if}
        </tr>
      {/each}
    </tbody>
  </table>
  {#if !all && !query && found.length > SHOWN}
    <button type="button" class="btn ghost" onclick={() => (all = true)}>Show all {found.length}</button>
  {/if}
{:else}
  <p class="muted none">No property is called "{query}".</p>
{/if}

<style>
  .head { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-top: 16px; }
  h5 { margin: 0; font-size: 13px; font-weight: 600; }
  .legend { margin: 4px 0 6px; }
  .search { position: relative; }
  .search :global(svg) { position: absolute; left: 8px; top: 6px; color: var(--faint); pointer-events: none; }
  .search input { padding-left: 28px; width: 200px; }
  .name { white-space: nowrap; width: 1%; }
  .ops { display: flex; flex-wrap: wrap; gap: 4px; }
  .none { margin-top: 8px; }
</style>
