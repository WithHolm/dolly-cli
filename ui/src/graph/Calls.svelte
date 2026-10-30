<script lang="ts">
  import { graph } from "../lib/api";
  import { succeeded } from "../lib/notice.svelte";
  import { poll } from "../lib/poll";
  import { prettyJSON, statusTone, timeOfDay } from "../lib/format";
  import Chip from "../lib/Chip.svelte";
  import CopyButton from "../lib/CopyButton.svelte";
  import Drawer from "../lib/Drawer.svelte";
  import Icon from "../lib/Icon.svelte";
  import { apiDocs } from "./docs.svelte";
  import { matchEndpoint, referenceHref } from "./reference";
  import type { Call } from "./types";

  const MAX = 500;
  const graphURL = `${location.origin}/v1.0`;

  // newest first
  let calls = $state.raw<Call[]>([]);
  let cursor = 0;
  let paused = $state(false);
  let filter = $state("");
  let errorsOnly = $state(false);
  let selected = $state.raw<Call | null>(null);

  $effect(() =>
    poll(async () => {
      if (paused) return;
      const a = await graph.get<{ calls: Call[]; cursor: number }>(`/requests?since=${cursor}`);
      cursor = a.cursor;
      if (a.calls.length) calls = [...a.calls.reverse(), ...calls].slice(0, MAX);
    }, 1000),
  );

  // the drawer links a call to its page in the API reference, so the list of operations is read when it opens
  $effect(() => {
    if (selected && !apiDocs.value) void apiDocs.load().catch(() => {});
  });

  const shown = $derived.by(() => {
    const q = filter.trim().toLowerCase();
    return calls.filter(
      (c) =>
        (!errorsOnly || c.status >= 400) &&
        (!q || `${c.method} ${c.path} ${c.query ?? ""} ${c.status} ${c.agent ?? ""}`.toLowerCase().includes(q)),
    );
  });

  async function clear() {
    if (await succeeded(() => graph.del("/requests"))) calls = [];
  }
</script>

<div class="bar">
  <label class="search">
    <Icon name="search" />
    <input type="search" placeholder="Filter by path, method, status or client" bind:value={filter} />
  </label>
  <label class="switch"><input type="checkbox" bind:checked={errorsOnly} /> Errors only</label>
  <span class="spacer"></span>
  <span class="muted count">{shown.length}{shown.length !== calls.length ? ` of ${calls.length}` : ""} calls</span>
  <Chip tone={paused ? "amber" : "green"}>{paused ? "Paused" : "Live"}</Chip>
  <button type="button" class="btn" onclick={() => (paused = !paused)} title={paused ? "Show new calls again" : "Stop the list, to read it"}>
    <Icon name={paused ? "play" : "pause"} />{paused ? "Resume" : "Pause"}
  </button>
  <button type="button" class="btn" onclick={clear} disabled={!calls.length}>Clear</button>
</div>

{#if !calls.length}
  <div class="empty">
    <strong>No calls yet</strong>
    Point your app at <code>{graphURL}</code> <CopyButton text={graphURL} title="Copy the address" />. Any token works.
  </div>
{:else if !shown.length}
  <div class="empty"><strong>No call matches the filter</strong>Clear the filter to see all {calls.length} calls.</div>
{:else}
  <table class="t fixed">
    <colgroup><col style="width: 88px" /><col style="width: 76px" /><col /><col style="width: 72px" /><col style="width: 56px" /><col style="width: 68px" /><col style="width: 200px" /></colgroup>
    <thead>
      <tr><th>Time</th><th>Method</th><th>Path</th><th>Status</th><th class="num" title="Resource units: what the call costs">RU</th><th class="num">ms</th><th>Client</th></tr>
    </thead>
    <tbody>
      {#each shown as c (c.id)}
        <tr class="click" onclick={() => (selected = c)} onkeydown={(e) => e.key === "Enter" && (selected = c)} tabindex="0">
          <td class="muted mono time">{timeOfDay(c.time)}</td>
          <td><span class={["method", c.method]}>{c.method}</span></td>
          <td class="cut">
            <div class="path">
              <span class="mono truncate" title="{c.path}{c.query ? '?' + c.query : ''}">{c.path}{#if c.query}<span class="faint">?{c.query}</span>{/if}</span>
              {#if c.batch}<Chip title="This call was a part of a $batch call">batch</Chip>{/if}
              {#if c.fault}<Chip tone="amber" title="Dolly changed the answer: {c.fault}">{c.fault}</Chip>{/if}
            </div>
          </td>
          <td><Chip tone={statusTone(c.status)}>{c.status}</Chip></td>
          <td class="num muted">{c.ru ?? ""}</td>
          <td class="num muted">{c.ms.toFixed(1)}</td>
          <td class="muted cut"><div class="truncate" title={c.agent}>{c.agent ?? ""}</div></td>
        </tr>
      {/each}
    </tbody>
  </table>
{/if}

{#if selected}
  {@const c = selected}
  <Drawer title="{c.method} {c.path}" onclose={() => (selected = null)}>
    {#snippet subtitle()}
      <Chip tone={statusTone(c.status)}>{c.status}</Chip>
      <span class="muted">{timeOfDay(c.time)} · {c.ms.toFixed(1)} ms · {c.bytes} bytes{c.ru ? ` · ${c.ru} RU` : ""}</span>
    {/snippet}
    {@const ref = apiDocs.value ? matchEndpoint(apiDocs.value.endpoints, c.method, c.path) : undefined}
    <dl>
      {#if ref && apiDocs.value}
        <dt>Reference</dt>
        <dd><a href={referenceHref(apiDocs.value.endpoints, ref)} class="mono">{ref.method} {ref.path}</a> in the API reference</dd>
      {:else if apiDocs.value}
        <dt>Reference</dt><dd class="muted">Dolly does not support this call.</dd>
      {/if}
      {#if c.query}<dt>Query</dt><dd class="mono">{c.query}</dd>{/if}
      {#if c.agent}<dt>Client</dt><dd>{c.agent}</dd>{/if}
      {#if c.consistency}<dt>ConsistencyLevel</dt><dd>{c.consistency}</dd>{/if}
      {#if c.fault}<dt>Dolly changed the answer</dt><dd>{c.fault}</dd>{/if}
    </dl>
    {#each [{ title: "Request", body: c.request }, { title: "Response", body: c.response }] as part (part.title)}
      <div class="body-head">
        <h3>{part.title}</h3>
        {#if part.body}<CopyButton text={prettyJSON(part.body)} title="Copy the {part.title.toLowerCase()}" />{/if}
      </div>
      {#if part.body}<pre>{prettyJSON(part.body)}</pre>{:else}<p class="muted">No body.</p>{/if}
    {/each}
  </Drawer>
{/if}

<style>
  .bar { display: flex; align-items: center; gap: 12px; margin-bottom: 12px; }
  .spacer { flex: 1; }
  .count { font-variant-numeric: tabular-nums; }
  .search { position: relative; display: block; width: min(360px, 100%); }
  .search :global(svg) { position: absolute; left: 8px; top: 6px; color: var(--faint); pointer-events: none; }
  .search input { width: 100%; padding-left: 28px; }

  .time { white-space: nowrap; }
  .path { display: flex; align-items: center; gap: 6px; min-width: 0; }
  .path .truncate { min-width: 0; }

  .method { font: 600 12px var(--mono); color: var(--muted); }
  .method.POST, .method.PATCH, .method.PUT { color: var(--amber); }
  .method.DELETE { color: var(--red); }

  dl { display: grid; grid-template-columns: max-content 1fr; gap: 4px 16px; margin: 12px 0 0; }
  dt { color: var(--muted); }
  dd { margin: 0; overflow-wrap: anywhere; }
  .body-head { display: flex; align-items: center; justify-content: space-between; margin: 20px 0 6px; }
  pre {
    margin: 0; padding: 12px; overflow: auto; max-height: 40vh;
    font: 12.5px/1.5 var(--mono); background: var(--sunken); border: 1px solid var(--line); border-radius: var(--radius);
  }
</style>
