<script lang="ts">
  import { countries, countryByCode, countryByName } from "./countries";
  import type { Profile } from "./profile";
  import { addTo, canAdd, removeFrom } from "./shares.svelte";
  import ShareBar from "./ShareBar.svelte";
  import { wiz } from "./state.svelte";

  const p = $derived(wiz.profile as Profile);

  // The country sets the usage location and the calling code of the office. They can be changed after.
  function setCountry(o: Profile["offices"][number], name: string) {
    const c = countryByName(name);
    o.country = name;
    if (c) [o.usage, o.callingCode] = [c.code, c.calling];
  }

  const add = () => {
    const c = countryByCode(p.organization.countryLetterCode);
    addTo(p.offices, { name: "", city: "", country: p.organization.country, usage: c?.code ?? p.organization.usageLocation, postal: "", callingCode: c?.calling ?? p.offices[0]?.callingCode ?? "" });
  };

  const nameOf = (o: Profile["offices"][number], i: number) => o.name || `Office ${i + 1}`;
</script>

<h3>Offices</h3>
<p class="muted">Define the offices here, and set how many people work in each one in the shares below. The first two offices are the buildings, with floors and rooms.</p>

<div class="offices">
  {#each p.offices as o, i (i)}
    <div class="office" role="group" aria-label="Office {i + 1}{o.name ? ', ' + o.name : ''}">
      <div class="line">
        <label>Name<input type="text" bind:value={o.name} aria-invalid={wiz.bad(`offices[${i}].name`)} /></label>
        <label>City<input type="text" bind:value={o.city} aria-invalid={wiz.bad(`offices[${i}].city`)} /></label>
        <label>Country
          <select value={o.country} onchange={(e) => setCountry(o, e.currentTarget.value)} aria-invalid={wiz.bad(`offices[${i}].country`)}>
            {#if !countryByName(o.country)}<option value={o.country}>{o.country || "Choose a country"}</option>{/if}
            {#each countries as c (c.code)}<option value={c.name}>{c.name}</option>{/each}
          </select>
        </label>
        <label class="s">Postal code<input type="text" bind:value={o.postal} aria-invalid={wiz.bad(`offices[${i}].postal`)} /></label>
        <label class="s">Usage location<input type="text" bind:value={o.usage} aria-invalid={wiz.bad(`offices[${i}].usage`)} /></label>
        <label class="s">Calling code +<input type="text" bind:value={o.callingCode} aria-invalid={wiz.bad(`offices[${i}].callingCode`)} /></label>
        <button class="btn ghost" onclick={() => removeFrom(p.offices, i)} disabled={p.offices.length < 3} aria-label="Remove office {i + 1}{o.name ? ', ' + o.name : ''}">Remove</button>
      </div>
    </div>
  {/each}
</div>
<div><button class="btn" onclick={add} disabled={!canAdd(p.offices)} title={canAdd(p.offices) ? undefined : "Every share is locked. Unlock one to make room."}>+ Office</button></div>

<fieldset>
  <legend>Share of people in each office</legend>
  <p class="muted">The shares add up to 100%. When you raise one, all the others give the same amount. The lock keeps a share as it is.</p>
  <div class="shares">
    {#each p.offices as o, i (i)}
      <div class="share">
        <span class="name" title={nameOf(o, i)}>{nameOf(o, i)}</span>
        <ShareBar bind:items={p.offices} {i} label="Share of people in office {i + 1}{o.name ? ', ' + o.name : ''}" />
      </div>
    {/each}
  </div>
</fieldset>

<style>
  .offices { display: grid; gap: 10px; }
  .office { padding: 12px; border: 1px solid var(--line); border-radius: var(--radius); background: var(--surface); }
  .line { display: flex; gap: 12px; align-items: end; flex-wrap: wrap; }
  .line label { display: grid; grid-template-columns: minmax(0, 1fr); gap: 4px; font-weight: 500; flex: 1; min-width: 130px; }
  .line label.s { flex: none; min-width: 0; width: 104px; }
  .line input, .line select { width: 100%; min-width: 0; }
  fieldset { display: grid; gap: 10px; margin: 0; padding: 0; border: 0; min-width: 0; }
  legend { padding: 0; margin-bottom: 8px; font-weight: 600; }
  .shares { display: grid; gap: 6px; }
  .share { display: grid; grid-template-columns: minmax(160px, 260px) 1fr; gap: 12px; align-items: center; }
  .share .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
