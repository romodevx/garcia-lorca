/**
 * Money conversion for the finance surface — pure, no React, so it is unit
 * tested in isolation (`money.test.ts`).
 *
 * The API layer speaks integer CENTS end to end. Pesos exist ONLY here, at the
 * boundary: `formatCents`/`centsToPesosInput` on the way out, `pesosToCents` in
 * the one place a human types an amount. No float ever crosses back into the
 * client/API layer.
 */

/** A comma that groups exactly three digits — a thousands separator, not a decimal point. */
const THOUSANDS_GROUP = /,(\d{3})(?!\d)/g;

/**
 * A peso amount: up to 9 whole digits (the charge ceiling) and at most two
 * decimals. Deliberately narrow — no sign, no exponent, no hex, no third
 * decimal — so nothing ambiguous can become a charge.
 */
const PESO_AMOUNT = /^(\d{1,9})(?:\.(\d{1,2}))?$/;

const CENTS_PER_PESO = 100;

/**
 * Currency display is pinned to es-MX/MXN regardless of the UI locale: the
 * amount IS Mexican pesos, so an English UI must not re-render it as USD.
 */
export function formatCents(cents: number): string {
  return (cents / CENTS_PER_PESO).toLocaleString("es-MX", { style: "currency", currency: "MXN" });
}

/** The peso value a human sees in an amount input, e.g. 185000 → "1850.00". */
export function centsToPesosInput(cents: number): string {
  return (cents / CENTS_PER_PESO).toFixed(2);
}

/**
 * Parse a typed peso amount into integer cents, or `NaN` when the text is not
 * unambiguously an amount — the caller MUST reject `NaN` instead of sending it.
 *
 * Strict on purpose. A permissive `Number()` read the decimal comma of
 * "1850,50" as a thousands separator and charged 185050 pesos instead of
 * 1850.50 — a 100× overcharge, multiplied again by bulk generation. So only
 * spaces and true thousands groups are stripped; everything else must match
 * `PESO_AMOUNT` exactly. Whole and fractional parts are summed as integers, so
 * no float rounding can shift a cent.
 */
export function pesosToCents(pesos: string): number {
  const normalized = pesos.replace(/\s/g, "").replace(THOUSANDS_GROUP, "$1");
  const amount = PESO_AMOUNT.exec(normalized);
  if (amount === null) {
    return Number.NaN;
  }
  const [, whole = "0", decimals = ""] = amount;
  return Number(whole) * CENTS_PER_PESO + Number(decimals.padEnd(2, "0"));
}
