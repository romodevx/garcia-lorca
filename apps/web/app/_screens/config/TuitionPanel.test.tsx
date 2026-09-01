import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Cycle, Level } from "../../config/api";
import * as financeApi from "../../finance/api";
import { TuitionPanel, pesosToCents, centsToPesos } from "./TuitionPanel";

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
];

beforeEach(() => {
  finance.listTuitionPlans.mockResolvedValue([
    { id: "tp1", cycleId: "c1", levelId: "lv2", enrollmentCents: 500000, monthlyCents: 320000,
      months: 10, firstBilledMonth: 9, dueDay: 10, lateFeeCents: 15000, active: true,
      policy: { dailyMode: false, lateFeeDailyCents: 0, surchargeCountsAllDays: true, earlyPaymentCents: 0, earlyPaymentDays: 0 }, schedule: [], monthlyDiscountCents: 0 },
  ]);
});
afterEach(() => vi.clearAllMocks());

describe("pesos and cents", () => {
  // Money is integer cents everywhere behind this screen, and the person typing
  // thinks in pesos. Getting this wrong by a factor of a hundred is the kind of
  // bug a school finds out about from a parent.
  it("round-trips without drifting", () => {
    expect(pesosToCents("3200")).toBe(320000);
    expect(pesosToCents("3,200.50")).toBe(320050);
    expect(pesosToCents("0.05")).toBe(5);
    expect(centsToPesos(320000)).toBe("3200.00");
    expect(centsToPesos(5)).toBe("0.05");
  });

  it("refuses what is not a number instead of silently reading zero", () => {
    expect(pesosToCents("")).toBeNull();
    expect(pesosToCents("abc")).toBeNull();
    expect(pesosToCents("-5")).toBeNull();
  });
});

describe("the tuition table", () => {
  it("shows a row per level, priced or not", async () => {
    render(<TuitionPanel cycle={cycle} levels={levels} />);

    // Primaria is priced; Kínder is not, and must say so rather than show zeros
    // that read like a free school.
    expect(await screen.findByText("Primaria")).toBeInTheDocument();
    const kinder = screen.getByRole("row", { name: /Kínder/ });
    expect(within(kinder).getByText(/Sin tabulador/)).toBeInTheDocument();
  });

  it("saves a level's prices in cents, from pesos typed by a person", async () => {
    const user = userEvent.setup();
    finance.upsertTuitionPlan.mockResolvedValue({
      id: "tp2", cycleId: "c1", levelId: "lv1", enrollmentCents: 400000, monthlyCents: 250000,
      months: 11, firstBilledMonth: 8, dueDay: 5, lateFeeCents: 10000, active: true,
      policy: { dailyMode: false, lateFeeDailyCents: 0, surchargeCountsAllDays: true, earlyPaymentCents: 0, earlyPaymentDays: 0 }, schedule: [], monthlyDiscountCents: 0,
    });
    render(<TuitionPanel cycle={cycle} levels={levels} />);
    await screen.findByText("Primaria");

    await user.click(within(screen.getByRole("row", { name: /Kínder/ })).getByRole("button", { name: /Definir|Editar/ }));
    const form = await screen.findByRole("form", { name: "Tabulador del nivel" });
    await user.type(within(form).getByLabelText("Inscripción"), "4000");
    await user.type(within(form).getByLabelText("Colegiatura mensual"), "2500");
    await user.clear(within(form).getByLabelText("Mensualidades"));
    await user.type(within(form).getByLabelText("Mensualidades"), "11");
    await user.clear(within(form).getByLabelText("Día límite"));
    await user.type(within(form).getByLabelText("Día límite"), "5");
    await user.type(within(form).getByLabelText("Recargo por mes vencido"), "100");
    await user.click(within(form).getByRole("button", { name: "Guardar tabulador" }));

    await waitFor(() => expect(finance.upsertTuitionPlan).toHaveBeenCalled());
    const sent = finance.upsertTuitionPlan.mock.calls[0]?.[1];
    expect(sent).toMatchObject({
      cycleId: "c1", levelId: "lv1",
      enrollmentCents: 400000, monthlyCents: 250000,
      months: 11, dueDay: 5, lateFeeCents: 10000,
    });
  });

  it("will not send a due day the calendar cannot honour", async () => {
    const user = userEvent.setup();
    render(<TuitionPanel cycle={cycle} levels={levels} />);
    await screen.findByText("Primaria");

    await user.click(within(screen.getByRole("row", { name: /Kínder/ })).getByRole("button", { name: /Definir|Editar/ }));
    const form = await screen.findByRole("form", { name: "Tabulador del nivel" });
    await user.type(within(form).getByLabelText("Colegiatura mensual"), "2500");
    await user.clear(within(form).getByLabelText("Día límite"));
    await user.type(within(form).getByLabelText("Día límite"), "31");
    await user.click(within(form).getByRole("button", { name: "Guardar tabulador" }));

    // The 31st does not exist in February. The server refuses it too, but a
    // person should not have to make a round trip to learn that.
    expect(await screen.findByText(/entre 1 y 28/)).toBeInTheDocument();
    expect(finance.upsertTuitionPlan).not.toHaveBeenCalled();
  });
});

describe("running the month", () => {
  it("reports what it did, skipped included", async () => {
    const user = userEvent.setup();
    finance.runBilling.mockResolvedValue({
      tuitionCreated: 30, enrollmentCreated: 10, lateFeesCreated: 4, skipped: 6, levelsWithoutPlan: [],
    });
    render(<TuitionPanel cycle={cycle} levels={levels} />);
    await screen.findByText("Primaria");

    await user.click(screen.getByRole("button", { name: /Generar cargos/ }));

    // "Ya existían" is what makes a second press legible instead of frightening.
    expect(await screen.findByText(/30 colegiaturas/)).toBeInTheDocument();
    expect(screen.getByText(/6 ya existían/)).toBeInTheDocument();
  });

  it("names the levels nobody priced, because a level billed at zero is silent", async () => {
    const user = userEvent.setup();
    finance.runBilling.mockResolvedValue({
      tuitionCreated: 0, enrollmentCreated: 0, lateFeesCreated: 0, skipped: 0,
      levelsWithoutPlan: ["Kínder", "Preparatoria"],
    });
    render(<TuitionPanel cycle={cycle} levels={levels} />);
    await screen.findByText("Primaria");

    await user.click(screen.getByRole("button", { name: /Generar cargos/ }));

    expect(await screen.findByText(/Kínder, Preparatoria/)).toBeInTheDocument();
  });
});

describe("TuitionPanel — cobranza", () => {
  // The $10 a day was configurable in the database and in the API and nowhere a
  // person could reach. This is that field.
  it("saves the daily surcharge and the early-payment reward", async () => {
    const user = userEvent.setup();
    render(<TuitionPanel cycle={cycle} levels={levels} />);

    await screen.findByRole("row", { name: /Kínder/ });
    await user.click(within(screen.getByRole("row", { name: /Kínder/ })).getByRole("button", { name: /Definir|Editar/ }));
    await user.type(screen.getByLabelText("Colegiatura mensual"), "3200");
    await user.click(screen.getByLabelText("Cobrar recargo por día"));
    await user.type(screen.getByLabelText("Recargo por día"), "10");
    await user.type(screen.getByLabelText("Pronto pago (monto)"), "100");
    await user.clear(screen.getByLabelText("Pronto pago (días hábiles)"));
    await user.type(screen.getByLabelText("Pronto pago (días hábiles)"), "5");
    await user.click(screen.getByRole("button", { name: /Guardar tabulador/ }));

    await waitFor(() => expect(finance.upsertTuitionPlan).toHaveBeenCalled());
    const sent = finance.upsertTuitionPlan.mock.calls[0]?.[1];
    expect(sent?.policy.dailyMode).toBe(true);
    expect(sent?.policy.lateFeeDailyCents).toBe(1000);
    expect(sent?.policy.earlyPaymentCents).toBe(10000);
    expect(sent?.policy.earlyPaymentDays).toBe(5);
  });

  // A daily rule with no amount charges nothing. The database refuses it; the
  // form says so before the round trip.
  it("refuses a daily rule with no amount", async () => {
    const user = userEvent.setup();
    render(<TuitionPanel cycle={cycle} levels={levels} />);

    await screen.findByRole("row", { name: /Kínder/ });
    await user.click(within(screen.getByRole("row", { name: /Kínder/ })).getByRole("button", { name: /Definir|Editar/ }));
    await user.type(screen.getByLabelText("Colegiatura mensual"), "3200");
    await user.click(screen.getByLabelText("Cobrar recargo por día"));
    await user.click(screen.getByRole("button", { name: /Guardar tabulador/ }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/recargo diario necesita un monto/);
    expect(finance.upsertTuitionPlan).not.toHaveBeenCalled();
  });
});

describe("TuitionPanel — cuotas por período", () => {
  // "Once meses, pero diciembre a la mitad y la otra mitad en marzo" is the
  // school's own example. One line, because they state it once a year and read
  // it back against a printed circular far more often than they edit it.
  it("saves a split year written as mes:monto", async () => {
    const user = userEvent.setup();
    render(<TuitionPanel cycle={cycle} levels={levels} />);

    await screen.findByRole("row", { name: /Kínder/ });
    await user.click(within(screen.getByRole("row", { name: /Kínder/ })).getByRole("button", { name: /Definir|Editar/ }));
    await user.type(screen.getByLabelText("Colegiatura mensual"), "3200");
    await user.type(screen.getByLabelText("Cuotas"), "9:3200, 12:1600, 3:4800");
    await user.click(screen.getByRole("button", { name: /Guardar tabulador/ }));

    await waitFor(() => expect(finance.upsertTuitionPlan).toHaveBeenCalled());
    expect(finance.upsertTuitionPlan.mock.calls[0]?.[1]?.schedule).toEqual([
      { month: 9, amountCents: 320000 },
      { month: 12, amountCents: 160000 },
      { month: 3, amountCents: 480000 },
    ]);
  });

  // A schedule nobody can parse would silently become "equal months", which is
  // the school's money quietly moving to different dates.
  it("refuses a schedule it cannot read", async () => {
    const user = userEvent.setup();
    render(<TuitionPanel cycle={cycle} levels={levels} />);

    await screen.findByRole("row", { name: /Kínder/ });
    await user.click(within(screen.getByRole("row", { name: /Kínder/ })).getByRole("button", { name: /Definir|Editar/ }));
    await user.type(screen.getByLabelText("Colegiatura mensual"), "3200");
    await user.type(screen.getByLabelText("Cuotas"), "diciembre la mitad");
    await user.click(screen.getByRole("button", { name: /Guardar tabulador/ }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/mes:monto/);
    expect(finance.upsertTuitionPlan).not.toHaveBeenCalled();
  });

  // The school's arrangement for maternal: one number on the tuition table
  // instead of a discount typed onto four hundred charges.
  it("saves a standing discount for the level", async () => {
    const user = userEvent.setup();
    render(<TuitionPanel cycle={cycle} levels={levels} />);

    await screen.findByRole("row", { name: /Kínder/ });
    await user.click(within(screen.getByRole("row", { name: /Kínder/ })).getByRole("button", { name: /Definir|Editar/ }));
    await user.type(screen.getByLabelText("Colegiatura mensual"), "3200");
    await user.type(screen.getByLabelText("Descuento del nivel (por mes)"), "500");
    await user.click(screen.getByRole("button", { name: /Guardar tabulador/ }));

    await waitFor(() => expect(finance.upsertTuitionPlan).toHaveBeenCalled());
    expect(finance.upsertTuitionPlan.mock.calls[0]?.[1]?.monthlyDiscountCents).toBe(50000);
  });

  // A discount bigger than the month would be a charge that owes negative money.
  it("refuses a discount bigger than the month", async () => {
    const user = userEvent.setup();
    render(<TuitionPanel cycle={cycle} levels={levels} />);

    await screen.findByRole("row", { name: /Kínder/ });
    await user.click(within(screen.getByRole("row", { name: /Kínder/ })).getByRole("button", { name: /Definir|Editar/ }));
    await user.type(screen.getByLabelText("Colegiatura mensual"), "3200");
    await user.type(screen.getByLabelText("Descuento del nivel (por mes)"), "4000");
    await user.click(screen.getByRole("button", { name: /Guardar tabulador/ }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/no puede ser mayor/);
    expect(finance.upsertTuitionPlan).not.toHaveBeenCalled();
  });
});
