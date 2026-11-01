import assert from "node:assert/strict";
import { test } from "node:test";
import { altText, callsText, firstKind, learnUrl, permissionList, statusTitle, summarize, variantLines, verdict, type KindNeeds, type NeedRow } from "./needs.ts";

const row = (status: NeedRow["status"], extra: Partial<NeedRow> = {}): NeedRow => ({ method: "GET", path: "/v1.0/users", calls: 1, errors: 0, last: "", status, ...extra });
const kind = (kind: KindNeeds["kind"], calls: number, rows: NeedRow[] = [], extra: Partial<KindNeeds> = {}): KindNeeds => ({ kind, calls, rows, minimum: [], missing: [], unavailable: [], assumed: [], granted: 0, unused: [], lower: [], ...extra });

test("verdict: nothing granted, missing, lower, or good", () => {
  assert.equal(verdict(kind("application", 1, [], { minimum: ["A"] })), null);
  assert.deepEqual(verdict(kind("application", 1, [], { minimum: ["A"], granted: 1, missing: ["A"] })), { tone: "amber", text: "Missing A" });
  assert.deepEqual(verdict(kind("application", 1, [], { minimum: ["A"], granted: 1, lower: [{ from: "B", to: ["A"], calls: 1 }] })), { tone: "amber", text: "Could be lowered: B to A" });
  assert.deepEqual(verdict(kind("application", 1, [], { minimum: ["A"], granted: 1 })), { tone: "green", text: "Looks good" });
});

test("callsText: one call, or many", () => {
  assert.equal(callsText(1), "1 call");
  assert.equal(callsText(0), "0 calls");
  assert.equal(callsText(1234), `${(1234).toLocaleString()} calls`);
});

test("altText: an alternative that needs two permissions", () => {
  assert.equal(altText("User.Read.All"), "User.Read.All");
  assert.equal(altText("User.Read and User.ReadBasic.All"), "User.Read + User.ReadBasic.All");
});

test("learnUrl: only a page of Microsoft Learn is a link", () => {
  assert.equal(learnUrl("https://learn.microsoft.com/graph/api/user-list?view=graph-rest-1.0"), "https://learn.microsoft.com/graph/api/user-list?view=graph-rest-1.0");
  for (const bad of ["javascript:alert(1)", "http://learn.microsoft.com/x", "https://learn.microsoft.com.evil.example/x", "https://example.com/", "", undefined, "not a url"]) {
    assert.equal(learnUrl(bad), "", String(bad));
  }
});

test("summarize: the calls by status", () => {
  const s = summarize(kind("application", 6, [row("covered"), row("missing"), row("missing"), row("noData"), row("notSupported")]));
  assert.deepEqual(s, { withData: 3, missing: 2, covered: 1, noData: 2 });
});

test("firstKind: the one with more calls", () => {
  assert.equal(firstKind([]), null);
  assert.equal(firstKind([kind("delegated", 2), kind("application", 9)]), "application");
  assert.equal(firstKind([kind("delegated", 3)]), "delegated");
});

test("variantLines: what a call needs for each type of object", () => {
  const r = row("varies", {
    variants: [
      { resource: "user", delegated: { least: ["GroupMember.ReadWrite.All"] }, application: { least: ["GroupMember.ReadWrite.All"] } },
      { resource: "device", delegated: { least: ["GroupMember.ReadWrite.All and Device.Read.All"] }, application: { notSupported: true } },
      { resource: "callRecording", detail: "communications/onlineMeetings/getAllRecordings", delegated: { notSupported: true }, application: { least: [] } },
    ],
  });
  assert.deepEqual(variantLines(r, "delegated"), ["user: GroupMember.ReadWrite.All", "device: GroupMember.ReadWrite.All + Device.Read.All", "callRecording (communications/onlineMeetings/getAllRecordings): not supported"]);
  assert.deepEqual(variantLines(r, "application"), ["user: GroupMember.ReadWrite.All", "device: not supported", "callRecording (communications/onlineMeetings/getAllRecordings): none"]);
  assert.deepEqual(variantLines(row("covered"), "delegated"), []);
  assert.match(statusTitle(row("varies")), /type of object/);
  // a table for each case of the page, with its heading
  const scenarios = row("varies", { variants: [{ scenario: "Permissions for channel", delegated: { least: ["ChannelMessage.Read.All"] }, application: { notSupported: true } }, { scenario: "Permissions for chat", delegated: { least: [] }, application: { least: ["Chat.Read.All"] } }] });
  assert.deepEqual(variantLines(scenarios, "application"), ["Permissions for channel: not supported", "Permissions for chat: Chat.Read.All"]);
});

test("statusTitle and permissionList", () => {
  assert.match(statusTitle(row("covered", { coveredBy: "A and B" })), /A \+ B/);
  assert.match(statusTitle(row("missing")), /none of the permissions/);
  assert.match(statusTitle(row("noData")), /No page of Microsoft Learn/);
  assert.equal(permissionList(["User.Read.All", "Group.Read.All"]), "User.Read.All, Group.Read.All");
});
