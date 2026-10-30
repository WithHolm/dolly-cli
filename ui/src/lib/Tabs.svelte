<script lang="ts">
  // A bar of tabs for a dialog. The arrow keys, Home and End move between the tabs. `prefix` makes the ids that the panel
  // points to: the tab of `x` has the id `<prefix>tab-x`, and the panel `<prefix>panel-x`.
  let {
    items,
    selected,
    onselect,
    label,
    prefix = "",
  }: {
    items: { id: string; label: string; count?: number }[];
    selected: string;
    onselect: (id: string) => void;
    label: string;
    prefix?: string;
  } = $props();

  function onkey(e: KeyboardEvent, i: number) {
    const n = items.length;
    const to = { ArrowRight: (i + 1) % n, ArrowLeft: (i + n - 1) % n, Home: 0, End: n - 1 }[e.key];
    if (to === undefined) return;
    e.preventDefault();
    onselect(items[to].id);
    queueMicrotask(() => (e.currentTarget as HTMLElement).parentElement?.querySelectorAll<HTMLElement>('[role="tab"]')[to]?.focus());
  }
</script>

<div role="tablist" aria-label={label}>
  {#each items as t, i (t.id)}
    <button
      type="button"
      role="tab"
      id="{prefix}tab-{t.id}"
      aria-selected={selected === t.id}
      aria-controls="{prefix}panel-{t.id}"
      tabindex={selected === t.id ? 0 : -1}
      class="tab"
      onclick={() => onselect(t.id)}
      onkeydown={(e) => onkey(e, i)}
    >
      {t.label}{#if t.count !== undefined}<span class="n">{t.count}</span>{/if}
    </button>
  {/each}
</div>

<style>
  [role="tablist"] { display: flex; gap: 2px; }
  .tab {
    display: flex; align-items: center; gap: 6px; margin-bottom: -1px; padding: 8px 12px; border: 0; border-bottom: 2px solid transparent;
    background: none; color: var(--muted); font: inherit; font-weight: 500; cursor: pointer;
  }
  .tab:hover { color: var(--text); }
  .tab[aria-selected="true"] { color: var(--text); border-bottom-color: var(--primary); }
  .n { font-size: 11.5px; font-variant-numeric: tabular-nums; padding: 0 6px; border-radius: 99px; background: var(--sunken); color: var(--muted); }
</style>
