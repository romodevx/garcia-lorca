import "./intl-polyfill";

import { DEFAULT_LOCALE, LOCALES, NAMESPACES } from "@garcia-lorca/i18n";
import i18n from "i18next";
import ICU from "i18next-icu";
import { initReactI18next } from "react-i18next";
import { buildResources } from "./config";

/**
 * The single i18next instance for the parent app.
 *
 * Binding contract (eng-review, 2026-06-16):
 *   - `i18next-icu` owns formatting — our messages are single-brace ICU, NOT
 *     i18next's native `{{double-brace}}` interpolation.
 *   - `escapeValue: false` — i18next-icu/intl-messageformat output is already
 *     safe for React Native <Text>; i18next's HTML escaping would corrupt it and
 *     diverge from the web (next-intl) render.
 *   - Resources are the same flat catalogs the web uses, nested via `toNested`.
 */
void i18n
  .use(ICU)
  .use(initReactI18next)
  .init({
    resources: buildResources(),
    lng: DEFAULT_LOCALE,
    fallbackLng: DEFAULT_LOCALE,
    supportedLngs: [...LOCALES],
    ns: [...NAMESPACES],
    defaultNS: "common",
    interpolation: { escapeValue: false },
    returnNull: false,
    // Resources are bundled inline (no async backend), so there is nothing to
    // suspend on. Disabling Suspense keeps `useTranslation` synchronous — no
    // boundary needed and no throw-on-first-render in tests or on device.
    react: { useSuspense: false },
  });

export default i18n;
