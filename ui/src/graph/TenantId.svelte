<script lang="ts">
  import { graph } from "../lib/api";
  import { succeeded } from "../lib/notice.svelte";
  import { poll } from "../lib/poll";
  import CopyButton from "../lib/CopyButton.svelte";

  // The ID of the tenant: it is in the tokens, and in /organization. A person can change it.
  let tenant = $state("");
  let editing = $state(false);
  let draft = $state("");

  async function load() {
    tenant = (await graph.get<{ id: string }>("/tenant")).id;
  }
  $effect(() => poll(load, 3000));

  async function save() {
    if (await succeeded(() => graph.put("/tenant", { id: draft.trim() }), "The tenant ID is changed.")) {
      editing = false;
      await load();
    }
  }
</script>

<div class="tenant">
  <span class="muted">Tenant ID</span>
  {#if editing}
    <form onsubmit={(e) => (e.preventDefault(), save())}>
      <input type="text" bind:value={draft} size="38" aria-label="Tenant ID" spellcheck="false" />
      <button type="submit" class="btn primary">Save</button>
      <button type="button" class="btn ghost" onclick={() => (editing = false)}>Cancel</button>
    </form>
  {:else}
    <code>{tenant}</code>
    <CopyButton text={tenant} title="Copy the tenant ID" />
    <button type="button" class="btn ghost" onclick={() => ((draft = tenant), (editing = true))}>Change</button>
  {/if}
</div>

<style>
  .tenant, .tenant form { display: flex; align-items: center; gap: 6px; }
</style>
