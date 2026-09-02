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
      policy: { dailyMode: false, lateFeeDailyCents: 0, surchargeCountsAllDays: true, earlyPaymentCents: 0, earlyPaymentDays: 0 }, schedule: [], monthlyDiscountCents: 0, fees: [], enrollmentDueOn: "" },
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
      policy: { dailyMode: false, lateFeeDailyCents: 0, surchargeCountsAllDays: true, earlyPaymentCents: 0, earlyPaymentDays: 0 }, schedule: [], monthlyDiscountCents: 0, fees: [], enrollmentDueOn: "",
    });
    render(<TuitionPanel cycle={cycle} levels={levels} />);
    await screen.findByText("Primaria");

    await user.click(within(screen.getByRole("row", { name: /Kínder/ })).getByRole("button", { name: /Definir|Editar/ }));
    const form = await screen.findByRole("dialog", { name: /^Precios de / });
    await user.type(within(form).getByLabelText("Inscripción"), "4000");
    await user.type(within(form).getByLabelText("Colegiatura mensual"), "2500");
    await user.clear(within(form).getByLabelText("Mensualidades"));
    await user.type(within(form).getByLabelText("Mensualidades"), "11");
    await user.clear(within(form).getByLabelText("Día límite"));
    await user.type(within(form).getByLabelText("Día límite"), "5");
    await user.click(within(form).getByRole("button", { name: "Guardar tabulador" }));

    await waitFor(() => expect(finance.upsertTuitionPlan).toHaveBeenCalled());
    const sent = finance.upsertTuitionPlan.mock.calls[0]?.[1];
    expect(sent).toMatchObject({
      cycleId: "c1", levelId: "lv1",
      enrollmentCents: 400000, monthlyCents: 250000,
      months: 11, dueDay: 5,
    });
  });

  it("will not send a due day the calendar cannot honour", async () => {
    const user = userEvent.setup();
    render(<TuitionPanel cycle={cycle} levels={levels} />);
    await screen.findByText("Primaria");

    await user.click(within(screen.getByRole("row", { name: /Kínder/ })).getByRole("button", { name: /Definir|Editar/ }));
    const form = await screen.findByRole("dialog", { name: /^Precios de / });
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
      tuitionCreated: 30, enrollmentCreated: 10, lateFeesCreated: 4, feesCreated: 0, enrollmentMoved: 0, skipped: 6, levelsWithoutPlan: [],
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
      tuitionCreated: 0, enrollmentCreated: 0, lateFeesCreated: 0, feesCreated: 0, enrollmentMoved: 0, skipped: 0,
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

// The mes:monto schedule and the level discount were removed at the school's
// request; splitting a payment is what a cuota does now. Their tests went with
// them — what replaced them is pinned under "cuotas" below.

/** Opens the price drawer of a level. */
async function openPrices(user: ReturnType<typeof userEvent.setup>, level: RegExp): Promise<HTMLElement> {
  render(<TuitionPanel cycle={cycle} levels={levels} />);
  await screen.findByText("Primaria");
  await user.click(within(screen.getByRole("row", { name: level })).getByRole("button", { name: /Definir|Editar/ }));
  return screen.findByRole("dialog", { name: /^Precios de / });
}

describe("what the school asked to stop seeing", () => {
  // The school does not use the fixed monthly surcharge and asked for it to go.
  // Sending zero is what disables it downstream, so the field leaving the form
  // has to mean "zero", not "unchanged".
  it("no longer offers the fixed monthly surcharge", async () => {
    const user = userEvent.setup();
    const drawer = await openPrices(user, /Kínder/);

    expect(within(drawer).queryByLabelText("Recargo por mes vencido")).not.toBeInTheDocument();
  });

  it("no longer offers the level discount", async () => {
    const user = userEvent.setup();
    const drawer = await openPrices(user, /Kínder/);

    expect(within(drawer).queryByLabelText(/Descuento del nivel/)).not.toBeInTheDocument();
  });

  // And the mes:monto text box, which is the thing the school called confusing.
  it("no longer asks anybody to write mes:monto", async () => {
    const user = userEvent.setup();
    const drawer = await openPrices(user, /Kínder/);

    expect(within(drawer).queryByLabelText("Cuotas")).not.toBeInTheDocument();
    expect(within(drawer).queryByText(/mes:monto/)).not.toBeInTheDocument();
  });

  // Saving a level that used to carry them clears them, rather than leaving a
  // surcharge nobody can see and nobody can turn off.
  it("saves a zero surcharge and no level discount", async () => {
    const user = userEvent.setup();
    finance.upsertTuitionPlan.mockResolvedValue({} as never);
    const drawer = await openPrices(user, /Primaria/);

    await user.click(within(drawer).getByRole("button", { name: "Guardar tabulador" }));

    await waitFor(() => expect(finance.upsertTuitionPlan).toHaveBeenCalled());
    expect(finance.upsertTuitionPlan.mock.calls[0]?.[1]).toMatchObject({
      lateFeeCents: 0,
      monthlyDiscountCents: 0,
    });
  });
});

describe("the inscription's own deadline", () => {
  // The inscription is a separate charge and the school collects it on its own
  // date — in June, for a year that opens in August.
  it("is sent as the school typed it", async () => {
    const user = userEvent.setup();
    finance.upsertTuitionPlan.mockResolvedValue({} as never);
    const drawer = await openPrices(user, /Kínder/);

    await user.type(within(drawer).getByLabelText("Colegiatura mensual"), "2500");
    await user.type(within(drawer).getByLabelText("Fecha límite de inscripción"), "2026-06-30");
    await user.click(within(drawer).getByRole("button", { name: "Guardar tabulador" }));

    await waitFor(() => expect(finance.upsertTuitionPlan).toHaveBeenCalled());
    expect(finance.upsertTuitionPlan.mock.calls[0]?.[1]).toMatchObject({ enrollmentDueOn: "2026-06-30" });
  });

  it("says so rather than sending a date nobody can read", async () => {
    const user = userEvent.setup();
    const drawer = await openPrices(user, /Kínder/);

    await user.type(within(drawer).getByLabelText("Colegiatura mensual"), "2500");
    await user.type(within(drawer).getByLabelText("Fecha límite de inscripción"), "30/06/2026");
    await user.click(within(drawer).getByRole("button", { name: "Guardar tabulador" }));

    expect(await within(drawer).findByRole("alert")).toHaveTextContent(/aaaa-mm-dd/i);
    expect(finance.upsertTuitionPlan).not.toHaveBeenCalled();
  });
});

describe("cuotas — the extra a school collects on top of the tuition", () => {
  // The whole point. The director says WHAT is collected and WHEN, and the
  // system does the dividing. Nobody types 800 twice.
  it("takes a name, a total and the months, and sends the whole amount", async () => {
    const user = userEvent.setup();
    finance.upsertTuitionPlan.mockResolvedValue({} as never);
    const drawer = await openPrices(user, /Kínder/);

    await user.type(within(drawer).getByLabelText("Colegiatura mensual"), "2500");
    await user.type(within(drawer).getByLabelText("Nombre de la cuota"), "Cuota de Julio");
    await user.type(within(drawer).getByLabelText("Monto total"), "1600");
    await user.click(within(drawer).getByRole("button", { name: "septiembre 2026" }));
    await user.click(within(drawer).getByRole("button", { name: "octubre 2026" }));
    await user.click(within(drawer).getByRole("button", { name: "Agregar cuota" }));
    await user.click(within(drawer).getByRole("button", { name: "Guardar tabulador" }));

    await waitFor(() => expect(finance.upsertTuitionPlan).toHaveBeenCalled());
    expect(finance.upsertTuitionPlan.mock.calls[0]?.[1]).toMatchObject({
      fees: [{ name: "Cuota de Julio", amountCents: 160000, months: [9, 10] }],
    });
  });

  // The proof the school asked for, before anything is saved: what each month
  // ends up costing once the fee is added.
  it("shows what each month will cost with the fee added", async () => {
    const user = userEvent.setup();
    const drawer = await openPrices(user, /Kínder/);

    await user.type(within(drawer).getByLabelText("Colegiatura mensual"), "2500");
    await user.type(within(drawer).getByLabelText("Nombre de la cuota"), "Cuota de Julio");
    await user.type(within(drawer).getByLabelText("Monto total"), "1600");
    await user.click(within(drawer).getByRole("button", { name: "septiembre 2026" }));
    await user.click(within(drawer).getByRole("button", { name: "octubre 2026" }));
    await user.click(within(drawer).getByRole("button", { name: "Agregar cuota" }));

    // $2,500 of tuition plus $800 of the fee.
    const september = await within(drawer).findByRole("row", { name: /septiembre 2026/ });
    expect(september).toHaveTextContent("3,300.00");
    // And a month with no fee is just the tuition.
    expect(within(drawer).getByRole("row", { name: /noviembre 2026/ })).toHaveTextContent("2,500.00");
  });

  it("lists what has been added and lets it be removed", async () => {
    const user = userEvent.setup();
    const drawer = await openPrices(user, /Kínder/);

    await user.type(within(drawer).getByLabelText("Nombre de la cuota"), "Graduación");
    await user.type(within(drawer).getByLabelText("Monto total"), "900");
    await user.click(within(drawer).getByRole("button", { name: "septiembre 2026" }));
    await user.click(within(drawer).getByRole("button", { name: "Agregar cuota" }));

    expect(await within(drawer).findByText("Graduación")).toBeInTheDocument();
    await user.click(within(drawer).getByRole("button", { name: "Quitar Graduación" }));
    await waitFor(() => expect(within(drawer).queryByText("Graduación")).not.toBeInTheDocument());
  });

  it("refuses a cuota with no months rather than saving one nobody is charged", async () => {
    const user = userEvent.setup();
    const drawer = await openPrices(user, /Kínder/);

    await user.type(within(drawer).getByLabelText("Nombre de la cuota"), "Graduación");
    await user.type(within(drawer).getByLabelText("Monto total"), "900");
    await user.click(within(drawer).getByRole("button", { name: "Agregar cuota" }));

    expect(await within(drawer).findByRole("alert")).toHaveTextContent(/en qué meses/i);
  });

  // Reopening a priced level shows what it already collects, or the director
  // would add the July fee a second time every time they touch the price.
  it("shows the cuotas a level already has", async () => {
    finance.listTuitionPlans.mockResolvedValue([
      { id: "tp1", cycleId: "c1", levelId: "lv2", enrollmentCents: 500000, monthlyCents: 320000,
        months: 10, firstBilledMonth: 9, dueDay: 10, lateFeeCents: 0, active: true,
        policy: { dailyMode: false, lateFeeDailyCents: 0, surchargeCountsAllDays: true, earlyPaymentCents: 0, earlyPaymentDays: 0 },
        schedule: [], monthlyDiscountCents: 0, enrollmentDueOn: "",
        fees: [{ name: "Cuota de Julio", amountCents: 160000, months: [9, 10] }] },
    ]);
    const user = userEvent.setup();
    const drawer = await openPrices(user, /Primaria/);

    expect(within(drawer).getByText("Cuota de Julio")).toBeInTheDocument();
  });
});

describe("what the run reports", () => {
  it("counts the cuotas it issued", async () => {
    const user = userEvent.setup();
    finance.runBilling.mockResolvedValue({
      tuitionCreated: 10, enrollmentCreated: 1, lateFeesCreated: 0, feesCreated: 8,
      enrollmentMoved: 0, skipped: 2, levelsWithoutPlan: [],
    });
    render(<TuitionPanel cycle={cycle} levels={levels} />);
    await screen.findByText("Primaria");

    await user.click(screen.getByRole("button", { name: /Generar cargos del mes/ }));

    expect(await screen.findByRole("status")).toHaveTextContent(/8 cuotas/);
  });
});

// A run that moves four hundred due dates has to say so. The deadline reaching
// pupils already billed is the point of the setting, and a silent correction is
// one nobody can audit.
it("says when it corrected inscription deadlines already issued", async () => {
  const user = userEvent.setup();
  finance.runBilling.mockResolvedValue({
    tuitionCreated: 0, enrollmentCreated: 0, lateFeesCreated: 0, feesCreated: 0,
    enrollmentMoved: 400, skipped: 4010, levelsWithoutPlan: [],
  });
  render(<TuitionPanel cycle={cycle} levels={levels} />);
  await screen.findByText("Primaria");

  await user.click(screen.getByRole("button", { name: /Generar cargos del mes/ }));

  expect(await screen.findByRole("status")).toHaveTextContent(/400 inscripciones/);
});

// And stays quiet when it moved nothing, or the director stops reading it.
it("says nothing about deadlines when it moved none", async () => {
  const user = userEvent.setup();
  finance.runBilling.mockResolvedValue({
    tuitionCreated: 10, enrollmentCreated: 1, lateFeesCreated: 0, feesCreated: 2,
    enrollmentMoved: 0, skipped: 0, levelsWithoutPlan: [],
  });
  render(<TuitionPanel cycle={cycle} levels={levels} />);
  await screen.findByText("Primaria");

  await user.click(screen.getByRole("button", { name: /Generar cargos del mes/ }));

  expect(await screen.findByRole("status")).not.toHaveTextContent(/fecha límite/);
});
