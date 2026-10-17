import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Cycle, Level } from "../../config/api";
import * as financeApi from "../../finance/api";
import type { TuitionPlan } from "../../finance/api";
import { TuitionPage } from "./TuitionPage";

vi.mock("../../finance/api", () => ({ upsertTuitionPlan: vi.fn() }));
const finance = vi.mocked(financeApi);

const CYCLE: Cycle = { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-24", endsOn: "2027-07-09" };
const LEVEL: Level = { id: "l1", name: "MATERNAL", sepCode: "MAT", enrollmentGranularity: "cycle" };

/** The QA's table: $3,600 until 31 July, $4,000 from 1 August. */
const PLAN: TuitionPlan = {
  id: "p1",
  cycleId: "c1",
  levelId: "l1",
  enrollmentCents: 0,
  monthlyCents: 280000,
  months: 10,
  firstBilledMonth: 9,
  dueDay: 10,
  lateFeeCents: 0,
  active: true,
  policy: { dailyMode: false, lateFeeDailyCents: 0, surchargeCountsAllDays: true, earlyPaymentCents: 0, earlyPaymentDays: 0 },
  schedule: [],
  monthlyDiscountCents: 0,
  fees: [],
  enrollmentDueOn: "",
  enrollmentStartsOn: "",
  enrollmentConcepts: [{
    name: "INSCRIPCIÓN",
    tiers: [
      { effectiveFrom: "2026-06-01", effectiveTo: "2026-07-31", amountCents: 360000 },
      { effectiveFrom: "2026-08-01", effectiveTo: "", amountCents: 400000 },
    ],
  }],
};

function open(props: Partial<React.ComponentProps<typeof TuitionPage>> = {}): void {
  render(<TuitionPage cycle={CYCLE} level={LEVEL} plan={PLAN} onClose={vi.fn()} onSaved={vi.fn()} today="2026-10-17" {...props} />);
}

const summary = (): string => screen.getByTestId("create-page-aside").textContent ?? "";

beforeEach(() => {
  finance.upsertTuitionPlan.mockResolvedValue({} as TuitionPlan);
});
afterEach(() => vi.clearAllMocks());

describe("CC-110 · a monthly fee of $0", () => {
  it("is refused on the field itself, with the focus on it, and nothing is sent", async () => {
    const user = userEvent.setup();
    open();
    const monthly = screen.getByLabelText("Colegiatura mensual");
    await user.clear(monthly);
    await user.type(monthly, "0");

    await user.click(screen.getByRole("button", { name: "Guardar tabulador" }));

    expect(finance.upsertTuitionPlan).not.toHaveBeenCalled();
    expect(monthly).toHaveAttribute("aria-invalid", "true");
    expect(monthly).toHaveAccessibleDescription(expect.stringContaining("La colegiatura mensual debe ser mayor a $0"));
    expect(monthly).toHaveFocus();
  });

  it("the server's refusal lands on the same field", async () => {
    finance.upsertTuitionPlan.mockRejectedValue(new Error("La colegiatura mensual debe ser mayor a $0"));
    const user = userEvent.setup();
    open();

    await user.click(screen.getByRole("button", { name: "Guardar tabulador" }));

    const monthly = screen.getByLabelText("Colegiatura mensual");
    expect(await screen.findByText("La colegiatura mensual debe ser mayor a $0")).toBeInTheDocument();
    expect(monthly).toHaveAttribute("aria-invalid", "true");
  });

  it("the error clears as soon as the field is fixed", async () => {
    const user = userEvent.setup();
    open();
    const monthly = screen.getByLabelText("Colegiatura mensual");
    await user.clear(monthly);
    await user.type(monthly, "0");
    await user.click(screen.getByRole("button", { name: "Guardar tabulador" }));

    await user.clear(monthly);
    await user.type(monthly, "2800");

    expect(monthly).not.toHaveAttribute("aria-invalid", "true");
  });
});

describe("CC-108 · the inscription price in force today", () => {
  it("'Año completo' counts the tier in force on the school's today, not the expired first one", () => {
    open();
    expect(summary()).toContain("Inscripción$4,000.00");
  });

  it("before the ladder starts, the first tier is what applies", () => {
    open({ today: "2026-05-15" });
    expect(summary()).toContain("Inscripción$3,600.00");
  });

  it("'Total a pagar por inscribirse' says its dates like the rest of the app", () => {
    open();
    const table = screen.getByText("Total a pagar por inscribirse").parentElement as HTMLElement;
    expect(within(table).getByText("Desde el 1 jun 2026")).toBeInTheDocument();
    expect(within(table).getByText("Del 1 ago 2026 en adelante")).toBeInTheDocument();
    expect(table.textContent).not.toMatch(/\d{4}-\d{2}-\d{2}/);
  });
});
