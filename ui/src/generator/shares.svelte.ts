import { SvelteSet } from "svelte/reactivity";
import { addShare, removeShare, setShare } from "./profile";

// Which items have a locked share. A locked item keeps its share: the others cannot take from it, and it cannot be set. The lock is a thing of
// the editor and not of the profile, so it is kept here by the item itself, and it is gone when the page is loaded again.

const locks = new SvelteSet<object>();

export const isLocked = (item: object): boolean => locks.has(item);

export function toggleLock(item: object): void {
  if (!locks.delete(item)) locks.add(item);
}

type Weighted = { weight?: number };

/** The shares of a list, with the locks. */
export const shareTo = <T extends Weighted>(items: T[], i: number, value: number) => setShare(items, i, value, isLocked);
export const addTo = <T extends Weighted>(items: T[], item: Omit<T, "weight">) => addShare(items, item, isLocked);
export const removeFrom = <T extends Weighted>(items: T[], i: number) => removeShare(items, i, isLocked);

/** An item can be added when at least one item is free to give a share. */
export const canAdd = (items: Weighted[]): boolean => items.length === 0 || items.some((x) => !locks.has(x));
