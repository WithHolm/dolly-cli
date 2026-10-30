<script lang="ts">
  import Avatar from "../lib/Avatar.svelte";
  import Icon from "../lib/Icon.svelte";
  import { directReports } from "./directory";
  import OrgNode from "./OrgNode.svelte";
  import type { Person } from "./types";

  // A person in the tree, with the people who report to them. The reports are read when the node is opened: the tree of a company can
  // be large, and the page needs only what a person looks at.
  let { person, onopen }: { person: Person; onopen: (id: string) => void } = $props();

  let open = $state(false);
  let reports = $state.raw<Person[] | null>(null);
  let failed = $state(false);

  async function toggle() {
    open = !open;
    if (open && reports === null) {
      try {
        reports = await directReports(person.id);
        failed = false;
      } catch {
        failed = true;
        open = false;
      }
    }
  }
</script>

<li role="treeitem" aria-expanded={person.reports ? open : undefined} aria-selected="false">
  <div class="node">
    {#if person.reports}
      <button type="button" class="toggle" aria-label="{open ? 'Close' : 'Open'} the {person.reports} {person.reports === 1 ? 'report' : 'reports'} of {person.name}" onclick={toggle}>
        <span class={["chevron", open && "turned"]}><Icon name="chevron" /></span>
      </button>
    {:else}
      <span class="toggle none"></span>
    {/if}
    <button type="button" class="who" onclick={() => onopen(person.id)} title="Open {person.name}">
      <Avatar name={person.name} size={24} />
      <span class="name">{person.name}</span>
      <span class="muted title truncate">{person.jobTitle}</span>
    </button>
    {#if person.reports}<span class="count muted">{person.reports} {person.reports === 1 ? "report" : "reports"}</span>{/if}
  </div>
  {#if failed}<p class="muted note">The reports could not be read.</p>{/if}
  {#if open && reports}
    <ul role="group">
      {#each reports as r (r.id)}
        <OrgNode person={r} {onopen} />
      {/each}
    </ul>
  {/if}
</li>

<style>
  li { list-style: none; }
  ul { margin: 0; padding: 0 0 0 20px; border-left: 1px solid var(--line); margin-left: 11px; }
  .node { display: flex; align-items: center; gap: 4px; min-height: 34px; }
  .toggle { flex: none; width: 22px; height: 22px; display: grid; place-items: center; padding: 0; border: 0; border-radius: 4px; background: none; color: var(--muted); cursor: pointer; }
  .toggle:hover:not(.none) { background: var(--sunken); color: var(--text); }
  .toggle.none { cursor: default; }
  .chevron { display: inline-grid; transition: transform 0.12s; }
  .chevron.turned { transform: rotate(90deg); }
  .who { display: flex; align-items: center; gap: 8px; min-width: 0; padding: 3px 6px; border: 0; border-radius: var(--radius); background: none; color: var(--text); font: inherit; cursor: pointer; text-align: left; }
  .who:hover { background: var(--sunken); }
  .name { font-weight: 500; white-space: nowrap; }
  .title { font-size: 12.5px; }
  .count { margin-left: 4px; font-size: 12px; white-space: nowrap; }
  .note { margin: 0 0 0 28px; }
</style>
