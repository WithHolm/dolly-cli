<script lang="ts">
  import { graph } from "../lib/api";
  import { succeeded } from "../lib/notice.svelte";
  import { poll } from "../lib/poll";
  import { dateTime, relative } from "../lib/format";
  import Chip from "../lib/Chip.svelte";
  import ConfirmButton from "../lib/ConfirmButton.svelte";
  import NewSubscription from "./NewSubscription.svelte";
  import type { Delivery, SubscribableResource, Subscription } from "./types";

  // The clients that are told when a user or a group changes (change notifications), and what dollycli sent them. A client makes a subscription with POST /subscriptions;
  // dollycli sends a POST to its notification URL for each change, once. A failure is here, and is not tried again.
  let subscriptions = $state.raw<Subscription[]>([]);
  let deliveries = $state.raw<Delivery[]>([]);
  let resources = $state.raw<SubscribableResource[]>([]);
  let loaded = $state(false);
  let creating = $state(false);
  let now = $state(Date.now());

  async function load() {
    const r = await graph.get<{ subscriptions: Subscription[] | null; deliveries: Delivery[] | null; resources: SubscribableResource[] | null }>("/subscriptions");
    subscriptions = r.subscriptions ?? [];
    deliveries = r.deliveries ?? [];
    resources = r.resources ?? [];
    now = Date.now(); // a send that just happened must not be "in 5 seconds"
    loaded = true;
  }

  // Sends the subscription a notification as if an object changed. The data does not change.
  async function test(s: Subscription, change: string) {
    if (await succeeded(() => graph.post(`/subscriptions/${s.id}/emulate`, { changeType: change }), `Sent a test: ${change}.`)) await load();
  }
  $effect(() => poll(load, 2000));
  $effect(() => {
    const t = setInterval(() => (now = Date.now()), 30_000);
    return () => clearInterval(t);
  });

  async function remove(s: Subscription) {
    if (await succeeded(() => graph.del(`/subscriptions/${s.id}`), "Deleted the subscription.")) await load();
  }

  const ok = (d: Delivery) => d.status >= 200 && d.status < 300;
  const pretty = (body: string) => {
    try {
      return JSON.stringify(JSON.parse(body), null, 2);
    } catch {
      return body;
    }
  };
  const expires = (iso: string) => relative(new Date(iso).getTime(), now);
</script>

<div class="top">
  <p class="hint">
    Change notifications: a client subscribes to a resource, and dollycli sends it a <code>POST</code> for each change. It can subscribe to
    {#each resources as r, i (r.pattern)}{i ? ", " : ""}<code>{r.pattern}</code>{/each}. Make one here, then use <em>Send test</em> to see what the receiver gets. The data does not change.
  </p>
  <button type="button" class="btn primary" onclick={() => (creating = true)} disabled={!resources.length}>New subscription</button>
</div>

{#if creating}
  <NewSubscription {resources} onclose={() => (creating = false)} ondone={() => { creating = false; load(); }} />
{/if}

{#if loaded && !subscriptions.length}
  <div class="empty"><strong>No subscriptions</strong>Make one with the button, or a client makes one with <code>POST /v1.0/subscriptions</code>.</div>
{:else if subscriptions.length}
  <table class="t" aria-label="Subscriptions">
    <thead><tr><th>Resource</th><th>Changes</th><th>Notification URL</th><th>Expires</th><th></th></tr></thead>
    <tbody>
      {#each subscriptions as s (s.id)}
        <tr>
          <td><code>{s.resource}</code></td>
          <td class="muted">{s.changeType.split(",").join(", ")}</td>
          <td class="mono url">{s.notificationUrl}</td>
          <td title={dateTime(s.expirationDateTime)}>{expires(s.expirationDateTime)}</td>
          <td class="act">
            <span class="test" role="group" aria-label="Send a test notification">
              Send test:
              {#each s.changeType.split(",") as c (c)}
                <button type="button" class="btn ghost" onclick={() => test(s, c)}>{c}</button>
              {/each}
            </span>
            <ConfirmButton plain ask="Delete it?" onconfirm={() => remove(s)}>Delete</ConfirmButton>
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
{/if}

<h3>Sent</h3>
{#if loaded && !deliveries.length}
  <p class="none">Nothing yet. Change a user or a group while a subscription is there, and the notification is here.</p>
{:else if deliveries.length}
  <table class="t" aria-label="Notifications that were sent">
    <thead><tr><th>Time</th><th>Change</th><th>Answer</th><th class="num">ms</th><th>To</th></tr></thead>
    <tbody>
      {#each deliveries as d, i (d.time + d.subscriptionId + i)}
        <tr>
          <td class="muted" title={dateTime(d.time)}>{relative(new Date(d.time).getTime(), now)}</td>
          <td>
            <span class="muted">{d.changeType}</span> <code>{d.resource}</code>
            <details><summary>What was sent</summary><pre>{pretty(d.body)}</pre></details>
          </td>
          <td>
            {#if ok(d)}<Chip tone="green">{d.status}</Chip>{:else if d.status}<Chip tone="red" title={d.error}>{d.status}</Chip>{:else}<Chip tone="red" title={d.error}>no answer</Chip>{/if}
          </td>
          <td class="num">{d.millis.toFixed(0)}</td>
          <td class="mono url">{d.url}</td>
        </tr>
      {/each}
    </tbody>
  </table>
{/if}

<style>
  .top { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; margin: 2px 0 12px; }
  .hint { margin: 0; color: var(--muted); max-width: 80ch; }
  .test { display: inline-flex; align-items: center; gap: 2px; margin-right: 8px; color: var(--muted); font-size: 12px; }
  .none { color: var(--faint); }
  h3 { margin: 20px 0 6px; }
  .url { overflow-wrap: anywhere; max-width: 38ch; }
  .act { text-align: right; white-space: nowrap; }
  details summary { cursor: pointer; color: var(--muted); font-size: 12px; margin-top: 2px; }
  pre { margin: 4px 0 0; padding: 8px; background: var(--sunken); border-radius: var(--radius); font-size: 12px; overflow: auto; max-width: 70ch; }
</style>
