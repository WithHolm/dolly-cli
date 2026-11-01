<script lang="ts">
  import { countries, countryByCode, locales, nameSets, namesForLocale } from "./countries";
  import { joinList, splitList, type Profile } from "./profile";
  import { addTo, canAdd, removeFrom } from "./shares.svelte";
  import ShareBar from "./ShareBar.svelte";
  import { wiz } from "./state.svelte";
  import WeightBar from "./WeightBar.svelte";

  const p = $derived(wiz.profile as Profile);
  const pe = $derived(p.people);

  const shares: ["guestPercent" | "syncedPercent" | "enabledPercent" | "contractorPercent", string][] = [
    ["guestPercent", "Guests"],
    ["syncedPercent", "Synced from on-premises (of the members)"],
    ["enabledPercent", "Enabled accounts"],
    ["contractorPercent", "Contractors"],
  ];

  const domains = $derived(pe.guestDomains ?? []); // the wizard makes sure it is there (normalize)
  const known = (code: string) => locales.some((l) => l.code === code);
  const labelOf = (code: string) => locales.find((l) => l.code === code)?.label ?? code;

  // Names are kept by language. A person gets a language by the shares of the languages, and the names follow it.
  const sets = $derived(p.names.byLanguage ?? {});
  const nameCountries = countries.filter((c) => nameSets[c.code]);
  let open = $state<Record<string, boolean>>({});

  function ensure(code: string) {
    p.names.byLanguage ??= {};
    return (p.names.byLanguage[code] ??= { first: [], last: [] });
  }

  const same = (a: string[], b: string[]) => a.length === b.length && a.every((x, i) => x === b[i]);
  /** The country whose list the names of a language are, if they are one. */
  const fromCountry = (code: string) => {
    const s = sets[code];
    return s ? (nameCountries.find((c) => same(nameSets[c.code].first, s.first) && same(nameSets[c.code].last, s.last))?.code ?? "") : "";
  };

  function fill(code: string, country: string) {
    const set = nameSets[country];
    if (!set) return;
    const target = ensure(code);
    target.first = [...set.first];
    target.last = [...set.last];
  }

  function changeLanguage(i: number, code: string) {
    const old = pe.languages[i].code;
    if (code === old) return;
    const oldNames = sets[old];
    const defaults = namesForLocale(old);
    const wasDefault = !oldNames || (defaults && same(defaults.first, oldNames.first) && same(defaults.last, oldNames.last));
    pe.languages[i].code = code;
    p.names.byLanguage ??= {};
    delete p.names.byLanguage[old];
    // names that came from the country follow the new language. Names that the person wrote stay.
    p.names.byLanguage[code] = wasDefault ? (namesForLocale(code) ?? oldNames ?? { first: [], last: [] }) : oldNames;
  }

  function addLanguage() {
    const wanted = countryByCode(p.organization.countryLetterCode)?.locale;
    const free = locales.find((l) => l.code === wanted && !pe.languages.some((x) => x.code === l.code)) ?? locales.find((l) => !pe.languages.some((x) => x.code === l.code));
    if (!free) return;
    if (!addTo(pe.languages, { code: free.code })) return;
    const names = namesForLocale(free.code) ?? { first: [], last: [] };
    p.names.byLanguage ??= {};
    p.names.byLanguage[free.code] = names;
    open[free.code] = true;
  }

  function removeLanguage(i: number) {
    const code = pe.languages[i].code;
    if (p.names.byLanguage) delete p.names.byLanguage[code];
    removeFrom(pe.languages, i);
  }
</script>

<h3>People</h3>

<fieldset>
  <legend>Shares</legend>
  <div class="rows">
    {#each shares as [key, label] (key)}
      <div class="row">
        <span class="label">{label}</span>
        <WeightBar value={pe[key]} scale={100} min={0} max={100} unit="%" label={label} onchange={(n) => (pe[key] = n)} />
      </div>
    {/each}
  </div>
</fieldset>

<fieldset>
  <legend>Guest domains</legend>
  <p class="muted">Where the mail of a guest is. A guest signs in with an address at the initial domain, and the mail is at one of these. The shares add up to 100%.</p>
  <div class="rows">
    {#each domains as d, i (i)}
      <div class="row">
        <input type="text" placeholder="partner.example" aria-label="Guest domain {i + 1}" bind:value={d.name} aria-invalid={wiz.bad("people.guestDomains")} />
        {#if domains.length > 1}<ShareBar bind:items={p.people.guestDomains!} {i} label="Share of guests at {d.name || 'guest domain ' + (i + 1)}" />{:else}<span></span>{/if}
        <button class="btn ghost" onclick={() => removeFrom(domains, i)} aria-label="Remove guest domain {d.name || i + 1}" disabled={domains.length < 2}>Remove</button>
      </div>
    {/each}
  </div>
  <div><button class="btn" onclick={() => addTo(domains, { name: "" })} disabled={!canAdd(domains)} title={canAdd(domains) ? undefined : "Every share is locked. Unlock one to make room."}>+ Domain</button></div>
</fieldset>

<fieldset>
  <legend>Languages and names</legend>
  <p class="muted">A person gets a language by these shares, and a name from the list of that language. With Norwegian 85% and English 15%, about 85 of 100 people have a Norwegian name. The shares add up to 100%.</p>
  <div class="langs">
    {#each pe.languages as l, i (i)}
      {@const names = sets[l.code] ?? { first: [], last: [] }}
      <div class="lang" role="group" aria-label="Language {i + 1}, {labelOf(l.code)}">
        <div class="row">
          <select aria-label="Language {i + 1}" value={l.code} onchange={(e) => changeLanguage(i, e.currentTarget.value)} aria-invalid={wiz.bad("people.languages")}>
            {#if !known(l.code)}<option value={l.code}>{l.code || "Choose a language"}</option>{/if}
            {#each locales as c (c.code)}<option value={c.code} disabled={c.code !== l.code && pe.languages.some((x) => x.code === c.code)}>{c.label}</option>{/each}
          </select>
          <ShareBar bind:items={p.people.languages} {i} label="Share of people who speak {labelOf(l.code)}" />
          <button class="btn ghost" onclick={() => removeLanguage(i)} aria-label="Remove language {labelOf(l.code)}" disabled={pe.languages.length < 2}>Remove</button>
        </div>
        <button class="toggle" aria-expanded={!!open[l.code]} onclick={() => (open[l.code] = !open[l.code])}>
          <span class="chev" class:turned={open[l.code]}>›</span> Names ({names.first.length} first, {names.last.length} last)
        </button>
        {#if open[l.code]}
          <div class="names">
            <label class="pick">Made-up names from
              <select value={fromCountry(l.code)} onchange={(e) => fill(l.code, e.currentTarget.value)}>
                {#if !fromCountry(l.code)}<option value="">Your own list</option>{/if}
                {#each nameCountries as c (c.code)}<option value={c.code}>{c.name}</option>{/each}
              </select>
            </label>
            <div class="two">
              <label>First names ({names.first.length})<textarea rows="7" aria-invalid={wiz.bad(`names.byLanguage[${l.code}]`)} value={joinList(names.first)} onchange={(e) => (ensure(l.code).first = splitList(e.currentTarget.value))}></textarea></label>
              <label>Last names ({names.last.length})<textarea rows="7" aria-invalid={wiz.bad(`names.byLanguage[${l.code}]`)} value={joinList(names.last)} onchange={(e) => (ensure(l.code).last = splitList(e.currentTarget.value))}></textarea></label>
            </div>
            <p class="muted">One name on a line, or several with commas. Accents stay in display names, and addresses use plain letters.</p>
          </div>
        {/if}
      </div>
    {/each}
  </div>
  <div><button class="btn" onclick={addLanguage} disabled={pe.languages.length >= locales.length || !canAdd(pe.languages)} title={canAdd(pe.languages) ? undefined : "Every share is locked. Unlock one to make room."}>+ Language</button></div>
</fieldset>

<style>
  fieldset { display: grid; gap: 10px; margin: 0; padding: 0; border: 0; min-width: 0; }
  legend { padding: 0; margin-bottom: 8px; font-weight: 600; }
  .rows, .langs { display: grid; gap: 6px; }
  .row { display: grid; grid-template-columns: minmax(200px, 300px) 1fr auto; gap: 12px; align-items: center; }
  .lang { display: grid; gap: 6px; padding: 10px 12px; border: 1px solid var(--line); border-radius: var(--radius); background: var(--surface); }
  .names { display: grid; gap: 10px; padding-left: 14px; border-left: 2px solid var(--line); }
  .two { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
  label { display: grid; gap: 4px; font-weight: 500; align-content: start; }
  .pick { max-width: 320px; }
  textarea { font: inherit; font-size: 13px; padding: 6px 8px; color: var(--text); background: var(--surface); border: 1px solid var(--line-strong); border-radius: var(--radius); resize: vertical; }
  .toggle { justify-self: start; display: inline-flex; gap: 6px; align-items: center; background: none; border: 0; padding: 2px 0; font: inherit; color: var(--muted); cursor: pointer; }
  .toggle:hover { color: var(--text); }
  .chev { display: inline-block; transition: transform 0.1s; }
  .chev.turned { transform: rotate(90deg); }
</style>
