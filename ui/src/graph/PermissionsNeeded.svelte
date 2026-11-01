<script lang="ts">
  import { graph } from "../lib/api";
  import { succeeded } from "../lib/notice.svelte";
  import { poll } from "../lib/poll";
  import { timeOfDay } from "../lib/format";
  import Chip from "../lib/Chip.svelte";
  import CopyButton from "../lib/CopyButton.svelte";
  import ConfirmButton from "../lib/ConfirmButton.svelte";
  import { GRAPH_APP_ID, altText, callsText, firstKind, kindLabel, learnUrl, permissionList, variantLines, verdict as verdictOf, type NeedKind, type Needs } from "./needs";
  import type { ServicePrincipal } from "./types";

  // The permissions that the calls of this client need. dollycli notes each call with the kind of token (delegated or application) and
  // looks up, in the pages of Microsoft Learn about the calls, which permissions allow it. The suggestion is a simple search over those
  // tables: it is a place to start, and not a promise that real Graph agrees. Text from the data goes in the page as text.

  let { sp }: { sp: ServicePrincipal } = $props();

  let needs = $state.raw<Needs | null>(null);
  let chosen = $state<NeedKind | null>(null);

  async function load() {
    needs = await graph.get<Needs>(`/service-principals/${sp.appId}/needs`);
    if (!chosen || !needs.kinds.some((k) => k.kind === chosen)) chosen = firstKind(needs.kinds);
  }
  $effect(() => poll(load, 3000));

  const current = $derived(needs?.kinds.find((k) => k.kind === chosen) ?? null);
  const verdict = $derived(current ? verdictOf(current) : null);
  const num = (n: number) => n.toLocaleString();

  // add what is missing, one permission at a time, with the call that the picker uses
  async function grantMissing() {
    if (!current) return;
    let added = 0;
    for (const value of current.missing) {
      const ok = await succeeded(() => graph.post(`/service-principals/${sp.appId}/grants`, { resourceAppId: GRAPH_APP_ID, kind: current.kind, value }));
      if (!ok) break;
      added++;
    }
    if (added) await succeeded(async () => {}, `Added ${added} ${added === 1 ? "permission" : "permissions"}.`);
    await load();
  }

  async function forget() {
    if (await succeeded(() => graph.del(`/service-principals/${sp.appId}/needs`), "Forgot the calls of this client.")) await load();
  }
</script>

<section>
  <h3>Needed by its calls</h3>

  {#if needs && needs.kinds.length === 0}
    <p class="none">No calls yet. Call dollycli with a token of this client.</p>
  {/if}

  {#if needs && needs.kinds.length > 0 && current}
    {#if needs.kinds.length > 1}
      <div class="kinds" role="radiogroup" aria-label="Kind of token">
        {#each needs.kinds as k (k.kind)}
          <button role="radio" aria-checked={k.kind === chosen} class:on={k.kind === chosen} onclick={() => (chosen = k.kind)}>{kindLabel(k.kind)} · {callsText(k.calls)}</button>
        {/each}
      </div>
    {/if}

    {#if current.minimum.length === 0}
      <p class="sum">No permission data for these {callsText(current.calls)}.</p>
    {:else}
      <p class="sum">
        <span class="label">Minimum</span>
        {#each current.minimum as p, i (p)}{#if i > 0}{", "}{/if}<code>{p}</code>{/each}
        <CopyButton text={permissionList(current.minimum)} title="Copy the permissions" />
      </p>
      {#each current.assumed as a (a.permission)}
        <p class="sum faint" title={a.note}>Assumed: <code>{a.permission}</code> includes {a.includes.join(", ")}. It is a guess, not from Microsoft's tables.</p>
      {/each}
      {#if verdict}
        <p class="sum">
          <Chip tone={verdict.tone}>{verdict.text}</Chip>
          {#if current.missing.length > 0}
            <ConfirmButton plain ask="Add {current.missing.length === 1 ? 'it' : 'them'}?" onconfirm={grantMissing}>Grant the missing</ConfirmButton>
          {/if}
        </p>
        {#if current.unused.length > 0}
          <p class="sum"><span class="label">Not used</span> {#each current.unused as p, i (p)}{#if i > 0}{", "}{/if}<code>{p}</code>{/each}</p>
        {/if}
      {/if}
      {#if current.unavailable.length > 0}
        <p class="sum faint">Not in dollycli's list, so it cannot grant: {current.unavailable.join(", ")}</p>
      {/if}
    {/if}

    <table class="t">
      <thead>
        <tr>
          <th>Call</th>
          <th class="num">Calls</th>
          <th title="Any one of these allows the call. A + B needs both.">Least privileged</th>
        </tr>
      </thead>
      <tbody>
        {#each current.rows as r (r.method + r.path)}
          {@const link = learnUrl(r.doc)}
          <tr>
            <td>
              <span class="method">{r.method}</span>
              {#if link}<a href={link} target="_blank" rel="noopener noreferrer" class="mono">{r.path}</a>{:else}<span class="mono">{r.path}</span>{/if}
              <span class="faint">{timeOfDay(r.last)}</span>
            </td>
            <td class="num">{num(r.calls)}{#if r.errors}<span class="err" title="{r.errors} of these answered with an error"> · {r.errors} failed</span>{/if}</td>
            <td>
              {#if r.status === "varies"}
                <details class="variants">
                  <summary>by case ({r.variants?.length ?? 0})</summary>
                  <ul>{#each variantLines(r, current.kind) as line, i (i)}<li>{line}</li>{/each}</ul>
                </details>
              {:else}
                {#each r.least ?? [] as alt, i (alt)}{#if i > 0}<span class="or"> or </span>{/if}<code>{altText(alt)}</code>{:else}<span class="faint">none</span>{/each}
              {/if}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>

    {#if needs.unknownCalls > 0}
      <p class="hint">
        {num(needs.unknownCalls)} {needs.unknownCalls === 1 ? "call came" : "calls came"} with a token that has neither <code>scp</code> nor <code>roles</code>, so dollycli cannot say which kind
        {needs.unknownCalls === 1 ? "it needs" : "they need"}. {needs.unknownCalls === 1 ? "It is" : "They are"} not in the table.
      </p>
    {/if}
    <div class="foot">
      <button type="button" class="btn ghost" onclick={forget} title="Forget the calls that dollycli noted for this client">Forget these calls</button>
    </div>
  {/if}
</section>

<style>
  section { margin-top: 4px; }
  .hint { margin: 2px 0 8px; color: var(--muted); max-width: 80ch; }
  .none { color: var(--muted); }
  .label { display: inline-block; min-width: 5.5em; color: var(--muted); }
  .kinds { display: inline-flex; border: 1px solid var(--line-strong); border-radius: var(--radius); overflow: hidden; margin: 6px 0 8px; height: 28px; }
  .kinds button { font: inherit; padding: 0 12px; border: 0; background: var(--surface); color: var(--muted); cursor: pointer; }
  .kinds button + button { border-left: 1px solid var(--line-strong); }
  .kinds button.on { background: var(--primary-soft); color: var(--text); font-weight: 600; }
  .sum { margin: 0 0 4px; line-height: 1.9; }
  .sum :global(.chip) { margin-right: 6px; }
  .sum code { margin: 0 1px; }
  table { margin-top: 4px; }
  .method { display: inline-block; min-width: 3.2em; color: var(--muted); font-size: 12px; font-weight: 600; }
  td .faint { margin-left: 8px; font-size: 12px; }
  .or { color: var(--muted); }
  .variants summary { cursor: pointer; color: var(--muted); }
  .variants ul { margin: 4px 0 0; padding-left: 16px; }
  .err { color: var(--red); }
  .foot { margin-top: 8px; }
</style>
