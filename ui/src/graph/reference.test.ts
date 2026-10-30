import assert from "node:assert/strict";
import { test } from "node:test";
import {
  buildNodes,
  buildPages,
  filterOperators,
  humanize,
  matchEndpoint,
  matches,
  missingOptions,
  referenceHref,
  resourceOf,
} from "./reference.ts";
import type { Endpoint } from "./types.ts";

const ep = (method: string, path: string, extra: Partial<Endpoint> = {}): Endpoint => ({
  method,
  path,
  area: "",
  summary: `${method} ${path}`,
  writes: method !== "GET",
  calls: 0,
  errors: 0,
  ...extra,
});

const groups = [
  ep("GET", "/v1.0/groups", { query: ["$select", "$filter"] }),
  ep("GET", "/v1.0/groups/{id}"),
  ep("POST", "/v1.0/groups"),
  ep("GET", "/v1.0/groups/{id}/members", { calls: 2 }),
  ep("GET", "/v1.0/groups/{id}/members/{type}", { calls: 1 }),
  ep("GET", "/v1.0/groups/{id}/members/$count"),
  ep("GET", "/v1.0/groups/{id}/members/{type}/$count"),
  ep("POST", "/v1.0/groups/{id}/members/$ref"),
  ep("GET", "/v1.0/groups/{id}/appRoleAssignments"),
];

test("the page of a call is the first part of its path", () => {
  assert.equal(resourceOf(ep("GET", "/v1.0/groups/{id}/members")), "groups");
  assert.equal(resourceOf(ep("POST", "/v1.0/$batch")), "$batch");
});

test("the server says the page of a call, and the section when the page only covers it", () => {
  const covered = { resource: "users", section: "Signed-in user" };
  const calls = [
    ep("GET", "/v1.0/users"),
    ep("GET", "/v1.0/users/{id}"),
    ep("GET", "/v1.0/users/{id}/manager"),
    ep("GET", "/v1.0/me", covered),
    ep("PATCH", "/v1.0/me", covered),
    ep("GET", "/v1.0/me/manager", covered),
  ];
  assert.equal(resourceOf(calls[3]), "users");
  const pages = buildPages(calls);
  assert.deepEqual(pages.map((p) => p.id), ["users"]);
  assert.deepEqual(
    pages[0].sections.map((s) => [s.title, s.nodes.length]),
    [["Users", 2], ["Manager", 1], ["Signed-in user", 3]],
  );
  // a call to /me finds its page in the reference
  assert.equal(matchEndpoint(calls, "GET", "/v1.0/me/manager")?.path, "/v1.0/me/manager");
  assert.match(referenceHref(calls, calls[5]), /^#\/graph\/api\/users\/GET%20%2Fv1.0%2Fme%2Fmanager$/);
});

test("the {type} and $count calls are folded into the main call", () => {
  const nodes = buildNodes(groups);
  const members = nodes.find((n) => n.key === "GET /v1.0/groups/{id}/members");
  assert.deepEqual(
    members?.variants.map((v) => v.path),
    ["/v1.0/groups/{id}/members/{type}", "/v1.0/groups/{id}/members/$count", "/v1.0/groups/{id}/members/{type}/$count"],
  );
  // they are not nodes of their own, and a POST on a $ref path is a call of its own
  assert.equal(nodes.some((n) => n.main.path.endsWith("/$count") || n.main.path.endsWith("/{type}")), false);
  assert.ok(nodes.some((n) => n.key === "POST /v1.0/groups/{id}/members/$ref"));
  assert.equal(nodes.length, groups.length - 3);
});

test("a call that ends like a variant, and has no main call, stays a call", () => {
  const nodes = buildNodes([ep("GET", "/v1.0/things/$count")]);
  assert.equal(nodes.length, 1);
});

test("the sections are the parts of the path, and the main section is first", () => {
  const [page] = buildPages(groups, "", [{ resource: "groups", name: "Groups", order: 1, description: "" }]);
  assert.equal(page.name, "Groups");
  assert.deepEqual(page.sections.map((s) => s.title), ["Groups", "Members", "App role assignments"]);
  assert.equal(page.count, groups.length - 3);
  assert.equal(page.calls, 3, "the calls of the variants are counted for the main call");
});

test("the name, the order, and the text of a page are from its file, and a call with no file comes last", () => {
  const docs = [
    { resource: "users", name: "People", order: 1, description: "The **people**." },
    { resource: "$batch", name: "Batch", order: 2, description: "Many in one." },
    { resource: "unused", name: "Unused", order: 3, description: "No call is on it." },
  ];
  const pages = buildPages([ep("GET", "/v1.0/widgets"), ep("POST", "/v1.0/$batch"), ep("GET", "/v1.0/users")], "", docs);
  assert.deepEqual(pages.map((p) => p.id), ["users", "$batch", "widgets"], "a page with no calls is not shown");
  assert.deepEqual(pages.map((p) => p.name), ["People", "Batch", "Widgets"]);
  assert.equal(pages[0].description, "The **people**.");
  assert.equal(pages[2].description, "");
});

test("a text keeps the calls that match, and a main call matches by its variants", () => {
  const [page] = buildPages(groups, "type");
  assert.deepEqual(page.sections.flatMap((s) => s.nodes.map((n) => n.key)), ["GET /v1.0/groups/{id}/members"]);
  assert.equal(buildPages(groups, "zzz").length, 1, "the page stays, with no calls");
  assert.equal(buildPages(groups, "zzz")[0].count, 0);
  assert.equal(matches(groups[0], "get GROUPS"), true);
  assert.equal(matches(groups[0], "users"), false);
});

test("humanize", () => {
  assert.equal(humanize("appRoleAssignments"), "App role assignments");
  assert.equal(humanize("directReports"), "Direct reports");
  assert.equal(humanize("$batch"), "Batch");
});

test("what Graph lists and Dolly does not answer", () => {
  const e = ep("GET", "/v1.0/users", { query: ["$select", "$filter"], graphQuery: ["$orderby", "$select", "$expand", "@id"] });
  assert.deepEqual(missingOptions(e), ["$orderby", "$expand"]);
  assert.deepEqual(missingOptions(ep("GET", "/v1.0/x")), []);
});

test("the words of Graph about $filter are read into parts", () => {
  assert.deepEqual(filterOperators("eq, ne, not, ge, le, in, startsWith, and eq on null values"), [
    "eq", "ne", "not", "ge", "le", "in", "startsWith", "eq on null values",
  ]);
  assert.deepEqual(filterOperators("eq and not"), ["eq", "not"]);
  assert.deepEqual(filterOperators("eq, not, /$count eq 0, /$count ne 0"), ["eq", "not", "/$count eq 0", "/$count ne 0"]);
});

test("a request is matched with the most specific call", () => {
  const list = [...groups, ep("GET", "/v1.0/groups/{id}/members/{type}/$count"), ep("GET", "/v1.0/places/{id}"), ep("GET", "/v1.0/places/{id}/{kind}")];
  assert.equal(matchEndpoint(list, "GET", "/v1.0/groups/abc/members")?.path, "/v1.0/groups/{id}/members");
  assert.equal(matchEndpoint(list, "GET", "/v1.0/groups/abc/members/microsoft.graph.user")?.path, "/v1.0/groups/{id}/members/{type}");
  assert.equal(matchEndpoint(list, "GET", "/v1.0/groups/abc/members/$count")?.path, "/v1.0/groups/{id}/members/$count", "$count is a literal part, and it wins over {type}");
  assert.equal(matchEndpoint(list, "POST", "/v1.0/groups")?.method, "POST");
  assert.equal(matchEndpoint(list, "GET", "/v1.0/devices"), undefined);
  assert.equal(matchEndpoint(list, "DELETE", "/v1.0/groups"), undefined);
});

test("the address of a call is the address of its main call", () => {
  const variant = groups[4];
  assert.equal(referenceHref(groups, variant), "#/graph/api/groups/" + encodeURIComponent("GET /v1.0/groups/{id}/members"));
  assert.equal(referenceHref(groups, groups[0]), "#/graph/api/groups/" + encodeURIComponent("GET /v1.0/groups"));
});
