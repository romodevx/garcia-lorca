import { fold } from "@garcia-lorca/ui/command-core";

/**
 * Whether a search box's query is found in any of the fields, the way the
 * office types: without accents, in any case, with stray spaces. "perez" finds
 * PÉREZ and "sofia" finds SOFÍA. An empty query matches everything.
 */
export function matchesSearch(query: string, ...fields: readonly string[]): boolean {
  const needle = fold(query);
  if (needle === "") return true;
  return fields.some((field) => fold(field).includes(needle));
}
