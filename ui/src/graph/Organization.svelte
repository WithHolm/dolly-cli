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

  // the address, as it is written on an envelope: street, then postal code and city, then state and country
  const address = $derived(
    org
      ? [org.street, [org.postalCode, org.city].filter(Boolean).join(" "), org.state, org.country && org.countryLetterCode ? `${org.country} (${org.countryLetterCode})` : (org.country ?? org.countryLetterCode)]
          .filter(Boolean)
          .join(", ")
      : "",
  );
  const list = (v?: string[]) => (v && v.length ? v.join(", ") : "");
  const number = new Intl.NumberFormat();
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
    <h3>Details</h3>
    <dl>
      {#if address}<dt>Address</dt><dd>{address}</dd>{/if}
      {#if list(org.businessPhones)}<dt>Phone</dt><dd>{list(org.businessPhones)}</dd>{/if}
      {#if org.preferredLanguage}<dt>Language</dt><dd>{org.preferredLanguage}</dd>{/if}
      {#if org.defaultUsageLocation}<dt>Usage location</dt><dd>{org.defaultUsageLocation} <span class="muted">(the default for new users)</span></dd>{/if}
      {#if org.createdDateTime}<dt>Created</dt><dd>{dateTime(org.createdDateTime)}</dd>{/if}
      {#if org.directorySizeQuota}
        <dt>Directory</dt><dd>{number.format(org.directorySizeQuota.used)} of {number.format(org.directorySizeQuota.total)} objects</dd>
      {/if}
      <dt>Sync</dt>
      <dd>
        {#if org.onPremisesSyncEnabled}
          From an on-premises directory{#if org.onPremisesLastSyncDateTime}, last on {dateTime(org.onPremisesLastSyncDateTime)}{/if}
        {:else}
          <span class="muted">Not synced from an on-premises directory</span>
        {/if}
      </dd>
      {#if org.partnerTenantType}<dt>Partner</dt><dd>{org.partnerTenantType}</dd>{/if}
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

  <section>
    <h3>Notifications and privacy</h3>
    <p class="muted hint">Who gets mail from Microsoft about the tenant. A tool can change these with <code>PATCH /organization/{org.id}</code>.</p>
    <dl>
      <dt>Technical</dt><dd>{list(org.technicalNotificationMails) || "None"}</dd>
      <dt>Marketing</dt><dd>{list(org.marketingNotificationEmails) || "None"}</dd>
      <dt>Security mail</dt><dd>{list(org.securityComplianceNotificationMails) || "None"}</dd>
      <dt>Security phone</dt><dd>{list(org.securityComplianceNotificationPhones) || "None"}</dd>
      <dt>Privacy contact</dt><dd>{org.privacyProfile?.contactEmail || "None"}</dd>
      <dt>Privacy statement</dt><dd>{org.privacyProfile?.statementUrl || "None"}</dd>
    </dl>
  </section>

  <section>
    <h3>Service plans</h3>
    <p class="muted hint">The plans of Microsoft 365 that the tenant has, with the IDs that a tool looks for.</p>
    <details>
      <summary>Assigned plans <span class="faint">{org.assignedPlans?.length ?? 0}</span></summary>
      <table class="t fixed">
        <colgroup><col style="width: 220px" /><col /><col style="width: 110px" /><col style="width: 180px" /></colgroup>
        <thead><tr><th>Service</th><th>Plan ID</th><th>Status</th><th>Assigned</th></tr></thead>
        <tbody>
          {#each org.assignedPlans ?? [] as p (p.servicePlanId)}
            <tr>
              <td>{p.service}</td>
              <td class="mono cut"><div class="truncate">{p.servicePlanId}</div></td>
              <td><Chip tone={p.capabilityStatus === "Enabled" ? "green" : undefined}>{p.capabilityStatus}</Chip></td>
              <td class="muted">{dateTime(p.assignedDateTime)}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </details>
    <details>
      <summary>Provisioned plans <span class="faint">{org.provisionedPlans?.length ?? 0}</span></summary>
      <table class="t fixed">
        <colgroup><col /><col style="width: 140px" /><col style="width: 160px" /></colgroup>
        <thead><tr><th>Service</th><th>Status</th><th>Provisioning</th></tr></thead>
        <tbody>
          {#each org.provisionedPlans ?? [] as p, i (p.service + i)}
            <tr>
              <td>{p.service}</td>
              <td><Chip tone={p.capabilityStatus === "Enabled" ? "green" : undefined}>{p.capabilityStatus}</Chip></td>
              <td class="muted">{p.provisioningStatus}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </details>
  </section>
{/if}

<style>
  header { display: flex; align-items: center; gap: 16px; margin-bottom: 16px; }
  header p { margin-top: 2px; }
  .settings { margin-bottom: 16px; }
  section { margin-bottom: 28px; }
  h3 { margin-bottom: 6px; }
  h3 .faint { font-weight: 400; }
  dl { display: grid; grid-template-columns: 140px minmax(0, 1fr); gap: 4px 16px; margin: 0; }
  dt { color: var(--muted); }
  dd { margin: 0; overflow-wrap: anywhere; }
  .hint { margin: 0 0 8px; max-width: 80ch; }
  details { border-top: 1px solid var(--line); padding: 8px 0; }
  details table { margin-top: 8px; }
  summary { cursor: pointer; font-weight: 500; }
  .counts { display: flex; gap: 32px; }
  .counts a { color: var(--muted); text-decoration: none; display: flex; align-items: baseline; gap: 6px; }
  .counts a:hover { color: var(--text); }
  .n { font-size: 24px; font-weight: 600; letter-spacing: -0.02em; color: var(--text); font-variant-numeric: tabular-nums; }
</style>
