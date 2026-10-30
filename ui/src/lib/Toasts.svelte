<script lang="ts">
  import Icon from "./Icon.svelte";
  import { notices } from "./notice.svelte";
</script>

<div class="toasts">
  {#each notices.items as n (n.id)}
    <div class={["toast", n.kind]} role={n.kind === "error" ? "alert" : "status"}>
      <span>{n.text}</span>
      <button type="button" class="btn icon ghost" title="Close" onclick={() => notices.dismiss(n.id)}><Icon name="x" /></button>
    </div>
  {/each}
</div>

<style>
  .toasts { position: fixed; right: 16px; bottom: 16px; z-index: 100; display: flex; flex-direction: column; gap: 8px; width: min(380px, calc(100vw - 32px)); }
  .toast {
    display: flex; align-items: center; justify-content: space-between; gap: 8px;
    padding: 8px 8px 8px 12px; border: 1px solid var(--line-strong); border-radius: var(--radius);
    background: var(--surface); box-shadow: var(--shadow-lg); animation: rise 0.15s ease-out;
  }
  .toast.error { border-color: var(--red); background: var(--red-soft); color: var(--red); }
  @keyframes rise { from { transform: translateY(8px); opacity: 0; } }
</style>
