import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Cycle, Level } from "../../config/api";
import * as financeApi from "../../finance/api";
import type { TuitionPlan } from "../../finance/api";
import { TuitionPanel } from "./TuitionPanel";

vi.mock("../../finance/api", () => ({
  listTuitionPlans: vi.fn(),
  upsertTuitionPlan: vi.fn(),
  runBilling: vi.fn(),
}));
const finance = vi.mocked(financeApi);

const c1: Cycle = { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" };
const c2: Cycle = { id: "c2", name: "2027-2028", status: "draft", startsOn: "2027-08-01", endsOn: "2028-06-30" };
const levels: Level[] = [
  { id: "lv1", name: "Kínder", sepCode: "K", enrollmentGranularity: "cycle" },
  { id: "lv2", name: "Primaria", sepCode: "P", enrollmentGranularity: "cycle" },
];

const plan = (cycleId: string, levelId: string, monthlyCents: number): TuitionPlan => ({
  id: `${cycleId}-${levelId}`, cycleId, levelId, enrollmentCents: 0, monthlyCents,
  months: 10, firstBilledMonth: 9, dueDay: 10, lateFeeCents: 0, active: true,
  policy: { dailyMode: false, lateFeeDailyCents: 0, surchargeCountsAllDays: true, earlyPaymentCents: 0, earlyPaymentDays: 0 },
  schedule: [], monthlyDiscountCents: 0, fees: [], enrollmentDueOn: "", enrollmentStartsOn: "", enrollmentConcepts: [],
});

function deferred<T>(): { promise: Promise<T>; resolve: (v: T) => void } {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((r) => (resolve = r));
  return { promise, resolve };
}

afterEach(() => vi.clearAllMocks());

// Step 7's status comes from the panel's own reload: no second GET.
describe("TuitionPanel onPlansLoaded", () => {
  it("hands over the cycle's plans", async () => {
    const rows = [plan("c1", "lv1", 250000)];
    finance.listTuitionPlans.mockResolvedValue(rows);
    const onPlansLoaded = vi.fn();
    render(<TuitionPanel cycle={c1} levels={levels} onPlansLoaded={onPlansLoaded} />);

    await waitFor(() => expect(onPlansLoaded).toHaveBeenCalledWith("c1", rows));
    expect(finance.listTuitionPlans).toHaveBeenCalledTimes(1);
  });

  it("says the read failed", async () => {
    finance.listTuitionPlans.mockRejectedValue(new Error("boom"));
    const onPlansLoaded = vi.fn();
    render(<TuitionPanel cycle={c1} levels={levels} onPlansLoaded={onPlansLoaded} />);

    await waitFor(() => expect(onPlansLoaded).toHaveBeenCalledWith("c1", "failed"));
  });

  it("hands over the plans again after Generar cargos re-reads them", async () => {
    finance.listTuitionPlans.mockResolvedValue([]);
    finance.runBilling.mockResolvedValue({
      tuitionCreated: 0, enrollmentCreated: 0, lateFeesCreated: 0, feesCreated: 0, enrollmentMoved: 0, skipped: 0, levelsWithoutPlan: [],
    });
    const onPlansLoaded = vi.fn();
    const user = userEvent.setup();
    render(<TuitionPanel cycle={c1} levels={levels} onPlansLoaded={onPlansLoaded} />);
    await waitFor(() => expect(onPlansLoaded).toHaveBeenCalledTimes(1));

    await user.click(screen.getByRole("button", { name: /Generar cargos/ }));

    await waitFor(() => expect(onPlansLoaded).toHaveBeenCalledTimes(2));
  });
});

// The reload used to keep whatever answered last: switching cycles while the
// first read was in flight could paint the old cycle's prices under the new one.
describe("TuitionPanel reload after a fast cycle switch", () => {
  it("drops the old cycle's late answer", async () => {
    const first = deferred<TuitionPlan[]>();
    finance.listTuitionPlans.mockImplementation((_url: string, cycleId: string) =>
      cycleId === "c1" ? first.promise : Promise.resolve([plan("c2", "lv1", 310000)]),
    );
    const onPlansLoaded = vi.fn();
    const { rerender } = render(<TuitionPanel cycle={c1} levels={levels} onPlansLoaded={onPlansLoaded} />);

    rerender(<TuitionPanel cycle={c2} levels={levels} onPlansLoaded={onPlansLoaded} />);
    expect(await screen.findByText(/\$3,100\.00/)).toBeInTheDocument();

    await act(async () => first.resolve([plan("c1", "lv1", 120000)]));

    expect(screen.getByText(/\$3,100\.00/)).toBeInTheDocument();
    expect(screen.queryByText(/\$1,200\.00/)).not.toBeInTheDocument();
    expect(onPlansLoaded).not.toHaveBeenCalledWith("c1", expect.anything());
  });
});
