// The use of a service principal over time (GET /_dolly/graph/usage/history). The server gives a point for every 10 seconds, with the resource
// units (RU), the calls, and the throttled calls. These helpers cut it to a range, group it so a long range has a few hundred bars, and say what
// the chart shows. They have no page in them, so a test can run them.

export interface UsagePoint {
  /** The first second of the bucket, as Unix time. */
  t: number;
  ru: number;
  calls: number;
  throttled: number;
}

export interface UsageHistory {
  bucketSeconds: number;
  retentionSeconds: number;
  now: number;
  points: UsagePoint[];
}

/** The limit of resource units of a client in a tenant, in a span (cost.json). */
export interface UsageLimit {
  scope: string;
  ru: number;
  seconds: number;
}

/** A bar of the chart: one bucket, or several together. `peak` is the most RU that one bucket had, the number to compare with the limit. */
export interface Bar {
  t: number;
  /** How many seconds the bar covers. */
  seconds: number;
  peak: number;
  ru: number;
  calls: number;
  throttled: number;
}

export interface Range {
  seconds: number;
  label: string;
}

const RANGES: Range[] = [
  { seconds: 60, label: "1 min" },
  { seconds: 300, label: "5 min" },
  { seconds: 900, label: "15 min" },
  { seconds: 3600, label: "1 hour" },
  { seconds: 6 * 3600, label: "6 hours" },
  { seconds: 24 * 3600, label: "24 hours" },
];

const label = (seconds: number) => (seconds % 3600 === 0 ? `${seconds / 3600} h` : seconds % 60 === 0 ? `${seconds / 60} min` : `${seconds} s`);

/** The ranges that the history holds: the usual ones that fit in the retention, and the retention itself if it is none of them. */
export function rangesFor(retentionSeconds: number): Range[] {
  const fit = RANGES.filter((r) => r.seconds <= retentionSeconds);
  if (!fit.some((r) => r.seconds === retentionSeconds)) fit.push({ seconds: retentionSeconds, label: label(retentionSeconds) });
  return fit;
}

/** The range to show first: 15 minutes, or the longest that the history holds if that is less. */
export function defaultRange(retentionSeconds: number): number {
  const fit = rangesFor(retentionSeconds);
  return (fit.find((r) => r.seconds === 900) ?? fit[fit.length - 1]).seconds;
}

/** The last `seconds` of the points. */
export function lastSeconds(points: readonly UsagePoint[], bucketSeconds: number, seconds: number): UsagePoint[] {
  return points.slice(-Math.max(1, Math.ceil(seconds / bucketSeconds)));
}

/** Group the points so that there are at most `max` bars. A bar has the peak of its buckets, and the sums of the rest. */
export function toBars(points: readonly UsagePoint[], bucketSeconds: number, max = 240): Bar[] {
  const factor = Math.max(1, Math.ceil(points.length / max));
  const bars: Bar[] = [];
  for (let i = 0; i < points.length; i += factor) {
    const group = points.slice(i, i + factor);
    bars.push({
      t: group[0].t,
      seconds: group.length * bucketSeconds,
      peak: Math.max(...group.map((p) => p.ru)),
      ru: group.reduce((a, p) => a + p.ru, 0),
      calls: group.reduce((a, p) => a + p.calls, 0),
      throttled: group.reduce((a, p) => a + p.throttled, 0),
    });
  }
  return bars;
}

export interface Summary {
  ru: number;
  calls: number;
  throttled: number;
  /** The most RU in one bucket, and when. */
  peak: number;
  peakAt: number | null;
  /** The peak as a share of the limit (0.8 is 80%), or null when the limit is not for a bucket of this length. */
  peakShare: number | null;
}

/** What the chart says in words. The limit counts only when its span is the span of a bucket: then a bucket and the limit are alike. */
export function summarize(points: readonly UsagePoint[], bucketSeconds: number, limit?: UsageLimit | null): Summary {
  let peak = 0;
  let peakAt: number | null = null;
  const s: Summary = { ru: 0, calls: 0, throttled: 0, peak: 0, peakAt: null, peakShare: null };
  for (const p of points) {
    s.ru += p.ru;
    s.calls += p.calls;
    s.throttled += p.throttled;
    if (p.ru > peak) [peak, peakAt] = [p.ru, p.t];
  }
  s.peak = peak;
  s.peakAt = peakAt;
  if (limit && limit.ru > 0 && limit.seconds === bucketSeconds) s.peakShare = peak / limit.ru;
  return s;
}

/** The top of the y axis: a round number above the biggest bar (and above the limit, when the chart is scaled to it). */
export function yMax(peak: number, limit?: number | null): number {
  const top = Math.max(peak * 1.1, limit ?? 0, 10);
  const pow = 10 ** Math.floor(Math.log10(top));
  for (const m of [1, 1.5, 2, 3, 5, 7.5, 10]) if (m * pow >= top) return m * pow;
  return 10 * pow;
}

/** The share of a limit as a text, with one decimal below 10%: 0.034 is "3.4%", 0.8 is "80%". */
export const percentOf = (share: number): string => `${share < 0.1 ? (share * 100).toFixed(1) : Math.round(share * 100)}%`;
