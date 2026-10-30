<script lang="ts">
  import type { Snippet } from "svelte";

  let {
    tone = "",
    title,
    children,
  }: { tone?: "" | "green" | "amber" | "red" | "primary"; title?: string; children: Snippet } = $props();
</script>

<span class={["chip", tone]} {title}>{@render children()}</span>
