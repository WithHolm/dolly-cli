<script lang="ts">
  import { TOTAL, type ExtensionAttribute, type Profile } from "./profile";
  import { addTo, canAdd, removeFrom } from "./shares.svelte";
  import ShareBar from "./ShareBar.svelte";
  import { wiz } from "./state.svelte";
  import WeightBar from "./WeightBar.svelte";

  const p = $derived(wiz.profile as Profile);
  const list = $derived(p.extensionAttributes ?? []);

  const indexes = Array.from({ length: 15 }, (_, k) => k + 1);
  const nextFree = () => indexes.find((i) => !list.some((a) => a.index === i)) ?? 1;
  const isPattern = (a: ExtensionAttribute) => a.pattern !== undefined;

  function setKind(a: ExtensionAttribute, pattern: boolean) {
    if (pattern === isPattern(a)) return;
    if (pattern) [a.pattern, a.values] = ["", undefined];
    else [a.pattern, a.values] = [undefined, [{ value: "", weight: TOTAL }]];
  }
</script>

<h3>Extension attributes</h3>
<p class="muted">Each user can have <code>extensionAttribute1</code> to <code>15</code>, for a cost center, a site, or a tag. An attribute takes its value from a list (the bar is the share of a value, and the shares add up to 100%) or from a pattern. A tool can change them on a user, so the profile only sets the start.</p>

<div class="attrs">
  {#each list as a, i (i)}
    <div class="attr" role="group" aria-label="Attribute row {i + 1}, extensionAttribute{a.index}">
      <div class="head">
        <label>Attribute
          <select value={a.index} aria-invalid={wiz.bad(`extensionAttributes[${i}].index`)} onchange={(e) => (a.index = Number(e.currentTarget.value))}>
            {#each indexes as n (n)}<option value={n}>extensionAttribute{n}</option>{/each}
          </select>
        </label>
        <div class="kind" role="radiogroup" aria-label="Made from">
          <button role="radio" aria-checked={!isPattern(a)} class:on={!isPattern(a)} onclick={() => setKind(a, false)}>Values</button>
          <button role="radio" aria-checked={isPattern(a)} class:on={isPattern(a)} onclick={() => setKind(a, true)}>Pattern</button>
        </div>
        <div class="fill">
          <span class="muted">Filled</span>
          <WeightBar value={a.fill ?? 100} scale={100} min={1} max={100} unit="%" label="Share of users with extensionAttribute{a.index}" onchange={(n) => (a.fill = n)} />
        </div>
        <button class="btn ghost" onclick={() => p.extensionAttributes?.splice(i, 1)} aria-label="Remove attribute row {i + 1}, extensionAttribute{a.index}">Remove</button>
      </div>
      {#if isPattern(a)}
        <label>Pattern
          <input type="text" bind:value={a.pattern} placeholder="K-&#123;1000-1099&#125;" aria-invalid={wiz.bad(`extensionAttributes[${i}].pattern`)} />
        </label>
      {:else}
        <div class="values">
          {#each a.values ?? [] as v, j (j)}
            <div class="value">
              <input type="text" placeholder="Value" aria-label="Value {j + 1} of extensionAttribute{a.index}" bind:value={v.value} aria-invalid={wiz.bad(`extensionAttributes[${i}].values`)} />
              <ShareBar bind:items={a.values!} i={j} label="Share of {v.value || 'value ' + (j + 1)}" />
              <button class="btn ghost" onclick={() => a.values && removeFrom(a.values, j)} aria-label="Remove value {v.value || j + 1}" disabled={(a.values?.length ?? 0) < 2}>Remove</button>
            </div>
          {/each}
          <div><button class="btn" onclick={() => a.values && addTo(a.values, { value: "" })} disabled={!canAdd(a.values ?? [])} title={canAdd(a.values ?? []) ? undefined : "Every share is locked. Unlock one to make room."}>+ Value</button></div>
        </div>
      {/if}
    </div>
  {:else}
    <p class="muted">This profile sets no extension attributes.</p>
  {/each}
</div>
<div><button class="btn" disabled={list.length >= 15} onclick={() => (p.extensionAttributes ??= []).push({ index: nextFree(), values: [{ value: "", weight: TOTAL }] })}>+ Attribute</button></div>

<style>
  .attrs { display: grid; gap: 10px; }
  .attr { display: grid; gap: 10px; padding: 12px; border: 1px solid var(--line); border-radius: var(--radius); background: var(--surface); }
  .head { display: flex; gap: 16px; align-items: end; flex-wrap: wrap; }
  .head label { display: grid; gap: 4px; font-weight: 500; }
  .kind { display: inline-flex; border: 1px solid var(--line-strong); border-radius: var(--radius); overflow: hidden; height: 28px; }
  .kind button { font: inherit; padding: 0 12px; border: 0; background: var(--surface); color: var(--muted); cursor: pointer; }
  .kind button + button { border-left: 1px solid var(--line-strong); }
  .kind button.on { background: var(--primary-soft); color: var(--text); font-weight: 600; }
  .fill { display: flex; align-items: center; gap: 8px; flex: 1; min-width: 220px; height: 28px; }
  .fill :global(.weight) { flex: 1; }
  label:has(input[type="text"]:not(.s)) { display: grid; gap: 4px; font-weight: 500; }
  .values { display: grid; gap: 6px; }
  .value { display: grid; grid-template-columns: minmax(160px, 260px) 1fr auto; gap: 12px; align-items: center; }
</style>
