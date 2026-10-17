import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type ComponentProps, type ReactElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Cycle, Level } from "../../config/api";
import * as financeApi from "../../finance/api";
import type { BillingRun, TuitionPlan } from "../../finance/api";
import { TuitionPanel } from "./TuitionPanel";

/*
 * Characterization tests (2026-10-09) for the "Colegiaturas" panel and its
 * pricing page, frozen before the Configuración académica redesign
 * (behavior.md §5/§5b: S5.1, S5.4, S5.6, "Generando…", column formats, A26,
 * TuitionPage save error / "Guardando…" / month validations).
 */

vi.mock("../../finance/api", () => ({
  listTuitionPlans: vi.fn(),
  upsertTuitionPlan: vi.fn(),
  runBilling: vi.fn(),
}));
const finance = vi.mocked(financeApi);

const cycle: Cycle = { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" };
const levels: Level[] = [
  { id: "lv1", name: "Kínder", sepCode: "K", enrollmentGranularity: "cycle" },
  { id: "lv2", name: "Primaria", sepCode: "P", enrollmentGranularity: "cycle" },
  { id: "lv3", name: "Secundaria", sepCode: "S", enrollmentGranularity: "cycle" },
];

const policy = { dailyMode: false, lateFeeDailyCents: 0, surchargeCountsAllDays: true, earlyPaymentCents: 0, earlyPaymentDays: 0 };
const plan = (levelId: string, extra: Partial<TuitionPlan> = {}): TuitionPlan => ({
  id: `tp-${levelId}`, cycleId: "c1", levelId, enrollmentCents: 0, monthlyCents: 320000,
  months: 10, firstBilledMonth: 9, dueDay: 10, lateFeeCents: 0, active: true, policy,
  schedule: [], monthlyDiscountCents: 0, fees: [], enrollmentDueOn: "", enrollmentStartsOn: "",
  enrollmentConcepts: [{ name: "INSCRIPCIÓN", tiers: [{ effectiveFrom: "2026-08-01", effectiveTo: "", amountCents: 450000 }] }],
  ...extra,
});

const RUN: BillingRun = {
  tuitionCreated: 0, enrollmentCreated: 0, feesCreated: 0, lateFeesCreated: 0, skipped: 0,
  enrollmentMoved: 0, levelsWithoutPlan: [], activitiesCreated: 0,
} as BillingRun;

function Host(props: Partial<ComponentProps<typeof TuitionPanel>> = {}): ReactElement {
  const [pricingLevelId, setPricingLevelId] = useState<string | undefined>(undefined);
  return <TuitionPanel cycle={cycle} levels={levels} pricingLevelId={pricingLevelId} onPrice={setPricingLevelId} {...props} />;
}

const row = (name: string): HTMLElement => screen.getByRole("row", { name: new RegExp(name) });

beforeEach(() => {
  finance.listTuitionPlans.mockResolvedValue([
    plan("lv1"),
    plan("lv2", { policy: { ...policy, dailyMode: true, lateFeeDailyCents: 2000 }, dueDay: 7, months: 11, firstBilledMonth: 8 }),
    plan("lv3", { lateFeeCents: 15000 }),
  ]);
});
afterEach(() => vi.clearAllMocks());

describe("TuitionPanel — characterization", () => {
  it("asks for levels first when the school has none", () => {
    render(<Host levels={[]} />);

    expect(screen.getByRole("heading", { name: "Colegiaturas" })).toBeInTheDocument();
    expect(screen.getByText("Registra los niveles del colegio para poder ponerles precio.")).toBeInTheDocument();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("formats each column: inscription, monthly, '{n} desde {mes}', 'día {n}', surcharge '—' | '$x' | '$x / día'", async () => {
    render(<Host />);

    await waitFor(() => expect(within(row("Kínder")).getByText("—")).toBeInTheDocument());
    const kinder = row("Kínder");
    expect(within(kinder).getByText("$4,500.00")).toBeInTheDocument();
    expect(within(kinder).getByText("$3,200.00")).toBeInTheDocument();
    expect(within(kinder).getByText("10 desde septiembre")).toBeInTheDocument();
    expect(within(kinder).getByText("día 10")).toBeInTheDocument();
    expect(within(kinder).getByRole("button", { name: "Editar" })).toBeInTheDocument();

    const primaria = row("Primaria");
    expect(within(primaria).getByText("11 desde agosto")).toBeInTheDocument();
    expect(within(primaria).getByText("día 7")).toBeInTheDocument();
    expect(within(primaria).getByText("$20.00 / día")).toBeInTheDocument();

    expect(within(row("Secundaria")).getByText("$150.00")).toBeInTheDocument();
  });

  it("says it could not load the plans", async () => {
    finance.listTuitionPlans.mockRejectedValue(new Error("down"));
    render(<Host />);

    expect(await screen.findByRole("alert")).toHaveTextContent("No pudimos cargar los tabuladores.");
  });

  it("'Generar cargos del mes' says 'Generando…' and is disabled while it runs, then reloads the plans", async () => {
    let finish: (run: BillingRun) => void = () => {};
    finance.runBilling.mockReturnValue(new Promise((resolve) => { finish = resolve; }));
    const user = userEvent.setup();
    render(<Host />);
    await waitFor(() => expect(finance.listTuitionPlans).toHaveBeenCalledTimes(1));

    await user.click(screen.getByRole("button", { name: "Generar cargos del mes" }));

    expect(screen.getByRole("button", { name: "Generando…" })).toBeDisabled();
    expect(finance.runBilling).toHaveBeenCalledWith(expect.any(String), "c1");

    finish(RUN);
    expect(await screen.findByRole("status")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Generar cargos del mes" })).toBeEnabled();
    await waitFor(() => expect(finance.listTuitionPlans).toHaveBeenCalledTimes(2));
    expect(screen.getByText("Se puede presionar las veces que haga falta: no cobra dos veces lo mismo.")).toBeInTheDocument();
  });

  it("says it could not generate the month's charges", async () => {
    finance.runBilling.mockRejectedValue(new Error("down"));
    const user = userEvent.setup();
    render(<Host />);

    await user.click(screen.getByRole("button", { name: "Generar cargos del mes" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("No se pudo generar los cargos del mes.");
  });
});

describe("TuitionPage (via TuitionPanel) — characterization", () => {
  async function openPrimaria(user: ReturnType<typeof userEvent.setup>): Promise<void> {
    render(<Host />);
    await waitFor(() => expect(within(row("Primaria")).getByRole("button", { name: "Editar" })).toBeInTheDocument());
    await user.click(within(row("Primaria")).getByRole("button", { name: "Editar" }));
    await screen.findByRole("heading", { name: "Precios de Primaria · 2026-2027" });
  }

  it("Cancelar returns to the table without saving", async () => {
    const user = userEvent.setup();
    await openPrimaria(user);

    await user.click(screen.getByRole("button", { name: "Cancelar" }));

    expect(await screen.findByRole("heading", { name: "Colegiaturas" })).toBeInTheDocument();
    expect(finance.upsertTuitionPlan).not.toHaveBeenCalled();
  });

  // The forms rule (T12): busy, never disabled, while saving.
  it("says 'Guardando…' and is busy, not disabled, while saving, then goes back to the table", async () => {
    let finish: () => void = () => {};
    finance.upsertTuitionPlan.mockReturnValue(new Promise((resolve) => { finish = () => resolve(plan("lv2")); }));
    const user = userEvent.setup();
    await openPrimaria(user);

    await user.click(screen.getByRole("button", { name: "Guardar tabulador" }));

    const busy = await screen.findByRole("button", { name: "Guardando…" });
    expect(busy).toBeEnabled();
    expect(busy).toHaveAttribute("aria-busy", "true");
    finish();
    expect(await screen.findByRole("heading", { name: "Colegiaturas" })).toBeInTheDocument();
  });

  it("a failed save says so and stays on the page", async () => {
    finance.upsertTuitionPlan.mockRejectedValue(new Error("down"));
    const user = userEvent.setup();
    await openPrimaria(user);

    await user.click(screen.getByRole("button", { name: "Guardar tabulador" }));

    expect(await screen.findByText("No se pudo guardar el tabulador. Revisa los datos.")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Precios de Primaria · 2026-2027" })).toBeInTheDocument();
  });

  it("refuses 13 instalments and a start month of 13, without saving", async () => {
    const user = userEvent.setup();
    await openPrimaria(user);

    await user.clear(screen.getByLabelText("Mensualidades"));
    await user.type(screen.getByLabelText("Mensualidades"), "13");
    await user.click(screen.getByRole("button", { name: "Guardar tabulador" }));
    expect(await screen.findByText("Las mensualidades van de 1 a 12.")).toBeInTheDocument();

    await user.clear(screen.getByLabelText("Mensualidades"));
    await user.type(screen.getByLabelText("Mensualidades"), "10");
    await user.clear(screen.getByLabelText("Mes de inicio"));
    await user.type(screen.getByLabelText("Mes de inicio"), "13");
    await user.click(screen.getByRole("button", { name: "Guardar tabulador" }));
    expect(await screen.findByText("El mes de inicio va de 1 a 12.")).toBeInTheDocument();

    expect(finance.upsertTuitionPlan).not.toHaveBeenCalled();
  });

  it("a new plan with only the monthly fee asks for the inscription", async () => {
    finance.listTuitionPlans.mockResolvedValue([]);
    const user = userEvent.setup();
    render(<Host />);
    await waitFor(() => expect(within(row("Kínder")).getByText("Sin tabulador · este nivel no se cobra")).toBeInTheDocument());
    await user.click(within(row("Kínder")).getByRole("button", { name: "Definir" }));
    await screen.findByRole("heading", { name: "Precios de Kínder · 2026-2027" });

    await user.type(screen.getByLabelText("Colegiatura mensual"), "2500");
    await user.click(screen.getByRole("button", { name: "Guardar tabulador" }));

    expect(await screen.findByText("Captura la inscripción: al menos un concepto con su monto y vigencia.")).toBeInTheDocument();
    expect(finance.upsertTuitionPlan).not.toHaveBeenCalled();
  });
});
