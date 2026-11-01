<script lang="ts">
  import { graph } from "../lib/api";
  import { route } from "../lib/route.svelte";
  import { notices } from "../lib/notice.svelte";
  import ConfirmButton from "../lib/ConfirmButton.svelte";
  import { wiz } from "./state.svelte";

  let busy = $state(false);
  const preview = $derived(wiz.preview);
  const pct = (n: number) => (preview && preview.users ? Math.round((n / preview.users) * 100) : 0);

  function download() {
    const url = URL.createObjectURL(new Blob([JSON.stringify(wiz.profile, null, 2) + "\n"], { type: "application/json" }));
    const name = (wiz.profile?.organization.companyName || "profile").toLowerCase().replace(/[^a-z0-9]+/g, "-");
    Object.assign(document.createElement("a"), { href: url, download: `${name}.json` }).click();
    URL.revokeObjectURL(url);
  }

  async function regenerate() {
    busy = true;
    try {
      const st = await graph.post<{ users: number; groups: number; servicePrincipals: number }>("/generate", wiz.body());
      notices.info(`dollycli made ${st.users} users, ${st.groups} groups and ${st.servicePrincipals} service principals from this profile.`);
      route.go("graph", "users");
    } catch (e) {
      notices.error(e instanceof Error ? e.message : String(e));
    } finally {
      busy = false;
    }
  }
</script>

<h3>Preview and regenerate</h3>
<div class="grid">
  <label>Users<input type="text" inputmode="numeric" bind:value={wiz.users} /></label>
  <label>Seed<input type="text" inputmode="numeric" bind:value={wiz.seed} /><span class="hint">The same profile and seed make the same people. Change the seed for other people.</span></label>
</div>

{#if preview}
  <dl class="facts">
    <dt>Users</dt><dd>{preview.users}</dd>
    <dt>Groups</dt><dd>{preview.groups}</dd>
    <dt>Service principals</dt><dd>{preview.servicePrincipals}</dd>
    <dt>Default domain</dt><dd>{preview.domain}</dd>
    <dt>Owner</dt><dd>{preview.owner}</dd>
    <dt>Admin account</dt><dd>{preview.adminAccount || "None"}</dd>
    <dt>Guests</dt><dd>{pct(preview.guests)}%</dd>
    <dt>Synced</dt><dd>{pct(preview.synced)}%</dd>
    <dt>Disabled</dt><dd>{pct(preview.disabled)}%</dd>
    <dt>Languages</dt><dd>{preview.languages.map((l) => `${l.value} ${pct(l.count)}%`).join(", ")}</dd>
  </dl>
  <p class="muted">The preview shows up to 2000 users and six people.</p>
  <div class="scroll">
    <table class="t">
      <thead><tr><th scope="col">Name</th><th scope="col">User principal name</th><th scope="col">Department</th><th scope="col">Job title</th><th scope="col">Office</th><th scope="col">Mobile</th><th scope="col">ID</th></tr></thead>
      <tbody>
        {#each preview.sample as u, i (i)}
          <tr><td>{u.displayName}</td><td class="mono">{u.userPrincipalName}</td><td>{u.department}</td><td>{u.jobTitle}</td><td>{u.officeLocation}</td><td>{u.mobilePhone}</td><td>{u.employeeId}</td></tr>
        {/each}
      </tbody>
    </table>
  </div>
{/if}

<div class="actions">
  <button class="btn" onclick={download}>Download the profile</button>
  {#if wiz.issues.length}
    <span class="muted">Fix the {wiz.issues.length === 1 ? "problem" : "problems"} above to regenerate.</span>
  {:else}
    <ConfirmButton plain onconfirm={regenerate} ask="Replace all the data?" title="The users, groups, service principals, and the history are replaced. This cannot be undone.">Regenerate using this profile</ConfirmButton>
  {/if}
</div>
<p class="muted">
  Regenerating replaces the users, groups, service principals, and the history. The data keeps this profile, so a restart, a reset, and the next
  regeneration use it too. A rollback cannot bring the old data back, and a delta token from before stops working.{busy ? " Working…" : ""}
</p>

<style>
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: 12px 16px; }
  label { display: grid; gap: 4px; font-weight: 500; align-content: start; }
  .hint { font-weight: 400; color: var(--muted); font-size: 12px; }
  .facts { display: grid; grid-template-columns: max-content 1fr; gap: 4px 16px; margin: 0; }
  .facts dt { color: var(--muted); }
  .facts dd { margin: 0; }
  .scroll { overflow-x: auto; }
  .actions { display: flex; gap: 8px; align-items: center; }
</style>
