import { describe, expect, it } from "vitest";

import { curpCheckDigit, curpNotices } from "./curpCheck";

// Test vectors cross-checked with python-stdnum 1.20 (stdnum.mx.curp.is_valid):
// GODE561231HDFNRR00 and PUCA150101HMNRCLA5 are valid; PUCA150101HMNRCLA0 (the
// QA's invented one, CC-314) is not.
describe("curpCheckDigit (RENAPO)", () => {
  it.each([
    ["GODE561231HDFNRR0", 0],
    ["PUCA150101HMNRCLA", 5],
    ["HEGB050312HDFRNN0", 1],
  ])("%s → %i", (first17, digit) => {
    expect(curpCheckDigit(first17)).toBe(digit);
  });
});

describe("curpNotices (CC-314): warnings, never a block", () => {
  it("a sound CURP with a matching birth date says nothing", () => {
    expect(curpNotices("GODE561231HDFNRR00", "1956-12-31")).toEqual([]);
    expect(curpNotices("PUCA150101HMNRCLA5", "2015-01-01")).toEqual([]);
  });

  it("a sound CURP without a birth date says nothing", () => {
    expect(curpNotices("PUCA150101HMNRCLA5", "")).toEqual([]);
  });

  it("an invented check digit is flagged", () => {
    const notices = curpNotices("PUCA150101HMNRCLA0", "");
    expect(notices).toHaveLength(1);
    expect(notices[0]).toMatch(/dígito verificador/);
  });

  it("is read case- and space-insensitively, as the server stores it", () => {
    expect(curpNotices("  puca150101hmnrcla5 ", "2015-01-01")).toEqual([]);
  });

  it("a birth date the CURP does not carry is flagged, naming both days", () => {
    const notices = curpNotices("PUCA150101HMNRCLA5", "2014-10-17");
    expect(notices).toHaveLength(1);
    expect(notices[0]).toContain("01/01/2015");
    expect(notices[0]).toContain("17/10/2014");
  });

  it("the century comes from the 17th character: a digit is 19xx, a letter 20xx", () => {
    expect(curpNotices("GODE561231HDFNRR00", "2056-12-31")[0]).toContain("31/12/1956");
  });

  it("a date that does not exist inside the CURP is flagged", () => {
    const first17 = "PUCA150231HMNRCLA";
    const notices = curpNotices(`${first17}${curpCheckDigit(first17)}`, "");
    expect(notices).toHaveLength(1);
    expect(notices[0]).toMatch(/no existe/);
  });

  it("the sex the CURP carries (H/M) is checked against the file's, when it has one", () => {
    // PUCA150101HMNRCLA5 is H: a boy (Masculino, "M" on the file).
    expect(curpNotices("PUCA150101HMNRCLA5", "", "M")).toEqual([]);
    const notices = curpNotices("PUCA150101HMNRCLA5", "", "F");
    expect(notices).toHaveLength(1);
    expect(notices[0]).toMatch(/hombre/);
    expect(curpNotices("PUCA150101HMNRCLA5", "", "X")).toEqual([]);
    expect(curpNotices("PUCA150101HMNRCLA5", "", "")).toEqual([]);
  });

  it("a CURP still being typed, or of the wrong shape, is left to the length and format checks", () => {
    expect(curpNotices("PUCA1501", "")).toEqual([]);
    expect(curpNotices("PUCA15010!HMNRCLA5", "")).toEqual([]);
  });
});
