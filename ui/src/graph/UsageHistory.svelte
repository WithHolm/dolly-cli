<script lang="ts">
  import { graph } from "../lib/api";
  import { poll } from "../lib/poll";
  import UsageChart from "./UsageChart.svelte";
  import { defaultRange, lastSeconds, percentOf, rangesFor, summarize, toBars, type UsageHistory, type UsageLimit } from "./usage";

  // The use of one service principal over time, or of all of them. The limit of Graph is for one client in the tenant, so it is drawn for one
  // client only. The server keeps the history for a time that is set (--usage-history), and the ranges follow it.

  let { app }: { app?: string } = $props();

  let history = $state.raw<UsageHistory | null>(null);
  let limit = $state.raw<UsageLimit | null>(null);
  let range = $state<number | null>(null);
  let scaleToLimit = $state(false);

  $effect(() =>
    poll(async () => {
      const r = await graph.get<{ history: UsageHistory; limit?: UsageLimit }>("/usage/history" + (app ? `?app=${encodeURIComponent(app)}` : ""));
      history = r.history;
      limit = r.limit ?? null;
      range ??= defaultRange(r.history.retentionSeconds);
    }, 5000),
  );

  const ranges = $derived(history ? rangesFor(history.retentionSeconds) : []);
  const chosen = $derived(Math.min(range ?? 900, history?.retentionSeconds ?? 900));
  const points = $derived(history ? lastSeconds(history.points, history.bucketSeconds, chosen) : []);
  const bars = $derived(history ? toBars(points, history.bucketSeconds) : []);
  const shownLimit = $derived(app && limit && history && limit.seconds === history.bucketSeconds ? limit : null);
  const summary = $derived(history ? summarize(points, history.bucketSeconds, shownLimit) : null);
  const rangeLabel = $derived(ranges.find((r) => r.seconds === chosen)?.label ?? "");
  const num = (n: number) => n.toLocaleString();

  const words = $derived.by(() => {
    if (!summary || !history) return "";
    const of = summary.peakShare !== null && shownLimit ? ` (${percentOf(summary.peakShare)} of the limit of ${num(shownLimit.ru)})` : "";
    return `Peak ${num(summary.peak)} RU per ${history.bucketSeconds} s${of}. ${num(summary.ru)} RU in ${num(summary.calls)} calls, ${num(summary.throttled)} throttled, in the last ${rangeLabel}.`;
  });
</script>

{#if history && summary}
  <div class="head">
    <div class="ranges" role="radiogroup" aria-label="Range of time">
      {#each ranges as r (r.seconds)}
        <button role="radio" aria-checked={r.seconds === chosen} class:on={r.seconds === chosen} onclick={() => (range = r.seconds)}>{r.label}</button>
      {/each}
    </div>
    {#if shownLimit}
      <label class="switch small" title="Make the top of the chart the limit, so you see how far the client is from it">
        <input type="checkbox" bind:checked={scaleToLimit} /> Scale to the limit
      </label>
    {/if}
  </div>
  {#if summary.calls === 0}
    <p class="muted empty">No calls in the last {rangeLabel}.</p>
  {:else}
    <UsageChart {bars} limit={shownLimit?.ru ?? null} bucketSeconds={history.bucketSeconds} {scaleToLimit} label={words} />
    <p class="words">
      <strong>Peak {num(summary.peak)} RU per {history.bucketSeconds} s</strong>{#if summary.peakShare !== null && shownLimit}{" "}· {percentOf(summary.peakShare)} of the limit of {num(shownLimit.ru)}{/if}{" "}
      · {num(summary.ru)} RU · {num(summary.calls)} calls{#if summary.throttled}{" "}· <span class="red">▼ {num(summary.throttled)} throttled</span>{/if}
    </p>
  {/if}
  {#if !app}<p class="hint">All clients together. A limit is for one client, so it is drawn in the details of a service principal.</p>{/if}
{:else}
  <p class="muted empty">Loading the use over time…</p>
{/if}

<style>
  .head { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 8px; flex-wrap: wrap; }
  .ranges { display: inline-flex; border: 1px solid var(--line-strong); border-radius: var(--radius); overflow: hidden; height: 28px; }
  .ranges button { font: inherit; padding: 0 12px; border: 0; background: var(--surface); color: var(--muted); cursor: pointer; }
  .ranges button + button { border-left: 1px solid var(--line-strong); }
  .ranges button.on { background: var(--primary-soft); color: var(--text); font-weight: 600; }
  .small { font-size: 13px; }
  .empty { margin: 12px 0; }
  .words { margin: 8px 0 0; font-variant-numeric: tabular-nums; }
  .red { color: var(--red); }
  .hint { margin: 4px 0 0; color: var(--muted); }
</style>
