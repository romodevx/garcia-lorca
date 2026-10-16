import { balanceLine, formatDate, formatDayWithYear, pesos } from "./format";

describe("pesos", () => {
  it("formats integer cents as an es-MX peso string", () => {
    expect(pesos(240000)).toBe("$2,400.00");
    expect(pesos(0)).toBe("$0.00");
  });
});

describe("formatDate", () => {
  it("formats an ISO date as a day + short month", () => {
    expect(formatDate("2026-06-04")).toBe("4 jun");
  });

  it("returns an empty string for an unparseable value", () => {
    expect(formatDate("")).toBe("");
    expect(formatDate("not-a-date")).toBe("");
  });
});

describe("formatDayWithYear", () => {
  it("reads an ISO day as the calendar day it names, with its year", () => {
    expect(formatDayWithYear("2026-10-15")).toBe("15 oct 2026");
  });

  it("returns an empty string for an unparseable value", () => {
    expect(formatDayWithYear("")).toBe("");
  });
});

// One balance, read the same everywhere (T5): below zero the school owes the
// family, which is said in words and never as a minus sign.
describe("balanceLine", () => {
  it("names a negative balance as credit, with a positive amount", () => {
    expect(balanceLine(-50000)).toBe("Saldo a favor $500.00");
  });

  it("names a positive balance as pending", () => {
    expect(balanceLine(320000)).toBe("Saldo pendiente $3,200.00");
  });
});
