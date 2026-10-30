<script lang="ts">
  import Avatar from "../lib/Avatar.svelte";
  import Chip from "../lib/Chip.svelte";
  import OrgNode from "./OrgNode.svelte";
  import type { Person } from "./types";

  // The organisation around one user, as a tree: the managers above (the head of the company at the top), the user, and the
  // people who report to the user. A person with reports can be opened to show them. A click on a name opens that person.
  let {
    chain,
    me,
    reports,
    onopen,
  }: { chain: Person[]; me: { id: string; name: string; jobTitle: string }; reports: Person[]; onopen: (id: string) => void } = $props();
</script>

{#snippet up(i: number)}
  {#if i < chain.length}
    {@const p = chain[i]}
    <li role="treeitem" aria-expanded="true" aria-selected="false">
      <div class="node">
        <button type="button" class="who" onclick={() => onopen(p.id)} title="Open {p.name}">
          <Avatar name={p.name} size={24} />
          <span class="name">{p.name}</span>
          <span class="muted title truncate">{p.jobTitle}</span>
          {#if i === 0}<Chip title="No manager: the top of the organisation">Head</Chip>{/if}
        </button>
      </div>
      <ul role="group">{@render up(i + 1)}</ul>
    </li>
  {:else}
    <li role="treeitem" aria-expanded={reports.length ? true : undefined} aria-selected="true" aria-current="true">
      <div class="node here">
        <span class="who">
          <Avatar name={me.name} size={24} />
          <span class="name">{me.name}</span>
          <span class="muted title truncate">{me.jobTitle}</span>
          <Chip tone="primary">This user</Chip>
        </span>
      </div>
      {#if reports.length}
        <ul role="group">
          {#each reports as r (r.id)}
            <OrgNode person={r} {onopen} />
          {/each}
        </ul>
      {:else}
        <p class="muted note">No direct reports.</p>
      {/if}
    </li>
  {/if}
{/snippet}

<ul class="root" role="tree" aria-label="The organisation around {me.name}">
  {@render up(0)}
</ul>
{#if chain.length === 0}
  <p class="muted note">{me.name} has no manager. This user is the head of the organisation.</p>
{/if}

<style>
  ul { margin: 0; padding: 0 0 0 20px; border-left: 1px solid var(--line); margin-left: 11px; }
  ul.root { padding: 0; border: 0; margin: 4px 0; }
  li { list-style: none; }
  .node { display: flex; align-items: center; min-height: 34px; }
  .who { display: flex; align-items: center; gap: 8px; min-width: 0; padding: 3px 8px; border: 0; border-radius: var(--radius); background: none; color: var(--text); font: inherit; text-align: left; }
  button.who { cursor: pointer; }
  button.who:hover { background: var(--sunken); }
  .node.here .who { background: color-mix(in srgb, var(--primary) 14%, transparent); }
  .name { font-weight: 500; white-space: nowrap; }
  .title { font-size: 12.5px; }
  .note { margin: 4px 0 4px 31px; }
</style>
