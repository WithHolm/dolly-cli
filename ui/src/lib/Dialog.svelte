<script lang="ts">
  import type { Snippet } from "svelte";
  import Icon from "./Icon.svelte";

  // A dialog in the middle of the page, on top of it. It is a modal <dialog>, so the browser keeps the focus in it and
  // closes it with Esc. A click outside it closes it too. Use the Drawer for a panel from the edge. The `tabs` snippet is a
  // bar of tabs under the header: it is part of the dialog, and does not scroll with the body.
  let {
    title,
    subtitle,
    tabs,
    onclose,
    children,
  }: { title: string; subtitle?: Snippet; tabs?: Snippet; onclose: () => void; children: Snippet } = $props();

  // When the page takes the dialog away (a link to another page), the close event must not call onclose: it would
  // send the address back to the page that the person just left.
  let removed = false;
  function open(dialog: HTMLDialogElement) {
    dialog.showModal();
    return () => {
      removed = true;
      if (dialog.open) dialog.close();
    };
  }
</script>

<dialog {@attach open} onclose={() => removed || onclose()} onclick={(e) => e.target === e.currentTarget && onclose()} aria-labelledby="dialog-title">
  <div class="panel" tabindex="-1" autofocus>
    <header>
      <div class="heading">
        <h2 id="dialog-title">{title}</h2>
        {#if subtitle}<div class="sub">{@render subtitle()}</div>{/if}
      </div>
      <button type="button" class="btn icon ghost" title="Close" onclick={onclose}><Icon name="x" /></button>
    </header>
    {#if tabs}<div class="tabs">{@render tabs()}</div>{/if}
    <div class="body">{@render children()}</div>
  </div>
</dialog>

<style>
  dialog {
    position: fixed; inset: 0; margin: auto;
    width: min(880px, calc(100vw - 32px)); height: min(720px, calc(100dvh - 48px)); max-width: none; max-height: none;
    padding: 0; border: 1px solid var(--line); border-radius: 10px;
    color: var(--text); background: var(--surface); box-shadow: var(--shadow-lg);
  }
  dialog::backdrop { background: rgb(0 0 0 / 0.45); }
  dialog[open] { animation: pop 0.14s ease-out; }
  @keyframes pop { from { transform: scale(0.98); opacity: 0; } }

  .panel { display: flex; flex-direction: column; height: 100%; }
  .panel:focus { outline: none; }
  header { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; padding: 16px 20px 12px; }
  .heading { min-width: 0; }
  .sub { margin-top: 4px; color: var(--muted); display: flex; flex-wrap: wrap; align-items: center; gap: 4px; }
  .tabs { padding: 0 20px; border-bottom: 1px solid var(--line); }
  .body { flex: 1; overflow-y: auto; padding: 8px 20px 24px; }
</style>
