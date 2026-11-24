<script lang="ts">
  import { route } from "../lib/route.svelte";
  import Calls from "./Calls.svelte";
  import Data from "./Data.svelte";
  import AppRegistrations from "./AppRegistrations.svelte";
  import Groups from "./Groups.svelte";
  import History from "./History.svelte";
  import Organization from "./Organization.svelte";
  import ServicePrincipals from "./ServicePrincipals.svelte";
  import Sites from "./Sites.svelte";
  import Subscriptions from "./Subscriptions.svelte";
  import Users from "./Users.svelte";

  const pages = [
    { id: "calls", label: "Calls", page: Calls },
    { id: "users", label: "Users", page: Users },
    { id: "groups", label: "Groups", page: Groups },
    { id: "app-registrations", label: "App registrations", page: AppRegistrations },
    { id: "service-principals", label: "Service principals", page: ServicePrincipals },
    { id: "sites", label: "Sites", page: Sites },
    { id: "subscriptions", label: "Subscriptions", page: Subscriptions },
    { id: "organization", label: "Organization", page: Organization },
    { id: "data", label: "Data", page: Data },
    { id: "history", label: "History", page: History },
  ];

  const current = $derived(pages.find((p) => p.id === route.page) ?? pages[0]);
  const Page = $derived(current.page);
</script>

<div class="layout">
  <nav aria-label="Graph">
    {#each pages as p (p.id)}
      <a href={route.href("graph", p.id)} aria-current={p.id === current.id ? "page" : undefined}>{p.label}</a>
    {/each}
  </nav>

  <div class="page">
    <Page />
  </div>
</div>

<style>
  /* The pages are a panel on the left, and the page is next to it. On a narrow screen the panel is a row above the page again. */
  .layout { display: grid; grid-template-columns: 176px minmax(0, 1fr); gap: 24px; align-items: start; }
  nav {
    position: sticky; top: 64px; /* under the bar of the portal (48px) */
    display: flex; flex-direction: column; gap: 1px; max-height: calc(100vh - 80px); overflow-y: auto;
  }
  a {
    padding: 7px 12px; color: var(--muted); text-decoration: none; font-weight: 500;
    border-left: 2px solid transparent; border-radius: 0 var(--radius) var(--radius) 0;
  }
  a:hover { color: var(--text); background: var(--sunken); }
  a[aria-current="page"] { color: var(--text); border-left-color: var(--primary); background: var(--sunken); }
  .page { min-width: 0; }

  @media (max-width: 860px) {
    .layout { grid-template-columns: minmax(0, 1fr); gap: 12px; }
    nav { position: static; flex-direction: row; gap: 4px; max-height: none; overflow-x: auto; overflow-y: visible; border-bottom: 1px solid var(--line); }
    a { flex: none; margin-bottom: -1px; border-left: 0; border-bottom: 2px solid transparent; border-radius: 0; }
    a:hover { background: none; }
    a[aria-current="page"] { background: none; border-bottom-color: var(--primary); }
  }
</style>
