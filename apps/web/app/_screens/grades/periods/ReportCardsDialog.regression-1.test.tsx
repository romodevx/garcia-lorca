import { readFileSync } from "node:fs";

import { render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../../config/api";
import type { GradingPeriod, PeriodsClient } from "../../../grades/periods";
import { ReportCardsDialog } from "./ReportCardsDialog";

// Regression: ISSUE-002 — at 360 px the boletas table scrolled sideways inside
// the dialog and "Descargar PDF" sat off-screen (house rule: no horizontal
// scroll inside a dialog). Below 560 px each row now splits: the pupil on top,
// version, average and the download below, each value keeping its label.
// Found by /qa on 2026-09-27
// Report: .gstack/qa-reports/qa-report-localhost-3002-2026-09-27-t7.md
vi.mock("../../../config/api", () => ({ listGradeLevels: vi.fn(), listLabelledGroups: vi.fn() }));
const config = vi.mocked(configApi);

const PERIOD: GradingPeriod = {
  termId: "t1", termName: "TRIMESTRE 1", ordinal: 1, levelId: "l1", levelName: "PRIMARIA",
  months: ["2026-09", "2026-10", "2026-11"], status: "closed", version: 2, progress: { captured: 8, expected: 36 },
};

afterEach(() => vi.clearAllMocks());

describe("ReportCardsDialog at phone width", () => {
  it("never scrolls sideways: the row splits and each value keeps its label", async () => {
    config.listGradeLevels.mockResolvedValue([{ id: "p1", levelId: "l1", name: "1° PRIMARIA", ordinal: 1 }]);
    config.listLabelledGroups.mockResolvedValue([{ id: "g1", cycleId: "c1", gradeLevelId: "p1", name: "B", capacity: 30, label: "1° PRIMARIA · B" }]);
    const client = {
      reportCards: vi.fn().mockResolvedValue([
        { id: "rc1", studentId: "s1", studentName: "DIEGO RAMÍREZ HERNÁNDEZ", groupLabel: "1° PRIMARIA · B", version: 2, issuedAt: "2026-09-27T22:29:03Z", generalAverage: 7.7 },
      ]),
    } as unknown as PeriodsClient;

    render(<ReportCardsDialog period={PERIOD} cycleId="c1" client={client} onClose={() => undefined} />);

    const table = await screen.findByRole("table", { name: "Boletas del grupo" });
    expect(table).toHaveClass("boletas-table");
    expect(table.parentElement).not.toHaveStyle({ overflowX: "auto" });
    expect(within(table).getByText("v2")).toHaveAttribute("data-label", "Versión");
    expect(within(table).getByText("7.7")).toHaveAttribute("data-label", "Promedio general");

    const css = readFileSync("app/globals.css", "utf8");
    expect(css).toMatch(/@media \(max-width: 560px\)\s*{[^@]*\.boletas-table tr\s*{[^}]*flex-wrap:\s*wrap/);
  });
});
