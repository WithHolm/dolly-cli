<script lang="ts">
  import type { Snippet } from "svelte";
  import Icon from "./Icon.svelte";

  // A panel from the right edge, on top of the page. It is a modal <dialog>, so the browser keeps the
  // focus in it and closes it with Esc. A click outside it closes it too.
  let {
    title,
    subtitle,
    onclose,
    children,
  }: { title: string; subtitle?: Snippet; onclose: () => void; children: Snippet } = $props();

  // When the page takes the drawer away (a link to another page), the close event must not call onclose:
  // it would send the address back to the page that the person just left.
  let removed = false;
  function open(dialog: HTMLDialogElement) {
    dialog.showModal();
    return () => {
      removed = true;
      if (dialog.open) dialog.close();
    };
  }
</script>

<dialog {@attach open} onclose={() => removed || onclose()} onclick={(e) => e.target === e.currentTarget && onclose()} aria-labelledby="drawer-title">
  <!-- the panel takes the focus when it opens, so no button of it shows a focus ring that nobody asked for -->
  <div class="panel" tabindex="-1" autofocus>
    <header>
      <div class="heading">
        <h2 id="drawer-title">{title}</h2>
        {#if subtitle}<div class="sub">{@render subtitle()}</div>{/if}
      </div>
      <button type="button" class="btn icon ghost" title="Close" onclick={onclose}><Icon name="x" /></button>
    </header>
    <div class="body">{@render children()}</div>
  </div>
</dialog>

<style>
  dialog {
    position: fixed; inset: 0 0 0 auto; margin: 0;
    width: min(680px, 100vw); height: 100dvh; max-width: none; max-height: none;
    padding: 0; border: 0; border-left: 1px solid var(--line);
    color: var(--text); background: var(--surface); box-shadow: var(--shadow-lg);
  }
  dialog::backdrop { background: rgb(0 0 0 / 0.35); }
  dialog[open] { animation: slide 0.16s ease-out; }
  @keyframes slide { from { transform: translateX(24px); opacity: 0; } }

  .panel { display: flex; flex-direction: column; height: 100%; }
  .panel:focus { outline: none; }
  header { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; padding: 16px 20px 12px; border-bottom: 1px solid var(--line); }
  .heading { min-width: 0; }
  .sub { margin-top: 4px; color: var(--muted); }
  .body { flex: 1; overflow-y: auto; padding: 8px 20px 24px; }
</style>
