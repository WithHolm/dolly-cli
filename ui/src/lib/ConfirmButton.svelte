<script lang="ts">
  import type { Snippet } from "svelte";

  // A button that needs a second click, for a change that is hard to undo. The second click
  // must come in 4 seconds; then the button goes back.
  let {
    onconfirm,
    ask = "Sure?",
    title,
    plain = false,
    children,
  }: { onconfirm: () => void | Promise<void>; ask?: string; title?: string; plain?: boolean; children: Snippet } = $props();

  let armed = $state(false);
  let timer: ReturnType<typeof setTimeout> | undefined;

  function click(e: MouseEvent) {
    e.stopPropagation();
    if (armed) {
      clearTimeout(timer);
      armed = false;
      void onconfirm();
      return;
    }
    armed = true;
    timer = setTimeout(() => (armed = false), 4000);
  }
</script>

<button type="button" class={["btn", armed ? "danger" : plain ? "" : "ghost"]} {title} onclick={click} onblur={() => (armed = false)}>
  {#if armed}{ask}{:else}{@render children()}{/if}
</button>
