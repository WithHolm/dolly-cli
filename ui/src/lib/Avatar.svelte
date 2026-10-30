<script lang="ts">
  // The face of a person or a group: the first letters of the name, in a color that the name decides. The eye finds
  // a person faster by a face than by a column of names. A group is a rounded square, a person is a circle.
  let { name, size = 28, square = false }: { name: string; size?: number; square?: boolean } = $props();

  const initials = $derived(
    name
      .split(/[\s._-]+/)
      .filter(Boolean)
      .slice(0, 2)
      .map((p) => p[0])
      .join("")
      .toUpperCase(),
  );
  const hue = $derived([...name].reduce((h, c) => (h * 31 + c.charCodeAt(0)) % 360, 7));
</script>

<span class={["avatar", square && "square"]} style:--h={hue} style:--size="{size}px" aria-hidden="true">{initials}</span>

<style>
  .avatar {
    flex: none; display: inline-grid; place-items: center; width: var(--size); height: var(--size);
    border-radius: 50%; font-size: calc(var(--size) * 0.4); font-weight: 600; letter-spacing: 0.02em;
    background: hsl(var(--h) 65% 90%); color: hsl(var(--h) 50% 28%);
  }
  .avatar.square { border-radius: 6px; }
  @media (prefers-color-scheme: dark) {
    .avatar { background: hsl(var(--h) 35% 22%); color: hsl(var(--h) 75% 85%); }
  }
</style>
