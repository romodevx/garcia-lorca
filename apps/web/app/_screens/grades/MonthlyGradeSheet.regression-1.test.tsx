// Regression: ISSUE-001 — an invalid value left in a saved cell still read "Guardado"
// Found by /qa on 2026-09-27
// Report: .gstack/qa-reports/qa-report-localhost-3002-2026-09-27-t6.md
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { GradeSheet, GradeSheetClient, SavedMonthlyGrade } from "../../grades/monthly";
import { MonthlyGradeSheet } from "./MonthlyGradeSheet";
import { useGradeSaves } from "./useGradeSaves";

const SHEET: GradeSheet = {
  assignmentId: "a1",
  month: "2026-09",
  months: ["2026-09"],
  scale: { min: 5, max: 10, decimals: 1, passing: 6 },
  editable: true,
  rows: [{ enrollmentId: "en1", studentId: "st1", name: "Ada Lovelace", value: null, updatedAt: null, rowEditable: true }],
};

function fakeClient(): GradeSheetClient {
  return {
    load: vi.fn().mockResolvedValue(SHEET),
    save: vi.fn().mockImplementation(
      async (i: { enrollmentId: string; month: string; value: number }): Promise<SavedMonthlyGrade> => ({ ...i, updatedAt: "2026-09-12T10:00:00Z" }),
    ),
    remove: vi.fn().mockResolvedValue(undefined),
  };
}

function Screen({ client }: { client: GradeSheetClient }) {
  const saves = useGradeSaves(client);
  return <MonthlyGradeSheet assignmentId="a1" client={client} saves={saves} />;
}

describe("MonthlyGradeSheet — an invalid value over a saved one", () => {
  it("does not say Guardado while the field holds a value that was never saved", async () => {
    const user = userEvent.setup();
    render(<Screen client={fakeClient()} />);
    const field = await screen.findByRole("textbox", { name: "Calificación de Ada Lovelace" });
    const cell = (): HTMLElement => field.closest("td") as HTMLElement;

    await user.type(field, "8{Enter}");
    expect(await within(cell()).findByText("Guardado")).toBeInTheDocument();

    await user.clear(field);
    await user.type(field, "4{Enter}");

    expect(within(cell()).getByText("La calificación debe estar entre 5 y 10")).toBeInTheDocument();
    expect(within(cell()).queryByText("Guardado")).toBeNull();
    expect(screen.getByText("1 sin guardar")).toBeInTheDocument();
  });

  it("says Guardado again once a valid value is saved", async () => {
    const user = userEvent.setup();
    render(<Screen client={fakeClient()} />);
    const field = await screen.findByRole("textbox", { name: "Calificación de Ada Lovelace" });
    const cell = (): HTMLElement => field.closest("td") as HTMLElement;

    await user.type(field, "8{Enter}");
    await within(cell()).findByText("Guardado");
    await user.clear(field);
    await user.type(field, "4{Enter}");
    await user.clear(field);
    await user.type(field, "9{Enter}");

    expect(await within(cell()).findByText("Guardado")).toBeInTheDocument();
    expect(within(cell()).queryByText("La calificación debe estar entre 5 y 10")).toBeNull();
  });
});
