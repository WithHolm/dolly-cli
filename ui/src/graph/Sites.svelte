<script lang="ts">
  import { graph } from "../lib/api";
  import { succeeded } from "../lib/notice.svelte";
  import { poll } from "../lib/poll";
  import Chip from "../lib/Chip.svelte";
  import ConfirmButton from "../lib/ConfirmButton.svelte";
  import SiteDialog from "./SiteDialog.svelte";
  import DriveDialog from "./DriveDialog.svelte";
  import DriveFiles from "./DriveFiles.svelte";
  import type { Site, SiteDrive, SitesAnswer } from "./types";

  // The SharePoint sites of the tenant, and the drives (document libraries) on them. Graph has no call to make a site, so a site is made here. A client reads it with
  // GET /sites, and a site of a Microsoft 365 group also with GET /groups/{id}/sites/root.
  let sites = $state.raw<Site[]>([]);
  let groups = $state.raw<{ id: string; displayName: string }[]>([]);
  let loaded = $state(false);
  // the dialog that is open: a new site, a change of a site, a new drive, or a change of a drive
  let dialog = $state<{ site?: Site; drive?: SiteDrive; on?: Site } | null>(null);
  // the files of a drive
  let files = $state<{ site: Site; drive: SiteDrive } | null>(null);
  let making = $state<"site" | "drive" | null>(null);

  async function load() {
    const r = await graph.get<SitesAnswer>("/sites");
    sites = r.sites ?? [];
    groups = r.groups ?? [];
    loaded = true;
  }
  $effect(() => poll(load, 3000));

  // The sites in the order of a tree: a site, then the sites below it.
  const rows = $derived.by(() => {
    const out: { site: Site; depth: number }[] = [];
    const walk = (parent: string, depth: number) => {
      for (const s of sites.filter((x) => x.parentId === parent)) {
        out.push({ site: s, depth });
        walk(s.id, depth + 1);
      }
    };
    walk("", 0);
    return out;
  });
  const hasSubsites = (s: Site) => sites.some((x) => x.parentId === s.id);
  const isRoot = (s: Site) => s.path === "" && !s.parentId;

  async function removeSite(s: Site) {
    if (await succeeded(() => graph.del(`/sites/${encodeURIComponent(s.id)}`), `Deleted the site ${s.displayName}.`)) await load();
  }
  async function removeDrive(d: SiteDrive) {
    if (await succeeded(() => graph.del(`/drives/${encodeURIComponent(d.id)}`), `Deleted the drive ${d.name}.`)) await load();
  }
  const done = () => {
    dialog = null;
    making = null;
    void load();
  };
</script>

<div class="top">
  <p class="hint">
    The sites of the tenant and the drives on them. A <strong>Microsoft 365 group site</strong> is found through its group (<code>GET /v1.0/groups/{"{id}"}/sites/root</code>).
    A <strong>SharePoint site</strong> has no group: a client finds it only by its link or by search (<code>GET /v1.0/sites?search=</code>).
  </p>
  <button type="button" class="btn primary" onclick={() => ((making = "site"), (dialog = {}))}>New site</button>
</div>

{#if making === "site" || (dialog && dialog.site && !dialog.drive && !dialog.on)}
  <SiteDialog site={dialog?.site} {sites} {groups} onclose={() => ((dialog = null), (making = null))} ondone={done} />
{/if}
{#if dialog && (dialog.on || dialog.drive)}
  <DriveDialog site={dialog.on!} drive={dialog.drive} onclose={() => ((dialog = null), (making = null))} ondone={done} />
{/if}

{#if files}
  <DriveFiles site={files.site} drive={files.drive} onclose={() => (files = null)} />
{/if}

{#if loaded}
  <table class="t" aria-label="Sites">
    <thead><tr><th>Site</th><th>Kind</th><th>Address</th><th>Drives</th><th></th></tr></thead>
    <tbody>
      {#each rows as { site: s, depth } (s.id)}
        <tr>
          <td style:padding-left="{12 + depth * 20}px">
            <strong>{s.displayName}</strong>
            {#if isRoot(s)}<Chip tone="primary" title="The root site of the tenant: /sites/root">root</Chip>{/if}
            {#if depth > 0}<Chip title="A subsite of the site above it">subsite</Chip>{/if}
            {#if s.description}<div class="muted desc">{s.description}</div>{/if}
          </td>
          <td>
            {#if s.kind === "group"}
              <Chip tone="green" title="The team site of a Microsoft 365 group">Microsoft 365 group</Chip>
              <div class="muted desc">{s.groupName ?? s.groupId}</div>
            {:else}
              <Chip title="A site with no group: found by its link or by search">SharePoint site</Chip>
            {/if}
          </td>
          <td class="mono url">{s.webUrl}</td>
          <td>
            {#each s.drives as d (d.id)}
              <div class="drive">
                <span title={d.description || undefined}>{d.name}</span>
                <button type="button" class="btn ghost" onclick={() => (files = { site: s, drive: d })}>Files</button>
                <button type="button" class="btn ghost" onclick={() => (dialog = { on: s, drive: d })}>Edit</button>
                {#if s.drives.length > 1}<ConfirmButton plain ask="Delete it?" onconfirm={() => removeDrive(d)}>Delete</ConfirmButton>{/if}
              </div>
            {/each}
            <button type="button" class="btn ghost" onclick={() => (dialog = { on: s })}>Add a drive</button>
          </td>
          <td class="act">
            <button type="button" class="btn ghost" onclick={() => (dialog = { site: s })}>Edit</button>
            {#if !isRoot(s) && !hasSubsites(s)}<ConfirmButton plain ask="Delete it?" onconfirm={() => removeSite(s)}>Delete</ConfirmButton>{/if}
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
{/if}

<style>
  .top { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; margin: 2px 0 12px; }
  .hint { margin: 0; color: var(--muted); max-width: 80ch; }
  .desc { font-size: 12px; }
  .url { overflow-wrap: anywhere; max-width: 42ch; }
  .drive { display: flex; align-items: center; gap: 2px; }
  .drive span { margin-right: 4px; }
  .act { text-align: right; white-space: nowrap; }
</style>
