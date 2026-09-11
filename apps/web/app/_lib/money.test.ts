import { describe, expect, it } from "vitest";

import { centsToPesosInput, formatCents, pesosToCents } from "./money";

/**
 * Money integrity. `pesosToCents` is the ONLY door a human-typed amount walks
 * through before it becomes an integer the gateway will charge, so it must be
 * strict: anything ambiguous is rejected (`NaN`) instead of silently coerced.
 *
 * The bug these cases pin: a permissive `Number(input.replace(/[\s,]/g, ""))`
 * read "1850,50" (a decimal comma) as 185050 pesos — a 100× overcharge — and
 * happily accepted "1e3" and "0x10" as amounts.
 */

describe("pesosToCents", () => {
  const rejected: readonly { input: string; why: string }[] = [
    { input: "", why: "empty" },
    { input: " ", why: "blank" },
    { input: ",", why: "a lone separator" },
    { input: ".", why: "a lone decimal point" },
    { input: "1850,50", why: "a DECIMAL comma — 100x overcharge if coerced" },
    { input: "1,8505", why: "an ambiguous comma group" },
    { input: "1.005", why: "more precision than a cent" },
    { input: "1850.999", why: "more precision than a cent" },
    { input: "1e3", why: "exponent notation" },
    { input: "0x10", why: "hex notation" },
    { input: "-1", why: "a negative amount" },
    { input: "-1850.50", why: "a negative amount" },
    { input: "+1850", why: "a signed amount" },
    { input: "1850.5.5", why: "two decimal points" },
    { input: "abc", why: "not a number" },
    { input: "1850 pesos", why: "trailing text" },
    { input: "$1850", why: "a currency symbol" },
    { input: "Infinity", why: "not a finite amount" },
    { input: "1234567890", why: "beyond the 9-digit peso ceiling" },
  ];

  it.each(rejected)("rejects %o", ({ input }) => {
    expect(pesosToCents(input)).toBeNaN();
  });

  const accepted: readonly { input: string; cents: number }[] = [
    { input: "0", cents: 0 },
    { input: "0.01", cents: 1 },
    { input: "1850", cents: 185000 },
    { input: "1850.5", cents: 185050 },
    { input: "1850.50", cents: 185050 },
    { input: "1850.05", cents: 185005 },
    { input: " 1850.50 ", cents: 185050 },
    { input: "1 850.50", cents: 185050 },
    // A comma is a THOUSANDS separator only when it groups exactly 3 digits.
    { input: "1,850.50", cents: 185050 },
    { input: "1,850", cents: 185000 },
    { input: "12,345,678.90", cents: 1234567890 },
    { input: "999999999.99", cents: 99999999999 },
  ];

  it.each(accepted)("parses $input to $cents cents", ({ input, cents }) => {
    expect(pesosToCents(input)).toBe(cents);
  });

  it("returns an exact integer for every accepted amount (no float residue)", () => {
    for (const { input } of accepted) {
      expect(Number.isInteger(pesosToCents(input))).toBe(true);
    }
  });

  it("round-trips a formatted input back to the same cents", () => {
    for (const cents of [0, 1, 99, 100, 185050, 99999999999]) {
      expect(pesosToCents(centsToPesosInput(cents))).toBe(cents);
    }
  });
});

describe("centsToPesosInput", () => {
  it.each([
    { cents: 0, text: "0.00" },
    { cents: 1, text: "0.01" },
    { cents: 99, text: "0.99" },
    { cents: 185000, text: "1850.00" },
    { cents: 185050, text: "1850.50" },
  ])("renders $cents cents as $text", ({ cents, text }) => {
    expect(centsToPesosInput(cents)).toBe(text);
  });
});

describe("formatCents", () => {
  it.each([
    { cents: 0, text: "$0.00" },
    { cents: 185000, text: "$1,850.00" },
    { cents: 185050, text: "$1,850.50" },
  ])("formats $cents cents as MXN $text", ({ cents, text }) => {
    // Pinned to es-MX/MXN regardless of UI locale: the amount IS pesos.
    expect(formatCents(cents)).toBe(text);
  });
});

describe("un solo formateador para todo el sistema", () => {
  it("el tablero ya no redondea a pesos enteros", () => {
    // Dashboard.tsx tenía su propio money() con Math.round(cents / 100), así
    // que $1,850.50 se mostraba como $1,851. Un peso de diferencia en un saldo
    // es un reclamo en ventanilla.
    expect(formatCents(185050)).toBe("$1,850.50");
  });

  it("los centavos sobreviven a cualquier monto", () => {
    expect(formatCents(1)).toBe("$0.01");
    expect(formatCents(99)).toBe("$0.99");
  });

  it("siempre son pesos mexicanos, aunque la interfaz esté en inglés", () => {
    // El monto ES pesos: una interfaz en inglés no puede repintarlo como USD.
    expect(formatCents(100000)).toContain("1,000.00");
  });
});
