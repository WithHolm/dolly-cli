<script lang="ts">
  import { render } from "./markdown";

  // The text of a documentation file. render() escapes all of it, so {@html} cannot add a tag.
  let { text }: { text: string } = $props();
  const html = $derived(render(text));
</script>

<div class="md">{@html html}</div>

<style>
  .md { max-width: 80ch; }
  .md :global(p) { margin: 6px 0; }
  .md :global(ul), .md :global(ol) { margin: 6px 0; padding-left: 20px; }
  .md :global(li) { margin: 2px 0; }
  .md :global(pre) { margin: 8px 0; padding: 8px 10px; overflow-x: auto; font-size: 12.5px; background: var(--sunken); border-radius: var(--radius); }
  .md :global(pre code) { padding: 0; background: none; }
  .md :global(table) { margin: 8px 0; border-collapse: collapse; font-size: 13px; }
  .md :global(th), .md :global(td) { padding: 4px 12px 4px 0; text-align: left; vertical-align: top; border-bottom: 1px solid var(--line); }
  .md :global(th) { font-weight: 600; color: var(--muted); }
  .md :global(a) { color: var(--primary); }
  .md :global(h4), .md :global(h5), .md :global(h6) { margin: 12px 0 2px; font-size: 13px; font-weight: 600; }
  .md :global(:first-child) { margin-top: 0; }
  .md :global(:last-child) { margin-bottom: 0; }
</style>
