<script lang="ts">
  import { graph } from "../lib/api";
  import { notices } from "../lib/notice.svelte";
  import Dialog from "../lib/Dialog.svelte";
  import type { Site } from "./types";

  // A new site, or the title and the description of one that is there. A site is made with the same checks as the history and Graph use (repo/sites.go): the name cannot
  // change later, because it is the end of the address.
  let {
    site,
    sites,
    groups,
    onclose,
    ondone,
  }: { site?: Site; sites: Site[]; groups: { id: string; displayName: string }[]; onclose: () => void; ondone: () => void } = $props();

  const editing = $derived(!!site);
  let kind = $state<"sharepoint" | "group">("sharepoint");
  let name = $state("");
  // The form starts from the site that it was opened for. The site does not change while the dialog is open: the list is behind it.
  // svelte-ignore state_referenced_locally
  let displayName = $state(site?.displayName ?? "");
  // svelte-ignore state_referenced_locally
  let description = $state(site?.description ?? "");
  let parentId = $state("");
  let groupId = $state("");
  let busy = $state(false);
  let error = $state("");

  // The address that the site will have, so that the person sees what a client has to know to find it.
  const parent = $derived(sites.find((s) => s.id === parentId));
  const address = $derived(kind === "group" ? "" : `${parent?.path ?? "/sites"}${parent ? "/" : "/"}${name || "name"}`);

  async function save(e: SubmitEvent) {
    e.preventDefault();
    error = "";
    busy = true;
    try {
      if (site) {
        await graph.patch(`/sites/${encodeURIComponent(site.id)}`, { displayName, description });
        notices.info(`Changed the site ${displayName}.`);
      } else {
        const body: Record<string, string> = { kind, description };
        if (displayName.trim()) body.displayName = displayName.trim();
        if (kind === "group") body.groupId = groupId;
        else {
          body.name = name.trim();
          if (parentId) body.parentId = parentId;
        }
        await graph.post("/sites", body);
        notices.info(`Made the site ${displayName.trim() || name || "of the group"}.`);
      }
      ondone();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    } finally {
      busy = false;
    }
  }
</script>

<Dialog title={editing ? "Change the site" : "New site"} {onclose}>
  <form onsubmit={save}>
    {#if !editing}
      <fieldset>
        <legend>Kind</legend>
        <label class="check"><input type="radio" bind:group={kind} value="sharepoint" /> SharePoint site</label>
        <label class="check"><input type="radio" bind:group={kind} value="group" disabled={!groups.length} /> Microsoft 365 group site</label>
      </fieldset>
      {#if kind === "group"}
        <label>
          Group
          <select bind:value={groupId} required>
            <option value="" disabled>Pick a group</option>
            {#each groups as g (g.id)}<option value={g.id}>{g.displayName}</option>{/each}
          </select>
          <span class="note">Only a Microsoft 365 group that has no site is here. The site is at the mail nickname of the group.</span>
        </label>
      {:else}
        <label>
          Name
          <input type="text" bind:value={name} required pattern="[a-z0-9][a-z0-9\-]*" maxlength="63" spellcheck="false" placeholder="projects" />
          <span class="note">The end of the address: lowercase letters, digits, and dashes. It cannot change later. The address will be <code>{address}</code>.</span>
        </label>
        <label>
          Below another site <span class="opt">(optional, makes a subsite)</span>
          <select bind:value={parentId}>
            <option value="">No: a site of its own</option>
            {#each sites.filter((s) => s.path !== "") as s (s.id)}<option value={s.id}>{s.displayName} ({s.path})</option>{/each}
          </select>
        </label>
      {/if}
    {/if}
    <label>
      Title {#if !editing}<span class="opt">(optional)</span>{/if}
      <input type="text" bind:value={displayName} required={editing} spellcheck="false" />
    </label>
    <label>
      Description <span class="opt">(optional)</span>
      <input type="text" bind:value={description} spellcheck="false" />
    </label>
    {#if error}<p class="error" role="alert">{error}</p>{/if}
    <div class="actions">
      <button type="button" class="btn ghost" onclick={onclose}>Cancel</button>
      <button type="submit" class="btn primary" class:busy disabled={busy || (!editing && kind === "group" && !groupId)}>{editing ? "Save" : "Create"}</button>
    </div>
  </form>
</Dialog>

<style>
  form { display: grid; gap: 14px; min-width: min(460px, 80vw); }
  label { display: grid; gap: 4px; font-weight: 500; }
  label.check { display: inline-flex; align-items: center; gap: 6px; font-weight: 400; margin-right: 14px; }
  fieldset { border: 0; padding: 0; margin: 0; display: flex; flex-wrap: wrap; align-items: center; gap: 2px 0; }
  legend { font-weight: 500; padding: 0; margin-bottom: 4px; }
  .note, .opt { font-weight: 400; color: var(--muted); font-size: 12px; }
  .error { margin: 0; color: var(--red); }
  .actions { display: flex; justify-content: flex-end; gap: 8px; }
</style>
