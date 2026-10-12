import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { listCycles } from "../config/api";
import { listCharges } from "../finance/api";
import { listStudents } from "../students/api";
import { useHomeKpis, type KpiAccess } from "./useHomeKpis";

vi.mock("../students/api", () => ({ listStudents: vi.fn() }));
vi.mock("../config/api", () => ({ listCycles: vi.fn() }));
vi.mock("../finance/api", () => ({ listCharges: vi.fn() }));

const ALL: KpiAccess = { students: true, cycles: true, ledger: true };
const NONE: KpiAccess = { students: false, cycles: false, ledger: false };
const ACTIVE = { id: "c1", name: "2026-2027", status: "active" };
const charge = (paidCents: number, remainingCents: number) => ({ paidCents, remainingCents });

/** A promise the test settles by hand, to look at the state in between. */
function deferred<T>(): { promise: Promise<T>; resolve: (v: T) => void } {
  let resolve: (v: T) => void = () => {};
  const promise = new Promise<T>((r) => (resolve = r));
  return { promise, resolve };
}

beforeEach(() => {
  vi.mocked(listStudents).mockReset().mockResolvedValue([]);
  vi.mocked(listCycles).mockReset().mockResolvedValue([ACTIVE] as never);
  vi.mocked(listCharges).mockReset().mockResolvedValue([]);
});

describe("useHomeKpis", () => {
  it("is loading — never a number — until each read answers", async () => {
    const students = deferred<never[]>();
    vi.mocked(listStudents).mockReturnValue(students.promise);
    const { result } = renderHook(() => useHomeKpis(ALL));

    expect(result.current.students).toEqual({ status: "loading" });
    expect(result.current.ledger).toEqual({ status: "loading" });
    await act(async () => students.resolve([]));
    expect(result.current.students).toEqual({ status: "ready", value: 0 });
  });

  it("counts the admission pipeline without rejected or withdrawn students", async () => {
    vi.mocked(listStudents).mockResolvedValue([{ admissionStatus: "enrolled" }, { admissionStatus: "rejected" }, { admissionStatus: "withdrawn" }, { admissionStatus: "applicant" }] as never);
    const { result } = renderHook(() => useHomeKpis(ALL));
    await waitFor(() => expect(result.current.students).toEqual({ status: "ready", value: 2 }));
  });

  it("sums the active cycle's ledger", async () => {
    vi.mocked(listCharges).mockResolvedValue([charge(500, 1000), charge(300, 0), charge(0, -20)] as never);
    const { result } = renderHook(() => useHomeKpis(ALL));
    await waitFor(() =>
      expect(result.current.ledger).toEqual({ status: "ready", value: { pendingCents: 1000, collectedCents: 800, withBalance: 1, charges: 3 } }),
    );
    expect(listCharges).toHaveBeenCalledWith(expect.anything(), { cycleId: "c1" });
    expect(result.current.cycle).toEqual({ name: "2026-2027", active: true });
  });

  it("says a failed read failed, and a retry reads again", async () => {
    vi.mocked(listStudents).mockRejectedValueOnce(new Error("500"));
    vi.mocked(listCharges).mockRejectedValueOnce(new Error("500"));
    const { result } = renderHook(() => useHomeKpis(ALL));
    await waitFor(() => expect(result.current.students).toEqual({ status: "error" }));
    await waitFor(() => expect(result.current.ledger).toEqual({ status: "error" }));

    act(() => result.current.retryStudents());
    expect(result.current.students).toEqual({ status: "loading" });
    await waitFor(() => expect(result.current.students).toEqual({ status: "ready", value: 0 }));

    act(() => result.current.retryLedger());
    await waitFor(() => expect(result.current.ledger.status).toBe("ready"));
    expect(listCycles).toHaveBeenCalledTimes(2);
  });

  it("a failed cycles read fails the ledger too", async () => {
    vi.mocked(listCycles).mockRejectedValueOnce(new Error("500"));
    const { result } = renderHook(() => useHomeKpis(ALL));
    await waitFor(() => expect(result.current.ledger).toEqual({ status: "error" }));
    expect(result.current.cycle).toBeUndefined();
  });

  it("with no cycle at all there is no ledger to read", async () => {
    vi.mocked(listCycles).mockResolvedValue([]);
    const { result } = renderHook(() => useHomeKpis(ALL));
    await waitFor(() => expect(result.current.ledger).toEqual({ status: "noCycle" }));
    expect(listCharges).not.toHaveBeenCalled();
  });

  // Today's home falls back to the first cycle when none is active; the hook
  // keeps that (the classic view must not change) and says it is not active.
  it("falls back to the first cycle, marked as not active", async () => {
    vi.mocked(listCycles).mockResolvedValue([{ id: "c0", name: "2025-2026", status: "closed" }] as never);
    const { result } = renderHook(() => useHomeKpis(ALL));
    await waitFor(() => expect(result.current.cycle).toEqual({ name: "2025-2026", active: false }));
    expect(listCharges).toHaveBeenCalledWith(expect.anything(), { cycleId: "c0" });
  });

  it("reads nothing the caller may not read", async () => {
    const { result } = renderHook(() => useHomeKpis(NONE));
    expect(result.current.students).toEqual({ status: "idle" });
    expect(result.current.ledger).toEqual({ status: "idle" });
    expect(listStudents).not.toHaveBeenCalled();
    expect(listCycles).not.toHaveBeenCalled();
  });

  it("reads the cycle name without the ledger for a config-only caller", async () => {
    const { result } = renderHook(() => useHomeKpis({ students: false, cycles: true, ledger: false }));
    await waitFor(() => expect(result.current.cycle?.name).toBe("2026-2027"));
    expect(result.current.ledger).toEqual({ status: "idle" });
    expect(listCharges).not.toHaveBeenCalled();
  });
});
