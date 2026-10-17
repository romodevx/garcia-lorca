import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as financeApi from "../../finance/api";
import { useClosedCut } from "./useClosedCut";

vi.mock("../../finance/api", () => ({ getCashUp: vi.fn() }));
const finance = vi.mocked(financeApi);

const cut = (closed: boolean): financeApi.CashUp =>
  ({
    deposits: [], cancelled: [], superseded: [], payments: [],
    cashCents: 0, cardCents: 0, transferCents: 0, totalCents: 0, creditNoteCents: 0,
    ...(closed ? { closure: { id: "c", day: "2026-10-16" } } : {}),
  }) as financeApi.CashUp;

/** A cash-up answer the test releases when it chooses. */
function deferred(): { promise: Promise<financeApi.CashUp>; resolve: (c: financeApi.CashUp) => void } {
  let resolve!: (c: financeApi.CashUp) => void;
  const promise = new Promise<financeApi.CashUp>((r) => { resolve = r; });
  return { promise, resolve };
}

afterEach(() => vi.clearAllMocks());

describe("useClosedCut", () => {
  // Tech-lead: Friday (closed) asked first, today (open) second, Friday's
  // answer arriving LAST must not raise the warning for today.
  it("ignores a late answer for a day no longer asked about", async () => {
    const friday = deferred();
    const today = deferred();
    finance.getCashUp.mockImplementation((_base, day) => (day === "2026-10-16" ? friday.promise : today.promise));
    const { result, rerender } = renderHook(({ day }) => useClosedCut(day, true), { initialProps: { day: "2026-10-16" } });

    rerender({ day: "" });
    await act(async () => today.resolve(cut(false)));
    await act(async () => friday.resolve(cut(true)));

    expect(result.current).toBe(false);
  });

  it("warns for the day asked about once its cut reads closed", async () => {
    finance.getCashUp.mockResolvedValue(cut(true));
    const { result } = renderHook(() => useClosedCut("", true));

    await act(async () => {});
    expect(result.current).toBe(true);
  });

  it("unmounting with the request pending sets no state and warns about nothing", async () => {
    const pending = deferred();
    finance.getCashUp.mockReturnValue(pending.promise);
    const errors = vi.spyOn(console, "error").mockImplementation(() => {});
    const { unmount } = renderHook(() => useClosedCut("", true));

    unmount();
    await act(async () => pending.resolve(cut(true)));

    expect(errors).not.toHaveBeenCalled();
    errors.mockRestore();
  });
});
