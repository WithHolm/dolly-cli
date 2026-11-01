// The profile of a generated tenant (see docs/profile.md). The server checks every profile in full, so this file has the shape and the
// small helpers of the editor, and no rules of its own.

/** extensionAttribute<index> of the users: weighted values or a pattern (not both). fill is the percent of users that have it, 100 if absent. */
export interface ExtensionAttribute {
  index: number;
  values?: { value: string; weight?: number }[];
  pattern?: string;
  fill?: number;
}

export interface Profile {
  version: 1;
  name?: string;
  description?: string;
  organization: Record<"displayName" | "companyName" | "domain" | "initialDomain" | "street" | "postalCode" | "city" | "state" | "country" | "countryLetterCode" | "usageLocation" | "preferredLanguage" | "phone", string> & {
    domainWeight?: number;
    domains?: { name: string; weight?: number }[];
    guestDomain?: string;
  };
  offices: { name: string; city: string; country: string; usage: string; postal: string; callingCode: string; weight?: number }[];
  departments: { name: string; weight?: number; titles: { name: string; weight?: number }[] }[];
  names: { first: string[]; last: string[]; byLanguage?: Record<string, { first: string[]; last: string[] }> };
  people: {
    guestPercent: number;
    syncedPercent: number;
    enabledPercent: number;
    contractorPercent: number;
    managerWord: string;
    languages: { code: string; weight?: number }[];
    guestDomains?: { name: string; weight?: number }[];
  };
  formats: { employeeId: { pattern: string; start: number }; mobilePhone: string; businessPhone: string; costCenter: string; streets: string[] };
  extensionAttributes?: ExtensionAttribute[];
  groups: {
    prefix: string;
    departmentGroups: boolean;
    officeGroups: boolean;
    dynamicDepartment?: string;
    microsoft365Departments?: string[];
    adminDepartment?: string;
    legalDepartment?: string;
    strayDepartments?: string[];
    apps?: { name: string; title: string; contributorDepartments: string[]; readerDepartments: string[] }[];
  };
  owner: { department: string; jobTitle: string };
  admin: { userPrincipalName: string; displayName: string; jobTitle: string; department: string };
}

/** What the server says about a profile (POST /_dolly/graph/profile/preview). */
export interface Preview {
  seed: number;
  users: number;
  groups: number;
  servicePrincipals: number;
  departments: { value: string; count: number }[];
  offices: { value: string; count: number }[];
  languages: { value: string; count: number }[];
  guests: number;
  synced: number;
  disabled: number;
  domain: string;
  owner: string;
  adminAccount: string;
  sample: Record<string, unknown>[];
}

/**
 * Weights are shares of a group (the offices, the job titles of a department, the languages, and so on). The sum of a group is TOTAL, and a
 * weight is a whole number from 1 up, so the page shows a share with one decimal: 343 of 1000 is 34.3%. A tenth of a percent is finer than a
 * pixel of a bar, so a drag looks smooth. The server only needs a ratio, so the numbers stay a valid profile.
 */
export const TOTAL = 1000;
/** A weight is shown in percent: this many weights are one percent. */
export const PER_PERCENT = TOTAL / 100;

/** A weight as a text in percent, with one decimal: 343 is "34.3". */
export const percentText = (weight: number): string => (weight / PER_PERCENT).toFixed(1);

/** Scale weights so that the sum is `total`, with the same ratios as far as whole numbers allow, and none below 1. */
export function toShares(weights: number[], total = TOTAL): number[] {
  const n = weights.length;
  if (n === 0) return [];
  if (n === 1) return [total];
  let base = weights.map((w) => (Number.isFinite(w) && w > 0 ? w : 0));
  if (base.every((w) => w === 0)) base = base.map(() => 1);
  const sum = base.reduce((a, b) => a + b, 0);
  const exact = base.map((w) => (w * total) / sum);
  const out = exact.map((x) => Math.max(1, Math.floor(x)));
  let diff = total - out.reduce((a, b) => a + b, 0);
  // give what is left to the biggest remainders, or take what is too much from the biggest values
  const byRemainder = exact.map((x, i) => i).sort((a, b) => exact[b] - Math.floor(exact[b]) - (exact[a] - Math.floor(exact[a])));
  for (let k = 0; diff > 0; k = (k + 1) % n, diff--) out[byRemainder[k]]++;
  while (diff < 0) {
    const biggest = out.indexOf(Math.max(...out));
    if (out[biggest] <= 1) break;
    out[biggest]--;
    diff++;
  }
  return out;
}

const sum = (w: number[]) => w.reduce((a, b) => a + b, 0);

/**
 * Give `total` to items that start at `base`, in equal parts: each gives (or takes) the same amount, as far as it can (none goes below 1, and
 * one that cannot give more leaves the rest to the others). The result is whole numbers that sum to `total`. The rounding leftover goes to the
 * biggest fractions, and then to the smaller items, so the bigger items give a little more and no item is picked every time.
 */
function pullEqual(base: number[], total: number): number[] {
  const x = base.map((b) => Math.max(1, b));
  let remaining = total - sum(x);
  let active = x.map((_, k) => k);
  for (let guard = 0; Math.abs(remaining) > 1e-9 && active.length > 0 && guard < 100; guard++) {
    const per = remaining / active.length;
    let applied = 0;
    for (const k of active) {
      const next = Math.max(1, x[k] + per);
      applied += next - x[k];
      x[k] = next;
    }
    remaining -= applied;
    active = active.filter((k) => remaining > 0 || x[k] > 1 + 1e-9);
  }
  const out = x.map((v) => Math.max(1, Math.floor(v + 1e-9)));
  let left = total - sum(out);
  const order = x.map((_, k) => k).sort((a, b) => x[b] - Math.floor(x[b]) - (x[a] - Math.floor(x[a])) || x[a] - x[b] || a - b);
  for (let k = 0; left > 0 && order.length > 0; k = (k + 1) % order.length, left--) out[order[k]]++;
  while (left < 0) {
    const biggest = out.indexOf(Math.max(...out));
    if (out[biggest] <= 1) break;
    out[biggest]--;
    left++;
  }
  return out;
}

/**
 * Set the share of item i. The other items give or take the difference in equal parts, and none goes below 1. The sum stays TOTAL. A locked
 * item does not change, and a locked item i cannot be set. `base` is where the free items were when a drag began: the pull is worked out from
 * there each time, so a drag in small steps takes from all of them and not from the same one.
 */
export function rebalance(weights: number[], i: number, value: number, locked: boolean[] = [], base: number[] = weights): number[] {
  const n = weights.length;
  if (n <= 1) return n === 1 ? [TOTAL] : [];
  if (locked[i]) return [...weights];
  const free = weights.map((_, k) => k).filter((k) => k !== i && !locked[k]);
  if (free.length === 0) return [...weights];
  const lockedSum = sum(weights.filter((_, k) => locked[k]));
  const out = [...weights];
  out[i] = Math.max(1, Math.min(TOTAL - lockedSum - free.length, Math.round(value)));
  const next = pullEqual(free.map((k) => base[k]), TOTAL - lockedSum - out[i]);
  free.forEach((k, j) => (out[k] = next[j]));
  return out;
}

type Weighted = { weight?: number };
const weightOf = (x: Weighted) => x.weight ?? 1;
const assign = (items: Weighted[], next: number[]) => items.forEach((x, k) => (x.weight = next[k]));

const grabbed = new WeakMap<object, number[]>();
/** A drag begins on this list: remember where the weights are. */
export const grab = (items: Weighted[]) => void grabbed.set(items, items.map(weightOf));
/** The drag is over. */
export const release = (items: Weighted[]) => void grabbed.delete(items);

/** Set the share of item i of a list of items that have a weight. `locked` says which items cannot change. */
export function setShare<T extends Weighted>(items: T[], i: number, value: number, locked: (x: T) => boolean = () => false): void {
  const base = grabbed.get(items);
  assign(items, rebalance(items.map(weightOf), i, value, items.map(locked), base?.length === items.length ? base : undefined));
}

/** Make the weights of a list sum to TOTAL, with the same ratios. The free items take what the locked ones leave. */
export function balance<T extends Weighted>(items: T[], locked: (x: T) => boolean = () => false): void {
  const free = items.map((_, k) => k).filter((k) => !locked(items[k]));
  if (free.length === 0) return;
  const room = TOTAL - sum(items.filter(locked).map(weightOf));
  const next = toShares(free.map((k) => weightOf(items[k])), room);
  free.forEach((k, j) => (items[k].weight = next[j]));
}

/**
 * Add an item with an equal share (TOTAL divided by the number of items). The free items give it, with the same ratios. It gives false (and
 * adds nothing) when every item is locked: there is nobody to take the share from.
 */
export function addShare<T extends Weighted>(items: T[], item: Omit<T, "weight">, locked: (x: T) => boolean = () => false): boolean {
  if (items.length === 0) {
    items.push({ ...item, weight: TOTAL } as T);
    return true;
  }
  const free = items.filter((x) => !locked(x));
  if (free.length === 0) return false;
  const room = TOTAL - sum(items.filter(locked).map(weightOf));
  const share = Math.min(Math.max(1, Math.round(TOTAL / (items.length + 1))), room - free.length);
  const next = toShares(free.map(weightOf), room - share);
  free.forEach((x, j) => (x.weight = next[j]));
  items.push({ ...item, weight: share } as T);
  return true;
}

/** Remove item i, and give its share to the free items, with the same ratios. If all the others are locked, the first one takes it. */
export function removeShare<T extends Weighted>(items: T[], i: number, locked: (x: T) => boolean = () => false): void {
  items.splice(i, 1);
  if (items.length === 0) return;
  if (items.every(locked)) items[0].weight = weightOf(items[0]) + TOTAL - sum(items.map(weightOf));
  else balance(items, locked);
}

/** The scale of a weight bar: the first of 2, 5, 10, 20, 50, 100 percent that holds the biggest value. The scale is in weights (TOTAL is 100%). */
export function niceScale(values: number[]): number {
  const top = Math.max(0, ...values) / PER_PERCENT;
  return ([2, 5, 10, 20, 50, 100].find((s) => s >= top) ?? 100) * PER_PERCENT;
}

export interface DomainRow {
  name: string;
  weight: number;
}

export const TENANT_SUFFIX = ".onmicrosoft.com";

/** The tenant name of an initial domain: acme.onmicrosoft.com gives acme. */
export const tenantOf = (initialDomain: string): string => (initialDomain.endsWith(TENANT_SUFFIX) ? initialDomain.slice(0, -TENANT_SUFFIX.length) : initialDomain);

/** All the domains of the addresses of the members, as rows: the initial domain first, then the default one if it is another, then the rest. */
export function domainRows(o: Profile["organization"]): DomainRow[] {
  const rows: DomainRow[] = [];
  const add = (name: string, weight: number | undefined) => {
    if (!rows.some((r) => r.name === name)) rows.push({ name, weight: weight || 1 });
  };
  const other = (name: string) => o.domains?.find((d) => d.name === name)?.weight;
  add(o.initialDomain, o.domain === o.initialDomain ? o.domainWeight : other(o.initialDomain));
  add(o.domain, o.domainWeight);
  for (const d of o.domains ?? []) add(d.name, d.weight);
  return rows;
}

/** Write the rows back: the initial domain is the first row, and one row is the default domain. */
export function applyDomainRows(o: Profile["organization"], rows: DomainRow[], defaultName: string): void {
  const def = rows.find((r) => r.name === defaultName) ?? rows[0];
  o.initialDomain = rows[0].name;
  o.domain = def.name;
  o.domainWeight = def.weight;
  o.domains = rows.filter((r) => r !== def).map((r) => ({ name: r.name, weight: r.weight }));
}

/**
 * Make a profile ready for the wizard: the guest domain of an old profile goes with the people, every group of weights sums to 100, and the
 * names are kept by language (the language of a person is picked first, and the names follow it). `namesFor` gives the names of a language
 * that the profile has no names for.
 */
export function normalize(p: Profile, namesFor: (locale: string) => { first: string[]; last: string[] } | undefined = () => undefined): Profile {
  if (p.organization.guestDomain && !(p.people.guestDomains?.length)) {
    p.people.guestDomains = [{ name: p.organization.guestDomain, weight: 1 }];
  }
  delete p.organization.guestDomain;
  p.people.guestDomains ??= [];
  p.organization.domains ??= [];

  balance(p.offices);
  balance(p.departments);
  for (const d of p.departments) balance(d.titles);
  balance(p.people.languages);
  balance(p.people.guestDomains);
  for (const a of p.extensionAttributes ?? []) if (a.values) balance(a.values);
  const domains = toShares([p.organization.domainWeight ?? 1, ...p.organization.domains.map((d) => d.weight ?? 1)]);
  p.organization.domainWeight = domains[0];
  p.organization.domains.forEach((d, k) => (d.weight = domains[k + 1]));

  if (!p.names.byLanguage) {
    // the names that the profile has go to its most common language, and the others get the names of their country
    const sets: Record<string, { first: string[]; last: string[] }> = {};
    const byShare = [...p.people.languages].sort((a, b) => (b.weight ?? 1) - (a.weight ?? 1));
    byShare.forEach((l, k) => {
      sets[l.code] = k === 0 ? { first: [...p.names.first], last: [...p.names.last] } : (namesFor(l.code) ?? { first: [...p.names.first], last: [...p.names.last] });
    });
    p.names.byLanguage = sets;
    p.names.first = [];
    p.names.last = [];
  }
  return p;
}

/** A list in a text box: one item on a line, or several with commas. Empty items are dropped. */
export const splitList = (text: string): string[] =>
  text
    .split(/[\n,]/)
    .map((s) => s.trim())
    .filter(Boolean);

export const joinList = (list: readonly string[] | undefined): string => (list ?? []).join("\n");

/** Job titles in a text box: "Tech Lead" or "Tech Lead: 3" (the number is the weight). */
export function splitTitles(text: string): { name: string; weight?: number }[] {
  return text
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const m = /^(.*\S)\s*:\s*(\d+)$/.exec(line);
      return m ? { name: m[1], weight: Number(m[2]) } : { name: line };
    });
}

export const joinTitles = (titles: { name: string; weight?: number }[]): string =>
  titles.map((t) => (t.weight && t.weight !== 1 ? `${t.name}: ${t.weight}` : t.name)).join("\n");

/** A problem that the server found: the errors of a profile are lines of "path: message" (see Profile.Validate). */
export interface Issue {
  path: string;
  message: string;
}

export function parseIssues(text: string): Issue[] {
  return text
    .split("\n")
    .map((l) => l.trim())
    .filter(Boolean)
    .map((l) => {
      const m = /^([\w.[\]]+):\s*(.*)$/.exec(l);
      return m ? { path: m[1], message: m[2] } : { path: "", message: l };
    });
}

const fieldNames: Record<string, string> = {
  "people.guestPercent": "Guests, % of all",
  "people.syncedPercent": "Synced from on-premises, % of members",
  "people.enabledPercent": "Enabled accounts, %",
  "people.contractorPercent": "Contractors, %",
  "people.managerWord": "Manager word",
  "people.languages": "Languages",
  "names.first": "First names",
  "names.last": "Last names",
  "formats.employeeId.pattern": "Employee ID",
  "formats.employeeId.start": "First employee number",
  "formats.mobilePhone": "Mobile phone",
  "formats.businessPhone": "Business phone",
  "formats.costCenter": "Cost center",
  "formats.streets": "Street names",
  "owner.department": "Owner's department",
  "owner.jobTitle": "Owner's job title",
  "admin.userPrincipalName": "Admin account name",
  "admin.displayName": "Admin display name",
  "admin.jobTitle": "Admin job title",
  "admin.department": "Admin department",
};

/** The name of a field as the page shows it, for a path of the profile. A path that the page does not know stays as it is. */
export function fieldName(path: string, organization: Record<string, string> = {}): string {
  const base = path.replace(/\[\d+\]$/, "");
  if (fieldNames[path] ?? fieldNames[base]) return fieldNames[path] ?? fieldNames[base];
  let m = /^organization\.(\w+)$/.exec(path);
  if (m) return organization[m[1]] ?? path;
  if ((m = /^organization\.domains\[(\d+)\]/.exec(path))) return `Domain ${Number(m[1]) + 2}`;
  if ((m = /^people\.guestDomains(?:\[(\d+)\])?/.exec(path))) return m[1] === undefined ? "Guest domains" : `Guest domain ${Number(m[1]) + 1}`;
  if ((m = /^departments\[(\d+)\]\.titles(?:\[(\d+)\])?/.exec(path))) return `Department ${Number(m[1]) + 1}, job title${m[2] === undefined ? "s" : " " + (Number(m[2]) + 1)}`;
  if ((m = /^extensionAttributes\[(\d+)\]\.values(?:\[(\d+)\])?/.exec(path))) return `Attribute row ${Number(m[1]) + 1}, value${m[2] === undefined ? "s" : " " + (Number(m[2]) + 1)}`;
  if ((m = /^offices\[(\d+)\]\.?(\w*)$/.exec(path))) return `Office ${Number(m[1]) + 1}${m[2] ? ", " + m[2] : ""}`;
  if ((m = /^departments\[(\d+)\]\.?(\w*)$/.exec(path))) return `Department ${Number(m[1]) + 1}${m[2] ? ", " + m[2] : ""}`;
  if ((m = /^extensionAttributes\[(\d+)\]\.?(\w*)$/.exec(path))) return `Attribute row ${Number(m[1]) + 1}${m[2] ? ", " + m[2] : ""}`;
  return path;
}

const accents: Record<string, string> = { å: "a", ä: "a", ö: "o", ø: "o", æ: "ae", ü: "u", ß: "ss", é: "e", è: "e", ê: "e", á: "a", à: "a", í: "i", ó: "o", ú: "u", ñ: "n", ç: "c", ð: "d", þ: "th", ł: "l", ś: "s", ż: "z", ź: "z", č: "c", š: "s", ž: "z", ř: "r", ě: "e" };

/** The name of an admin account, as the server makes it: {first} {last} {f} {l} in lowercase with no accents, {First} {Last} as written. */
export function adminName(pattern: string, first: string, last: string): string {
  const plain = (s: string) => [...s.toLowerCase()].map((c) => accents[c] ?? c).join("").replace(/[^a-z0-9._-]/g, "");
  const tokens: Record<string, string> = {
    first: plain(first),
    last: plain(last),
    f: plain(first).slice(0, 1),
    l: plain(last).slice(0, 1),
    First: first,
    Last: last,
  };
  return pattern.replace(/\{(\w+)\}/g, (m, t: string) => tokens[t] ?? m);
}

/** A department gets another name: everything in the profile that points to it follows. */
export function renameDepartment(p: Profile, from: string, to: string): void {
  const swap = (v: string | undefined) => (v === from ? to : v);
  const swapAll = (l: string[] | undefined) => l?.map((v) => (v === from ? to : v));
  const g = p.groups;
  g.dynamicDepartment = swap(g.dynamicDepartment);
  g.adminDepartment = swap(g.adminDepartment);
  g.legalDepartment = swap(g.legalDepartment);
  g.microsoft365Departments = swapAll(g.microsoft365Departments);
  g.strayDepartments = swapAll(g.strayDepartments);
  for (const a of g.apps ?? []) {
    a.contributorDepartments = swapAll(a.contributorDepartments) ?? [];
    a.readerDepartments = swapAll(a.readerDepartments) ?? [];
  }
  p.owner.department = swap(p.owner.department) ?? "";
  p.admin.department = swap(p.admin.department) ?? "";
}
