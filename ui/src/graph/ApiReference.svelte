<script lang="ts">
  import { poll } from "../lib/poll";
  import { route } from "../lib/route.svelte";
  import { statusTone } from "../lib/format";
  import Chip from "../lib/Chip.svelte";
  import Icon from "../lib/Icon.svelte";
  import Markdown from "../lib/Markdown.svelte";
  import { apiDocs } from "./docs.svelte";
  import OperationDetail from "./OperationDetail.svelte";
  import { buildPages, type Node } from "./reference";

  // The address says what is open: #/graph/api/<resource>/<call>. A link from another page (a call in the
  // list of calls) opens the call here, and a call that is open can be sent to someone as a link.
  $effect(() => poll(() => apiDocs.load(), 2000));

  let text = $state("");
  const docs = $derived(apiDocs.value);
  const pages = $derived(docs ? buildPages(docs.endpoints, text, docs.pages) : []);
  const unsupported = $derived(docs?.unsupported ?? []);

  const pageId = $derived(route.segments[2] ?? "");
  const openKey = $derived(route.segments[3] ?? "");
  const showUnsupported = $derived(pageId === "unsupported");
  const current = $derived(pages.find((p) => p.id === pageId) ?? pages[0]);

  const toggle = (key: string, isOpen: boolean) => route.go("graph", "api", current.id, isOpen ? "" : key);

  // the hints on a closed call: what else its path can end with
  const hints = (node: Node) => [
    ...(node.variants.some((v) => v.path.includes("{type}")) ? ["/{type}"] : []),
    ...(node.variants.some((v) => v.path.endsWith("$count")) ? ["/$count"] : []),
  ];
  const calls = (node: Node) => [node.main, ...node.variants].reduce((sum, e) => sum + e.calls, 0);

  // an open call scrolls into view, so a link to it shows it
  const reveal = (isOpen: boolean) => (el: HTMLElement) => {
    if (isOpen) el.scrollIntoView({ block: "nearest" });
  };
</script>

<div class="layout">
  <aside>
    <label class="search">
      <Icon name="search" />
      <span class="sr-only">Filter the calls</span>
      <input type="search" placeholder="Filter the calls" bind:value={text} />
    </label>
    <nav aria-label="Resources">
      {#each pages as p (p.id)}
        <a href={route.href("graph", "api", p.id)} aria-current={!showUnsupported && p.id === current?.id ? "page" : undefined} class={{ none: !p.count }}>
          <span>{p.name}</span>
          <span class="count">{p.count}</span>
        </a>
      {/each}
      {#if unsupported.length}
        <a href={route.href("graph", "api", "unsupported")} aria-current={showUnsupported ? "page" : undefined}>
          <span>Not supported yet</span>
          <Chip tone="amber">{unsupported.length}</Chip>
        </a>
      {/if}
    </nav>
  </aside>

  <div class="content">
    {#if showUnsupported}
      <header>
        <h2>Not supported yet</h2>
        <p class="muted">A client called these, and dollycli has no answer. This list shows what to add next.</p>
      </header>
      <table class="t fixed">
        <colgroup><col style="width: 84px" /><col /><col style="width: 72px" /><col style="width: 110px" /></colgroup>
        <tbody>
          {#each unsupported as u (u.method + u.path)}
            <tr>
              <td class="mono muted method">{u.method}</td>
              <td class="mono cut"><div class="truncate" title={u.path}>{u.path}</div></td>
              <td><Chip tone={statusTone(u.status)}>{u.status}</Chip></td>
              <td class="num muted">{u.calls} {u.calls === 1 ? "call" : "calls"}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    {:else if docs && current}
      <header>
        <h2>{current.name}</h2>
        <div class="muted"><Markdown text={current.description} /></div>
      </header>

      {#each current.sections as section (section.title)}
        <section>
          <h3>{section.title} <span class="faint">{section.nodes.length}</span></h3>
          <ul>
            {#each section.nodes as node (node.key)}
              {@const isOpen = openKey === node.key}
              {@const used = calls(node)}
              <li class={{ open: isOpen }} {@attach reveal(isOpen)}>
                <button type="button" class="row" aria-expanded={isOpen} onclick={() => toggle(node.key, isOpen)}>
                  <span class={["chevron", isOpen && "turned"]}><Icon name="chevron" /></span>
                  <span class={["method", node.main.method]}>{node.main.method}</span>
                  <code class="path truncate" title={node.main.path}>{node.main.path}</code>
                  <span class="summary muted truncate">{node.main.summary}</span>
                  <span class="hints">{#each hints(node) as h (h)}<Chip>{h}</Chip>{/each}</span>
                  <span class="num used" title={used ? `${used} calls from clients` : "No client has called this"}>{used || ""}</span>
                </button>
                {#if isOpen}
                  <div class="detail"><OperationDetail {node} {docs} /></div>
                {/if}
              </li>
            {/each}
          </ul>
        </section>
      {:else}
        <div class="empty"><strong>No call matches "{text}"</strong>Clear the filter, or look at another resource in the list.</div>
      {/each}
    {/if}
  </div>
</div>

<style>
  .layout { display: grid; grid-template-columns: 220px minmax(0, 1fr); gap: 32px; align-items: start; }
  aside { position: sticky; top: 64px; }
  .search { position: relative; display: block; margin-bottom: 12px; }
  .search :global(svg) { position: absolute; left: 8px; top: 6px; color: var(--faint); pointer-events: none; }
  .search input { width: 100%; padding-left: 28px; }

  nav { display: flex; flex-direction: column; gap: 2px; }
  nav a {
    display: flex; align-items: center; justify-content: space-between; gap: 8px;
    padding: 5px 10px; border-radius: var(--radius); color: var(--muted); text-decoration: none; font-weight: 500;
  }
  nav a:hover { background: var(--sunken); color: var(--text); }
  nav a[aria-current="page"] { background: var(--primary-soft); color: var(--primary); }
  nav a.none { opacity: 0.5; }
  .count { font-size: 12px; font-weight: 400; color: var(--faint); font-variant-numeric: tabular-nums; }

  .content { min-width: 0; }
  header { margin-bottom: 20px; }
  header p { margin-top: 2px; }
  section { margin-bottom: 28px; }
  h3 { margin-bottom: 6px; }

  ul { margin: 0; padding: 0; list-style: none; border-top: 1px solid var(--line); }
  li { border-bottom: 1px solid var(--line); }
  li.open { background: var(--sunken); box-shadow: inset 2px 0 var(--primary); }

  .row {
    display: grid; grid-template-columns: 16px 68px minmax(180px, 36%) minmax(0, 1fr) auto 44px; align-items: center; gap: 10px;
    width: 100%; padding: 8px 12px 8px 8px; text-align: left;
    font: inherit; color: inherit; background: none; border: 0; cursor: pointer;
  }
  li:not(.open) .row:hover { background: var(--sunken); }
  .chevron { display: flex; color: var(--faint); transition: transform 0.12s; }
  .chevron.turned { transform: rotate(90deg); color: var(--text); }
  .method { font: 600 12px var(--mono); color: var(--muted); }
  .method.POST, .method.PATCH, .method.PUT { color: var(--amber); }
  .method.DELETE { color: var(--red); }
  .path { font-size: 12.5px; }
  .summary { min-width: 0; }
  .hints { display: flex; gap: 4px; }
  .used { color: var(--muted); }
  .detail { padding: 4px 16px 20px 34px; cursor: default; }
  .method:global(.mono) { width: auto; }
</style>
