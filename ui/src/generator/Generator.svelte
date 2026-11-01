<script lang="ts">
  import { graph } from "../lib/api";
  import { route } from "../lib/route.svelte";
  import { notices as notice } from "../lib/notice.svelte";
  import norwayText from "../../../docs/profiles/norway.json?raw";
  import type { Profile } from "./profile";
  import { wiz } from "./state.svelte";
  import StepAdmin from "./StepAdmin.svelte";
  import StepAttributes from "./StepAttributes.svelte";
  import StepDepartments from "./StepDepartments.svelte";
  import StepFormats from "./StepFormats.svelte";
  import StepMake from "./StepMake.svelte";
  import StepOffices from "./StepOffices.svelte";
  import StepOrganization from "./StepOrganization.svelte";
  import StepPeople from "./StepPeople.svelte";

  // A wizard that makes a profile (docs/profile.md), shows what it would make, and makes the data from it. The server checks the profile in
  // full and answers with the field that is wrong, so the editor has no rules of its own. A step shows its errors when the person presses
  // Next, and not while the step is being filled in. Every value goes in the page as text.

  const steps = [
    { id: "start", label: "Start" },
    { id: "organization", label: "Organization" },
    { id: "offices", label: "Offices" },
    { id: "departments", label: "Departments" },
    { id: "people", label: "People" },
    { id: "formats", label: "Formats" },
    { id: "attributes", label: "Attributes" },
    { id: "admin", label: "Owner and admin" },
    { id: "make", label: "Preview and regenerate" },
  ];

  const step = $derived(steps.find((s) => s.id === route.page) ?? steps[0]);
  const index = $derived(steps.indexOf(step));

  $effect(() => wiz.save());
  $effect(() => {
    if (!wiz.profile) return;
    return wiz.schedule(JSON.stringify(wiz.body()));
  });

  async function start(kind: "default" | "current" | "norway") {
    try {
      const profile =
        kind === "norway" ? (JSON.parse(norwayText) as Profile) : await graph.get<Profile>(kind === "current" ? "/profile/current" : "/profile/default");
      wiz.start(profile);
      route.go("generator", "organization");
    } catch (e) {
      notice.error(e instanceof Error ? e.message : String(e));
    }
  }

  async function open(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    input.value = "";
    if (!file) return;
    try {
      const next = JSON.parse(await file.text());
      if (typeof next !== "object" || next === null || Array.isArray(next)) throw new Error("The file is not a profile: it must be a JSON object.");
      wiz.start(next as Profile);
      route.go("generator", "organization");
    } catch (err) {
      notice.error(err instanceof Error ? err.message : String(err));
    }
  }

  function startOver() {
    wiz.startOver();
    route.go("generator");
  }

  // Next asks the server first. A step with a problem stays, and shows it.
  async function next() {
    await wiz.validate();
    if (wiz.issuesOf(step.id).length) {
      wiz.tried[step.id] = true;
      return;
    }
    route.go("generator", steps[index + 1].id);
  }

  const shown = $derived(step.id === "make" ? wiz.issues : wiz.tried[step.id] ? wiz.issuesOf(step.id) : []);
</script>

<div class="head">
  <h2>Generator</h2>
  <p class="muted">Describe a tenant, look at what dollycli would make from it, and regenerate the data. The people and the company are made up.</p>
</div>

<div class="layout">
  <nav aria-label="Steps">
    {#each steps as s, i (s.id)}
      {#if i === 0 || wiz.profile}
        <a href={route.href("generator", s.id)} aria-current={s.id === step.id ? "step" : undefined}><span class="n">{i + 1}</span>{s.label}</a>
      {:else}
        <span class="off" aria-disabled="true"><span class="n">{i + 1}</span>{s.label}</span>
      {/if}
    {/each}
    {#if wiz.profile}
      <button class="btn ghost over" onclick={startOver}>Start over</button>
    {/if}
  </nav>

  <section>
    {#if shown.length}
      <div class="problem" role="alert">
        <strong>The profile has {shown.length === 1 ? "a problem" : `${shown.length} problems`}.</strong>
        <ul>
          {#each shown as i, n (n)}
            <li>
              {#if i.label}<strong>{i.label}:</strong>&nbsp;{/if}{i.message}
              {#if step.id === "make" && i.step}<a href={route.href("generator", i.step)}>Fix it</a>{/if}
            </li>
          {/each}
        </ul>
      </div>
    {/if}

    {#if step.id === "start" || !wiz.profile}
      <h3>Start from</h3>
      <div class="cards">
        <button class="card" onclick={() => start("current")}><strong>The current data</strong><span class="muted">The profile this data was made from. Change it, and regenerate.</span></button>
        <button class="card" onclick={() => start("default")}><strong>The default</strong><span class="muted">Acme, with Nordic offices. What dollycli makes with no profile.</span></button>
        <button class="card" onclick={() => start("norway")}><strong>Fjordvik (Norway)</strong><span class="muted">An energy company with six Norwegian offices, Norwegian names, and admin accounts named adm.&lt;first name&gt;.</span></button>
        <div class="card file">
          <strong>A file</strong><span class="muted">A profile you saved before (JSON).</span>
          <label class="btn">Choose a file<input type="file" accept=".json,application/json" onchange={open} hidden /></label>
        </div>
      </div>
      {#if wiz.profile}
        <p class="muted">You have a profile in progress: <strong>{wiz.profile.name || wiz.profile.organization.displayName}</strong>. A start above replaces it. <a href={route.href("generator", "organization")}>Continue with it</a>.</p>
      {/if}
    {:else if step.id === "organization"}
      <StepOrganization />
    {:else if step.id === "offices"}
      <StepOffices />
    {:else if step.id === "departments"}
      <StepDepartments />
    {:else if step.id === "people"}
      <StepPeople />
    {:else if step.id === "formats"}
      <StepFormats />
    {:else if step.id === "attributes"}
      <StepAttributes />
    {:else if step.id === "admin"}
      <StepAdmin />
    {:else}
      <StepMake />
    {/if}

    {#if wiz.profile && step.id !== "start"}
      <div class="foot">
        {#if index > 0}<a class="btn" href={route.href("generator", steps[index - 1].id)}>Back</a>{/if}
        {#if index < steps.length - 1}<button class="btn primary" onclick={next}>Next</button>{/if}
      </div>
    {/if}
  </section>
</div>

<style>
  .head { margin-bottom: 20px; display: grid; gap: 4px; }
  .head h2 { font-size: 20px; letter-spacing: -0.02em; }
  .layout { display: grid; grid-template-columns: 200px minmax(0, 1fr); gap: 32px; align-items: start; }
  nav { display: grid; gap: 2px; position: sticky; top: 64px; }
  nav a, nav .off { display: flex; align-items: center; gap: 8px; padding: 6px 8px; border-radius: var(--radius); color: var(--muted); text-decoration: none; font-weight: 500; }
  nav a:hover { background: var(--sunken); color: var(--text); }
  nav a[aria-current="step"] { background: var(--primary-soft); color: var(--text); }
  nav .off { opacity: 0.6; }
  nav .over { justify-self: start; margin-top: 8px; }
  .n { display: inline-grid; place-items: center; width: 20px; height: 20px; border-radius: 50%; border: 1px solid var(--line-strong); font-size: 11px; }
  section { display: grid; gap: 16px; min-width: 0; }
  section :global(h3) { font-size: 18px; font-weight: 650; letter-spacing: -0.015em; }
  .cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 12px; }
  .card { display: grid; gap: 6px; align-content: start; justify-items: start; text-align: left; font: inherit; color: inherit; padding: 16px; background: var(--surface); border: 1px solid var(--line); border-radius: var(--radius); cursor: pointer; }
  .card:hover { border-color: var(--primary); }
  .card.file { cursor: default; }
  .card.file:hover { border-color: var(--line); }
  .card.file .btn { margin-top: 4px; }
  :global([aria-invalid="true"]) { border-color: var(--red) !important; }
  .problem { padding: 8px 12px; border-radius: var(--radius); color: var(--red); background: var(--red-soft); }
  .problem ul { margin: 4px 0 0; padding-left: 18px; }
  .problem a { margin-left: 6px; color: inherit; }
  .foot { display: flex; gap: 8px; align-items: center; padding-top: 8px; border-top: 1px solid var(--line); }
  .foot a.btn { text-decoration: none; }
</style>
