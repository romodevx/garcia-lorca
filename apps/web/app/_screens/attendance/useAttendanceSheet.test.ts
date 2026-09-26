import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { AttendanceSheet } from "../../attendance/sheet";
import { useAttendanceSheet, type LoadSheet, type SaveSheet, type SheetTarget } from "./useAttendanceSheet";

const target: SheetTarget = { groupId: "g-1", date: "2026-09-21" };

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

  it("save devuelve la lista que guardó, o undefined si falló", async () => {
    const load = vi.fn<LoadSheet>().mockResolvedValue(sheet);
    const save = vi.fn<SaveSheet>().mockResolvedValueOnce(undefined).mockRejectedValueOnce(new Error("boom"));
    const { result } = renderHook(() => useAttendanceSheet(target, load, save));
    await waitFor(() => expect(result.current.sheet).toBeDefined());

    let first: unknown;
    await act(async () => {
      first = await result.current.save();
    });
    let second: unknown = "unset";
    await act(async () => {
      second = await result.current.save();
    });

    expect(first).toEqual(target);
    expect(second).toBeUndefined();
  });

  it("si la fecha cambió durante el guardado, devuelve la lista que se guardó, no la nueva", async () => {
    let finishSave: () => void = () => undefined;
    const load = vi.fn<LoadSheet>().mockImplementation((q) => Promise.resolve({ ...sheet, date: q.date }));
    const save = vi.fn<SaveSheet>().mockImplementation(() => new Promise<void>((resolve) => { finishSave = resolve; }));
    const { result, rerender } = renderHook(({ t }) => useAttendanceSheet(t, load, save), { initialProps: { t: target } });
    await waitFor(() => expect(result.current.sheet).toBeDefined());

    let saved: Promise<unknown> = Promise.resolve();
    act(() => {
      saved = result.current.save();
    });
    rerender({ t: { ...target, date: "2026-09-20" } });
    await act(async () => {
      finishSave();
      await saved;
    });

    await expect(saved).resolves.toEqual(target);
  });

  it("mientras se guarda la lista anterior, la nueva sí acepta cambios", async () => {
    let finishSave: () => void = () => undefined;
    const load = vi.fn<LoadSheet>().mockImplementation((q) => Promise.resolve({ ...sheet, date: q.date }));
    const save = vi.fn<SaveSheet>().mockImplementation(() => new Promise<void>((resolve) => { finishSave = resolve; }));
    const other = { ...target, date: "2026-09-20" };
    const { result, rerender } = renderHook(({ t }) => useAttendanceSheet(t, load, save), { initialProps: { t: target } });
    await waitFor(() => expect(result.current.sheet).toBeDefined());

    let saved: Promise<unknown> = Promise.resolve();
    act(() => {
      saved = result.current.save();
    });
    rerender({ t: other });
    await waitFor(() => expect(result.current.loadedFor).toEqual(other));
    act(() => result.current.setStatus("e-1", "absent"));

    expect(result.current.rows[0]?.status).toBe("absent");
    await act(async () => {
      finishSave();
      await saved;
    });
  });

  it("dice para qué lista es la hoja cargada", async () => {
    const load = vi.fn<LoadSheet>().mockImplementation((q) => Promise.resolve({ ...sheet, date: q.date }));
    const save = vi.fn<SaveSheet>();
    const { result, rerender } = renderHook(({ t }) => useAttendanceSheet(t, load, save), { initialProps: { t: target } });
    await waitFor(() => expect(result.current.loadedFor).toEqual(target));

    rerender({ t: { ...target, subjectId: "sub-1" } });

    await waitFor(() => expect(result.current.loadedFor).toEqual({ ...target, subjectId: "sub-1" }));
  });
});
