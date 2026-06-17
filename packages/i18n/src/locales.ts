/**
 * Locale identity for the monorepo.
 *
 * Two distinct concepts, deliberately decoupled (eng-review decision, 2026-06-16):
 *
 *   catalog id    — keys the message directories & app locale switch: `es` | `en`.
 *                   Region-agnostic so a single `es` catalog serves all of LatAm.
 *   format locale — the CLDR tag fed to the ICU engine for number/currency/date
 *                   formatting: `es` → `es-MX` (MXN grouping, dd/mm/yyyy), `en` → `en`.
 *
 * Catalog id drives message lookup; format locale drives Intl formatting. They are
 * not interchangeable — feeding `es` (not `es-MX`) to the engine would render the
 * wrong currency/date once the finance/invoices namespaces land.
 */

export const LOCALES = ["es", "en"] as const;

export type Locale = (typeof LOCALES)[number];

/** Default locale for the system: Mexican Spanish. */
export const DEFAULT_LOCALE: Locale = "es";

/** Maps a catalog id to its CLDR format locale for the ICU engine. */
export const FORMAT_LOCALE: Record<Locale, string> = {
  es: "es-MX",
  en: "en",
};

export function isLocale(value: string): value is Locale {
  return (LOCALES as readonly string[]).includes(value);
}
