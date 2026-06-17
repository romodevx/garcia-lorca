import {
  messages as catalogs,
  LOCALES,
  NAMESPACES,
  toNested,
  type Locale,
  type Namespace,
  type NestedMessages,
} from "@garcia-lorca/i18n";

/**
 * Mobile i18n configuration (react-i18next + i18next-icu).
 *
 * Like the web binding, i18next treats "." as nesting and cannot consume flat
 * dotted keys, so each namespace catalog from `@garcia-lorca/i18n` is run through
 * `toNested` before i18next sees it. The package stays the flat, type-safe,
 * parity-checked source of truth.
 */

export const LOCALE_STORAGE_KEY = "gl-locale";

export type I18nResources = Record<Locale, Record<Namespace, NestedMessages>>;

/** Build the nested i18next resource tree for every locale + namespace. */
export function buildResources(): I18nResources {
  const resources = {} as I18nResources;
  for (const locale of LOCALES) {
    const byNamespace = {} as Record<Namespace, NestedMessages>;
    for (const namespace of NAMESPACES) {
      byNamespace[namespace] = toNested(catalogs[locale][namespace]);
    }
    resources[locale] = byNamespace;
  }
  return resources;
}
