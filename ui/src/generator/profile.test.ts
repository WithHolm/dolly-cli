import assert from "node:assert/strict";
import { test } from "node:test";
import {
  PER_PERCENT,
  TOTAL,
  addShare,
  adminName,
  applyDomainRows,
  domainRows,
  fieldName,
  grab,
  joinTitles,
  niceScale,
  normalize,
  parseIssues,
  percentText,
  rebalance,
  release,
  removeShare,
  renameDepartment,
  setShare,
  splitList,
  splitTitles,
  tenantOf,
  toShares,
  type Profile,
} from "./profile.ts";

test("splitList: lines and commas, no empty items", () => {
  assert.deepEqual(splitList("Ola, Kari\nNils\n\n  Åse  ,"), ["Ola", "Kari", "Nils", "Åse"]);
});

test("titles: a weight after a colon, and back", () => {
  const titles = splitTitles("Tech Lead: 3\nEngineer\n  Data: Science : 2 ");
  assert.deepEqual(titles, [{ name: "Tech Lead", weight: 3 }, { name: "Engineer" }, { name: "Data: Science", weight: 2 }]);
  assert.equal(joinTitles(titles), "Tech Lead: 3\nEngineer\nData: Science: 2");
});

test("adminName: the same names as the server makes", () => {
  assert.equal(adminName("a.{first}", "Philip", "Meholm"), "a.philip");
  assert.equal(adminName("{first}.a.{last}", "Håkon", "Ødegård"), "hakon.a.odegard");
  assert.equal(adminName("adm-{f}{last}", "Philip", "Meholm"), "adm-pmeholm");
  assert.equal(adminName("{First} (adm)", "Philip", "Meholm"), "Philip (adm)");
  assert.equal(adminName("a.{middle}", "Philip", "Meholm"), "a.{middle}");
});

test("parseIssues and fieldName: a path of the server becomes the name of a field", () => {
  const issues = parseIssues("people.guestPercent: must be from 0 to 100\noffices[1].callingCode: must be digits\nthe profile is bigger than 5 bytes");
  assert.deepEqual(issues, [
    { path: "people.guestPercent", message: "must be from 0 to 100" },
    { path: "offices[1].callingCode", message: "must be digits" },
    { path: "", message: "the profile is bigger than 5 bytes" },
  ]);
  assert.equal(fieldName("people.guestPercent"), "Guests, % of all");
  assert.equal(fieldName("offices[1].callingCode"), "Office 2, callingCode");
  assert.equal(fieldName("names.first[3]"), "First names");
  assert.equal(fieldName("organization.domain", { domain: "Default domain" }), "Default domain");
  assert.equal(fieldName("extensionAttributes[0].pattern"), "Attribute row 1, pattern");
  assert.equal(fieldName("groups.prefix"), "groups.prefix");
});

const sum = (w: number[]) => w.reduce((a, b) => a + b, 0);
const spread = (w: number[]) => Math.max(...w) - Math.min(...w);

test("a share is shown in percent with one decimal", () => {
  assert.equal(TOTAL, 1000);
  assert.equal(PER_PERCENT, 10);
  assert.equal(percentText(343), "34.3");
  assert.equal(percentText(600), "60.0");
  assert.equal(percentText(1), "0.1");
});

test("toShares: any weights become shares that sum to TOTAL, none below 1", () => {
  for (const w of [[1], [1, 1], [1, 1, 1], [24, 14, 12, 8, 14, 5, 3, 9], [0, 0], [5, 0, 5], [1000, 1], [85, 15], [3, 3, 3, 3, 3, 3, 3]]) {
    const out = toShares(w);
    assert.equal(sum(out), TOTAL, JSON.stringify(w));
    assert.ok(out.every((x) => Number.isInteger(x) && x >= 1), JSON.stringify(out));
  }
  assert.deepEqual(toShares([85, 15]), [850, 150]);
  assert.deepEqual(toShares([1, 1, 1]), [334, 333, 333]);
  assert.deepEqual(toShares([2, 2], 50), [25, 25]);
  assert.deepEqual(toShares([5]), [TOTAL]);
});

test("rebalance: the others give or take the difference in equal parts, and the sum stays TOTAL", () => {
  assert.deepEqual(rebalance([500, 300, 200], 0, 600), [600, 250, 150]);
  assert.deepEqual(rebalance([500, 300, 200], 0, 400), [400, 350, 250]);
  assert.deepEqual(rebalance([850, 150], 0, 700), [700, 300]);
  assert.deepEqual(rebalance([1000], 0, 300), [1000]);
  // an odd difference: the rounding leftover goes to the smaller items, so the bigger ones give a little more
  assert.deepEqual(rebalance([400, 300, 300], 0, 401), [401, 300, 299]);
  assert.deepEqual(rebalance([400, 400, 200], 2, 201), [400, 399, 201]);
  // an item does not go below 1, and the rest of the difference goes to the others
  assert.deepEqual(rebalance([300, 690, 10], 0, 600), [600, 399, 1]);
  assert.deepEqual(rebalance([100, 100, 800], 2, 1000), [1, 1, 998]);
  // a lot of random sets keep the invariants
  let seed = 7;
  const rnd = (n: number) => ((seed = (seed * 1103515245 + 12345) % 2147483648) % n);
  for (let r = 0; r < 500; r++) {
    const n = 2 + rnd(9);
    const start = toShares(Array.from({ length: n }, () => 1 + rnd(500)));
    const i = rnd(n);
    const v = rnd(1300) - 100;
    const out = rebalance(start, i, v);
    assert.equal(sum(out), TOTAL, `${start} ${i} ${v} -> ${out}`);
    assert.ok(out.every((x) => Number.isInteger(x) && x >= 1), `${start} ${i} ${v} -> ${out}`);
    assert.equal(out[i], Math.max(1, Math.min(TOTAL - (n - 1), v)), `${start} ${i} ${v} -> ${out}`);
  }
});

test("a drag in steps of 0.1% takes from all the others, and the others move by 0.1% at most per step", () => {
  const items = [{ weight: 250 }, { weight: 250 }, { weight: 250 }, { weight: 250 }];
  grab(items);
  let last = items.map((x) => x.weight);
  let biggestJump = 0;
  for (let v = 251; v <= 400; v++) {
    setShare(items, 0, v); // the pointer moves 0.1% at a time
    const now = items.map((x) => x.weight);
    biggestJump = Math.max(biggestJump, ...now.map((w, k) => Math.abs(w - last[k])).slice(1));
    last = now;
  }
  release(items);
  assert.equal(items[0].weight, 400);
  assert.equal(sum(items.map((x) => x.weight)), TOTAL);
  assert.ok(spread(items.slice(1).map((x) => x.weight)) <= 1, "the others gave the same amount");
  assert.ok(biggestJump <= 1, `no other share jumps by more than 0.1% in a step, it jumped ${biggestJump / PER_PERCENT}%`);
  // the same without a grab (a key pressed again and again) does not empty one item either
  const keys = [{ weight: 250 }, { weight: 250 }, { weight: 250 }, { weight: 250 }];
  for (let v = 251; v <= 400; v++) setShare(keys, 0, v);
  assert.ok(spread(keys.slice(1).map((x) => x.weight)) <= 1);
});

test("a locked item does not change, and the free items share the rest", () => {
  assert.deepEqual(rebalance([400, 300, 200, 100], 0, 500, [false, false, true, false]), [500, 250, 200, 50]);
  assert.deepEqual(rebalance([400, 300, 200, 100], 2, 500, [false, false, true, false]), [400, 300, 200, 100]); // a locked item cannot be set
  assert.deepEqual(rebalance([400, 300, 300], 0, 600, [false, true, true]), [400, 300, 300]); // nobody to give or take
  // the top is what the locked ones leave, less 1 for each free item
  assert.deepEqual(rebalance([400, 300, 200, 100], 0, 999, [false, false, true, false]), [798, 1, 200, 1]);
  const items = [{ weight: 500 }, { weight: 300 }, { weight: 200 }];
  const locked = (x: { weight?: number }) => x === items[1];
  setShare(items, 0, 900, locked); // too much: the locked 30% stays, and the one free item keeps 0.1%
  assert.deepEqual(items.map((x) => x.weight), [699, 300, 1]);
  // add: the locked items keep theirs. With all locked there is nobody to take from.
  const list: { name: string; weight?: number }[] = [{ name: "a", weight: 600 }, { name: "b", weight: 400 }];
  assert.equal(addShare(list, { name: "c" }, (x) => x.name === "a"), true);
  assert.equal(list[0].weight, 600);
  assert.equal(sum(list.map((x) => x.weight!)), TOTAL);
  assert.equal(addShare(list, { name: "d" }, () => true), false);
  assert.equal(list.length, 3);
  // remove: the free items take it back. If all the others are locked, the first takes it.
  removeShare(list, 2, (x) => x.name === "a");
  assert.deepEqual(list.map((x) => x.weight), [600, 400]);
  removeShare(list, 1, () => true);
  assert.deepEqual(list.map((x) => x.weight), [TOTAL]);
});

test("shares of a list: add gives an equal share, remove gives it back", () => {
  const items = [{ name: "a", weight: 600 }, { name: "b", weight: 400 }];
  addShare(items, { name: "c" });
  assert.equal(sum(items.map((x) => x.weight ?? 0)), TOTAL);
  assert.equal(items[2].weight, 333);
  assert.ok(items[0].weight! > items[1].weight!, "the ratio stays");
  removeShare(items, 2);
  assert.deepEqual(items.map((x) => x.weight), [600, 400]);
  const none: { name: string; weight?: number }[] = [];
  addShare(none, { name: "first" });
  assert.equal(none[0].weight, TOTAL);
  const set = [{ weight: 500 }, { weight: 500 }];
  setShare(set, 1, 800);
  assert.deepEqual(set.map((x) => x.weight), [200, 800]);
});

test("niceScale: the scale of a bar is the first round percent that holds the biggest value (in weights)", () => {
  assert.equal(niceScale([10, 10, 10]), 20); // 1% each: a scale of 2%
  assert.equal(niceScale([10, 30, 50]), 50);
  assert.equal(niceScale([10, 30, 80]), 100);
  assert.equal(niceScale([120]), 200);
  assert.equal(niceScale([510]), 1000);
  assert.equal(niceScale([]), 20);
  assert.equal(niceScale([5000]), 1000);
});

test("domains: rows from the profile, and back", () => {
  const o = { initialDomain: "acme.onmicrosoft.com", domain: "acme.example", domainWeight: 6, domains: [{ name: "acme.com", weight: 3 }] } as Profile["organization"];
  const rows = domainRows(o);
  assert.deepEqual(rows, [
    { name: "acme.onmicrosoft.com", weight: 1 },
    { name: "acme.example", weight: 6 },
    { name: "acme.com", weight: 3 },
  ]);
  // another default: the old default becomes one of the others
  applyDomainRows(o, rows, "acme.com");
  assert.equal(o.domain, "acme.com");
  assert.equal(o.domainWeight, 3);
  assert.deepEqual(o.domains, [{ name: "acme.onmicrosoft.com", weight: 1 }, { name: "acme.example", weight: 6 }]);
  // the initial domain as the default: one row, no others
  const one = { initialDomain: "acme.onmicrosoft.com", domain: "acme.onmicrosoft.com", domainWeight: 2, domains: [] } as unknown as Profile["organization"];
  assert.deepEqual(domainRows(one), [{ name: "acme.onmicrosoft.com", weight: 2 }]);
  assert.equal(tenantOf("acme.onmicrosoft.com"), "acme");
  assert.equal(tenantOf("other.example"), "other.example");
});

function sample(): Profile {
  return {
    organization: { guestDomain: "partner.example", domain: "a.example", initialDomain: "a.onmicrosoft.com", domainWeight: 6, domains: [{ name: "a.com", weight: 3 }] },
    offices: [{ name: "Oslo", weight: 5 }, { name: "Bergen", weight: 3 }],
    departments: [{ name: "Drift", weight: 24, titles: [{ name: "A", weight: 6 }, { name: "B", weight: 2 }] }, { name: "IT", weight: 14, titles: [{ name: "C" }] }],
    people: { languages: [{ code: "nb-NO", weight: 85 }, { code: "en-US", weight: 15 }] },
    names: { first: ["Ola"], last: ["Berg"] },
    extensionAttributes: [{ index: 2, values: [{ value: "x", weight: 92 }, { value: "y", weight: 8 }] }, { index: 1, pattern: "K-{1-9}" }],
  } as unknown as Profile;
}

test("normalize: the guest domain goes to the people, every group sums to TOTAL, and the names are by language", () => {
  const p = normalize(sample(), (locale) => (locale === "en-US" ? { first: ["Jack"], last: ["Smith"] } : undefined));
  assert.deepEqual(p.people.guestDomains, [{ name: "partner.example", weight: TOTAL }]);
  assert.equal(p.organization.guestDomain, undefined);
  const total = (l: { weight?: number }[]) => l.reduce((a, b) => a + (b.weight ?? 0), 0);
  assert.equal(total(p.offices), TOTAL);
  assert.equal(total(p.departments), TOTAL);
  for (const d of p.departments) assert.equal(total(d.titles), TOTAL);
  assert.equal(total(p.people.languages), TOTAL);
  assert.deepEqual(p.people.languages.map((l) => l.weight), [850, 150]);
  assert.equal(total(p.extensionAttributes![0].values!), TOTAL);
  assert.equal((p.organization.domainWeight ?? 0) + total(p.organization.domains!), TOTAL);
  assert.deepEqual(p.organization.domains?.length, 1);
  // the names of the profile go to its most common language, the other language gets the names of its country
  assert.deepEqual(p.names.byLanguage, { "nb-NO": { first: ["Ola"], last: ["Berg"] }, "en-US": { first: ["Jack"], last: ["Smith"] } });
  assert.deepEqual([p.names.first, p.names.last], [[], []]);
  // a second run changes nothing
  const again = JSON.stringify(normalize(structuredClone(p)));
  assert.equal(again, JSON.stringify(p));
  // a profile that the wizard kept with the old total (100) is scaled up with the same ratios
  const old = sample();
  old.people.languages = [{ code: "nb-NO", weight: 85 }, { code: "en-US", weight: 15 }];
  assert.deepEqual(normalize(old).people.languages.map((l) => l.weight), [850, 150]);
});

test("renameDepartment: every place that points to it follows", () => {
  const p = {
    groups: { dynamicDepartment: "IT", microsoft365Departments: ["IT", "HR"], strayDepartments: ["HR"], apps: [{ name: "x", title: "X", contributorDepartments: ["IT"], readerDepartments: ["HR"] }] },
    owner: { department: "IT", jobTitle: "" },
    admin: { department: "HR" },
  } as unknown as Profile;
  renameDepartment(p, "IT", "Tech");
  assert.equal(p.groups.dynamicDepartment, "Tech");
  assert.deepEqual(p.groups.microsoft365Departments, ["Tech", "HR"]);
  assert.deepEqual(p.groups.apps?.[0].contributorDepartments, ["Tech"]);
  assert.equal(p.owner.department, "Tech");
  assert.equal(p.admin.department, "HR");
  assert.deepEqual(p.groups.strayDepartments, ["HR"]);
});
