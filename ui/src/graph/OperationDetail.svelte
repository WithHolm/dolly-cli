<script lang="ts">
  import Chip from "../lib/Chip.svelte";
  import CopyButton from "../lib/CopyButton.svelte";
  import Icon from "../lib/Icon.svelte";
  import Markdown from "../lib/Markdown.svelte";
  import { route } from "../lib/route.svelte";
  import { statusTone } from "../lib/format";
  import FilterProperties from "./FilterProperties.svelte";
  import { missingOptions, nodeKey, resourceOf, type Node } from "./reference";
  import type { ApiDocs } from "./types";

  let { node, docs }: { node: Node; docs: ApiDocs } = $props();

  const e = $derived(node.main);
  const origin = location.origin;
  const url = $derived(origin + e.path);
  // $filter and $search have a part of their own below
  const options = $derived(
    (e.query ?? [])
      .filter((name) => name !== "$filter" && name !== "$search")
      .map((name) => {
        const o = docs.options.find((o) => o.name === name);
        return { name, text: o?.text ?? "", usage: o?.usage ?? [] };
      }),
  );
  const gaps = $derived(missingOptions(e));
  const hasFilter = $derived((e.query ?? []).includes("$filter"));
  const hasSearch = $derived((e.query ?? []).includes("$search"));
  const properties = $derived(e.schema ? (docs.filterProperties[e.schema] ?? []) : []);

  // a variant is a call that adds a part to the path of this one
  const suffix = (v: { path: string }) => v.path.slice(e.path.length);
  const example = (path: string) => origin + path.replace("{type}", "microsoft.graph.user");

  const curl = $derived(
    `curl -H "Authorization: Bearer any"${e.writes ? ` -X ${e.method} -H "Content-Type: application/json" -d '{}'` : ""} "${url}"`,
  );
  const link = $derived(origin + "/" + route.href("graph", "api", resourceOf(e), nodeKey(e)));

  // The text of the call from the markdown files. The parts titled Filtering and Search go next to the
  // text that the code makes for $filter and $search. The other parts have a heading of their own.
  const part = (title: string) => e.notes?.sections?.find((s) => s.title.toLowerCase() === title);
  const filterNote = $derived(part("filtering"));
  const searchNote = $derived(part("search"));
  const otherNotes = $derived((e.notes?.sections ?? []).filter((s) => !["filtering", "search"].includes(s.title.toLowerCase())));
</script>

<div class="detail">
  <div class="line">
    <code class="url">{url}</code>
    <CopyButton text={url} label="Copy URL" />
    <CopyButton text={link} label="Copy link to this page" title="A link that opens this call in the portal" />
    {#if e.doc}
      <a class="btn" href={e.doc} target="_blank" rel="noopener noreferrer" title="The page of Microsoft Learn about this call">
        <Icon name="external" /> Microsoft Learn
      </a>
    {/if}
  </div>
  <p class="summary">{e.summary}</p>
  {#if e.notes?.body}
    <div class="notes"><Markdown text={e.notes.body} /></div>
  {/if}

  {#if node.variants.length}
    <h4>Also possible</h4>
    <p class="muted">These add a part to the path. They are folded into this call.</p>
    <table class="t">
      <tbody>
        {#each node.variants as v (v.path)}
          <tr>
            <td class="mono suffix">…{suffix(v)}</td>
            <td>{v.summary}</td>
            <td class="num"><CopyButton text={example(v.path)} title="Copy an example: {example(v.path)}" /></td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}

  {#if options.length || gaps.length}
    <h4>Query options</h4>
    <table class="t">
      <tbody>
        {#each options as o (o.name)}
          <tr>
            <td class="mono name">{o.name}</td>
            <td class="muted">
              {o.text}
              {#if o.usage.length}
                <!-- the forms, like the help of a command: what to type, and what it does -->
                <div class="usage">
                  {#each o.usage as u (u.syntax)}
                    <div class="form">
                      <code class="syntax">{u.syntax}</code>
                      <span class="note">{u.note}</span>
                      <CopyButton text="{url}?{o.name}={u.example}" title="Copy a call with {o.name}={u.example}" />
                    </div>
                  {/each}
                </div>
              {/if}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
    {#if gaps.length}
      <p class="gap">
        Graph also lists {#each gaps as g, i (g)}<Chip tone="amber">{g}</Chip>{i < gaps.length - 1 ? " " : ""}{/each}
        for this call. Dolly does not answer {gaps.length === 1 ? "it" : "them"} yet.
      </p>
    {/if}
  {/if}

  {#if hasFilter}
    <h4>$filter</h4>
    <p class="muted">The forms of <code>$filter</code>. Which properties each one works on, and when it needs an advanced query, is in the table below.</p>
    <table class="t">
      <thead><tr><th>Form</th><th>Example</th><th>Note</th></tr></thead>
      <tbody>
        {#each docs.filterForms as f (f.name)}
          <tr>
            <td>
              <div>{f.name}</div>
              <code class="faint">{f.syntax}</code>
            </td>
            <td>
              <div class="example">
                <code>{f.example}</code>
                <CopyButton text="{url}?$filter={f.example}" title="Copy a call with this filter" />
              </div>
            </td>
            <td class="muted">
              {f.note}
              {#if f.needsConsistency}<Chip tone="amber" title="Send the header ConsistencyLevel: eventual">needs ConsistencyLevel</Chip>{/if}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
    {#if properties.length || (e.schema && docs.filterRules[e.schema])}
      <FilterProperties rules={e.schema ? docs.filterRules[e.schema] : undefined} {properties} schema={e.schema ?? ""} />
    {/if}
    {#if filterNote}
      <div class="callout"><span class="label">For this call</span><Markdown text={filterNote.body} /></div>
    {/if}
  {:else if filterNote}
    <h4>$filter</h4>
    <div class="callout"><span class="label">For this call</span><Markdown text={filterNote.body} /></div>
  {/if}

  {#if hasSearch}
    {@const s = docs.searchForm}
    <h4>$search</h4>
    <p>
      <code>{s.syntax}</code>
      {#if s.needsConsistency}<Chip tone="amber" title="Send the header ConsistencyLevel: eventual">needs ConsistencyLevel</Chip>{/if}
    </p>
    <p class="muted">{s.note}</p>
    <p class="example">
      <code>{s.example}</code>
      <CopyButton text="{url}?$search={s.example}" title="Copy a call with this search" />
    </p>
    {#if searchNote}
      <div class="callout"><span class="label">For this call</span><Markdown text={searchNote.body} /></div>
    {/if}
  {:else if searchNote}
    <h4>$search</h4>
    <div class="callout"><span class="label">For this call</span><Markdown text={searchNote.body} /></div>
  {/if}

  {#each otherNotes as n (n.title)}
    <h4>{n.title}</h4>
    <Markdown text={n.body} />
  {/each}

  <h4>Use</h4>
  {#if e.calls}
    <p>
      Clients called this {e.calls} {e.calls === 1 ? "time" : "times"}.
      {#if e.errors}<Chip tone="red">{e.errors} {e.errors === 1 ? "error" : "errors"}</Chip>{/if}
      {#if e.lastStatus}Last status <Chip tone={statusTone(e.lastStatus)}>{e.lastStatus}</Chip>{/if}
    </p>
  {:else}
    <p class="muted">No client has called this yet.</p>
  {/if}
  <p class="example">
    <code class="curl">{curl}</code>
    <CopyButton text={curl} label="Copy as curl" />
  </p>
</div>

<style>
  .detail :global(h4) { margin: 20px 0 4px; font-size: 13px; font-weight: 600; }
  .line { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
  .url { margin-right: 4px; overflow-wrap: anywhere; }
  .summary { margin-top: 10px; max-width: 80ch; }
  .notes { margin-top: 6px; }
  .usage { margin-top: 8px; display: grid; gap: 6px; }
  .usage .form { display: grid; grid-template-columns: minmax(0, 28rem) minmax(0, 1fr) auto; gap: 2px 16px; align-items: start; }
  .usage .syntax { color: var(--text); overflow-wrap: anywhere; }
  @media (max-width: 800px) { .usage .form { grid-template-columns: minmax(0, 1fr) auto; } .usage .note { grid-column: 1; } }
  /* what the files say about this call, next to what the code says about all calls */
  .callout { margin-top: 10px; padding: 2px 0 2px 12px; border-left: 2px solid var(--primary); }
  .callout .label { display: block; margin-bottom: 2px; font-size: 12px; font-weight: 600; color: var(--muted); }
  .detail p { margin: 4px 0; }
  .detail table { margin-top: 6px; }
  .detail .t td { padding: 5px 10px; vertical-align: top; }
  .detail .t tbody tr:hover { background: transparent; }
  /* the head of a table in the detail is not sticky, and it has the color of the row that is open */
  .detail :global(.t th) { position: static; background: transparent; }
  .suffix, .name { white-space: nowrap; color: var(--text); width: 1%; }
  .example { display: flex; align-items: center; gap: 4px; }
  .curl { overflow-wrap: anywhere; color: var(--muted); }
  .gap { margin-top: 10px; }
  a.btn { text-decoration: none; }
</style>
