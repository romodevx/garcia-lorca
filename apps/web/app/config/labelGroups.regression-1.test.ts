// Regression: ISSUE-001 — groups of different levels read the same in every picker
// Found by /qa on 2026-09-28
// Report: .gstack/qa-reports/qa-report-localhost-3002-2026-09-28-full.md
//
// The config screen asks for the grade "as the school names it", so a school
// writes "1°" under PRIMARIA and "1°" under SECUNDARIA. The label used only the
// grade's name, and the teacher-assignment picker offered "1° · A" twice.
import { afterEach, describe, expect, it, vi } from "vitest";

import { labelGroups, listLabelledGroups } from "./api";

const levels = [
  { id: "lp", name: "PRIMARIA", sepCode: "PRI", enrollmentGranularity: "CYCLE" as const },
  { id: "ls", name: "SECUNDARIA", sepCode: "SEC", enrollmentGranularity: "CYCLE" as const },
];
const grades = [
  { id: "p1", levelId: "lp", name: "1°", ordinal: 1 },
  { id: "s1", levelId: "ls", name: "1°", ordinal: 1 },
  { id: "k1", levelId: "lk", name: "1° Kínder", ordinal: 1 },
  { id: "p2", levelId: "lp", name: "2° Primaria", ordinal: 2 },
];
const groups = [
  { id: "a", cycleId: "c1", gradeLevelId: "p1", name: "A", capacity: 30 },
  { id: "b", cycleId: "c1", gradeLevelId: "s1", name: "A", capacity: 30 },
  { id: "c", cycleId: "c1", gradeLevelId: "k1", name: "A", capacity: 30 },
  { id: "d", cycleId: "c1", gradeLevelId: "p2", name: "A", capacity: 30 },
];

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

describe("labelGroups with grades named the same in two levels", () => {
  it("adds the level when the grade's name does not already carry it", () => {
    const labels = labelGroups(groups, grades, levels).map((g) => g.label);
    expect(labels).toEqual(["1° PRIMARIA · A", "1° SECUNDARIA · A", "1° Kínder · A", "2° Primaria · A"]);
  });

  it("keeps the grade-only label when no levels are given", () => {
    expect(labelGroups(groups.slice(0, 1), grades).map((g) => g.label)).toEqual(["1° · A"]);
  });
});

describe("listLabelledGroups", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("reads the levels so two first grades can be told apart", async () => {
    vi.stubGlobal("fetch", vi.fn((url: string) => {
      if (url.includes("/grade-levels")) return Promise.resolve(jsonResponse({ gradeLevels: grades }));
      if (url.includes("/levels")) return Promise.resolve(jsonResponse({ levels }));
      return Promise.resolve(jsonResponse({ groups: groups.slice(0, 2) }));
    }));
    const out = await listLabelledGroups("http://gw", "c1");
    expect(out.map((g) => g.label)).toEqual(["1° PRIMARIA · A", "1° SECUNDARIA · A"]);
  });

  it("still labels with the grade when the levels cannot be read", async () => {
    vi.stubGlobal("fetch", vi.fn((url: string) => {
      if (url.includes("/grade-levels")) return Promise.resolve(jsonResponse({ gradeLevels: grades }));
      if (url.includes("/levels")) return Promise.resolve(jsonResponse({}, 500));
      return Promise.resolve(jsonResponse({ groups: groups.slice(0, 2) }));
    }));
    const out = await listLabelledGroups("http://gw", "c1");
    expect(out.map((g) => g.label)).toEqual(["1° · A", "1° · A"]);
  });
});
