<script lang="ts">
  import { graphPatch } from "../lib/graphApi";
  import { succeeded } from "../lib/notice.svelte";
  import { poll } from "../lib/poll";
  import { route } from "../lib/route.svelte";
  import Avatar from "../lib/Avatar.svelte";
  import Chip from "../lib/Chip.svelte";
  import CopyButton from "../lib/CopyButton.svelte";
  import Dialog from "../lib/Dialog.svelte";
  import Tabs from "../lib/Tabs.svelte";
  import OrgTree from "./OrgTree.svelte";
  import { getUser, userApps } from "./directory";
  import { humanize } from "./reference";
  import type { GroupKind, UserApps, UserDetail } from "./types";

  // The details of one user, in a dialog with tabs: the properties (and the job, which can be changed), the organisation as a tree
  // (the managers, the user, the reports), the groups, and the apps (assigned, and consented to). `id` and `tab` are in the address, so
  // a link opens the dialog on a tab. `navigate` changes the address: another user (from the tree), or another tab.
  let {
    id,
    tab: wanted,
    onclose,
    navigate,
  }: { id: string; tab?: string; onclose: () => void; navigate: (id: string, tab: string) => void } = $props();

  const TABS = [
    { id: "properties", label: "Properties" },
    { id: "organisation", label: "Organisation" },
    { id: "groups", label: "Groups" },
    { id: "apps", label: "Enterprise applications" },
  ];
  const tab = $derived(TABS.some((t) => t.id === wanted) ? wanted! : "properties");

  let user = $state.raw<UserDetail | null>(null);
  let apps = $state.raw<UserApps | null>(null);
  let editing = $state(false);
  let draft = $state({ department: "", jobTitle: "", officeLocation: "", accountEnabled: true });

  // The dialog stays when the person goes to another user (a click in the tree), so the data is read again for the new id. An answer
  // for an earlier id must not replace the data of this one.
  $effect(() => {
    const target = id;
    user = apps = null;
    editing = false;
    return poll(async () => {
      const u = await getUser(target);
      const a = await userApps(target, u.groups);
      if (target === id) [user, apps] = [u, a];
    }, 3000);
  });

  function edit() {
    if (!user) return;
    draft = { department: user.department, jobTitle: user.jobTitle, officeLocation: user.officeLocation, accountEnabled: user.accountEnabled };
    editing = true;
  }

  async function save() {
    if (!user) return;
    const current = user as unknown as Record<string, unknown>;
    const set = Object.fromEntries(Object.entries(draft).filter(([k, v]) => v !== current[k]));
    if (!Object.keys(set).length) {
      editing = false;
      return;
    }
    if (await succeeded(() => graphPatch(`/users/${encodeURIComponent(user!.id)}`, set), "Saved.")) {
      editing = false;
      user = await getUser(id);
    }
  }

  // ---- the properties, in sections ----
  const SECTIONS = [
    { title: "Identity", names: ["givenName", "surname", "mail", "mailNickname", "employeeId", "companyName", "userType", "employeeHireDate"] },
    { title: "Phone and language", names: ["mobilePhone", "faxNumber", "preferredLanguage"] },
    { title: "Address", names: ["streetAddress", "city", "state", "postalCode", "country", "usageLocation"] },
  ];
  const IN_HEADER = ["displayName", "userPrincipalName"];
  const sections = $derived.by(() => {
    const fields = Object.entries(user?.fields ?? {}).filter(([, v]) => v);
    const used = new Set([...IN_HEADER, ...SECTIONS.flatMap((s) => s.names)]);
    const out = SECTIONS.map((s) => ({ title: s.title, rows: s.names.map((n) => [n, user?.fields[n] ?? ""] as const).filter(([, v]) => v) }));
    out.push({ title: "Extension attributes", rows: fields.filter(([n]) => n.startsWith("extensionAttribute")) });
    out.push({ title: "Other", rows: fields.filter(([n]) => !used.has(n) && !n.startsWith("extensionAttribute")) });
    return out.filter((s) => s.rows.length);
  });

  const kinds: Record<GroupKind, string> = { assigned: "Assigned", dynamic: "Dynamic", microsoft365: "Microsoft 365" };
  const appCount = $derived((apps?.assigned.length ?? 0) + (apps?.consents.length ?? 0));
  const counts = $derived<Record<string, number | undefined>>({ groups: user?.groups.length, apps: apps ? appCount : undefined });

  const SHOWN_SCOPES = 6;
</script>

<Dialog title={user?.displayName ?? "User"} {onclose}>
  {#snippet subtitle()}
    {#if user}
      <code>{user.userPrincipalName}</code><CopyButton text={user.userPrincipalName} title="Copy the user principal name" />
      {#if !user.accountEnabled}<Chip tone="amber" title="The account is disabled: the user cannot sign in">Disabled</Chip>{/if}
      {#if user.userType}<Chip>{user.userType}</Chip>{/if}
      <span class="faint">ID</span><code class="id">{user.id}</code><CopyButton text={user.id} title="Copy the object ID" />
    {/if}
  {/snippet}

  {#snippet tabs()}
    <Tabs
      items={TABS.map((t) => ({ ...t, count: counts[t.id] }))}
      selected={tab}
      onselect={(t) => navigate(id, t)}
      label="Details of the user"
      prefix="user"
    />
  {/snippet}

  {#if !user}
    <p class="muted loading">Loading…</p>
  {:else}
    <div role="tabpanel" id="userpanel-{tab}" aria-labelledby="usertab-{tab}" tabindex="-1">
      {#if tab === "properties"}
        <section>
          <div class="head">
            <h3>Job</h3>
            {#if !editing}<button type="button" class="btn" onclick={edit}>Edit</button>{/if}
          </div>
          {#if editing}
            <form onsubmit={(e) => (e.preventDefault(), save())}>
              <label>Department <input type="text" bind:value={draft.department} /></label>
              <label>Job title <input type="text" bind:value={draft.jobTitle} /></label>
              <label>Office <input type="text" bind:value={draft.officeLocation} /></label>
              <label class="switch"><input type="checkbox" bind:checked={draft.accountEnabled} /> The account is enabled</label>
              <div class="actions">
                <button type="submit" class="btn primary">Save</button>
                <button type="button" class="btn ghost" onclick={() => (editing = false)}>Cancel</button>
              </div>
            </form>
          {:else}
            <dl>
              <dt>Department</dt><dd>{user.department || "–"}</dd>
              <dt>Job title</dt><dd>{user.jobTitle || "–"}</dd>
              <dt>Office</dt><dd>{user.officeLocation || "–"}</dd>
            </dl>
          {/if}
        </section>
        {#each sections as s (s.title)}
          <section>
            <h3>{s.title}</h3>
            <dl>
              {#each s.rows as [name, value] (name)}
                <dt>{humanize(name)}</dt><dd class={s.title === "Identity" || s.title === "Address" || s.title === "Phone and language" ? "" : "mono"}>{value}</dd>
              {/each}
            </dl>
          </section>
        {/each}
      {:else if tab === "organisation"}
        <p class="hint">The managers above, this user, and the people who report to this user. Open a person with a click on the name. A person with reports has an arrow.</p>
        <OrgTree
          chain={user.chain}
          me={{ id: user.id, name: user.displayName, jobTitle: user.jobTitle }}
          reports={user.directReports}
          onopen={(pid) => navigate(pid, "organisation")}
        />
      {:else if tab === "groups"}
        <p class="hint">The groups that this user is a direct member of.</p>
        {#each user.groups as g (g.id)}
          <a class="row" href={route.href("graph", "groups", g.id)}>
            <Avatar name={g.name} size={24} square />
            <span class="truncate">{g.name}</span>
            <Chip tone={g.kind === "dynamic" ? "primary" : ""}>{kinds[g.kind]}</Chip>
          </a>
        {:else}
          <div class="empty"><strong>No groups</strong>This user is not a member of any group.</div>
        {/each}
      {:else if tab === "apps"}
        <section>
          <h3>Assigned <span class="faint">{apps?.assigned.length ?? 0}</span></h3>
          <p class="hint">Enterprise applications where this user has an app role. The role is given to the user, or to a group that the user is in.</p>
          {#each apps?.assigned ?? [] as a (a.appId + (a.via ?? ""))}
            <div class="row">
              <Avatar name={a.name} size={24} square />
              <span class="truncate">{a.name}</span>
              <span class="chips">
                {#each a.roles as r (r)}<Chip>{r}</Chip>{/each}
                {#if a.via}<Chip tone="primary" title="The role is given to this group">through {a.via}</Chip>{/if}
              </span>
            </div>
          {:else}
            <p class="faint none">This user has no app role, and neither has a group that the user is in.</p>
          {/each}
        </section>
        <section>
          <h3>Consents <span class="faint">{apps?.consents.length ?? 0}</span></h3>
          <p class="hint">Enterprise applications that may act for this user. They have the permissions that the user agreed to, and the ones that an administrator agreed to for everyone.</p>
          {#each apps?.consents ?? [] as c (c.appId + c.resource + c.forAll)}
            <div class="row">
              <Avatar name={c.name} size={24} square />
              <span class="truncate">{c.name} <span class="muted">to {c.resource}</span></span>
              <span class="chips">
                {#each c.scopes.slice(0, SHOWN_SCOPES) as s (s)}<Chip>{s}</Chip>{/each}
                {#if c.scopes.length > SHOWN_SCOPES}<Chip title={c.scopes.slice(SHOWN_SCOPES).join(", ")}>+{c.scopes.length - SHOWN_SCOPES}</Chip>{/if}
                {#if c.forAll}<Chip tone="amber" title="An administrator agreed for every user">all users</Chip>{/if}
              </span>
            </div>
          {:else}
            <p class="faint none">No consent applies to this user.</p>
          {/each}
        </section>
      {/if}
    </div>
  {/if}
</Dialog>

<style>
  [role="tabpanel"]:focus { outline: none; }

  .loading { padding: 24px 0; }
  .id { font-size: 12px; }
  section { margin-top: 16px; }
  .head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 4px; }
  h3 .faint { font-weight: 400; }
  .hint { margin: 8px 0 4px; color: var(--muted); max-width: 70ch; }
  section .hint { margin: 0 0 6px; }
  dl { display: grid; grid-template-columns: 160px minmax(0, 1fr); gap: 4px 16px; margin: 6px 0 0; }
  dt { color: var(--muted); }
  dd { margin: 0; overflow-wrap: anywhere; }

  form { display: grid; gap: 8px; max-width: 360px; margin-top: 8px; }
  form label:not(.switch) { display: grid; gap: 2px; color: var(--muted); }
  form input[type="text"] { width: 100%; }
  .actions { display: flex; gap: 6px; margin-top: 4px; }

  .row {
    display: flex; align-items: center; gap: 10px; min-height: 36px; padding: 4px 8px; margin: 0 -8px; border-radius: var(--radius);
    color: var(--text); text-decoration: none;
  }
  a.row:hover { background: var(--sunken); }
  .row .chips { margin-left: auto; display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 4px; }
  .none { margin: 6px 0; }
</style>
