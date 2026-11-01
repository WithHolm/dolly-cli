<script lang="ts">
  import { TOTAL, renameDepartment, type Profile } from "./profile";
  import { addTo, canAdd, removeFrom } from "./shares.svelte";
  import ShareBar from "./ShareBar.svelte";
  import { wiz } from "./state.svelte";

  const p = $derived(wiz.profile as Profile);

  // A job title row is one line. Nothing here parses text: a title is its own row with its own weight.
  let open = $state<Record<number, boolean>>({ 0: true });

  // a title is a manager role when it has the word (the server matches the text exactly, with its case)
  const isManager = (title: string) => p.people.managerWord !== "" && title.includes(p.people.managerWord);
  const managerCount = (d: Profile["departments"][number]) => d.titles.filter((t) => isManager(t.name)).length;
</script>

<h3>Departments and job titles</h3>
<p class="muted">The bar of a department is the share of the people who work in it. Under it, the job titles have their own shares of that department. Each group adds up to 100%: when you raise one, all the others give the same amount. The lock keeps a share as it is.</p>

<fieldset>
  <legend>Managers</legend>
  <label class="word">Word that makes a job title a manager role
    <input type="text" bind:value={p.people.managerWord} aria-invalid={wiz.bad("people.managerWord")} aria-describedby="manager-hint" />
  </label>
  <p class="muted" id="manager-hint">
    A person whose job title has this word is a manager. The first manager of a department is its head, and the others in the department report to the head.
    The heads report to the head of the first department. Everyone else reports to a manager of their department. If a department has no manager title,
    its first person is the head. The titles that count are marked below.
  </p>
</fieldset>

<div class="depts">
  {#each p.departments as d, i (i)}
    <div class="dept" role="group" aria-label="Department {i + 1}{d.name ? ', ' + d.name : ''}">
      <div class="head">
        <label class="name">Department
          <input type="text" value={d.name} aria-invalid={wiz.bad(`departments[${i}].name`)}
            onchange={(e) => { renameDepartment(p, d.name, e.currentTarget.value); d.name = e.currentTarget.value; }} />
        </label>
        <ShareBar bind:items={p.departments} {i} label="Share of people in {d.name || 'department ' + (i + 1)}" />
        <button class="btn ghost" onclick={() => removeFrom(p.departments, i)} aria-label="Remove department {d.name || i + 1}" disabled={p.departments.length < 2}>Remove</button>
      </div>
      <button class="toggle" aria-expanded={!!open[i]} onclick={() => (open[i] = !open[i])}>
        <span class="chev" class:turned={open[i]}>›</span> Job titles ({d.titles.length}){managerCount(d) === 0 ? " · no manager title: the first person is the head" : ` · ${managerCount(d)} manager ${managerCount(d) === 1 ? "title" : "titles"}`}
      </button>
      {#if open[i]}
        <div class="titles">
          {#each d.titles as t, j (j)}
            <div class="title">
              <div class="tin">
                <input type="text" placeholder="Job title" aria-label="Job title {j + 1} of {d.name || 'department ' + (i + 1)}" bind:value={t.name} aria-invalid={wiz.bad(`departments[${i}].titles`)} />
                {#if isManager(t.name)}<span class="badge" title="This title has the manager word">manager</span>{/if}
              </div>
              <ShareBar bind:items={d.titles} i={j} label="Share of {t.name || 'job title ' + (j + 1)}" />
              <button class="btn ghost" onclick={() => removeFrom(d.titles, j)} aria-label="Remove job title {t.name || j + 1}" disabled={d.titles.length < 2}>Remove</button>
            </div>
          {/each}
          <div><button class="btn" onclick={() => addTo(d.titles, { name: "" })} disabled={!canAdd(d.titles)} title={canAdd(d.titles) ? undefined : "Every share is locked. Unlock one to make room."}>+ Job title</button></div>
        </div>
      {/if}
    </div>
  {/each}
</div>
<div><button class="btn" disabled={!canAdd(p.departments)} title={canAdd(p.departments) ? undefined : "Every share is locked. Unlock one to make room."} onclick={() => { if (addTo(p.departments, { name: "", titles: [{ name: "", weight: TOTAL }] })) open[p.departments.length - 1] = true; }}>+ Department</button></div>

<style>
  fieldset { display: grid; gap: 8px; margin: 0; padding: 0; border: 0; min-width: 0; }
  legend { padding: 0; margin-bottom: 8px; font-weight: 600; }
  .word { display: grid; gap: 4px; font-weight: 500; max-width: 320px; }
  .tin { position: relative; }
  .tin input { width: 100%; padding-right: 72px; }
  .badge { position: absolute; right: 6px; top: 50%; transform: translateY(-50%); padding: 1px 6px; border-radius: 9px; font-size: 11px; background: var(--primary-soft); color: var(--primary); }
  .depts { display: grid; gap: 10px; }
  .dept { display: grid; gap: 8px; padding: 12px; border: 1px solid var(--line); border-radius: var(--radius); background: var(--surface); }
  .head { display: grid; grid-template-columns: minmax(180px, 260px) 1fr auto; gap: 12px; align-items: end; }
  .name { display: grid; gap: 4px; font-weight: 500; }
  .head :global(.weight) { align-self: end; }
  .toggle { justify-self: start; display: inline-flex; gap: 6px; align-items: center; background: none; border: 0; padding: 2px 0; font: inherit; color: var(--muted); cursor: pointer; }
  .toggle:hover { color: var(--text); }
  .chev { display: inline-block; transition: transform 0.1s; }
  .chev.turned { transform: rotate(90deg); }
  .titles { display: grid; gap: 6px; padding-left: 14px; border-left: 2px solid var(--line); }
  .title { display: grid; grid-template-columns: minmax(180px, 260px) 1fr auto; gap: 12px; align-items: center; }
</style>
