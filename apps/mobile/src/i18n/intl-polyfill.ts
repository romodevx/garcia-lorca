/**
 * Hermes Intl polyfill — load FIRST, before any ICU formatting.
 *
 * React Native's Hermes engine historically ships without full `Intl` locale
 * data, so `Intl.PluralRules` can be absent or locale-incomplete on device. That
 * would break i18next-icu's plural/select rendering (it routes through
 * `intl-messageformat`, which calls `Intl.PluralRules`). We install the FormatJS
 * polyfill with es + en data unconditionally: the mobile bundle only ever runs
 * on Hermes (or Node under Jest), so paying the polyfill cost is the safe,
 * deterministic choice over branching on a capability check.
 *
 * Order matters: canonical-locales → locale → plural-rules → locale data.
 */

import "@formatjs/intl-getcanonicallocales/polyfill.js";
import "@formatjs/intl-locale/polyfill.js";
import "@formatjs/intl-pluralrules/polyfill-force.js";
import "@formatjs/intl-pluralrules/locale-data/es";
import "@formatjs/intl-pluralrules/locale-data/en";
