import { formatDate, pesos } from "./format";

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
