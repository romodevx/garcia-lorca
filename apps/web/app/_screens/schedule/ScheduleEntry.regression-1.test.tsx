// Regression: ISSUE-003 — "Cubrir ahora" in Docentes landed on «Semana»: the
// catch-all page remounts AppShell on a new path, so its state was lost.
// Found by /qa on 2026-10-02
// Report: .gstack/qa-reports/qa-report-localhost-2026-10-02.md
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as api from "../../schedule/api";
import { requestCoverFocus } from "../../schedule/coverFocus";
import { sampleBoard } from "../../schedule/__fixtures__/board";
import { ScheduleEntry } from "./ScheduleEntry";

vi.mock("../../schedule/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../schedule/api")>()),
  getBoard: vi.fn(),
  listUncovered: vi.fn(),
  listAbsences: vi.fn(),
}));

afterEach(() => vi.clearAllMocks());

describe("ScheduleEntry — the cover request survives the remount", () => {
  it("opens «Por cubrir» once for a request made before it mounted, then «Semana»", async () => {
    vi.mocked(api.getBoard).mockResolvedValue(sampleBoard());
    vi.mocked(api.listUncovered).mockResolvedValue([]);
    vi.mocked(api.listAbsences).mockResolvedValue([]);
    requestCoverFocus({ startsOn: "2026-10-05", endsOn: "2026-10-06" });
    render(<ScheduleEntry enabled permissions={["schedule:read", "schedule:write"]} onBack={() => undefined} />);
    expect(await screen.findByRole("tab", { name: "Por cubrir" })).toHaveAttribute("aria-selected", "true");
    cleanup();
    render(<ScheduleEntry enabled permissions={["schedule:read", "schedule:write"]} onBack={() => undefined} />);
    expect(await screen.findByRole("tab", { name: "Semana" })).toHaveAttribute("aria-selected", "true");
  });
});
