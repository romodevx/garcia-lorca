import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

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

/** useUndoableRemoval's grace window, plus a beat. */
const UNDO_WINDOW_MS = 8_100;

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

  it("hands over the new count after a day is added", async () => {
    config.listSchoolEvents.mockResolvedValue([]);
    config.listNonWorkingPeriods.mockResolvedValueOnce([]).mockResolvedValue([day("a")]);
    config.createNonWorkingPeriod.mockResolvedValue(day("a"));
    const onLoaded = vi.fn();
    const user = userEvent.setup();
    render(<CalendarPanel cycle={cycle} onLoaded={onLoaded} />);
    await waitFor(() => expect(onLoaded).toHaveBeenCalledWith("c1", 0));

    await user.click(screen.getByRole("button", { name: "Nuevo día cerrado" }));
    await user.type(screen.getByLabelText("Nombre"), "Patria");
    await user.type(screen.getByLabelText("Del día"), "2026-09-16");
    await user.keyboard("{Enter}");

    await waitFor(() => expect(onLoaded).toHaveBeenLastCalledWith("c1", 1));
  });

  describe("removals", () => {
    beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }));
    afterEach(() => vi.useRealTimers());

    it("hands over the new count once a removal commits", async () => {
      config.listSchoolEvents.mockResolvedValue([]);
      config.listNonWorkingPeriods.mockResolvedValueOnce([day("a"), day("b")]).mockResolvedValue([day("b")]);
      config.deleteNonWorkingPeriod.mockResolvedValue(undefined);
      const onLoaded = vi.fn();
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      render(<CalendarPanel cycle={cycle} onLoaded={onLoaded} />);
      await waitFor(() => expect(onLoaded).toHaveBeenCalledWith("c1", 2));

      await user.click(screen.getByRole("button", { name: "Quitar a" }));
      await act(async () => {
        vi.advanceTimersByTime(UNDO_WINDOW_MS);
      });

      await waitFor(() => expect(onLoaded).toHaveBeenLastCalledWith("c1", 1));
    });

    it("hands over the restored count when a removal is undone, without reading again", async () => {
      config.listSchoolEvents.mockResolvedValue([]);
      config.listNonWorkingPeriods.mockResolvedValue([day("a"), day("b")]);
      const onLoaded = vi.fn();
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      render(<CalendarPanel cycle={cycle} onLoaded={onLoaded} />);
      await waitFor(() => expect(onLoaded).toHaveBeenCalledWith("c1", 2));
      onLoaded.mockClear();

      await user.click(screen.getByRole("button", { name: "Quitar a" }));
      await user.click(screen.getByRole("button", { name: "Deshacer" }));

      expect(onLoaded).toHaveBeenLastCalledWith("c1", 2);
      expect(config.listNonWorkingPeriods).toHaveBeenCalledTimes(1);
      expect(config.deleteNonWorkingPeriod).not.toHaveBeenCalled();
    });
  });
});
