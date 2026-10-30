const clock = new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false });
const date = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "medium" });
const ago = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });

/** 14:03:22 */
export const timeOfDay = (t: string | number | Date) => clock.format(new Date(t));

/** Oct 28, 2026, 2:03:22 PM */
export const dateTime = (t: string | number | Date) => date.format(new Date(t));

/** "3 minutes ago" */
export function relative(t: string | number | Date, now = Date.now()): string {
  const seconds = Math.round((new Date(t).getTime() - now) / 1000);
  for (const [unit, size] of [
    ["day", 86400],
    ["hour", 3600],
    ["minute", 60],
  ] as const) {
    if (Math.abs(seconds) >= size) return ago.format(Math.round(seconds / size), unit);
  }
  return ago.format(seconds, "second");
}

/** groupBy puts the items in groups by a key. The groups are in the order in which their keys first appear. */
export function groupBy<T>(items: readonly T[], key: (item: T) => string): [string, T[]][] {
  const groups = new Map<string, T[]>();
  for (const item of items) {
    const k = key(item);
    const group = groups.get(k);
    if (group) group.push(item);
    else groups.set(k, [item]);
  }
  return [...groups];
}

/** The first 8 characters of a GUID: enough to tell two apart. */
export const shortId = (id: string) => id.slice(0, 8);

/** The tone of a status code: success, a fault of the caller, or 429 and the faults of the server. */
export function statusTone(status: number): "green" | "amber" | "red" {
  if (status === 429 || status >= 500) return "red";
  if (status >= 400) return "amber";
  return "green";
}

/** The text of a JSON body, laid out, or the text as it is when it is not JSON. */
export function prettyJSON(text: string): string {
  try {
    return JSON.stringify(JSON.parse(text), null, 2);
  } catch {
    return text;
  }
}
