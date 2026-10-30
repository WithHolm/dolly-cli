import { graphAll, graphBatch, graphGet, graphSend, type GraphList } from "../lib/graphApi";
import type { GroupDetail, GroupInfo, GroupKind, GroupRef, Member, Organization, Person, UserApp, UserApps, UserConsent, UserDetail, UserInfo } from "./types";

// What the Users page reads, through the Graph API (see lib/graphApi.ts). Each function gives the same shapes as before, so the
// components do not know where the data comes from.

const q = (s: string) => encodeURIComponent(s);
const DEFAULT_ACCESS = "00000000-0000-0000-0000-000000000000"; // an assignment with no app role

// ---- the list ----

const LIST_SELECT = "id,displayName,userPrincipalName,department,jobTitle,officeLocation,accountEnabled,userType";
// $search takes "property:text" clauses. These are the properties that the page finds a user by.
const FIND_BY = ["displayName", "userPrincipalName", "department", "jobTitle", "officeLocation"];

interface GraphUser {
  id: string;
  displayName: string | null;
  userPrincipalName: string | null;
  department: string | null;
  jobTitle: string | null;
  officeLocation: string | null;
  accountEnabled: boolean | null;
  userType: string | null;
}

const toInfo = (u: GraphUser): UserInfo => ({
  id: u.id,
  displayName: u.displayName ?? "",
  userPrincipalName: u.userPrincipalName ?? "",
  department: u.department ?? "",
  jobTitle: u.jobTitle ?? "",
  officeLocation: u.officeLocation ?? "",
  accountEnabled: u.accountEnabled ?? true,
  userType: u.userType ?? "",
  fields: {},
});

/** The users that match the text (the start of a name, user principal name, department, title, or office), and how many match. */
export async function listUsers(text: string, limit: number): Promise<{ users: UserInfo[]; total: number }> {
  const words = text.replace(/["\\]/g, " ").trim();
  const search = words ? `&$search=${q(FIND_BY.map((p) => `"${p}:${words}"`).join(" OR "))}` : "";
  const page = await graphGet<GraphList<GraphUser>>(`/users?$select=${LIST_SELECT}&$top=${limit}&$count=true${search}`, true);
  return { users: page.value.map(toInfo), total: page["@odata.count"] ?? page.value.length };
}

// ---- one user ----

// The properties of the Properties tab. A property that the user does not have is null in the answer, and is left out of the page.
const FIELD_NAMES = [
  "givenName", "surname", "mail", "mailNickname", "employeeId", "companyName", "employeeHireDate",
  "mobilePhone", "faxNumber", "preferredLanguage",
  "streetAddress", "city", "state", "postalCode", "country", "usageLocation",
];

type Manager = { id: string; displayName: string | null; jobTitle: string | null; manager?: Manager };

const person = (m: { id: string; displayName: string | null; jobTitle: string | null }, reports = 0): Person => ({
  id: m.id,
  name: m.displayName ?? "",
  jobTitle: m.jobTitle ?? "",
  reports,
});

/** The people who report to a user, each with how many people report to them (one batch of counts). */
export async function directReports(id: string): Promise<Person[]> {
  const list = await graphAll<{ id: string; displayName: string | null; jobTitle: string | null }>(
    `/users/${q(id)}/directReports?$select=id,displayName,jobTitle&$top=999`,
  );
  if (!list.length) return [];
  const counts = await graphBatch<number>(
    list.map((r) => ({ id: r.id, url: `/users/${r.id}/directReports/$count`, headers: { ConsistencyLevel: "eventual" } })),
  );
  return list.map((r, i) => person(r, counts[i].status === 200 ? Number(counts[i].body) : 0));
}

const kindOf = (g: { groupTypes?: string[]; membershipRule?: string | null }): GroupKind =>
  g.groupTypes?.includes("DynamicMembership") || g.membershipRule ? "dynamic" : g.groupTypes?.includes("Unified") ? "microsoft365" : "assigned";

/** The groups that the user is a direct member of. */
export async function userGroups(id: string): Promise<GroupRef[]> {
  const groups = await graphAll<{ id: string; displayName: string | null; groupTypes?: string[]; membershipRule?: string | null }>(
    `/users/${q(id)}/memberOf/microsoft.graph.group?$select=id,displayName,groupTypes,membershipRule&$top=999`,
  );
  return groups.map((g) => ({ id: g.id, name: g.displayName ?? "", kind: kindOf(g) }));
}

/** A user with the people around them (the managers, the reports) and the groups. */
export async function getUser(id: string): Promise<UserDetail> {
  const select = ["id", "displayName", "userPrincipalName", "department", "jobTitle", "officeLocation", "accountEnabled", "userType", ...FIELD_NAMES, "onPremisesExtensionAttributes"].join(",");
  const [u, reports, groups] = await Promise.all([
    graphGet<GraphUser & Record<string, unknown> & { manager?: Manager }>(
      `/users/${q(id)}?$select=${select}&$expand=manager($levels=max;$select=id,displayName,jobTitle)`,
      true,
    ),
    directReports(id),
    userGroups(id),
  ]);

  // $expand=manager($levels=max) nests the managers: the manager of the user, and above that, the manager of the manager
  const chain: Person[] = [];
  for (let m = u.manager; m; m = m.manager) chain.unshift(person(m));

  const fields: Record<string, string> = {};
  for (const name of FIELD_NAMES) if (typeof u[name] === "string") fields[name] = u[name] as string;
  for (const [name, value] of Object.entries((u.onPremisesExtensionAttributes as Record<string, string | null> | undefined) ?? {})) {
    if (value) fields[name] = value;
  }
  const manager = chain.at(-1);
  return {
    ...toInfo(u),
    fields,
    manager: manager ? { id: manager.id, name: manager.name } : null,
    chain,
    directReports: reports,
    groups,
  };
}

// ---- groups ----

interface GraphGroup {
  id: string;
  displayName: string | null;
  description?: string | null;
  mailNickname?: string | null;
  groupTypes?: string[];
  membershipRule?: string | null;
  membershipRuleProcessingState?: string | null;
  securityEnabled?: boolean | null;
  mailEnabled?: boolean | null;
  isAssignableToRole?: boolean | null;
  createdDateTime?: string | null;
}

const GROUP_SELECT = "id,displayName,groupTypes,membershipRule,membershipRuleProcessingState";

const toGroup = (g: GraphGroup): GroupInfo => {
  const kind = kindOf(g);
  return {
    id: g.id,
    name: g.displayName ?? "",
    kind,
    users: 0,
    groups: 0,
    others: 0,
    dynamic: kind === "dynamic",
    rule: g.membershipRule ?? undefined,
    state: g.membershipRuleProcessingState === "Paused" ? "Paused" : g.membershipRule ? "On" : undefined,
  };
};

type Typed = { "@odata.type"?: string };
const typeOf = (m: Typed): Member["type"] =>
  m["@odata.type"]?.endsWith("group") ? "group" : m["@odata.type"]?.endsWith("servicePrincipal") ? "servicePrincipal" : "user";

/** Every group, with how many users, groups, and service principals are direct members (one batch of lists, for 20 groups at a time). */
export async function listGroups(): Promise<GroupInfo[]> {
  // by name: Graph gives the groups in no order that a person can use
  const groups = (await graphAll<GraphGroup>(`/groups?$select=${GROUP_SELECT}&$top=999`)).map(toGroup).sort((a, b) => a.name.localeCompare(b.name));
  const lists = await graphBatch<GraphList<Typed>>(groups.map((g) => ({ id: g.id, url: `/groups/${g.id}/members?$select=id&$top=999` })));
  groups.forEach((g, i) => {
    for (const m of lists[i].status === 200 ? lists[i].body.value : []) {
      const t = typeOf(m);
      if (t === "user") g.users++;
      else if (t === "group") g.groups++;
      else g.others++;
    }
  });
  return groups;
}

/** A group with its owners and the groups that hold it. The counts of members come from `groupMembers`. */
export async function getGroup(id: string): Promise<GroupDetail> {
  const select = `${GROUP_SELECT},description,mailNickname,securityEnabled,mailEnabled,isAssignableToRole,createdDateTime`;
  const [g, owners, parents] = await Promise.all([
    graphGet<GraphGroup>(`/groups/${q(id)}?$select=${select}`),
    graphAll<{ id: string; displayName: string | null }>(`/groups/${q(id)}/owners?$select=id,displayName&$top=999`),
    graphAll<{ id: string; displayName: string | null }>(`/groups/${q(id)}/memberOf/microsoft.graph.group?$select=id,displayName&$top=999`),
  ]);
  return {
    ...toGroup(g),
    description: g.description ?? undefined,
    mailNickname: g.mailNickname ?? undefined,
    securityEnabled: g.securityEnabled ?? false,
    mailEnabled: g.mailEnabled ?? false,
    roleAssignable: g.isAssignableToRole ?? false,
    created: g.createdDateTime ?? undefined,
    owners: owners.map((o) => ({ id: o.id, name: o.displayName ?? "" })),
    memberOf: parents.map((p) => ({ id: p.id, name: p.displayName ?? "" })),
  };
}

/** The direct members of a group (the first `limit`), and how many there are. */
export async function groupMembers(id: string, limit: number): Promise<{ members: Member[]; total: number }> {
  const page = await graphGet<GraphList<Typed & { id: string; displayName: string | null; userPrincipalName?: string | null; appId?: string | null }>>(
    `/groups/${q(id)}/members?$select=id,displayName,userPrincipalName,appId&$top=${limit}&$count=true`,
    true,
  );
  const members = page.value.map((m): Member => ({ id: m.id, type: typeOf(m), name: m.displayName ?? "", detail: m.userPrincipalName ?? m.appId ?? undefined }));
  return { members, total: page["@odata.count"] ?? members.length };
}

/** Change the rule of a dynamic group, and say if its members are worked out. */
export const saveRule = (id: string, rule: string, state: "On" | "Paused") =>
  graphSend("PATCH", `/groups/${q(id)}`, { membershipRule: rule, membershipRuleProcessingState: state });

const ref = (memberId: string) => ({ "@odata.id": `https://graph.microsoft.com/v1.0/directoryObjects/${memberId}` });
export const addMember = (id: string, memberId: string) => graphSend("POST", `/groups/${q(id)}/members/$ref`, ref(memberId));
export const removeMember = (id: string, memberId: string) => graphSend("DELETE", `/groups/${q(id)}/members/${q(memberId)}/$ref`);

// ---- the organization ----

/** The organization of the tenant, and how many users, groups, and service principals it has (the number in each list, one call each). */
export async function getOrganization(): Promise<{ org: Organization; users: number; groups: number; servicePrincipals: number }> {
  const count = async (path: string) => (await graphGet<GraphList<unknown>>(`${path}?$top=1&$select=id&$count=true`, true))["@odata.count"] ?? 0;
  const [orgs, users, groups, servicePrincipals] = await Promise.all([
    graphGet<GraphList<Organization>>("/organization"),
    count("/users"),
    count("/groups"),
    count("/servicePrincipals"),
  ]);
  return { org: orgs.value[0], users, groups, servicePrincipals };
}

// ---- the apps of a user ----

interface Assignment { resourceId: string; resourceDisplayName: string | null; appRoleId: string }
interface Grant { clientId: string; consentType: string; principalId: string | null; resourceId: string; scope: string | null }
interface Sp { id: string; displayName: string | null; appId: string; appRoles?: { id: string; displayName: string | null; value: string | null }[] }

/** The app roles that the user has (given to the user, or to a group that the user is in), and the consents that apply to the user. */
export async function userApps(id: string, groups: GroupRef[]): Promise<UserApps> {
  // one batch: the assignments of the user and of each group, and the consents of the user and of everyone
  const asked = [
    { id: "user", url: `/users/${q(id)}/appRoleAssignments?$top=999` },
    ...groups.map((g) => ({ id: `group:${g.id}`, url: `/groups/${g.id}/appRoleAssignments?$top=999` })),
    { id: "mine", url: `/oauth2PermissionGrants?$filter=${q(`principalId eq '${id}'`)}&$top=999`, headers: { ConsistencyLevel: "eventual" } },
    { id: "all", url: `/oauth2PermissionGrants?$filter=${q("consentType eq 'AllPrincipals'")}&$top=999`, headers: { ConsistencyLevel: "eventual" } },
  ];
  const answers = new Map((await graphBatch<GraphList<never>>(asked)).map((r) => [r.id, r]));
  const list = <T>(key: string): T[] => (answers.get(key)?.status === 200 ? ((answers.get(key)!.body as unknown as GraphList<T>).value ?? []) : []);

  const assignments: { a: Assignment; via: string }[] = [
    ...list<Assignment>("user").map((a) => ({ a, via: "" })),
    ...groups.flatMap((g) => list<Assignment>(`group:${g.id}`).map((a) => ({ a, via: g.name }))),
  ];
  const grants = [...list<Grant>("mine"), ...list<Grant>("all")].filter(
    (g, i, all) => (g.consentType === "AllPrincipals" || g.principalId?.toLowerCase() === id.toLowerCase()) && all.findIndex((x) => x.clientId === g.clientId && x.resourceId === g.resourceId && x.consentType === g.consentType && x.principalId === g.principalId) === i,
  );

  // the service principals that are named: the resources of the assignments, and the clients and resources of the consents
  const spIds = [...new Set([...assignments.map((x) => x.a.resourceId), ...grants.flatMap((g) => [g.clientId, g.resourceId])])];
  const sps = new Map<string, Sp>();
  for (const r of await graphBatch<Sp>(spIds.map((s) => ({ id: s, url: `/servicePrincipals/${s}?$select=id,displayName,appId,appRoles` })))) {
    if (r.status === 200) sps.set(r.body.id, r.body);
  }

  const assigned = new Map<string, UserApp>();
  for (const { a, via } of assignments) {
    const res = sps.get(a.resourceId);
    if (!res) continue;
    const role = a.appRoleId === DEFAULT_ACCESS ? "Default access" : (res.appRoles?.find((r) => r.id === a.appRoleId)?.displayName ?? a.appRoleId);
    const key = `${res.appId}|${via}`;
    const app = assigned.get(key) ?? { appId: res.appId, name: res.displayName ?? "", roles: [], via: via || undefined };
    assigned.set(key, app);
    if (!app.roles.includes(role)) app.roles.push(role);
  }

  const consents: UserConsent[] = [];
  for (const g of grants) {
    const [client, res] = [sps.get(g.clientId), sps.get(g.resourceId)];
    if (!client || !res) continue;
    consents.push({
      appId: client.appId,
      name: client.displayName ?? "",
      resource: res.displayName ?? "",
      scopes: (g.scope ?? "").split(/\s+/).filter(Boolean).sort(),
      forAll: g.consentType === "AllPrincipals",
    });
  }

  const cmp = (a: string, b: string) => a.toLowerCase().localeCompare(b.toLowerCase());
  return {
    assigned: [...assigned.values()].map((a) => ({ ...a, roles: a.roles.sort() })).sort((a, b) => cmp(a.name, b.name) || cmp(a.via ?? "", b.via ?? "")),
    consents: consents.sort((a, b) => cmp(a.name, b.name) || cmp(a.resource, b.resource)),
  };
}
