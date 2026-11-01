<script lang="ts">
  import { graph } from "../lib/api";
  import { poll } from "../lib/poll";
  import { route } from "../lib/route.svelte";
  import CopyButton from "../lib/CopyButton.svelte";
  import type { AppRegistration, ServicePrincipal } from "./types";

  // The app registrations of this tenant. An app registration is the definition of an app, in its home tenant only. The service principal with the same
  // client ID is the instance of it: the Service principals page has it.
  let list = $state.raw<AppRegistration[]>([]);
  let sps = $state.raw<ServicePrincipal[]>([]);
  let loaded = $state(false);

  async function load() {
    const [a, s] = await Promise.all([
      graph.get<{ applications: AppRegistration[] }>("/applications"),
      graph.get<{ servicePrincipals: ServicePrincipal[] }>("/service-principals"),
    ]);
    list = a.applications;
    sps = s.servicePrincipals;
    loaded = true;
  }
  $effect(() => poll(load, 3000));

  const spOf = (a: AppRegistration) => sps.find((s) => s.id === a.servicePrincipal);
</script>

<p class="hint">
  The definition of an app. It lives in its home tenant only, and it says what the app asks for. The service principal is the instance of the app in this tenant, and it has
  the same client ID.
</p>

{#if loaded && !list.length}
  <div class="empty"><strong>No app registrations</strong>An app of another tenant has a service principal here, and no registration.</div>
{:else if list.length}
  <table class="t" aria-label="App registrations">
    <thead>
      <tr>
        <th>App registration</th>
        <th>Who can sign in</th>
        <th class="num" title="requiredResourceAccess: the permissions that the app asks for">Asks for</th>
        <th>Service principal</th>
      </tr>
    </thead>
    <tbody>
      {#each list as a (a.id)}
        {@const sp = spOf(a)}
        <tr>
          <td>
            <strong>{a.displayName}</strong>
            <div class="id muted"><code>{a.appId}</code><CopyButton text={a.appId} title="Copy the client ID" /></div>
          </td>
          <td class="muted">{a.signInAudience}</td>
          <td class={["num", !a.requested && "faint"]}>{a.requested} {a.requested === 1 ? "permission" : "permissions"}</td>
          <td>
            {#if sp}
              <a href={route.href("graph", "service-principals", a.appId)}>{sp.displayName}</a>
            {:else}
              <span class="faint">None yet. A client that signs in with this client ID gets one.</span>
            {/if}
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
{/if}

<style>
  .hint { margin: 2px 0 12px; color: var(--muted); max-width: 80ch; }
  .id { display: flex; align-items: center; gap: 2px; font-size: 12px; }
</style>
