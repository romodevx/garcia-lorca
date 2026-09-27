// Regression: ISSUE-003 — a level without numeric grades showed counters and a "(0–0)" scale
// Found by /qa on 2026-09-27
// Report: .gstack/qa-reports/qa-report-localhost-3002-2026-09-27-t6.md
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { GradeSheet, GradeSheetClient } from "../../grades/monthly";
import { MonthlyGradeSheet } from "./MonthlyGradeSheet";
import { useGradeSaves } from "./useGradeSaves";

const NOT_GRADED = "Este nivel no lleva calificación numérica";

/** What the gateway really answers for Kínder: 200, grading off, a zero scale, read-only rows. */
const KINDER_SHEET: GradeSheet = {
  assignmentId: "a1",
  month: "2026-09",
  months: ["2026-08", "2026-09"],
  scale: { min: 0, max: 0, decimals: 0, passing: 0 },
  gradingEnabled: false,
  editable: false,
  readOnlyReason: NOT_GRADED,
  rows: [{ enrollmentId: "en1", studentId: "st1", name: "Valentina Castro", value: null, updatedAt: null, rowEditable: false, rowReadOnlyReason: NOT_GRADED }],
};

function Screen({ client }: { client: GradeSheetClient }) {
  const saves = useGradeSaves(client);
  return <MonthlyGradeSheet assignmentId="a1" client={client} saves={saves} />;
}

describe("MonthlyGradeSheet — a level that does not grade", () => {
  it("shows only why, without counters, scale or table", async () => {
    const client: GradeSheetClient = { load: vi.fn().mockResolvedValue(KINDER_SHEET), save: vi.fn(), remove: vi.fn() };
    render(<Screen client={client} />);

    expect(await screen.findByText(NOT_GRADED)).toBeInTheDocument();
    expect(screen.queryByRole("table")).toBeNull();
    expect(screen.queryByText("Capturadas")).toBeNull();
    expect(screen.queryByText(/0–0/)).toBeNull();
    expect(screen.queryByText("Todo guardado")).toBeNull();
  });
});
