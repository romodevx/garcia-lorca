/**
 * Recents = a short MRU list of **command IDs only** (never record data).
 *
 * SECURITY: storing only command IDs means a user who later loses access to a
 * module leaks nothing — the ID simply fails to resolve against the (flag-
 * filtered) registry on the next open and is dropped. Never persist titles,
 * names, amounts, or any record fields here.
 */

export const RECENTS_KEY = "gl-cmd-recents";
const MAX_RECENTS = 5;

/** Prepend `id` to the MRU list, de-duplicating and capping length. */
export function pushRecent(existing: readonly string[], id: string, max: number = MAX_RECENTS): string[] {
  return [id, ...existing.filter((existingId) => existingId !== id)].slice(0, max);
}
