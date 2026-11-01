<script lang="ts">
  import { PER_PERCENT, grab, niceScale, release } from "./profile";
  import { isLocked, shareTo, toggleLock } from "./shares.svelte";
  import WeightBar from "./WeightBar.svelte";

  // The bar of one item in a list of shares (the list adds up to 100%). The list is bound, so the bar can change the others.

  let {
    items = $bindable(),
    i,
    label,
    after,
  }: { items: { weight?: number }[]; i: number; label: string; after?: () => void } = $props();

  const scale = $derived(niceScale(items.map((x) => x.weight ?? 1)));
</script>

<WeightBar
  value={items[i].weight ?? 1}
  {scale}
  unit="%"
  divisor={PER_PERCENT}
  {label}
  locked={isLocked(items[i])}
  onlock={items.length > 1 ? () => toggleLock(items[i]) : undefined}
  onstart={() => grab(items)}
  onend={() => release(items)}
  onchange={(n) => {
    shareTo(items, i, n);
    after?.();
  }}
/>
