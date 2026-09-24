import { describe, expect, it } from "vitest";

import { birthDateError, phoneDigits, phoneError, postalCodeError } from "./profileRules";

describe("profile rules", () => {
  it.each([
    ["5512345678", undefined],
    ["55 1234 5678", undefined],
    ["(55) 1234-5678", undefined],
    ["555123", "Un teléfono lleva 10 dígitos."],
    ["551234567890", "Un teléfono lleva 10 dígitos."],
  ])("phone %s", (raw, want) => {
    expect(phoneError(raw)).toBe(want);
  });

  it("keeps only the digits of a phone", () => {
    expect(phoneDigits(" (55) 1234-5678 ")).toBe("5512345678");
  });

  it.each([
    ["01000", undefined],
    ["1000", "El código postal lleva 5 dígitos."],
    ["0100A", "El código postal lleva 5 dígitos."],
  ])("postal code %s", (raw, want) => {
    expect(postalCodeError(raw)).toBe(want);
  });

  it("refuses a birth date after today", () => {
    expect(birthDateError("2026-09-25", "2026-09-24")).toBe("La fecha de nacimiento no puede ser futura.");
    expect(birthDateError("2026-09-24", "2026-09-24")).toBeUndefined();
    expect(birthDateError("2019-03-12", "2026-09-24")).toBeUndefined();
  });
});
