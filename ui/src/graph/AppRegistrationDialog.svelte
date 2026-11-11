<script lang="ts">
  import { route } from "../lib/route.svelte";
  import CopyButton from "../lib/CopyButton.svelte";
  import Dialog from "../lib/Dialog.svelte";
  import Tabs from "../lib/Tabs.svelte";
  import AppRolesPanel from "./AppRolesPanel.svelte";
  import ExposeApiPanel from "./ExposeApiPanel.svelte";
  import type { AppRegistration } from "./types";

  // Everything about one app registration that the portal can change: its app roles, and the API that it exposes. `tab` is in the address.
  // `changed` tells the page to read the data again.
  let { app, tab: wanted, onclose, navigate, changed }: { app: AppRegistration; tab?: string; onclose: () => void; navigate: (tab: string) => void; changed: () => Promise<void> } = $props();

  const TABS = $derived([
    { id: "app-roles", label: "App roles", count: app.roles },
    { id: "expose-an-api", label: "Expose an API" },
  ]);
  const tab = $derived(TABS.some((t) => t.id === wanted) ? wanted! : "app-roles");
</script>

<Dialog title={app.displayName} {onclose}>
  {#snippet subtitle()}
    <span class="faint">Client ID</span><code>{app.appId}</code><CopyButton text={app.appId} title="Copy the client ID" />
  {/snippet}

  {#snippet tabs()}
    <Tabs items={TABS} selected={tab} onselect={navigate} label="Details of the app registration" prefix="app" />
  {/snippet}

  <div role="tabpanel" id="apppanel-{tab}" aria-labelledby="apptab-{tab}" tabindex="-1">
    {#if tab === "app-roles"}
      <AppRolesPanel {app} {changed} />
      {#if app.servicePrincipal}
        <p class="hint">
          Who has the roles is on <a href={route.href("graph", "service-principals", app.appId, "users-and-groups")}>the service principal of the app</a>, on the tab Users and groups.
        </p>
      {:else}
        <p class="hint">The app has no service principal in this tenant yet. A client that signs in with this client ID gets one, and then the roles can be assigned.</p>
      {/if}
    {:else}
      <ExposeApiPanel {app} {changed} />
    {/if}
  </div>
</Dialog>

<style>
  [role="tabpanel"]:focus { outline: none; }
  .hint { margin: 16px 0 0; color: var(--muted); max-width: 72ch; }
</style>
