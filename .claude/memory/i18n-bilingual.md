# i18n — bilingual es-MX (default) + en

**Rule (binding, from the user):** the system is bilingual. Default **es-MX**, secondary
**en**. The UI is translated; the **backend returns CODES, not translated text**.

## Hard constraints
- **UI translation** flows through the shared package **`@garcia-lorca/i18n`** — cross-cutting
  infra, NOT a module, **no feature flag** (like pnpm). See [[package-manager-pnpm]].
  - Web (`apps/web`): **next-intl** binding over the package's catalogs.
  - Mobile (`apps/mobile`): **react-i18next** (+ `i18next-icu`) binding over the same catalogs.
- **Official documents** (boletas, constancias, kárdex) and **SEP terms**: **Spanish-only**,
  out of the translation layer.
- **Backend returns error CODES** (e.g. `error.auth.invalid_credentials`), never localized
  strings. The front maps codes → text via the catalogs. Codes are added to the `auth`
  namespace only when each service's proto contract freezes (iam at M01).
- **CI gate:** fails on hardcoded UI strings or a translation key missing in any language
  (total es⇄en parity).

## Package design (built on branch `chore/i18n-package`, 2026-06-16)
- **One ICU MessageFormat catalog → two bindings.** The package exports raw ICU messages
  (flat JSON per locale/namespace) + types + a parity validator + a framework-agnostic
  formatter. It imports **no** UI framework; each app applies its own binding.
- **Type-safe keys, zero codegen:** `keyof typeof <flat-json>` (with `resolveJsonModule`)
  yields literal key unions — a bad key fails compilation. Requires **flat keys**, enforced
  by the parity validator.
- **Parity validator** checks more than keys: ICU **argument-signature** parity, valid ICU,
  mandatory `other` branch, non-empty values, flat keys. (Key-only parity lets a message
  with a divergent `{placeholder}` crash at render in one language.)
- **Locale id ≠ format locale:** catalog id `es`/`en`; the CLDR tag fed to the ICU engine is
  `es` → **`es-MX`** (correct MXN/date formatting for finance/invoices later), `en` → `en`.
- **Framework-agnostic `formatMessage`** kept (shares the pinned `intl-messageformat` the
  apps use — same engine, not a third) for tests, scripts, and backend email/notification text.
- **Mobile contract:** disable i18next native `{{double-brace}}` interpolation + `escapeValue:false`
  so `i18next-icu` owns formatting; **verify/polyfill Hermes Intl** before any plural/number
  message ships (Hermes historically lacks full `Intl`).

Seeded namespaces this cycle: `common` (full) + `auth` (UI keys only; error codes deferred to M01).
See [[web-frontend-design]] and [[mobile-app-design]] for the surfaces that consume it.
