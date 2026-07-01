/**
 * Fold a string to a lowercase, diacritic-free, whitespace-normalized form for
 * accent-insensitive matching. Applied to BOTH the query and the haystack so
 * `maria` hits `María` and `nino` hits `niño` — the #1 correctness requirement
 * for Spanish search.
 *
 *   "María"  → NFD → "Mari" + "a"+◌́  → strip marks → "maria"
 *   "niño"   → NFD → "nin" + "o"+…, "n"+◌̃ → strip marks → "nino"
 */
export function fold(value: string): string {
  return value
    .normalize("NFD")
    // Strip Unicode combining marks (the decomposed accents/tilde).
    .replace(/[̀-ͯ]/g, "")
    .toLowerCase()
    .replace(/\s+/g, " ")
    .trim();
}
