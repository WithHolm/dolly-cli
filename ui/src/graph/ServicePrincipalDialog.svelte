<script lang="ts">
  import { graph } from "../lib/api";
  import { succeeded } from "../lib/notice.svelte";
  import { dateTime } from "../lib/format";
  import Chip from "../lib/Chip.svelte";
  import CopyButton from "../lib/CopyButton.svelte";
  import Dialog from "../lib/Dialog.svelte";
  import Icon from "../lib/Icon.svelte";
  import Tabs from "../lib/Tabs.svelte";
  import PermissionsPanel from "./PermissionsPanel.svelte";
  import type { ServicePrincipal, Usage } from "./types";

  // Everything about one service principal, in a dialog with tabs: the overview (who it is, what it has asked for, and how Dolly
  // treats its calls), the sign-in (permission mode and credential), and the permissions. `tab` is in the address.
  // `changed` tells the page to read the data again, after a change.
  let {
    sp,
    usage,
    throttle,
    tab: wanted,
    onclose,
    navigate,
    changed,
  }: {
    sp: ServicePrincipal;
    usage?: Usage;
    throttle: boolean;
    tab?: string;
    onclose: () => void;
    navigate: (tab: string) => void;
    changed: () => Promise<void>;
  } = $props();

  const TABS = [
    { id: "overview", label: "Overview" },
    { id: "sign-in", label: "Sign-in" },
    { id: "permissions", label: "Permissions" },
  ];
  const tab = $derived(TABS.some((t) => t.id === wanted) ? wanted! : "overview");

  async function change(action: () => Promise<unknown>, done?: string) {
    if (await succeeded(action, done)) await changed();
  }
  const base = $derived(`/service-principals/${sp.appId}`);
  const setFlag = (flag: "exemptFromThrottle" | "hideCalls", on: boolean) => change(() => graph.put(`${base}/flags`, { [flag]: on }));
  const setMode = (mode: string) => change(() => graph.put(`${base}/mode`, { mode }));
  const armCredential = () => change(() => graph.post(`${base}/credential/arm`));
  const clearCredential = () => change(() => graph.del(`${base}/credential`));

  const kind = $derived(sp.builtin || sp.system ? "Built in" : sp.auto ? "Auto-created" : "Registered");
</script>

<Dialog title={sp.displayName} {onclose}>
  {#snippet subtitle()}
    <span class="faint">Client ID</span><code>{sp.appId}</code><CopyButton text={sp.appId} title="Copy the client ID" />
    {#if sp.builtin || sp.system}<Chip tone="primary">built in</Chip>{:else if sp.auto}<Chip>auto-created</Chip>{/if}
  {/snippet}

  {#snippet tabs()}
    <Tabs items={TABS} selected={tab} onselect={navigate} label="Details of the service principal" prefix="sp" />
  {/snippet}

  <div role="tabpanel" id="sppanel-{tab}" aria-labelledby="sptab-{tab}" tabindex="-1">
    {#if tab === "overview"}
      <section>
        <h3>Identity</h3>
        <dl>
          <dt>Name</dt><dd>{sp.displayName}</dd>
          <dt>Kind</dt><dd>{kind}</dd>
          <dt>Client ID</dt><dd class="mono">{sp.appId}</dd>
          <dt>Object ID</dt><dd class="mono">{sp.id}<CopyButton text={sp.id} title="Copy the object ID" /></dd>
        </dl>
      </section>

      <section>
        <h3>What it has asked for</h3>
        <dl>
          <dt>Calls</dt><dd class="num">{usage?.calls ?? 0}</dd>
          <dt title="Resource units">Cost (RU)</dt><dd class="num">{usage?.ru ?? 0}</dd>
          <dt>Throttled</dt><dd class={["num", usage?.throttled && "red"]}>{usage?.throttled ?? 0}</dd>
        </dl>
      </section>

      <section>
        <h3>How dollycli treats its calls</h3>
        <label class="switch">
          <input type="checkbox" checked={sp.exemptFromThrottle} onchange={(e) => setFlag("exemptFromThrottle", e.currentTarget.checked)} />
          Never throttle this client
        </label>
        <p class="hint">
          The cost of its calls is still counted. {#if !throttle}Throttling is off now (see the switch above the list), so this changes nothing until it is on.{:else}The other clients are throttled at the limits of Graph.{/if}
        </p>
        <label class="switch">
          <input type="checkbox" checked={sp.hideCalls} onchange={(e) => setFlag("hideCalls", e.currentTarget.checked)} />
          Keep its calls out of the list of calls
        </label>
        <p class="hint">The pages Calls and Sign-in do not show its calls and its sign-ins, and they do not push the ones of other clients out of the list. The calls are still counted.</p>
      </section>
    {:else if tab === "sign-in"}
      {#if sp.builtin}
        <p class="muted">This is a built-in service principal. Nothing signs in as it, so it has no permission mode and no credential.</p>
      {:else}
        <section>
          <h3>Permission mode</h3>
          <p class="hint">Open: every call is allowed, and dollycli records what it needs. Enforce: only granted permissions are allowed (not active yet).</p>
          <select value={sp.mode} onchange={(e) => setMode(e.currentTarget.value)} aria-label="Permission mode of {sp.displayName}">
            <option value="open">Open</option>
            <option value="enforce">Enforce</option>
          </select>
        </section>
        <section>
          <h3>Credential</h3>
          <p class="hint">How the sign-in API checks the secret or the certificate of the client.</p>
          {#if sp.credential.state === "armed"}
            <Chip tone="amber">Waiting for a secret</Chip>
            <button type="button" class="btn ghost" onclick={clearCredential}>Cancel</button>
          {:else if sp.credential.state === "captured"}
            <Chip tone="primary" title="Kept {sp.credential.at ? dateTime(sp.credential.at) : ''}"><Icon name="lock" size={12} /> {sp.credential.kind} {sp.credential.hint}…</Chip>
            <button type="button" class="btn ghost" onclick={clearCredential}>Accept any again</button>
          {:else}
            <span class="muted">Accepts any</span>
            <button type="button" class="btn ghost" onclick={armCredential} title="The next secret that a client shows is kept, and only that one is accepted from then on.">Capture next secret</button>
          {/if}
        </section>
      {/if}
    {:else}
      <PermissionsPanel {sp} />
    {/if}
  </div>
</Dialog>

<style>
  [role="tabpanel"]:focus { outline: none; }
  section { margin-top: 16px; }
  section:first-child { margin-top: 4px; }
  dl { display: grid; grid-template-columns: 120px minmax(0, 1fr); gap: 4px 16px; margin: 6px 0 0; }
  dt { color: var(--muted); }
  dd { margin: 0; overflow-wrap: anywhere; display: flex; align-items: center; gap: 2px; }
  dd.num { font-variant-numeric: tabular-nums; }
  dd :global(.btn.icon) { width: 22px; height: 22px; }
  .switch { margin-top: 8px; }
  .red { color: var(--red); }
  .hint { margin: 2px 0 8px; color: var(--muted); max-width: 70ch; }
  .switch + .hint { margin: 2px 0 16px 26px; }
  select { margin-top: 2px; }
</style>
