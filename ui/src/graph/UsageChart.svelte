<script lang="ts">
  import { timeOfDay } from "../lib/format";
  import { yMax, type Bar } from "./usage";

  // Bars of the resource units (RU) over time. A bar is the most RU that one bucket of 10 seconds had, so it can be compared with the limit,
  // which is for 10 seconds too. A bar with throttled calls is red and has a mark at the top, so the colour is not the only sign. The numbers
  // on the chart come from the data and are formatted here: nothing from the data is put in the page as markup.

  let {
    bars,
    limit = null,
    bucketSeconds,
    scaleToLimit = false,
    label,
  }: { bars: Bar[]; limit?: number | null; bucketSeconds: number; scaleToLimit?: boolean; label: string } = $props();

  // The chart is drawn at the width it has, so the text is as big as it is made and is not scaled with the picture.
  let W = $state(640);
  const H = 168;
  const L = 52;
  const R = 10;
  const T = 12;
  const B = 22;
  const pw = $derived(Math.max(100, W - L - R));
  const ph = H - T - B;

  const peak = $derived(Math.max(0, ...bars.map((b) => b.peak)));
  const top = $derived(yMax(peak, scaleToLimit ? limit : null));
  const step = $derived(pw / Math.max(1, bars.length));
  const bw = $derived(Math.max(1, step - (bars.length < 120 ? 1 : 0.4)));
  const y = (v: number) => T + ph - (v / top) * ph;
  const limitY = $derived(limit != null && limit <= top ? y(limit) : null);
  const long = $derived(bars.length > 0 && bars.length * bucketSeconds >= 900);
  const clock = (t: number) => (long ? timeOfDay(t * 1000).slice(0, 5) : timeOfDay(t * 1000));
  const ticks = $derived(bars.length < 2 ? [] : [0, 0.25, 0.5, 0.75, 1].map((f) => Math.min(bars.length - 1, Math.round(f * (bars.length - 1)))));
  const yticks = $derived([0, top / 2, top]);
  const num = (n: number) => n.toLocaleString();

  let hover = $state<number | null>(null);
  function move(e: PointerEvent) {
    const r = (e.currentTarget as SVGElement).getBoundingClientRect();
    const x = e.clientX - r.left - L;
    hover = x < 0 || x > pw ? null : Math.min(bars.length - 1, Math.max(0, Math.floor(x / step)));
  }
  const tip = $derived(hover === null ? null : bars[hover]);
</script>

<div class="chart" bind:clientWidth={W}>
  <svg viewBox="0 0 {W} {H}" width={W} height={H} role="img" aria-label={label} onpointermove={move} onpointerleave={() => (hover = null)}>
    {#each yticks as v (v)}
      <line x1={L} x2={W - R} y1={y(v)} y2={y(v)} class="grid" />
      <text x={L - 6} y={y(v) + 4} class="axis" text-anchor="end">{num(Math.round(v))}</text>
    {/each}
    {#each bars as b, i (i)}
      <rect x={L + i * step} y={y(b.peak)} width={bw} height={Math.max(0, T + ph - y(b.peak))} class={["bar", b.throttled > 0 && "hit", hover === i && "on"]} />
      {#if b.throttled > 0}
        <path d="M{L + i * step + bw / 2 - 4} {T - 8} l4 7 l4 -7 z" class="mark" />
      {/if}
    {/each}
    {#if limitY !== null}
      <line x1={L} x2={W - R} y1={limitY} y2={limitY} class="limit" />
      <text x={L + 4} y={limitY - 4} class="limit-text">limit {num(limit ?? 0)} RU per {bucketSeconds} s</text>
    {/if}
    {#each ticks as i (i)}
      <text x={L + i * step + bw / 2} y={H - 6} class="axis" text-anchor={i === 0 ? "start" : i === bars.length - 1 ? "end" : "middle"}>{clock(bars[i].t)}</text>
    {/each}
  </svg>
  {#if tip}
    <div class="tip" style:left="{Math.min(Math.max(((L + (hover ?? 0) * step) / W) * 100, 12), 78)}%" role="presentation">
      <strong>{clock(tip.t)}{tip.seconds > bucketSeconds ? ` for ${tip.seconds} s` : ""}</strong>
      <span>Peak {num(tip.peak)} RU per {bucketSeconds} s</span>
      <span>{num(tip.ru)} RU in all, {num(tip.calls)} calls</span>
      {#if tip.throttled}<span class="red">▼ {num(tip.throttled)} throttled</span>{/if}
    </div>
  {/if}
</div>

<style>
  .chart { position: relative; }
  svg { display: block; }
  .grid { stroke: var(--line); stroke-width: 1; }
  .axis { fill: var(--muted); font-size: 11px; font-variant-numeric: tabular-nums; }
  .bar { fill: var(--primary); opacity: 0.75; }
  .bar.on { opacity: 1; }
  .bar.hit { fill: var(--red); opacity: 0.9; }
  .mark { fill: var(--red); }
  .limit { stroke: var(--amber); stroke-width: 1.5; stroke-dasharray: 5 4; }
  .limit-text { fill: var(--amber); font-size: 11px; font-weight: 600; }
  .tip { position: absolute; top: 4px; display: grid; gap: 2px; padding: 6px 8px; font-size: 12px; background: var(--surface); border: 1px solid var(--line-strong); border-radius: var(--radius); box-shadow: var(--shadow-sm); pointer-events: none; white-space: nowrap; }
  .tip .red { color: var(--red); }
</style>
