<script lang="ts">
  // A small set of line icons, drawn at 16px. They are sized to the text next to them.
  // Every shape is a path, so the icon needs no markup from a string.
  const icons = {
    copy: ["M9 11a2 2 0 0 1 2-2h7a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2h-7a2 2 0 0 1-2-2z", "M5 15V6a2 2 0 0 1 2-2h9"],
    check: ["M5 12.5l4.5 4.5L19 7.5"],
    x: ["M6 6l12 12M18 6L6 18"],
    search: ["M17.5 11a6.5 6.5 0 1 1-13 0a6.5 6.5 0 0 1 13 0z", "M16 16l4.5 4.5"],
    trash: ["M5 7h14M10 7V5h4v2M7 7l1 12h8l1-12"],
    user: ["M15.5 8a3.5 3.5 0 1 1-7 0a3.5 3.5 0 0 1 7 0z", "M5 20c1-4 4-5.5 7-5.5s6 1.5 7 5.5"],
    plug: ["M9 3v5M15 3v5M7 8h10v3a5 5 0 0 1-10 0V8zM12 16v5"],
    lock: ["M8 11h8a2 2 0 0 1 2 2v5a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2v-5a2 2 0 0 1 2-2z", "M9 11V8a3 3 0 0 1 6 0v3"],
    undo: ["M9 14L4 9l5-5", "M4 9h10a6 6 0 0 1 0 12h-3"],
    pause: ["M9 5v14M15 5v14"],
    play: ["M8 5l11 7-11 7z"],
    chevron: ["M9 6l6 6-6 6"],
    external: ["M14 5h5v5", "M19 5l-8 8", "M17 14v4a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V8a1 1 0 0 1 1-1h4"],
  } as const;

  let { name, size = 16 }: { name: keyof typeof icons; size?: number } = $props();
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width="2"
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden="true"
>
  {#each icons[name] as d (d)}
    <path {d} />
  {/each}
</svg>

<style>
  svg { flex: none; }
</style>
