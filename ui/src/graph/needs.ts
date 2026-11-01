// The permissions that the calls of a service principal need (GET /_dolly/graph/service-principals/<client ID>/needs). The server notes the calls of
// a client by kind of token, and gives, for each call, the permissions that the page of Microsoft Learn about it says. These helpers say it in words.

export type NeedKind = "delegated" | "application";
export type NeedStatus = "covered" | "missing" | "noData" | "notSupported" | "varies";

/** A row of a table by type of object: what the call needs depends on the object (a user, a group, a device). */
export interface Variant {
  /** The heading of the table, when the page has more than one: "Permissions for channel". */
  scenario?: string;
  resource?: string;
  detail?: string;
  delegated: { least?: string[]; higher?: string[]; notSupported?: boolean };
  application: { least?: string[]; higher?: string[]; notSupported?: boolean };
}

export interface NeedRow {
  method: string;
  path: string;
  page?: string;
  doc?: string;
  variants?: Variant[];
  calls: number;
  errors: number;
  last: string;
  status: NeedStatus;
  /** Alternatives with the least privilege. One is enough. "A and B" needs both. */
  least?: string[];
  higher?: string[];
  coveredBy?: string;
}

export interface KindNeeds {
  kind: NeedKind;
  calls: number;
  rows: NeedRow[];
  /** A set of permissions that allows every call that has data, with the least privilege. */
  minimum: string[];
  /** What to add to what the client has, and dollycli can grant. */
  missing: string[];
  /** What to add, and dollycli cannot grant: the pages of Learn are newer than its list of the permissions of Graph. */
  unavailable: string[];
  /** Permissions of the minimum that rest on a guess (implies.json): assumed to include more. */
  assumed: { permission: string; includes: string[]; note: string }[];
  /** How many permissions of Microsoft Graph of this kind the client has. With none there is nothing to compare. */
  granted: number;
  /** Granted, and named by none of the calls. */
  unused: string[];
  /** Granted, allows its calls, and a lower permission would do. */
  lower: { from: string; to: string[]; calls: number }[];
}

export interface Verdict {
  tone: "green" | "amber" | "";
  text: string;
}

/** The one line that compares what the client has with what its calls need. Null when the client has nothing to compare. */
export function verdict(k: KindNeeds): Verdict | null {
  if (k.granted === 0 || k.minimum.length === 0) return null;
  const missing = k.missing.length + k.unavailable.length;
  if (missing > 0) return { tone: "amber", text: `Missing ${[...k.missing, ...k.unavailable].join(", ")}` };
  if (k.lower.length > 0) return { tone: "amber", text: "Could be lowered: " + k.lower.map((l) => `${l.from} to ${l.to.join(", ")}`).join("; ") };
  return { tone: "green", text: "Looks good" };
}

export interface Needs {
  appId: string;
  kinds: KindNeeds[];
  unknownCalls: number;
  source: { commit: string; commitDate: string };
}

/** Microsoft Graph, the resource of these permissions. */
export const GRAPH_APP_ID = "00000003-0000-0000-c000-000000000000";

export const kindLabel = (kind: NeedKind): string => (kind === "delegated" ? "Delegated" : "Application");

/** An alternative that needs two permissions is written "A + B". */
export const altText = (alt: string): string => alt.split(" and ").join(" + ");

/** A page of Learn is a link only when it is on learn.microsoft.com: the address is from the data of dollycli, and a page must not follow another. */
export function learnUrl(doc: string | undefined): string {
  try {
    const u = new URL(doc ?? "");
    return u.protocol === "https:" && u.hostname === "learn.microsoft.com" ? u.href : "";
  } catch {
    return "";
  }
}

export const statusText: Record<NeedStatus, string> = {
  covered: "Granted",
  missing: "Missing",
  noData: "No permission data",
  notSupported: "Not supported",
  varies: "Depends on the case",
};

/** The rows of a call that depends on the object, as lines of text for a title: "user: User.Read.All". The kind says which token to show. */
export function variantLines(r: NeedRow, kind: NeedKind): string[] {
  return (r.variants ?? []).map((v) => {
    const n = v[kind];
    const what = n.notSupported ? "not supported" : (n.least ?? []).map(altText).join(" or ") || "none";
    const label = [v.scenario, v.resource].filter(Boolean).join(" · ") || "table";
    return `${label}${v.detail ? ` (${v.detail})` : ""}: ${what}`;
  });
}

/** What the status says, as a sentence for a title. */
export function statusTitle(r: NeedRow): string {
  switch (r.status) {
    case "covered":
      return `The client has ${altText(r.coveredBy ?? "a permission that allows this call")}.`;
    case "missing":
      return "The client has none of the permissions that allow this call.";
    case "noData":
      return "No page of Microsoft Learn has a table of permissions for this call.";
    case "varies":
      return "What this call needs depends on the type of object it is about, or on the case (a channel or a chat). The page of Microsoft Learn has a table for each.";
    default:
      return "The page of Microsoft Learn says that this kind of token cannot make this call.";
  }
}

export interface Summary {
  /** The calls that have permission data. */
  withData: number;
  missing: number;
  covered: number;
  noData: number;
}

export function summarize(k: KindNeeds): Summary {
  const s: Summary = { withData: 0, missing: 0, covered: 0, noData: 0 };
  for (const r of k.rows) {
    if (r.status === "missing") [s.withData, s.missing] = [s.withData + 1, s.missing + 1];
    else if (r.status === "covered") [s.withData, s.covered] = [s.withData + 1, s.covered + 1];
    else s.noData++; // no data, not supported, and the calls that depend on the object: none of them is in the suggestion
  }
  return s;
}

/** "1 call", "2 calls". */
export const callsText = (n: number): string => `${n.toLocaleString()} ${n === 1 ? "call" : "calls"}`;

/** The permissions as text for a list or a command: delegated ones are scopes in one line, application ones are roles. */
export const permissionList = (list: readonly string[]): string => list.join(", ");

/** The kind to show first: the one with more calls. */
export function firstKind(kinds: readonly KindNeeds[]): NeedKind | null {
  if (kinds.length === 0) return null;
  return [...kinds].sort((a, b) => b.calls - a.calls)[0].kind;
}
