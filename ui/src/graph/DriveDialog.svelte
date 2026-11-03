<script lang="ts">
  import { graph } from "../lib/api";
  import { notices } from "../lib/notice.svelte";
  import Dialog from "../lib/Dialog.svelte";
  import type { Site, SiteDrive } from "./types";

  // A new drive (a document library) on a site, or the name and the description of one that is there. The type of a drive is always documentLibrary.
  let { site, drive, onclose, ondone }: { site: Site; drive?: SiteDrive; onclose: () => void; ondone: () => void } = $props();

  // The form starts from the drive that it was opened for. The drive does not change while the dialog is open: the list is behind it.
  // svelte-ignore state_referenced_locally
  let name = $state(drive?.name ?? "");
  // svelte-ignore state_referenced_locally
  let description = $state(drive?.description ?? "");
  let busy = $state(false);
  let error = $state("");

  async function save(e: SubmitEvent) {
    e.preventDefault();
    error = "";
    busy = true;
    try {
      const body = { name: name.trim(), description };
      if (drive) await graph.patch(`/drives/${encodeURIComponent(drive.id)}`, body);
      else await graph.post(`/sites/${encodeURIComponent(site.id)}/drives`, body);
      notices.info(drive ? `Changed the drive ${name.trim()}.` : `Made the drive ${name.trim()} on ${site.displayName}.`);
      ondone();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    } finally {
      busy = false;
    }
  }
</script>

<Dialog title={drive ? "Change the drive" : "New drive"} {onclose}>
  {#snippet subtitle()}on {site.displayName}{/snippet}
  <form onsubmit={save}>
    <label>
      Name
      <input type="text" bind:value={name} required maxlength="128" spellcheck="false" placeholder="Documents" />
    </label>
    <label>
      Description <span class="opt">(optional)</span>
      <input type="text" bind:value={description} spellcheck="false" />
    </label>
    {#if error}<p class="error" role="alert">{error}</p>{/if}
    <div class="actions">
      <button type="button" class="btn ghost" onclick={onclose}>Cancel</button>
      <button type="submit" class="btn primary" class:busy disabled={busy}>{drive ? "Save" : "Create"}</button>
    </div>
  </form>
</Dialog>

<style>
  form { display: grid; gap: 14px; min-width: min(460px, 80vw); }
  label { display: grid; gap: 4px; font-weight: 500; }
  .opt { font-weight: 400; color: var(--muted); font-size: 12px; }
  .error { margin: 0; color: var(--red); }
  .actions { display: flex; justify-content: flex-end; gap: 8px; }
</style>
