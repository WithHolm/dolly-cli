<script lang="ts">
  import { graph } from "../lib/api";
  import { succeeded } from "../lib/notice.svelte";
  import { poll } from "../lib/poll";
  import { dateTime, relative } from "../lib/format";
  import ConfirmButton from "../lib/ConfirmButton.svelte";
  import Icon from "../lib/Icon.svelte";
  import type { HistoryEvent } from "./types";

  let events = $state.raw<HistoryEvent[]>([]);
  let total = $state(0);
  let kept = $state(true);
  let loaded = $state(false);
  let now = $state(Date.now());

  async function load() {
    const h = await graph.get<{ events: HistoryEvent[] | null; total: number; kept: boolean }>("/history?limit=200");
    events = h.events ?? [];
    total = h.total;
    kept = h.kept;
    loaded = true;
  }
  $effect(() => poll(load, 3000));
  // "3 minutes ago" stays right while the page is open
  $effect(() => {
    const t = setInterval(() => (now = Date.now()), 30_000);
    return () => clearInterval(t);
  });

  const who = (e: HistoryEvent) => (e.actor.source === "graph" ? `tool ${e.actor.agent ?? ""}`.trim() : e.actor.source === "login" ? "sign-in" : "admin");

  async function goBack(e: HistoryEvent) {
    if (await succeeded(() => graph.post("/rollback", { before: e.seq }), `The data is as it was before change ${e.seq}.`)) await load();
  }
</script>

{#if loaded && !kept}
  <div class="empty">
    <strong>This dollycli keeps no history</strong>
    Its data is only in memory. Start it with <code>--dir</code> to keep every change, with the time and who made it.
  </div>
{:else if loaded && !events.length}
  <div class="empty"><strong>No change yet</strong>The data is the seed. A change from a tool or from here shows up on this list.</div>
{:else}
  {#if total > events.length}<p class="muted note">The last {events.length} of {total} changes.</p>{/if}
  <ol>
    {#each events as e (e.seq)}
      <li>
        <span class={["mark", e.actor.source]}><Icon name={e.actor.source === "admin" ? "user" : "plug"} size={14} /></span>
        <div class="text">
          <div>{e.text}</div>
          <div class="meta muted">
            <span>#{e.seq}</span> · <time datetime={e.time} title={dateTime(e.time)}>{relative(e.time, now)}</time> · <span class="truncate">{who(e)}</span>
          </div>
        </div>
        <span class="go">
          <ConfirmButton onconfirm={() => goBack(e)} ask="Go back?" title="Take the data back to how it was before this change. The change stays in the history.">
            <Icon name="undo" /> Go back to before this
          </ConfirmButton>
        </span>
      </li>
    {/each}
  </ol>
{/if}

<style>
  .note { margin-bottom: 8px; }
  ol { margin: 0; padding: 0; list-style: none; }
  li { position: relative; display: grid; grid-template-columns: 28px minmax(0, 1fr) auto; gap: 12px; align-items: start; padding: 8px 8px 8px 0; border-radius: var(--radius); }
  li:hover { background: var(--sunken); }
  /* the line that joins the changes in time */
  li::before { content: ""; position: absolute; left: 13px; top: 0; bottom: 0; width: 1px; background: var(--line); }
  li:first-child::before { top: 16px; }
  li:last-child::before { bottom: calc(100% - 16px); }
  .mark {
    position: relative; z-index: 1; display: grid; place-items: center; width: 28px; height: 28px; margin-top: -2px;
    border-radius: 50%; color: var(--muted); background: var(--surface); border: 1px solid var(--line-strong);
  }
  .mark.graph { color: var(--primary); background: var(--primary-soft); border-color: transparent; }
  .meta { margin-top: 2px; font-size: 12.5px; display: flex; gap: 4px; min-width: 0; }
  .go { opacity: 0; }
  li:hover .go, li:focus-within .go { opacity: 1; }
</style>
