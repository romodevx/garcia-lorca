import { describe, expect, it } from "vitest";

import type { Level, Term } from "../../../../config/api";
import { levelTermsLine, levelToOpen } from "./termsCoverage";

const MONTHS = ["2026-08", "2026-09", "2026-10"];
const A: Level = { id: "a", name: "PRIMARIA", sepCode: "", enrollmentGranularity: "cycle" };
const B: Level = { id: "b", name: "SECUNDARIA", sepCode: "", enrollmentGranularity: "cycle" };
const term = (id: string, levelId: string, months: string[]): Term => ({ id, cycleId: "c", levelId, name: id, ordinal: 1, startsOn: "2026-08-01", endsOn: "2026-10-31", months });

describe("levelTermsLine", () => {
  it.each([
    { name: "no terms", terms: [], line: "Sin periodos" },
    { name: "one term covering every month", terms: [term("t1", "a", MONTHS)], line: "1 periodo" },
    { name: "terms covering every month", terms: [term("t1", "a", ["2026-08"]), term("t2", "a", ["2026-09", "2026-10"])], line: "2 periodos" },
    { name: "a month left out", terms: [term("t1", "a", ["2026-08"])], line: "Faltan meses" },
  ])("says $line for $name", ({ terms, line }) => {
    expect(levelTermsLine(terms, MONTHS)).toBe(line);
  });
});

describe("levelToOpen", () => {
  it("opens the first level that still lacks periods", () => {
    expect(levelToOpen([A, B], [term("t1", "a", MONTHS)], MONTHS)).toBe("b");
  });

  it("opens a level whose periods leave a month out", () => {
    expect(levelToOpen([A, B], [term("t1", "a", ["2026-08"]), term("t2", "b", MONTHS)], MONTHS)).toBe("a");
  });

  it("opens the first level when every level is complete", () => {
    expect(levelToOpen([A, B], [term("t1", "a", MONTHS), term("t2", "b", MONTHS)], MONTHS)).toBe("a");
  });

  it("opens nothing when there are no levels", () => {
    expect(levelToOpen([], [], MONTHS)).toBeUndefined();
  });
});
