<script lang="ts">
  import { route } from "../lib/route.svelte";
  import Calls from "./Calls.svelte";
  import Data from "./Data.svelte";
  import ApiReference from "./ApiReference.svelte";
  import Groups from "./Groups.svelte";
  import History from "./History.svelte";
  import Organization from "./Organization.svelte";
  import ServicePrincipals from "./ServicePrincipals.svelte";
  import Users from "./Users.svelte";

  const pages = [
    { id: "calls", label: "Calls", page: Calls },
    { id: "users", label: "Users", page: Users },
    { id: "groups", label: "Groups", page: Groups },
    { id: "service-principals", label: "Service principals", page: ServicePrincipals },
    { id: "organization", label: "Organization", page: Organization },
    { id: "data", label: "Data", page: Data },
    { id: "history", label: "History", page: History },
    { id: "api", label: "API", page: ApiReference },
  ];

  const current = $derived(pages.find((p) => p.id === route.page) ?? pages[0]);
  const Page = $derived(current.page);
</script>

<nav aria-label="Graph">
  {#each pages as p (p.id)}
    <a href={route.href("graph", p.id)} aria-current={p.id === current.id ? "page" : undefined}>{p.label}</a>
  {/each}
</nav>

<div class="page">
  <Page />
</div>

<style>
  nav { display: flex; gap: 4px; margin-bottom: 16px; border-bottom: 1px solid var(--line); }
  a {
    margin-bottom: -1px; padding: 8px 12px; color: var(--muted); text-decoration: none;
    border-bottom: 2px solid transparent; font-weight: 500;
  }
  a:hover { color: var(--text); }
  a[aria-current="page"] { color: var(--text); border-bottom-color: var(--primary); }
  .page { min-width: 0; }
</style>
