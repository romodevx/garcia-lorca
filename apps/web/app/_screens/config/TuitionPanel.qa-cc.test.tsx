import { render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Cycle, Level } from "../../config/api";
import * as financeApi from "../../finance/api";
import { TuitionPanel } from "./TuitionPanel";

vi.mock("../../finance/api", () => ({ listTuitionPlans: vi.fn(), upsertTuitionPlan: vi.fn(), runBilling: vi.fn() }));
const finance = vi.mocked(financeApi);

const cycle: Cycle = { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-24", endsOn: "2027-07-09" };
const levels: Level[] = [{ id: "lv1", name: "MATERNAL", sepCode: "M", enrollmentGranularity: "cycle" }];

afterEach(() => vi.clearAllMocks());

// QA CC-108: the table read $3,600 — the tier that ended on 31 July.
describe("TuitionPanel · the inscription column", () => {
  it("shows the price in force on the school's today", async () => {
    finance.listTuitionPlans.mockResolvedValue([{
      id: "tp1", cycleId: "c1", levelId: "lv1", enrollmentCents: 0, monthlyCents: 280000,
      months: 10, firstBilledMonth: 9, dueDay: 10, lateFeeCents: 0, active: true,
      policy: { dailyMode: false, lateFeeDailyCents: 0, surchargeCountsAllDays: true, earlyPaymentCents: 0, earlyPaymentDays: 0 },
      schedule: [], monthlyDiscountCents: 0, fees: [], enrollmentDueOn: "", enrollmentStartsOn: "",
      enrollmentConcepts: [{ name: "INSCRIPCIÓN", tiers: [
        { effectiveFrom: "2026-06-01", effectiveTo: "2026-07-31", amountCents: 360000 },
        { effectiveFrom: "2026-08-01", effectiveTo: "", amountCents: 400000 },
      ] }],
    }]);

    render(<TuitionPanel cycle={cycle} levels={levels} today="2026-10-17" />);

    const row = await screen.findByRole("row", { name: /MATERNAL/ });
    expect(within(row).getByText("$4,000.00")).toBeInTheDocument();
    expect(within(row).queryByText("$3,600.00")).toBeNull();
  });
});
