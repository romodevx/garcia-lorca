import { balanceLine, formatDate, formatDayWithYear, pesos, totalBalanceLine } from "./format";

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

// ISSUE-T14-106: on the phone (Hermes) toLocaleDateString ignored the es-MX
// options and the account read "Al 2026-10-17". Dates are built from a table,
// like attendanceView, so the device's Intl cannot change them.
describe("dates do not depend on the device's Intl", () => {
  let spy: jest.SpyInstance;
  beforeEach(() => {
    spy = jest.spyOn(Date.prototype, "toLocaleDateString").mockImplementation(function (this: Date) {
      return this.toISOString().slice(0, 10);
    });
  });
  afterEach(() => spy.mockRestore());

  it("writes the account's day with the month in words", () => {
    expect(formatDayWithYear("2026-10-17")).toBe("17 oct 2026");
  });

  it("writes a row's day with the month in words, short and long", () => {
    expect(formatDate("2026-06-04")).toBe("4 jun");
    expect(formatDate("2026-06-04", "long")).toBe("4 de junio");
  });
});

// ISSUE-T14-106: the header amount is the whole account, and says so.
describe("totalBalanceLine", () => {
  it("labels the account's balance as the total", () => {
    expect(totalBalanceLine(2520000)).toBe("Saldo total $25,200.00");
  });

  it("still names a balance below zero as credit", () => {
    expect(totalBalanceLine(-50000)).toBe("Saldo a favor $500.00");
  });
});
