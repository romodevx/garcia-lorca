import { type Locale } from "../locales";
import enAuth from "./en/auth.json";
import enCommon from "./en/common.json";
import enPalette from "./en/palette.json";
import esAuth from "./es/auth.json";
import esCommon from "./es/common.json";
import esPalette from "./es/palette.json";

/**
 * Typed catalog assembly + key derivation.
 *
 * Type-safe keys with ZERO codegen (eng-review decision, 2026-06-16): with
 * `resolveJsonModule`, `keyof typeof <flat-json>` yields the literal union of the
 * file's keys. Values widen to `string` (irrelevant for key safety). The whole
 * mechanism only holds while catalogs stay FLAT — the parity validator enforces
 * that, so a nested key fails a test instead of silently degrading the types.
 *
 *   es/common.json ──keyof typeof──> "app.name" | "nav.dashboard" | … (CommonKey)
 *   es/auth.json   ──keyof typeof──> "login.title" | "field.email.label" | … (AuthKey)
 *   MessageKey = `common.${CommonKey}` | `auth.${AuthKey}`   (fully-qualified)
 */

export const NAMESPACES = ["common", "auth", "palette"] as const;
export type Namespace = (typeof NAMESPACES)[number];

/** All raw ICU catalogs, keyed by locale then namespace. `es` is the reference. */
export const messages = {
  es: { common: esCommon, auth: esAuth, palette: esPalette },
  en: { common: enCommon, auth: enAuth, palette: enPalette },
} as const satisfies Record<Locale, Record<Namespace, Record<string, string>>>;

/** Literal key unions derived from the reference (`es`) catalogs. */
export type CommonKey = keyof typeof esCommon;
export type AuthKey = keyof typeof esAuth;
export type PaletteKey = keyof typeof esPalette;

/** Per-namespace key lookup: `NamespaceKey<"auth">` = the auth catalog's keys. */
export type NamespaceKey<N extends Namespace> = N extends "common"
  ? CommonKey
  : N extends "auth"
    ? AuthKey
    : N extends "palette"
      ? PaletteKey
      : never;

/** Fully-qualified message key, e.g. `"common.action.save"` | `"palette.nav.students"`. */
export type MessageKey = `common.${CommonKey}` | `auth.${AuthKey}` | `palette.${PaletteKey}`;

/** The raw ICU catalog for one locale + namespace. */
export function catalog<N extends Namespace>(locale: Locale, namespace: N): Record<NamespaceKey<N>, string> {
  return messages[locale][namespace] as Record<NamespaceKey<N>, string>;
}
