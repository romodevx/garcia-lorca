import { render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../../config/api";
import type { GradingPeriod, PeriodsClient } from "../../../grades/periods";
import { ReportCardsDialog } from "./ReportCardsDialog";

vi.mock("../../../config/api", () => ({ listGradeLevels: vi.fn(), listLabelledGroups: vi.fn() }));
const config = vi.mocked(configApi);

const PERIOD: GradingPeriod = {
  termId: "t1", termName: "1.er trimestre", ordinal: 1, levelId: "l1", levelName: "PREESCOLAR",
  startsOn: "2026-08-31", endsOn: "2026-11-30", status: "closed", version: 1, progress: { captured: 1, expected: 1 },
};

afterEach(() => vi.clearAllMocks());

// Preescolar's boletas carry letters: no average to show, and never the
// "Sin calificaciones" notice for a pupil who has letters.
describe("ReportCardsDialog — letters (preescolar)", () => {
  it("says the boleta has letters where a numeric one shows its average", async () => {
    config.listGradeLevels.mockResolvedValue([{ id: "p1", levelId: "l1", name: "2° PREESCOLAR", ordinal: 1 }]);
    config.listLabelledGroups.mockResolvedValue([{ id: "g1", cycleId: "c1", gradeLevelId: "p1", name: "A", capacity: 30, label: "2° PREESCOLAR · A" }]);
    const client = {
      reportCards: vi.fn().mockResolvedValue([
        { id: "rc1", studentId: "s1", studentName: "KIM ORTIZ", groupLabel: "2° PREESCOLAR · A", version: 1, issuedAt: "2026-11-30T22:00:00Z", generalAverage: null, hasGrades: true, gradingLetters: true },
      ]),
    } as unknown as PeriodsClient;

    render(<ReportCardsDialog period={PERIOD} cycleId="c1" client={client} onClose={() => undefined} />);

    const table = await screen.findByRole("table", { name: "Boletas del grupo" });
    expect(within(table).getByText("Con letras (A–D)")).toHaveAttribute("data-label", "Promedio general");
    expect(within(table).queryByText(/Sin calificaciones/)).toBeNull();
  });
});
