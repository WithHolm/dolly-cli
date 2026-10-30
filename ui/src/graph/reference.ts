// The logic of the API reference, with no Svelte in it: which page a call is on, which calls are
// variants of a main call, and how to read what Graph says about $filter. The tests are in reference.test.ts.

import type { Endpoint, PageDoc } from "./types";

const BASE = "/v1.0/";

/** The part of the path after /v1.0/, split. */
const parts = (path: string) => (path.startsWith(BASE) ? path.slice(BASE.length) : path.replace(/^\//, "")).split("/");

/**
 * The key of the page of a call. The server says it, from the markdown files: the first part of the path
 * (users, groups, ...), or the page that covers it (/me is on the page of the users). A call with no
 * page of its own gets the first part of its path.
 */
export const resourceOf = (e: Endpoint) => e.resource ?? parts(e.path)[0];

/** The key of a call, and of its node in the address: GET /v1.0/groups/{id}/members */
export const nodeKey = (e: Pick<Endpoint, "method" | "path">) => `${e.method} ${e.path}`;

/** A call and the variants of it that are folded into it. */
export interface Node {
  key: string;
  main: Endpoint;
  variants: Endpoint[];
}

export interface Section {
  title: string;
  nodes: Node[];
}

export interface ResourcePage {
  id: string;
  name: string;
  description: string;
  sections: Section[];
  count: number; // the calls (main calls, not variants)
  calls: number; // how often a client called them, with the variants
}

// A variant is a GET on a main call with a part added: only one type of member, or only the number.
const VARIANT_SUFFIXES = ["/{type}/$count", "/{type}", "/$count"];

/** The main call that this call is a variant of, or undefined when it is a main call. */
function variantBase(e: Endpoint, gets: Map<string, Endpoint>): Endpoint | undefined {
  if (e.method !== "GET") return undefined;
  for (const suffix of VARIANT_SUFFIXES) {
    if (e.path.endsWith(suffix)) {
      const base = gets.get(e.path.slice(0, -suffix.length));
      if (base) return base;
    }
  }
  return undefined;
}

/** buildNodes makes a node of each main call, and folds the variants into them. The order is the order of the server. */
export function buildNodes(endpoints: readonly Endpoint[]): Node[] {
  const gets = new Map(endpoints.filter((e) => e.method === "GET").map((e) => [e.path, e]));
  const nodes = new Map<string, Node>();
  for (const e of endpoints) {
    if (!variantBase(e, gets)) nodes.set(nodeKey(e), { key: nodeKey(e), main: e, variants: [] });
  }
  for (const e of endpoints) {
    const base = variantBase(e, gets);
    if (base) nodes.get(nodeKey(base))?.variants.push(e);
  }
  return [...nodes.values()];
}

/** The main call that an operation is on: itself, or the call that it is a variant of. */
export function mainOf(endpoints: readonly Endpoint[], e: Endpoint): Endpoint {
  const gets = new Map(endpoints.filter((x) => x.method === "GET").map((x) => [x.path, x]));
  return variantBase(e, gets) ?? e;
}

/** camelCase as words: appRoleAssignments gives "App role assignments". */
export function humanize(name: string): string {
  const words = name.replace(/^\$/, "").replace(/([a-z0-9])([A-Z])/g, "$1 $2").toLowerCase();
  return words.charAt(0).toUpperCase() + words.slice(1);
}

/** The section of a call on its page: the part of the path after the ID (members, manager, ...), or the main section. */
function sectionTitle(e: Endpoint, mainTitle: string): string {
  if (e.section) return e.section;
  const sub = parts(e.path)
    .slice(1)
    .find((p) => !p.startsWith("{") && !p.startsWith("$"));
  return sub ? humanize(sub) : mainTitle;
}

/** Whether every word of the text is in the call: method, path, or summary. */
export function matches(e: Endpoint, text: string): boolean {
  const hay = `${e.method} ${e.path} ${e.summary}`.toLowerCase();
  return text
    .toLowerCase()
    .split(/\s+/)
    .filter(Boolean)
    .every((w) => hay.includes(w));
}

/**
 * buildPages groups the operations into the pages of the reference. The name, the order, and the
 * description of a page come from its markdown file (docs). A call whose page has no file still gets a page,
 * after the others. With a text, only the calls that match it (a main call also matches when one
 * of its variants does) are in the pages.
 */
export function buildPages(endpoints: readonly Endpoint[], text = "", docs: readonly PageDoc[] = []): ResourcePage[] {
  const byResource = new Map<string, Endpoint[]>();
  for (const e of endpoints) byResource.set(resourceOf(e), [...(byResource.get(resourceOf(e)) ?? []), e]);

  const known = docs.map((d) => d.resource);
  const order = [...known, ...[...byResource.keys()].filter((k) => !known.includes(k))];
  return order
    .filter((id) => byResource.has(id))
    .map((id) => {
      const doc = docs.find((d) => d.resource === id);
      const info = { id, name: doc?.name ?? humanize(id), description: doc?.description ?? "" };
      const sections: Section[] = [];
      for (const node of buildNodes(byResource.get(id)!)) {
        if (text && ![node.main, ...node.variants].some((e) => matches(e, text))) continue;
        const title = sectionTitle(node.main, info.name);
        let section = sections.find((s) => s.title === title);
        if (!section) sections.push((section = { title, nodes: [] }));
        section.nodes.push(node);
      }
      // the main section first, then the others in the order that they came
      sections.sort((a, b) => Number(b.title === info.name) - Number(a.title === info.name));
      const nodes = sections.flatMap((s) => s.nodes);
      return {
        id,
        name: info.name,
        description: info.description,
        sections,
        count: nodes.length,
        calls: nodes.reduce((sum, n) => sum + [n.main, ...n.variants].reduce((s, e) => s + e.calls, 0), 0),
      };
    });
}

/** The options that the description of Graph lists for a call and that Dolly does not answer. */
export const missingOptions = (e: Endpoint) => (e.graphQuery ?? []).filter((o) => o.startsWith("$") && !(e.query ?? []).includes(o));

/** The parts of what Graph says about $filter on a property: "eq, ne, and eq on null values" gives three parts. */
export function filterOperators(note: string): string[] {
  return note
    .split(/\s*,\s*(?:and\s+)?|\s+and\s+/)
    .map((s) => s.trim())
    .filter(Boolean);
}

/** The call that answers a request, from the path of the request: /v1.0/groups/abc/members gives GET /v1.0/groups/{id}/members. */
export function matchEndpoint(endpoints: readonly Endpoint[], method: string, path: string): Endpoint | undefined {
  const given = path.split("/");
  let best: { e: Endpoint; literal: number } | undefined;
  for (const e of endpoints) {
    if (e.method !== method) continue;
    const want = e.path.split("/");
    if (want.length !== given.length) continue;
    let literal = 0;
    let ok = true;
    for (let i = 0; i < want.length && ok; i++) {
      if (want[i].startsWith("{")) continue;
      if (want[i].toLowerCase() === given[i].toLowerCase()) literal++;
      else ok = false;
    }
    if (ok && (!best || literal > best.literal)) best = { e, literal };
  }
  return best?.e;
}

/** The address of the page of a call in the reference: #/graph/api/groups/GET%20%2Fv1.0%2F... */
export function referenceHref(endpoints: readonly Endpoint[], e: Endpoint): string {
  return `#/graph/api/${encodeURIComponent(resourceOf(e))}/${encodeURIComponent(nodeKey(mainOf(endpoints, e)))}`;
}
