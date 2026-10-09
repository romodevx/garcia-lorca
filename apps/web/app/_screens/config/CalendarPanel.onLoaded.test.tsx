import { render, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../config/api";
import { CalendarPanel } from "./CalendarPanel";

vi.mock("../../config/api", () => ({
  listNonWorkingPeriods: vi.fn(),
  createNonWorkingPeriod: vi.fn(),
  deleteNonWorkingPeriod: vi.fn(),
  listSchoolEvents: vi.fn(),
  createSchoolEvent: vi.fn(),
  deleteSchoolEvent: vi.fn(),
}));
const config = vi.mocked(configApi);

const cycle = { id: "c1", name: "2026-2027", status: "active" as const, startsOn: "2026-08-01", endsOn: "2027-06-30" };
const day = (id: string) => ({ id, cycleId: "c1", label: id, kind: "holiday" as const, startsOn: "2026-09-16", endsOn: "2026-09-16" });

afterEach(() => vi.clearAllMocks());

// Step 8's line ("3 días cerrados") comes from the panel's own read: no second GET.
describe("CalendarPanel onLoaded", () => {
  it("hands over how many closed days the cycle has", async () => {
    config.listSchoolEvents.mockResolvedValue([]);
    config.listNonWorkingPeriods.mockResolvedValue([day("a"), day("b"), day("c")]);
    const onLoaded = vi.fn();
    render(<CalendarPanel cycle={cycle} onLoaded={onLoaded} />);

    await waitFor(() => expect(onLoaded).toHaveBeenCalledWith("c1", 3));
    expect(config.listNonWorkingPeriods).toHaveBeenCalledTimes(1);
  });

  it("says the read failed, so the step is left unmarked instead of checking forever", async () => {
    config.listSchoolEvents.mockResolvedValue([]);
    config.listNonWorkingPeriods.mockRejectedValue(new Error("boom"));
    const onLoaded = vi.fn();
    render(<CalendarPanel cycle={cycle} onLoaded={onLoaded} />);

    await waitFor(() => expect(onLoaded).toHaveBeenCalledWith("c1", "failed"));
  });
});
