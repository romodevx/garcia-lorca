import { messages as catalogs, NAMESPACES, toNested, type Locale, type Namespace } from "@garcia-lorca/i18n";

/**
 * Web i18n configuration (next-intl, WITHOUT i18n routing).
 *
 * The active locale is held in a cookie so the Server Component layout can pick
 * it up and render server-authoritatively — no hydration text mismatch, no
 * `[locale]` route segment. The toggle writes the cookie and refreshes.
 *
 * next-intl cannot consume flat dotted keys (dots = nesting), so each namespace
 * catalog from `@garcia-lorca/i18n` is run through `toNested` before next-intl
 * sees it. The package stays the flat, type-safe, parity-checked source of truth.
 */

export const LOCALE_COOKIE = "gl-locale";

/** Build the nested next-intl message tree for a locale: { common: {...}, auth: {...} }. */
export function buildMessages(locale: Locale): Record<Namespace, Record<string, unknown>> {
  const result = {} as Record<Namespace, Record<string, unknown>>;
  for (const namespace of NAMESPACES) {
    result[namespace] = toNested(catalogs[locale][namespace]);
  }
  return result;
}
