<script lang="ts">
  import { graph } from "../lib/api";
  import { notices, succeeded } from "../lib/notice.svelte";
  import { dateTime } from "../lib/format";
  import Dialog from "../lib/Dialog.svelte";
  import ConfirmButton from "../lib/ConfirmButton.svelte";
  import type { DriveItem, Site, SiteDrive } from "./types";

  // The folders and files on a drive. A client reads them with GET /drives/{id}/root/children and the other calls of the Drives page, and changes them with the
  // same calls. Here a person makes some by hand, so that a tool has something to read: a file has a text, or a size (a file with a size reads as a sentence, again and again).
  let { site, drive, onclose }: { site: Site; drive: SiteDrive; onclose: () => void } = $props();

  let items = $state.raw<DriveItem[]>([]);
  let loaded = $state(false);
  // the form that is open: a new folder, a new file, or a change of an item (its name, and the folder that holds it)
  let form = $state<{ kind: "folder" | "file" | "edit"; item?: DriveItem } | null>(null);
  let name = $state("");
  let parentId = $state("");
  let content = $state<"text" | "size">("size");
  let text = $state("");
  let size = $state(10240);
  let busy = $state(false);
  let error = $state("");

  const base = $derived(`/drives/${encodeURIComponent(drive.id)}`);
  async function load() {
    items = (await graph.get<{ items: DriveItem[] | null }>(`${base}/items`)).items ?? [];
    loaded = true;
  }
  $effect(() => {
    void load();
  });

  // The items in the order of a tree: a folder, then what is in it. Folders come before files in each folder.
  const rows = $derived.by(() => {
    const out: { item: DriveItem; depth: number }[] = [];
    const walk = (parent: string, depth: number) => {
      const here = items.filter((i) => i.parentId === parent).sort((a, b) => Number(b.folder) - Number(a.folder) || a.name.localeCompare(b.name));
      for (const item of here) {
        out.push({ item, depth });
        if (item.folder) walk(item.id, depth + 1);
      }
    };
    walk("", 0);
    return out;
  });
  const folders = $derived(rows.filter((r) => r.item.folder));
  // a folder cannot move into itself, or into a folder in itself
  const inside = (id: string, ancestor: string): boolean => {
    for (let cur = items.find((i) => i.id === id); cur; cur = items.find((i) => i.id === cur!.parentId)) if (cur.id === ancestor) return true;
    return false;
  };
  const targets = $derived(folders.filter((f) => !form?.item || !inside(f.item.id, form.item.id)));

  const bytes = (n: number) => (n < 1024 ? `${n} B` : n < 1024 ** 2 ? `${(n / 1024).toFixed(1)} KB` : `${(n / 1024 ** 2).toFixed(1)} MB`);

  function open(kind: "folder" | "file" | "edit", item?: DriveItem, parent = "") {
    form = { kind, item };
    name = item?.name ?? "";
    parentId = item ? item.parentId : parent;
    error = "";
  }

  async function save(e: SubmitEvent) {
    e.preventDefault();
    if (!form) return;
    error = "";
    busy = true;
    try {
      if (form.kind === "edit" && form.item) {
        const body: Record<string, unknown> = {};
        if (name.trim() !== form.item.name) body.name = name.trim();
        if (parentId !== form.item.parentId) body.parentReference = { id: parentId || "root" };
        if (Object.keys(body).length) await graph.patch(`${base}/items/${encodeURIComponent(form.item.id)}`, body);
      } else {
        const body: Record<string, unknown> = { parentId, name: name.trim(), folder: form.kind === "folder" };
        if (form.kind === "file") {
          if (content === "text") body.text = text;
          else body.size = size;
        }
        await graph.post(`${base}/items`, body);
        notices.info(`Made the ${form.kind} ${name.trim()}.`);
      }
      form = null;
      await load();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    } finally {
      busy = false;
    }
  }

  async function remove(i: DriveItem) {
    if (await succeeded(() => graph.del(`${base}/items/${encodeURIComponent(i.id)}`), `Deleted ${i.name}.`)) await load();
  }
</script>

<Dialog title={drive.name} {onclose}>
  {#snippet subtitle()}on {site.displayName}{/snippet}
  <div class="box">
    <div class="bar">
      <button type="button" class="btn" onclick={() => open("folder")}>New folder</button>
      <button type="button" class="btn" onclick={() => open("file")}>New file</button>
    </div>

    {#if form}
      <form onsubmit={save}>
        <strong>{form.kind === "edit" ? `Change ${form.item?.name}` : form.kind === "folder" ? "New folder" : "New file"}</strong>
        <label>
          Name
          <input type="text" bind:value={name} required maxlength="255" spellcheck="false" placeholder={form.kind === "file" ? "report.docx" : "Reports"} />
        </label>
        <label>
          In the folder
          <select bind:value={parentId}>
            <option value="">(the root)</option>
            {#each targets as f (f.item.id)}<option value={f.item.id}>{" ".repeat(f.depth * 3)}{f.item.name}</option>{/each}
          </select>
        </label>
        {#if form.kind === "file"}
          <fieldset>
            <legend>Content</legend>
            <label class="check"><input type="radio" bind:group={content} value="size" /> A size (reads as a sentence, again and again)</label>
            <label class="check"><input type="radio" bind:group={content} value="text" /> A text</label>
          </fieldset>
          {#if content === "size"}
            <label>Size in bytes <input type="number" bind:value={size} min="0" max="4194304" required /></label>
          {:else}
            <label>Text <textarea bind:value={text} rows="4" spellcheck="false" required></textarea></label>
          {/if}
        {/if}
        {#if error}<p class="error" role="alert">{error}</p>{/if}
        <div class="actions">
          <button type="button" class="btn ghost" onclick={() => (form = null)}>Cancel</button>
          <button type="submit" class="btn primary" class:busy disabled={busy}>{form.kind === "edit" ? "Save" : "Create"}</button>
        </div>
      </form>
    {/if}

    {#if loaded && !rows.length}
      <p class="muted">Nothing here yet. A client makes folders and files with <code>POST /v1.0/drives/{"{id}"}/root/children</code> and <code>PUT …/root:/path:/content</code>.</p>
    {:else if rows.length}
      <table class="t" aria-label="Folders and files">
        <thead><tr><th>Name</th><th class="num">Size</th><th>Changed</th><th></th></tr></thead>
        <tbody>
          {#each rows as { item: i, depth } (i.id)}
            <tr>
              <td style:padding-left="{12 + depth * 20}px">
                <span class="kind" aria-hidden="true">{i.folder ? "▸" : "·"}</span>
                {#if i.folder}<strong>{i.name}</strong>{:else}<span title={i.mimeType}>{i.name}</span>{/if}
              </td>
              <td class="num nowrap">{bytes(i.size)}</td>
              <td class="muted nowrap">{dateTime(i.modified)}</td>
              <td class="act">
                {#if i.folder}
                  <button type="button" class="btn ghost" onclick={() => open("folder", undefined, i.id)}>Add folder</button>
                  <button type="button" class="btn ghost" onclick={() => open("file", undefined, i.id)}>Add file</button>
                {/if}
                <button type="button" class="btn ghost" onclick={() => open("edit", i)}>Edit</button>
                <ConfirmButton plain ask="Delete it?" onconfirm={() => remove(i)}>Delete</ConfirmButton>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </div>
</Dialog>

<style>
  .box { display: grid; gap: 14px; min-width: min(820px, 86vw); }
  .bar { display: flex; gap: 8px; }
  form { display: grid; gap: 12px; padding: 12px; background: var(--sunken); border-radius: var(--radius); }
  label { display: grid; gap: 4px; font-weight: 500; }
  label.check { display: inline-flex; align-items: center; gap: 6px; font-weight: 400; margin-right: 14px; }
  fieldset { border: 0; padding: 0; margin: 0; display: flex; flex-wrap: wrap; align-items: center; gap: 2px 0; }
  legend { font-weight: 500; padding: 0; margin-bottom: 4px; }
  .error { margin: 0; color: var(--red); }
  .actions { display: flex; justify-content: flex-end; gap: 8px; }
  .kind { display: inline-block; width: 1.2em; color: var(--muted); }
  .nowrap { white-space: nowrap; }
  .act { text-align: right; white-space: nowrap; }
  p { margin: 0; }
</style>
