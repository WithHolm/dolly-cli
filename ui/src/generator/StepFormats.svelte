<script lang="ts">
  import { joinList, splitList, type Profile } from "./profile";
  import { wiz } from "./state.svelte";

  const p = $derived(wiz.profile as Profile);
  const num = (v: string) => Number(v.replace(/\D/g, "")) || 0;
</script>

<h3>Formats</h3>
<p class="muted">
  <code>#</code> is a digit, <code>{"{1000-1099}"}</code> is a number in a range, and <code>{"{cc}"}</code> is the calling code of the office. Anything else stays as written. The
  <a href="https://github.com/withholm/dolly-cli/blob/main/docs/profile.md#patterns" target="_blank" rel="noopener noreferrer">pattern language</a> has no code in it.
</p>
<div class="grid">
  <label>Employee ID<input type="text" bind:value={p.formats.employeeId.pattern} aria-invalid={wiz.bad("formats.employeeId.pattern")} /></label>
  <label>First employee number<input type="text" inputmode="numeric" value={p.formats.employeeId.start} aria-invalid={wiz.bad("formats.employeeId.start")} onchange={(e) => (p.formats.employeeId.start = num(e.currentTarget.value))} /></label>
  <label>Mobile phone<input type="text" bind:value={p.formats.mobilePhone} aria-invalid={wiz.bad("formats.mobilePhone")} /></label>
  <label>Business phone<input type="text" bind:value={p.formats.businessPhone} aria-invalid={wiz.bad("formats.businessPhone")} /></label>
  <label>Cost center<input type="text" bind:value={p.formats.costCenter} aria-invalid={wiz.bad("formats.costCenter")} /></label>
  <label>Street names ({p.formats.streets.length})<textarea rows="4" aria-invalid={wiz.bad("formats.streets")} value={joinList(p.formats.streets)} onchange={(e) => (p.formats.streets = splitList(e.currentTarget.value))}></textarea></label>
</div>
<p class="muted">The groups and the demo apps come from the profile you started with. To change them, download the profile, edit the file, and load it again.</p>

<style>
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: 12px 16px; }
  label { display: grid; gap: 4px; font-weight: 500; align-content: start; }
  textarea { font: inherit; font-size: 13px; padding: 6px 8px; color: var(--text); background: var(--surface); border: 1px solid var(--line-strong); border-radius: var(--radius); resize: vertical; }
</style>
