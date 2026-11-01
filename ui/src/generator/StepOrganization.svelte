<script lang="ts">
  import { countries, countryByCode } from "./countries";
  import { applyDomainRows, domainRows, TENANT_SUFFIX, tenantOf, toShares, type DomainRow, type Profile } from "./profile";
  import { addTo, canAdd, removeFrom } from "./shares.svelte";
  import ShareBar from "./ShareBar.svelte";
  import { wiz } from "./state.svelte";

  // Two things are needed: the display name and the tenant name. The initial domain follows the tenant name, and the country sets the usage
  // location, the language and the phone code. Everything else can be changed under "More".

  const p = $derived(wiz.profile as Profile);
  const o = $derived(p.organization);

  // The domain rows are kept here (the first one is the initial domain), so a row does not move when another one becomes the default.
  let rows = $state<DomainRow[]>(shares(domainRows(wiz.profile!.organization)));
  function shares(list: DomainRow[]): DomainRow[] {
    const w = toShares(list.map((r) => r.weight));
    return list.map((r, k) => ({ ...r, weight: w[k] }));
  }
  let defaultName = $state(wiz.profile!.organization.domain);
  const tenant = $derived(tenantOf(rows[0].name));

  const write = () => applyDomainRows(o, rows, defaultName);

  const stripFake = (s: string) => s.replace(/\s*\(fake\)\s*$/i, "").trim();
  function setDisplayName(v: string) {
    // the company name on the users follows the display name, until it is changed by hand
    if (stripFake(o.displayName).startsWith(o.companyName)) o.companyName = stripFake(v);
    o.displayName = v;
  }

  function setTenant(v: string) {
    const name = v.toLowerCase().replace(/[^a-z0-9-]/g, "").slice(0, 63);
    const before = rows[0].name;
    rows[0].name = name + TENANT_SUFFIX;
    if (defaultName === before) defaultName = rows[0].name;
    write();
  }

  function setCountry(code: string) {
    const c = countryByCode(code);
    if (!c) return;
    const before = countryByCode(o.countryLetterCode);
    o.country = c.name;
    o.countryLetterCode = o.usageLocation = c.code;
    o.preferredLanguage = c.language;
    // the phone follows the country when it has the code of the old one
    if (before && o.phone.startsWith("+" + before.calling)) o.phone = "+" + c.calling + o.phone.slice(before.calling.length + 1);
    else if (!o.phone) o.phone = "+" + c.calling;
  }
</script>

<h3>Organization</h3>

<div class="grid">
  <label>Display name
    <input type="text" value={o.displayName} oninput={(e) => setDisplayName(e.currentTarget.value)} aria-invalid={wiz.bad("organization.displayName")} />
  </label>
  <div class="field">
    <label for="tenant" class="l">Tenant name</label>
    <span class="suffixed"><input id="tenant" type="text" aria-describedby="tenant-hint" value={tenant} oninput={(e) => setTenant(e.currentTarget.value)} aria-invalid={wiz.bad("organization.initialDomain")} /><span class="suffix">{TENANT_SUFFIX}</span></span>
    <span class="hint" id="tenant-hint">The initial domain. Letters, digits, and hyphens.</span>
  </div>
  <div class="field">
    <label for="country" class="l">Country and language</label>
    <select id="country" aria-describedby="country-hint" value={o.countryLetterCode} onchange={(e) => setCountry(e.currentTarget.value)} aria-invalid={wiz.bad("organization.countryLetterCode")}>
      {#if !countryByCode(o.countryLetterCode)}<option value={o.countryLetterCode}>{o.country} ({o.preferredLanguage})</option>{/if}
      {#each countries as c (c.code)}<option value={c.code}>{c.name} · {c.languageName}</option>{/each}
    </select>
    <span class="hint" id="country-hint">Sets the country code, the usage location, the language, and the phone code. You can change them under More.</span>
  </div>
</div>

<fieldset>
  <legend>Domains</legend>
  <div class="domains">
    {#each rows as r, i (i)}
      <div class="domain">
        <input type="radio" name="default-domain" aria-label="Default domain: {r.name}" checked={defaultName === r.name} onchange={() => { defaultName = r.name; write(); }} />
        {#if i === 0}
          <span class="fixed"><span class="mono">{r.name}</span> <span class="muted">initial</span></span>
        {:else}
          <input type="text" class="name" placeholder="acme.example" aria-label="Domain {i + 1}" value={r.name} aria-invalid={wiz.bad("organization.domains")}
            oninput={(e) => { const was = r.name; r.name = e.currentTarget.value.trim().toLowerCase(); if (defaultName === was) defaultName = r.name; write(); }} />
        {/if}
        {#if rows.length > 1}
          <ShareBar bind:items={rows} {i} after={write} label="Share of addresses at {r.name || 'domain ' + (i + 1)}" />
        {/if}
        {#if i > 0}
          <button class="btn ghost" aria-label="Remove domain {r.name || i + 1}" onclick={() => { if (defaultName === r.name) defaultName = rows[0].name; removeFrom(rows, i); write(); }}>Remove</button>
        {:else}
          <span class="remove-space"></span>
        {/if}
      </div>
    {/each}
  </div>
  <div><button class="btn" disabled={!canAdd(rows)} title={canAdd(rows) ? undefined : "Every share is locked. Unlock one to make room."} onclick={() => { addTo(rows, { name: "" }); write(); }}>+ Domain</button></div>
  <p class="muted">The radio marks the default domain. It is the domain of the owner and of the admin account. With more than one domain, the addresses of the users follow the shares, which add up to 100%.</p>
</fieldset>

<details>
  <summary>More: company, address, language, and phone</summary>
  <div class="grid more">
    <label>Company name (on the users)<input type="text" bind:value={o.companyName} aria-invalid={wiz.bad("organization.companyName")} /></label>
    <label>Street<input type="text" bind:value={o.street} aria-invalid={wiz.bad("organization.street")} /></label>
    <label>Postal code<input type="text" bind:value={o.postalCode} aria-invalid={wiz.bad("organization.postalCode")} /></label>
    <label>City<input type="text" bind:value={o.city} aria-invalid={wiz.bad("organization.city")} /></label>
    <label>State<input type="text" bind:value={o.state} aria-invalid={wiz.bad("organization.state")} /></label>
    <label>Country<input type="text" bind:value={o.country} aria-invalid={wiz.bad("organization.country")} /></label>
    <label>Country code<input type="text" bind:value={o.countryLetterCode} aria-invalid={wiz.bad("organization.countryLetterCode")} /></label>
    <label>Default usage location<input type="text" bind:value={o.usageLocation} aria-invalid={wiz.bad("organization.usageLocation")} /></label>
    <label>Language<input type="text" bind:value={o.preferredLanguage} aria-invalid={wiz.bad("organization.preferredLanguage")} /></label>
    <label>Phone<input type="text" bind:value={o.phone} aria-invalid={wiz.bad("organization.phone")} /></label>
    <label class="wide">Name of the profile<input type="text" bind:value={p.name} /></label>
  </div>
</details>

<style>
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: 12px 16px; }
  .more { margin-top: 12px; }
  .wide { grid-column: 1 / -1; }
  label { display: grid; grid-template-columns: minmax(0, 1fr); gap: 4px; font-weight: 500; align-content: start; }
  .field { display: grid; grid-template-columns: minmax(0, 1fr); gap: 4px; align-content: start; }
  .field .l { font-weight: 500; }
  .remove-space { width: 74px; }
  .hint { font-weight: 400; color: var(--muted); font-size: 12px; }
  .suffixed { display: flex; align-items: center; gap: 0; }
  .suffixed input { flex: 1; min-width: 0; border-top-right-radius: 0; border-bottom-right-radius: 0; }
  .suffix { height: 28px; display: inline-flex; align-items: center; padding: 0 8px; color: var(--muted); background: var(--sunken); border: 1px solid var(--line-strong); border-left: 0; border-radius: 0 var(--radius) var(--radius) 0; white-space: nowrap; font-weight: 400; }
  fieldset { display: grid; gap: 12px; margin: 0; padding: 0; border: 0; min-width: 0; }
  legend { padding: 0; margin-bottom: 8px; font-weight: 600; }
  .domains { display: grid; gap: 8px; }
  .domain { display: grid; grid-template-columns: 20px minmax(180px, 280px) 1fr auto; gap: 12px; align-items: center; }
  .domain .name { width: 100%; }
  .fixed { padding: 4px 0; }
  details { border-top: 1px solid var(--line); padding-top: 12px; }
  summary { cursor: pointer; font-weight: 500; color: var(--muted); }
  summary:hover { color: var(--text); }
</style>
