import { IntlMessageFormat } from "intl-messageformat";

import { DEFAULT_LOCALE, FORMAT_LOCALE, type Locale } from "../locales";

/** Values accepted by ICU interpolation (plain, framework-agnostic). */
export type MessageValues = Record<string, string | number | Date>;

/**
 * Format a raw ICU MessageFormat string with the same `intl-messageformat`
 * engine the apps use. Framework-agnostic on purpose: no React, no next-intl,
 * no i18next. The `locale` is a catalog id (`es` | `en`); it is mapped to its
 * CLDR FORMAT locale (`es` → `es-MX`) before reaching the engine.
 *
 * For the apps, prefer the framework binding (next-intl / react-i18next-icu).
 * This helper is for tests, scripts, and backend-adjacent rendering.
 */
export function formatMessage(
  message: string,
  values?: MessageValues,
  locale: Locale = DEFAULT_LOCALE,
): string {
  const formatter = new IntlMessageFormat(message, FORMAT_LOCALE[locale]);
  return String(formatter.format(values));
}
