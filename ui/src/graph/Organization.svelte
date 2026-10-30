<script lang="ts">
  import { poll } from "../lib/poll";
  import { route } from "../lib/route.svelte";
  import { dateTime } from "../lib/format";
  import Avatar from "../lib/Avatar.svelte";
  import Chip from "../lib/Chip.svelte";
  import { getOrganization } from "./directory";
  import TenantId from "./TenantId.svelte";
  import type { Organization } from "./types";

  let org = $state.raw<Organization | null>(null);
  let counts = $state.raw<{ users: number; groups: number; servicePrincipals: number } | null>(null);

  $effect(() =>
    poll(async () => {
      const r = await getOrganization();
      org = r.org;
      counts = r;
    }, 4000),
  );

  const place = $derived([org?.city, org?.country].filter(Boolean).join(", "));
</script>

{#if org}
  <header>
    <Avatar name={org.displayName} size={48} square />
    <div>
      <h2>{org.displayName}</h2>
      <p class="muted">{org.tenantType === "AAD" ? "A Microsoft Entra ID tenant" : org.tenantType} · what <code>/organization</code> gives to a client</p>
    </div>
  </header>

  <div class="settings"><TenantId /></div>

  <section>
    <dl>
      {#if place}<dt>Place</dt><dd>{place}{org.countryLetterCode ? ` (${org.countryLetterCode})` : ""}</dd>{/if}
      {#if org.preferredLanguage}<dt>Language</dt><dd>{org.preferredLanguage}</dd>{/if}
      {#if org.createdDateTime}<dt>Created</dt><dd>{dateTime(org.createdDateTime)}</dd>{/if}
    </dl>
  </section>

  {#if counts}
    <section class="counts">
      <a href={route.href("graph", "users")}><span class="n">{counts.users}</span> users</a>
      <a href={route.href("graph", "groups")}><span class="n">{counts.groups}</span> groups</a>
      <a href={route.href("graph", "service-principals")}><span class="n">{counts.servicePrincipals}</span> service principals</a>
    </section>
  {/if}

  <section>
    <h3>Verified domains <span class="faint">{org.verifiedDomains.length}</span></h3>
    <table class="t fixed">
      <colgroup><col /><col style="width: 160px" /><col style="width: 40%" /></colgroup>
      <thead><tr><th>Domain</th><th></th><th>Can be used for</th></tr></thead>
      <tbody>
        {#each org.verifiedDomains as d (d.name)}
          <tr>
            <td class="mono cut"><div class="truncate">{d.name}</div></td>
            <td>
              {#if d.isDefault}<Chip tone="primary" title="The default domain of new users">default</Chip>{/if}
              {#if d.isInitial}<Chip title="The domain that the tenant started with">initial</Chip>{/if}
            </td>
            <td class="muted cut"><div class="truncate" title={d.capabilities}>{d.capabilities}</div></td>
          </tr>
        {/each}
      </tbody>
    </table>
  </section>
{/if}

<style>
  header { display: flex; align-items: center; gap: 16px; margin-bottom: 16px; }
  header p { margin-top: 2px; }
  .settings { margin-bottom: 16px; }
  section { margin-bottom: 28px; }
  h3 { margin-bottom: 6px; }
  h3 .faint { font-weight: 400; }
  dl { display: grid; grid-template-columns: 110px minmax(0, 1fr); gap: 4px 16px; margin: 0; }
  dt { color: var(--muted); }
  dd { margin: 0; }
  .counts { display: flex; gap: 32px; }
  .counts a { color: var(--muted); text-decoration: none; display: flex; align-items: baseline; gap: 6px; }
  .counts a:hover { color: var(--text); }
  .n { font-size: 24px; font-weight: 600; letter-spacing: -0.02em; color: var(--text); font-variant-numeric: tabular-nums; }
</style>
