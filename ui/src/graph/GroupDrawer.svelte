<script lang="ts">
  import { succeeded } from "../lib/notice.svelte";
  import { poll } from "../lib/poll";
  import { route } from "../lib/route.svelte";
  import { dateTime } from "../lib/format";
  import Avatar from "../lib/Avatar.svelte";
  import Chip from "../lib/Chip.svelte";
  import CopyButton from "../lib/CopyButton.svelte";
  import Drawer from "../lib/Drawer.svelte";
  import Icon from "../lib/Icon.svelte";
  import { addMember, getGroup, groupMembers, listUsers, removeMember, saveRule as putRule } from "./directory";
  import type { GroupDetail, GroupKind, Member, UserInfo } from "./types";

  let { id, onclose }: { id: string; onclose: () => void } = $props();

  const LIMIT = 200;
  let group = $state.raw<GroupDetail | null>(null);
  let members = $state.raw<{ members: Member[]; total: number } | null>(null);

  // the rule is edited here; a poll must not take the text away while a person types
  let rule = $state("");
  let processing = $state<"On" | "Paused">("On");
  let ruleTouched = $state(false);

  async function load() {
    const [g, m] = await Promise.all([getGroup(id), groupMembers(id, LIMIT)]);
    group = g;
    members = m;
    if (!ruleTouched) {
      rule = g.rule ?? "";
      processing = g.state ?? "On";
    }
  }
  // The page makes a new drawer for each group ({#key}), so nothing of another group is left in this one.
  $effect(() => poll(load, 3000));

  /** change does an action, and reads the group again when it worked. */
  async function change(action: () => Promise<unknown>, done: string) {
    if (await succeeded(action, done)) await load();
  }

  const saveRule = async () => {
    if (await succeeded(() => putRule(id, rule, processing), "The rule is saved.")) {
      ruleTouched = false;
      await load();
    }
  };
  const remove = (m: Member) => change(() => removeMember(id, m.id), `Removed ${m.name}.`);
  const add = (u: UserInfo) => change(() => addMember(id, u.id), `Added ${u.displayName}.`);

  // ---- a user to add ----
  let query = $state("");
  let found = $state.raw<UserInfo[]>([]);
  $effect(() => {
    const text = query.trim();
    if (!text) {
      found = [];
      return;
    }
    let stale = false;
    const timer = setTimeout(async () => {
      const r = await listUsers(text, 8);
      if (!stale) found = r.users;
    }, 200);
    return () => {
      stale = true;
      clearTimeout(timer);
    };
  });
  const inGroup = $derived(new Set((members?.members ?? []).map((m) => m.id.toLowerCase())));

  const kinds: Record<GroupKind, string> = { assigned: "Assigned", dynamic: "Dynamic", microsoft365: "Microsoft 365" };
  const href = (m: Member) => (m.type === "user" ? route.href("graph", "users", m.id) : m.type === "group" ? route.href("graph", "groups", m.id) : undefined);
  const flags = $derived(
    group ? [group.securityEnabled && "Security", group.mailEnabled && "Mail enabled", group.roleAssignable && "Can have roles"].filter((f): f is string => !!f) : [],
  );
</script>

<Drawer title={group?.name ?? "Group"} {onclose}>
  {#snippet subtitle()}
    {#if group}
      <Chip tone={group.kind === "dynamic" ? "primary" : ""}>{kinds[group.kind]}</Chip>
      {#if group.state === "Paused"}<Chip tone="amber">Paused</Chip>{/if}
      <code>{group.id}</code><CopyButton text={group.id} title="Copy the object ID" />
    {/if}
  {/snippet}

  {#if group}
    <section>
      <dl>
        {#if group.description}<dt>Description</dt><dd>{group.description}</dd>{/if}
        {#if group.mailNickname}<dt>Mail nickname</dt><dd class="mono">{group.mailNickname}</dd>{/if}
        {#if group.created}<dt>Created</dt><dd>{dateTime(group.created)}</dd>{/if}
        {#if flags.length}<dt>Flags</dt><dd class="flags">{#each flags as f (f)}<Chip>{f}</Chip>{/each}</dd>{/if}
        <dt>Owners</dt>
        <dd>
          {#each group.owners as o (o.id)}<a class="inline" href={route.href("graph", "users", o.id)}>{o.name}</a>{" "}{:else}<span class="faint">None</span>{/each}
        </dd>
        {#if group.memberOf.length}
          <dt>In groups</dt>
          <dd>{#each group.memberOf as p (p.id)}<a class="inline" href={route.href("graph", "groups", p.id)}>{p.name}</a>{" "}{/each}</dd>
        {/if}
      </dl>
    </section>

    {#if group.dynamic}
      <section>
        <h3>Membership rule</h3>
        <p class="hint">The members come from this rule. Dolly works them out again when the rule or a user changes. A rule that Entra would refuse is refused here too.</p>
        <textarea bind:value={rule} oninput={() => (ruleTouched = true)} rows="3" spellcheck="false" aria-label="Membership rule" class="mono"></textarea>
        <div class="actions">
          <label>
            <span class="sr-only">Processing</span>
            <select bind:value={processing} onchange={() => (ruleTouched = true)}>
              <option value="On">Processing is on</option>
              <option value="Paused">Processing is paused: keep the members</option>
            </select>
          </label>
          <button type="button" class="btn primary" disabled={!ruleTouched} onclick={saveRule}>Save the rule</button>
        </div>
      </section>
    {/if}

    <section>
      <h3>Members <span class="faint">{members?.total ?? group.users + group.groups + group.others}</span></h3>
      {#if !group.dynamic}
        <label class="add">
          <Icon name="search" />
          <span class="sr-only">Add a user</span>
          <input type="search" placeholder="Add a user: search by name" bind:value={query} />
        </label>
        {#if found.length}
          <ul class="results">
            {#each found as u (u.id)}
              {@const has = inGroup.has(u.id.toLowerCase())}
              <li>
                <Avatar name={u.displayName} size={24} />
                <span class="truncate">{u.displayName} <span class="muted">{u.userPrincipalName}</span></span>
                <button type="button" class="btn" disabled={has} onclick={() => add(u)}>{#if has}<Icon name="check" /> Member{:else}Add{/if}</button>
              </li>
            {/each}
          </ul>
        {/if}
      {/if}

      {#if members}
        <ul class="members">
          {#each members.members as m (m.id)}
            {@const link = href(m)}
            <li>
              <Avatar name={m.name} size={24} square={m.type !== "user"} />
              {#if link}<a class="truncate" href={link}>{m.name}</a>{:else}<span class="truncate">{m.name}</span>{/if}
              <span class="muted truncate detail">{m.detail ?? ""}</span>
              {#if m.type !== "user"}<Chip>{m.type === "group" ? "group" : "service principal"}</Chip>{/if}
              {#if !group.dynamic}
                <button type="button" class="btn icon ghost danger hover" title="Remove {m.name}" onclick={() => remove(m)}><Icon name="trash" /></button>
              {/if}
            </li>
          {:else}
            <li class="faint none">No members.</li>
          {/each}
        </ul>
        {#if members.total > members.members.length}
          <p class="muted">Showing {members.members.length} of {members.total}.</p>
        {/if}
      {/if}
    </section>
  {/if}
</Drawer>

<style>
  section { margin-top: 20px; }
  h3 .faint { font-weight: 400; }
  .hint { margin: 2px 0 8px; color: var(--muted); }
  dl { display: grid; grid-template-columns: 110px minmax(0, 1fr); gap: 4px 16px; margin: 6px 0 0; }
  dt { color: var(--muted); }
  dd { margin: 0; overflow-wrap: anywhere; }
  .flags { display: flex; gap: 4px; flex-wrap: wrap; }
  .inline { color: var(--primary); text-decoration: none; }
  .inline:hover { text-decoration: underline; }

  textarea {
    width: 100%; padding: 8px; font-size: 12.5px; color: var(--text); background: var(--surface);
    border: 1px solid var(--line-strong); border-radius: var(--radius); resize: vertical;
  }
  .actions { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-top: 8px; }

  .add { position: relative; display: block; margin: 8px 0; }
  .add :global(svg) { position: absolute; left: 8px; top: 6px; color: var(--faint); pointer-events: none; }
  .add input { width: 100%; padding-left: 28px; }

  ul { margin: 0; padding: 0; list-style: none; }
  .results { margin-bottom: 8px; border: 1px solid var(--line); border-radius: var(--radius); }
  li { display: flex; align-items: center; gap: 10px; min-height: 36px; padding: 2px 8px; }
  li + li { border-top: 1px solid var(--line); }
  .results li > span { flex: 1; min-width: 0; }
  .members { border: 1px solid var(--line); border-radius: var(--radius); }
  .members li:hover { background: var(--sunken); }
  .members a { color: var(--text); text-decoration: none; font-weight: 500; }
  .members a:hover { text-decoration: underline; }
  .detail { flex: 1; min-width: 0; }
  .members li > :global(.chip) { margin-left: auto; }
  .none { justify-content: center; }
  .hover { opacity: 0; }
  li:hover .hover, li:focus-within .hover, .hover:focus-visible { opacity: 1; }
</style>
