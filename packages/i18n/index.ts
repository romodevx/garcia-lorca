/**
 * @garcia-lorca/i18n — shared bilingual (es-MX default, en) i18n infrastructure.
 *
 * Cross-cutting infra (NO module, NO feature flag — like pnpm). It exports raw
 * ICU MessageFormat catalogs, type-safe keys, a parity validator, and a
 * framework-agnostic formatter. It imports NO UI framework: `apps/web` applies
 * the next-intl binding and `apps/mobile` the react-i18next binding, both over
 * these same catalogs.
 *
 * Official documents (boletas, constancias, kárdex) and SEP terms stay
 * Spanish-only and are out of this package's scope; the backend returns error
 * CODES, never translated text.
 */

export {
  DEFAULT_LOCALE,
  FORMAT_LOCALE,
  isLocale,
  LOCALES,
  type Locale,
} from "./src/locales";

export {
  catalog,
  messages,
  NAMESPACES,
  type AttendanceKey,
  type AuthKey,
  type CommonKey,
  type FinanceKey,
  type HomeKey,
  type MessageKey,
  type Namespace,
  type NamespaceKey,
} from "./src/messages/index";

export { formatMessage, type MessageValues } from "./src/format/format";

export { toNested, type NestedMessages } from "./src/nest";

export {
  checkParity,
  type ParityIssue,
  type ParityIssueKind,
  type RawCatalog,
} from "./src/parity/parity";
