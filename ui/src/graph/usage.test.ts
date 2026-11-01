import assert from "node:assert/strict";
import { test } from "node:test";
import { defaultRange, lastSeconds, percentOf, rangesFor, summarize, toBars, yMax, type UsagePoint } from "./usage.ts";

const points = (rus: number[], throttled: number[] = []): UsagePoint[] => rus.map((ru, i) => ({ t: 1000 + i * 10, ru, calls: ru ? 1 : 0, throttled: throttled[i] ?? 0 }));

test("rangesFor: the usual ranges that fit, and the retention itself if it is none of them", () => {
  assert.deepEqual(rangesFor(3600).map((r) => r.label), ["1 min", "5 min", "15 min", "1 hour"]);
  assert.deepEqual(rangesFor(24 * 3600).map((r) => r.seconds), [60, 300, 900, 3600, 21600, 86400]);
  assert.deepEqual(rangesFor(120).map((r) => r.label), ["1 min", "2 min"]);
  assert.deepEqual(rangesFor(90 * 60).map((r) => r.label), ["1 min", "5 min", "15 min", "1 hour", "90 min"]);
});

test("defaultRange: 15 minutes, or the longest that the history holds", () => {
  assert.equal(defaultRange(3600), 900);
  assert.equal(defaultRange(300), 300);
  assert.equal(defaultRange(60), 60);
});

test("lastSeconds: the newest buckets of a range", () => {
  const p = points(Array.from({ length: 360 }, (_, i) => i));
  assert.equal(lastSeconds(p, 10, 300).length, 30);
  assert.equal(lastSeconds(p, 10, 300)[29].ru, 359);
  assert.equal(lastSeconds(p, 10, 5).length, 1);
  assert.equal(lastSeconds(p, 10, 99999).length, 360);
});

test("toBars: a long range is grouped, and a bar keeps the peak and the sums", () => {
  const bars = toBars(points(Array.from({ length: 8640 }, (_, i) => (i === 5000 ? 900 : 1))), 10);
  assert.ok(bars.length <= 240, `${bars.length} bars`);
  assert.equal(bars.reduce((a, b) => a + b.ru, 0), 8639 + 900);
  assert.equal(Math.max(...bars.map((b) => b.peak)), 900);
  assert.equal(bars[0].seconds, 36 * 10);
  // a short range is one bar for each bucket
  const few = toBars(points([5, 0, 7]), 10);
  assert.deepEqual(few.map((b) => [b.peak, b.ru, b.seconds]), [[5, 5, 10], [0, 0, 10], [7, 7, 10]]);
});

test("summarize: the totals, the peak, and the peak as a share of the limit", () => {
  const p = points([100, 3500, 0, 700], [0, 4, 0, 0]);
  const s = summarize(p, 10, { scope: "Tenant_Application", ru: 3500, seconds: 10 });
  assert.deepEqual([s.ru, s.calls, s.throttled, s.peak, s.peakAt, s.peakShare], [4300, 3, 4, 3500, 1010, 1]);
  // a limit of another span cannot be compared with a bucket
  assert.equal(summarize(p, 10, { scope: "x", ru: 150000, seconds: 20 }).peakShare, null);
  assert.equal(summarize(p, 10, null).peakShare, null);
  const empty = summarize(points([0, 0]), 10, { scope: "x", ru: 3500, seconds: 10 });
  assert.deepEqual([empty.peak, empty.peakAt, empty.peakShare], [0, null, 0]);
});

test("yMax: a round number above the biggest bar, and above the limit when asked", () => {
  assert.equal(yMax(0), 10);
  assert.equal(yMax(7), 10);
  assert.equal(yMax(120), 150);
  assert.equal(yMax(3400), 5000);
  assert.equal(yMax(120, 3500), 5000);
  assert.equal(yMax(900, 8000), 10000);
});

test("percentOf: one decimal below 10%", () => {
  assert.equal(percentOf(0.034), "3.4%");
  assert.equal(percentOf(0.8), "80%");
  assert.equal(percentOf(1), "100%");
  assert.equal(percentOf(1.8), "180%");
});
