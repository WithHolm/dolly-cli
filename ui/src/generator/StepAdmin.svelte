<script lang="ts">
  import { adminName, type Profile } from "./profile";
  import { wiz } from "./state.svelte";

  const p = $derived(wiz.profile as Profile);
  const owner = { first: "Philip", last: "Meholm" }; // the owner keeps this name in every profile
  const adminUpn = $derived(`${adminName(p.admin.userPrincipalName, owner.first, owner.last)}@${p.organization.domain}`);
  const departments = $derived(p.departments.map((d) => d.name));
</script>

<h3>Owner and admin account</h3>
<p class="muted">The owner is Philip Meholm in every profile. The profile says where the owner works, and how the admin account is named. Both use the default domain.</p>
<div class="grid">
  <label>Owner's department
    <select bind:value={p.owner.department} aria-invalid={wiz.bad("owner.department")}>{#each departments as d (d)}<option>{d}</option>{/each}</select>
  </label>
  <label>Owner's job title<input type="text" bind:value={p.owner.jobTitle} aria-invalid={wiz.bad("owner.jobTitle")} /></label>
  <label>Admin account name
    <input type="text" bind:value={p.admin.userPrincipalName} aria-invalid={wiz.bad("admin.userPrincipalName")} />
    <span class="hint">Use {"{first}"} {"{last}"} {"{f}"} {"{l}"}. Examples: a.{"{first}"}, adm.{"{first}"}, {"{first}"}.a.{"{last}"}</span>
  </label>
  <label>Admin display name<input type="text" bind:value={p.admin.displayName} aria-invalid={wiz.bad("admin.displayName")} /><span class="hint">Use {"{First}"} and {"{Last}"} for the names as written.</span></label>
  <label>Admin job title<input type="text" bind:value={p.admin.jobTitle} aria-invalid={wiz.bad("admin.jobTitle")} /></label>
  <label>Admin department
    <select bind:value={p.admin.department} aria-invalid={wiz.bad("admin.department")}>{#each departments as d (d)}<option>{d}</option>{/each}</select>
  </label>
</div>
<p>The admin account will be <code>{adminUpn}</code>.</p>

<style>
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: 12px 16px; }
  label { display: grid; gap: 4px; font-weight: 500; align-content: start; }
  .hint { font-weight: 400; color: var(--muted); font-size: 12px; }
</style>
