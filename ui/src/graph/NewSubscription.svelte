<script lang="ts">
  import { graph } from "../lib/api";
  import { notices } from "../lib/notice.svelte";
  import Dialog from "../lib/Dialog.svelte";
  import ObjectPicker from "./ObjectPicker.svelte";
  import type { SubscribableResource } from "./types";

  // A subscription made from the portal: the same call that a client makes (POST /subscriptions), with the same checks. dollycli sends the validation request to the URL
  // first, so the receiver must be running.
  let { resources, ondone, onclose }: { resources: SubscribableResource[]; ondone: () => void; onclose: () => void } = $props();

  const CHANGES = ["created", "updated", "deleted"];
  const LIFETIMES = [
    { label: "1 hour", minutes: 60 },
    { label: "1 day", minutes: 24 * 60 },
    { label: "7 days", minutes: 7 * 24 * 60 },
    { label: "28 days", minutes: 28 * 24 * 60 },
  ];
  const kinds: Record<string, string> = { user: "user", group: "group" };

  let picked = $state("");
  const pattern = $derived(picked || resources[0]?.pattern || "");
  let objectId = $state("");
  let changes = $state<string[]>([...CHANGES]);
  // Where the notifications go: to dollycli itself (it answers the validation request and takes each notification with 202, so no receiver has to run), or to a URL.
  // The address is the one that this page was opened at, which the server reaches too.
  const OWN_URL = `${location.origin}/dolly-receiver/notifications`;
  let target = $state<"dolly" | "custom">("dolly");
  let url = $state("");
  let minutes = $state(24 * 60);
  let clientState = $state("");
  let busy = $state(false);
  let error = $state("");

  const chosen = $derived(resources.find((r) => r.pattern === pattern));
  const needsId = $derived(pattern.includes("{id}"));
  const kind = $derived(kinds[chosen?.kind ?? ""] ?? "object");
  // a lifetime that the resource refuses is not offered
  const lifetimes = $derived(LIFETIMES.filter((l) => !chosen || l.minutes <= chosen.maxMinutes));
  const label = (p: string) => {
    const k = kinds[resources.find((r) => r.pattern === p)?.kind ?? ""] ?? "object";
    return p.includes("{id}") ? `${p}: one ${k}` : `${p}: every ${k}`;
  };

  async function create(e: SubmitEvent) {
    e.preventDefault();
    error = "";
    busy = true;
    try {
      const body: Record<string, string> = {
        resource: needsId ? pattern.replace("{id}", objectId.trim()) : pattern,
        changeType: CHANGES.filter((c) => changes.includes(c)).join(","),
        notificationUrl: target === "dolly" ? OWN_URL : url.trim(),
        expirationDateTime: new Date(Date.now() + minutes * 60_000).toISOString(),
      };
      if (clientState.trim()) body.clientState = clientState.trim();
      await graph.post("/subscriptions", body);
      notices.info("Made the subscription.");
      ondone();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    } finally {
      busy = false;
    }
  }
</script>

<Dialog title="New subscription" {onclose}>
  <form onsubmit={create}>
    <label>
      Resource
      <select value={pattern} onchange={(e) => ((picked = e.currentTarget.value), (objectId = ""))}>
        {#each resources as r (r.pattern)}
          <option value={r.pattern} selected={r.pattern === pattern}>{label(r.pattern)}</option>
        {/each}
      </select>
    </label>
    {#if needsId}
      {#key kind}<ObjectPicker kind={kind === "group" ? "group" : "user"} bind:id={objectId} label={kind === "group" ? "Which group" : "Which user"} />{/key}
    {/if}
    <fieldset>
      <legend>Notify when it is</legend>
      {#each CHANGES as c (c)}
        <label class="check"><input type="checkbox" value={c} bind:group={changes} /> {c}</label>
      {/each}
    </fieldset>
    <label>
      Send notifications to
      <select bind:value={target}>
        <option value="dolly">dollycli (this server)</option>
        <option value="custom">A URL of my own</option>
      </select>
    </label>
    {#if target === "custom"}
      <label>
        Notification URL
        <input type="text" bind:value={url} required placeholder="http://localhost:5000/notify" spellcheck="false" />
        <span class="note">dollycli sends a validation request to it first. It must answer 200 with the token, as text.</span>
      </label>
    {:else}
      <p class="note">dollycli answers the validation request and takes each notification itself (<code>{OWN_URL}</code>). What it sent is in the log below.</p>
    {/if}
    <label>
      Expires in
      <select bind:value={minutes}>
        {#each lifetimes as l (l.minutes)}<option value={l.minutes}>{l.label}</option>{/each}
      </select>
    </label>
    <label>
      Client state <span class="opt">(optional)</span>
      <input type="text" bind:value={clientState} maxlength="128" spellcheck="false" />
    </label>
    {#if error}<p class="error" role="alert">{error}</p>{/if}
    <div class="actions">
      <button type="button" class="btn ghost" onclick={onclose}>Cancel</button>
      <button type="submit" class="btn primary" class:busy disabled={busy || !changes.length || (needsId && !objectId)}>Create</button>
    </div>
  </form>
</Dialog>

<style>
  form { display: grid; gap: 14px; min-width: min(460px, 80vw); }
  label { display: grid; gap: 4px; font-weight: 500; }
  label.check { display: inline-flex; align-items: center; gap: 6px; font-weight: 400; margin-right: 14px; }
  fieldset { border: 0; padding: 0; margin: 0; display: flex; flex-wrap: wrap; align-items: center; gap: 2px 0; }
  legend { font-weight: 500; padding: 0; margin-bottom: 4px; }
  .note, .opt { font-weight: 400; color: var(--muted); font-size: 12px; }
  p.note { margin: -6px 0 0; }
  .error { margin: 0; color: var(--red); }
  .actions { display: flex; justify-content: flex-end; gap: 8px; }
</style>
