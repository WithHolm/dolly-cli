<script lang="ts">
  import { route } from "./lib/route.svelte";
  import Toasts from "./lib/Toasts.svelte";
  import ApiReference from "./graph/ApiReference.svelte";
  import { legacyReferenceAddress } from "./graph/reference";
  import Graph from "./graph/Graph.svelte";
  import Login from "./login/Login.svelte";
  import Generator from "./generator/Generator.svelte";

  // The APIs that this Dolly runs come from the server. A page is here for the ones that have one.
  // The generator is not an API: it makes the data of Graph, so it is in the bar when Graph runs. The reference of the API is a page of the
  // bar too: it is about the API, and not about the tenant, so it is not one of the pages of Graph.
  const pages: Record<string, typeof Graph> = { graph: Graph, login: Login, generator: Generator, api: ApiReference };
  const labels: Record<string, string> = { graph: "Graph", login: "Sign-in", generator: "Generator", api: "API" };

  // The reference was at #/graph/api/...: an old link goes to the new address, with no new step in the history.
  $effect(() => {
    const now = legacyReferenceAddress(route.segments);
    if (now) location.replace(route.href(...now));
  });
  let apis = $state<string[]>(["graph"]);
  $effect(() => {
    void fetch("/_dolly/apis")
      .then((r) => r.json())
      .then((a: { apis: string[] }) => (apis = a.apis))
      .catch(() => {});
  });

  const tabs = $derived(apis.includes("graph") ? [...apis, "generator", "api"] : apis);
  const Page = $derived(pages[route.api]);
</script>

<header>
  <div class="bar">
    <a class="brand" href="#/">dollycli</a>
    <nav aria-label="APIs">
      {#each tabs as name (name)}
        <a href={route.href(name)} aria-current={name === route.api ? "page" : undefined}>{labels[name] ?? name}</a>
      {/each}
    </nav>
  </div>
</header>

<main>
  {#if Page}
    <Page />
  {:else}
    <div class="empty"><strong>No page for "{route.api}" yet</strong>dollycli runs this API, and the portal has no page for it.</div>
  {/if}
</main>

<Toasts />

<style>
  header { position: sticky; top: 0; z-index: 10; background: var(--surface); border-bottom: 1px solid var(--line); }
  .bar { display: flex; align-items: center; gap: 24px; max-width: var(--page-width); margin: 0 auto; padding: 0 24px; height: 48px; }
  .brand { font-weight: 700; font-size: 16px; letter-spacing: -0.02em; color: var(--text); text-decoration: none; }
  nav { display: flex; gap: 4px; align-self: stretch; }
  nav a {
    display: flex; align-items: center; padding: 0 10px; color: var(--muted); text-decoration: none; font-weight: 500;
    border-bottom: 2px solid transparent; margin-bottom: -1px;
  }
  nav a:hover { color: var(--text); }
  nav a[aria-current="page"] { color: var(--text); border-bottom-color: var(--primary); }
  main { max-width: var(--page-width); margin: 0 auto; padding: 16px 24px 64px; }
</style>
