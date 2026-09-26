import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { AttendanceSheet } from "../../attendance/sheet";
import { useAttendanceSheet, type LoadSheet, type SaveSheet } from "./useAttendanceSheet";

const target = { groupId: "g-1", date: "2026-09-21" };

const sheet: AttendanceSheet = {
  mode: "daily",
  date: "2026-09-21",
  editable: true,
  windowDays: 3,
  session: null,
  rows: [{ enrollmentId: "e-1", studentId: "s-1", name: "Ana", status: "present", note: "" }],
};

describe("useAttendanceSheet", () => {
  it("un guardado exitoso deja la lista limpia aunque la relectura falle", async () => {
    const load = vi.fn<LoadSheet>().mockResolvedValueOnce(sheet).mockRejectedValue(new Error("HTTP 503"));
    const save = vi.fn<SaveSheet>().mockResolvedValue(undefined);
    const { result } = renderHook(() => useAttendanceSheet(target, load, save));
    await waitFor(() => expect(result.current.sheet).toBeDefined());

    act(() => result.current.setStatus("e-1", "absent"));
    expect(result.current.dirty).toBe(true);

    await act(() => result.current.save());

    expect(save).toHaveBeenCalledTimes(1);
    expect(load).toHaveBeenCalledTimes(2);
    expect(result.current.dirty).toBe(false);
    expect(result.current.rows[0]?.status).toBe("absent");
    expect(result.current.saveError).toBeUndefined();
  });

  it("si la relectura funciona, la sesión y las filas vienen del servidor", async () => {
    const reread: AttendanceSheet = {
      ...sheet,
      session: { takenAt: "2026-09-21T14:00:00Z", takenByName: "Gabriela Rojas" },
      rows: [{ ...sheet.rows[0]!, status: "absent" }],
    };
    const load = vi.fn<LoadSheet>().mockResolvedValueOnce(sheet).mockResolvedValueOnce(reread);
    const save = vi.fn<SaveSheet>().mockResolvedValue(undefined);
    const { result } = renderHook(() => useAttendanceSheet(target, load, save));
    await waitFor(() => expect(result.current.sheet).toBeDefined());

    act(() => result.current.setStatus("e-1", "absent"));
    await act(() => result.current.save());

    expect(result.current.sheet?.session?.takenByName).toBe("Gabriela Rojas");
    expect(result.current.dirty).toBe(false);
  });
});
