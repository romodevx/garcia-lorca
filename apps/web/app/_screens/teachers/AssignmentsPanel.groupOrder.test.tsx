import { render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { listLabelledGroups, type Cycle } from "../../config/api";
import { AssignmentsPanel } from "./AssignmentsPanel";

const BASE = "http://gw";
const CYCLE: Cycle = { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-24", endsOn: "2027-07-09" };

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { "Content-Type": "application/json" } });
}

afterEach(() => vi.unstubAllGlobals());

// QA 2026-10-15, ISSUE-018: Docentes offered the groups in the server's order
// (Secundaria before Preescolar, 2° before 1°). Asistencia already sorted them.
describe("AssignmentsPanel — groups in school order (ISSUE-018)", () => {
  it("offers the groups by level, then grade, then name", async () => {
    const payloads: Record<string, unknown> = {
      "/groups": {
        groups: [
          { id: "s1A", cycleId: "c1", gradeLevelId: "sec1", name: "A", capacity: 30 },
          { id: "p2A", cycleId: "c1", gradeLevelId: "pri2", name: "A", capacity: 30 },
          { id: "p1B", cycleId: "c1", gradeLevelId: "pri1", name: "B", capacity: 30 },
          { id: "p1A", cycleId: "c1", gradeLevelId: "pri1", name: "A", capacity: 30 },
          { id: "k3A", cycleId: "c1", gradeLevelId: "pre3", name: "A", capacity: 30 },
        ],
      },
      "/grade-levels": {
        gradeLevels: [
          { id: "sec1", levelId: "sec", name: "1°", ordinal: 1 },
          { id: "pri2", levelId: "pri", name: "2°", ordinal: 2 },
          { id: "pri1", levelId: "pri", name: "1°", ordinal: 1 },
          { id: "pre3", levelId: "pre", name: "3°", ordinal: 3 },
        ],
      },
      "/levels": {
        levels: [
          { id: "sec", name: "SECUNDARIA", sepCode: "", enrollmentGranularity: "cycle", sortOrder: 3 },
          { id: "pre", name: "PREESCOLAR", sepCode: "", enrollmentGranularity: "cycle", sortOrder: 1 },
          { id: "pri", name: "PRIMARIA", sepCode: "", enrollmentGranularity: "cycle", sortOrder: 2 },
        ],
      },
    };
    vi.stubGlobal("fetch", vi.fn((url: string) => {
      const path = new URL(url).pathname;
      const key = Object.keys(payloads).find((k) => path.endsWith(k)) ?? "/groups";
      return Promise.resolve(jsonResponse(payloads[key]));
    }));

    render(
      <AssignmentsPanel
        cycles={[CYCLE]}
        teachers={[]}
        loadGroups={(cycleId) => listLabelledGroups(BASE, cycleId)}
        loadSubjects={vi.fn().mockResolvedValue([])}
        loadAssignments={vi.fn().mockResolvedValue([])}
        onCreate={vi.fn()}
      />,
    );

    const select = screen.getByLabelText("Grupo", { selector: "select" });
    await within(select).findByRole("option", { name: "1° PRIMARIA · A" });
    const labels = within(select).getAllByRole("option").map((o) => o.textContent).filter((t) => t !== "");
    expect(labels).toEqual(["3° PREESCOLAR · A", "1° PRIMARIA · A", "1° PRIMARIA · B", "2° PRIMARIA · A", "1° SECUNDARIA · A"]);
  });
});
