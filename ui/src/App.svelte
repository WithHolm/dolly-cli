<script lang="ts">
  import { route } from "./lib/route.svelte";
  import Toasts from "./lib/Toasts.svelte";
  import Graph from "./graph/Graph.svelte";
  import Login from "./login/Login.svelte";

  // The APIs that this Dolly runs come from the server. A page is here for the ones that have one.
  const pages: Record<string, typeof Graph> = { graph: Graph, login: Login };
  const labels: Record<string, string> = { graph: "Graph", login: "Sign-in" };

  let apis = $state<string[]>(["graph"]);
  $effect(() => {
    void fetch("/_dolly/apis")
      .then((r) => r.json())
      .then((a: { apis: string[] }) => (apis = a.apis))
      .catch(() => {});
  });

  const Page = $derived(pages[route.api]);
</script>

<header>
  <div class="bar">
    <a class="brand" href="#/">Dolly</a>
    <nav aria-label="APIs">
      {#each apis as name (name)}
        <a href={route.href(name)} aria-current={name === route.api ? "page" : undefined}>{labels[name] ?? name}</a>
      {/each}
    </nav>
  </div>
</header>

<main>
  {#if Page}
    <Page />
  {:else}
    <div class="empty"><strong>No page for "{route.api}" yet</strong>Dolly runs this API, and the portal has no page for it.</div>
  {/if}
</main>

<Toasts />

<style>
  header { position: sticky; top: 0; z-index: 10; background: var(--surface); border-bottom: 1px solid var(--line); }
  .bar { display: flex; align-items: center; gap: 24px; max-width: 1200px; margin: 0 auto; padding: 0 24px; height: 48px; }
  .brand { font-weight: 700; font-size: 16px; letter-spacing: -0.02em; color: var(--text); text-decoration: none; }
  nav { display: flex; gap: 4px; align-self: stretch; }
  nav a {
    display: flex; align-items: center; padding: 0 10px; color: var(--muted); text-decoration: none; font-weight: 500;
    border-bottom: 2px solid transparent; margin-bottom: -1px;
  }
  nav a:hover { color: var(--text); }
  nav a[aria-current="page"] { color: var(--text); border-bottom-color: var(--primary); }
  main { max-width: 1200px; margin: 0 auto; padding: 16px 24px 64px; }
</style>
