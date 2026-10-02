import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Cycle, Level } from "../../config/api";
import type { EnrollmentConcept } from "../../finance/api";
import * as financeApi from "../../finance/api";
import { TuitionPanel } from "./TuitionPanel";

/**
 * El panel ya no guarda a qué nivel se le están poniendo precios: eso vive en
 * `Catalog`, porque capturar precios OCUPA la pantalla y el catálogo entero deja
 * de dibujarse. Aquí se monta el anfitrión mínimo que hace ese trabajo.
 */
function Host(props: Partial<React.ComponentProps<typeof TuitionPanel>> = {}): React.ReactElement {
  const [pricingLevelId, setPricingLevelId] = useState<string | undefined>(undefined);
  return (
    <TuitionPanel
      cycle={cycle}
      levels={levels}
      pricingLevelId={pricingLevelId}
      onPrice={setPricingLevelId}
      {...props}
    />
  );
}
import { pesosToCents, centsToPesos, inscriptionTotals, ladderProblem } from "./tuition-math";

vi.mock("../../finance/api", () => ({
  listTuitionPlans: vi.fn(),
  upsertTuitionPlan: vi.fn(),
  runBilling: vi.fn(),
}));

const finance = vi.mocked(financeApi);

/**
 * Captura la inscripción como la pide el formulario desde que dejó de haber un
 * campo único: un concepto, y un monto con la fecha desde la que aplica.
 */
async function captureInscription(
  user: ReturnType<typeof userEvent.setup>,
  form: HTMLElement,
  amount = "4000",
): Promise<void> {
  await user.type(within(form).getByLabelText("Nuevo concepto"), "Inscripción");
  await user.click(within(form).getByRole("button", { name: /Agregar concepto/ }));
  await user.type(within(form).getByLabelText("Monto"), amount);
  await user.type(within(form).getByLabelText("Vigente desde"), "2026-08-01");
  await user.click(within(form).getByRole("button", { name: /Agregar monto/ }));
}

const cycle: Cycle = { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" };
const levels: Level[] = [
  { id: "lv1", name: "Kínder", sepCode: "K", enrollmentGranularity: "cycle" },
  { id: "lv2", name: "Primaria", sepCode: "P", enrollmentGranularity: "cycle" },
];

beforeEach(() => {
  finance.listTuitionPlans.mockResolvedValue([
    { id: "tp1", cycleId: "c1", levelId: "lv2", enrollmentCents: 500000, monthlyCents: 320000,
      months: 10, firstBilledMonth: 9, dueDay: 10, lateFeeCents: 15000, active: true,
      policy: { dailyMode: false, lateFeeDailyCents: 0, surchargeCountsAllDays: true, earlyPaymentCents: 0, earlyPaymentDays: 0 }, schedule: [], monthlyDiscountCents: 0, fees: [], enrollmentDueOn: "", enrollmentStartsOn: "", enrollmentConcepts: [] },
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

  it("la coma decimal NO se lee como separador de miles", () => {
    // Este lector borraba TODA coma, así que "1850,50" se volvía 185050 pesos:
    // un sobrecobro de cien veces, multiplicado además por la generación masiva
    // de cargos. Es el mismo defecto que _lib/money.ts documenta haber
    // corregido, que seguía vivo aquí en una cuarta copia del lector.
    expect(pesosToCents("1850,50")).toBeNull();
  });
});

describe("the tuition table", () => {
  it("shows a row per level, priced or not", async () => {
    render(<Host />);

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
      policy: { dailyMode: false, lateFeeDailyCents: 0, surchargeCountsAllDays: true, earlyPaymentCents: 0, earlyPaymentDays: 0 }, schedule: [], monthlyDiscountCents: 0, fees: [], enrollmentDueOn: "", enrollmentStartsOn: "", enrollmentConcepts: [],
    });
    render(<Host />);
    await screen.findByText("Primaria");

    await user.click(within(screen.getByRole("row", { name: /Kínder/ })).getByRole("button", { name: /Definir|Editar/ }));
    await screen.findByRole("heading", { name: /^Precios de / });
    await expandCollapsedSections(user);
    const form = document.body;
    await captureInscription(user, form);
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
      // La inscripción viaja en los conceptos; el monto único siempre va en cero.
      enrollmentCents: 0,
      enrollmentConcepts: [{
        name: "INSCRIPCIÓN",
        tiers: [{ effectiveFrom: "2026-08-01", effectiveTo: "", amountCents: 400000 }],
      }],
      monthlyCents: 250000,
      months: 11, dueDay: 5,
    });
  });

  it("will not send a due day the calendar cannot honour", async () => {
    const user = userEvent.setup();
    render(<Host />);
    await screen.findByText("Primaria");

    await user.click(within(screen.getByRole("row", { name: /Kínder/ })).getByRole("button", { name: /Definir|Editar/ }));
    await screen.findByRole("heading", { name: /^Precios de / });
    await expandCollapsedSections(user);
    const form = document.body;
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
    render(<Host />);
    await screen.findByText("Primaria");

    await user.click(screen.getByRole("button", { name: /Generar cargos/ }));

    // "Ya existían" is what makes a second press legible instead of frightening.
    expect(await screen.findByText(/30 colegiaturas/)).toBeInTheDocument();
    expect(screen.getByText(/6 ya existían/)).toBeInTheDocument();
  });

  // T17: the same run issues the activities' charges.
  it("says how many activity charges it issued", async () => {
    const user = userEvent.setup();
    finance.runBilling.mockResolvedValue({
      tuitionCreated: 0, enrollmentCreated: 0, lateFeesCreated: 0, feesCreated: 0, enrollmentMoved: 0, skipped: 0, levelsWithoutPlan: [],
      activitiesCreated: 5,
    });
    render(<Host />);
    await screen.findByText("Primaria");

    await user.click(screen.getByRole("button", { name: /Generar cargos/ }));

    expect(await screen.findByText(/5 cargos de actividades/)).toBeInTheDocument();
  });

  it("names the levels nobody priced, because a level billed at zero is silent", async () => {
    const user = userEvent.setup();
    finance.runBilling.mockResolvedValue({
      tuitionCreated: 0, enrollmentCreated: 0, lateFeesCreated: 0, feesCreated: 0, enrollmentMoved: 0, skipped: 0,
      levelsWithoutPlan: ["Kínder", "Preparatoria"],
    });
    render(<Host />);
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
    render(<Host />);

    await screen.findByRole("row", { name: /Kínder/ });
    await user.click(within(screen.getByRole("row", { name: /Kínder/ })).getByRole("button", { name: /Definir|Editar/ }));
    await expandCollapsedSections(user);
    await captureInscription(user, document.body);
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
    render(<Host />);

    await screen.findByRole("row", { name: /Kínder/ });
    await user.click(within(screen.getByRole("row", { name: /Kínder/ })).getByRole("button", { name: /Definir|Editar/ }));
    await expandCollapsedSections(user);
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

/**
 * Abre el tabulador de un nivel.
 *
 * Ya NO es un cajón: catorce campos en cinco secciones son una página, y el
 * encabezado de esa página es lo que se busca ahora. Las secciones plegables se
 * abren aquí mismo, porque cada prueba de abajo mira campos que en la pantalla
 * real están a un clic — no hay por qué repetir ese clic quince veces.
 */
async function openPrices(user: ReturnType<typeof userEvent.setup>, level: RegExp): Promise<HTMLElement> {
  render(<Host />);
  await screen.findByText("Primaria");
  await user.click(within(screen.getByRole("row", { name: level })).getByRole("button", { name: /Definir|Editar/ }));
  await screen.findByRole("heading", { name: /^Precios de / });
  await expandCollapsedSections(user);
  return document.body;
}

/** Cobranza y Cuotas nacen plegadas: divulgación progresiva, no un secreto. */
async function expandCollapsedSections(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  // El nombre accesible del encabezado lleva su número y su resumen:
  // "3 Cobranza recargo fijo". Se busca por el nombre de la sección, no anclado.
  for (const section of [/Cobranza/, /Cuotas/]) {
    const button = screen.queryByRole("button", { name: section });
    if (button !== null && button.getAttribute("aria-expanded") === "false") {
      await user.click(button);
    }
  }
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

    await captureInscription(user, drawer);
    await user.type(within(drawer).getByLabelText("Colegiatura mensual"), "2500");
    await user.type(within(drawer).getByLabelText("Fecha límite de inscripción"), "2026-06-30");
    await user.click(within(drawer).getByRole("button", { name: "Guardar tabulador" }));

    await waitFor(() => expect(finance.upsertTuitionPlan).toHaveBeenCalled());
    expect(finance.upsertTuitionPlan.mock.calls[0]?.[1]).toMatchObject({ enrollmentDueOn: "2026-06-30" });
  });

  it("una fecha ilegible ya no puede escribirse", async () => {
    // Esta prueba pedía el mensaje "se escribe aaaa-mm-dd" al teclear
    // "30/06/2026". Ese mensaje se borró y no se repuso: con un calendario, la
    // fecha sólo puede salir bien formada o vacía, así que el error dejó de
    // poder ocurrir. Lo que se defiende ahora es la garantía que lo sustituye.
    const user = userEvent.setup();
    const drawer = await openPrices(user, /Kínder/);

    const fecha = within(drawer).getByLabelText("Fecha límite de inscripción");
    expect(fecha).toHaveAttribute("type", "date");
  });
});

describe("cuotas — the extra a school collects on top of the tuition", () => {
  // The whole point. The director says WHAT is collected and WHEN, and the
  // system does the dividing. Nobody types 800 twice.
  it("takes a name, a total and the months, and sends the whole amount", async () => {
    const user = userEvent.setup();
    finance.upsertTuitionPlan.mockResolvedValue({} as never);
    const drawer = await openPrices(user, /Kínder/);

    await captureInscription(user, drawer);
    await user.type(within(drawer).getByLabelText("Colegiatura mensual"), "2500");
    await user.type(within(drawer).getByLabelText("Nombre de la cuota"), "Cuota de Julio");
    await user.type(within(drawer).getByLabelText("Monto total"), "1600");
    await user.click(within(drawer).getByRole("button", { name: "septiembre 2026" }));
    await user.click(within(drawer).getByRole("button", { name: "octubre 2026" }));
    await user.click(within(drawer).getByRole("button", { name: "Agregar cuota" }));
    await user.click(within(drawer).getByRole("button", { name: "Guardar tabulador" }));

    await waitFor(() => expect(finance.upsertTuitionPlan).toHaveBeenCalled());
    expect(finance.upsertTuitionPlan.mock.calls[0]?.[1]).toMatchObject({
      fees: [{ name: "CUOTA DE JULIO", amountCents: 160000, months: [9, 10] }],
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

    expect(await within(drawer).findByText("GRADUACIÓN")).toBeInTheDocument();
    await user.click(within(drawer).getByRole("button", { name: "Quitar GRADUACIÓN" }));
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
        schedule: [], monthlyDiscountCents: 0, enrollmentDueOn: "", enrollmentStartsOn: "", enrollmentConcepts: [],
        fees: [{ name: "Cuota de Julio", amountCents: 160000, months: [9, 10] }] },
    ]);
    const user = userEvent.setup();
    const drawer = await openPrices(user, /Primaria/);

    // Lo que ya está guardado se muestra TAL CUAL. La mayúscula es una regla de
    // captura, no de despliegue: re-escribir lo que el colegio ya tiene haría
    // que un dato pareciera haber cambiado sin que nadie lo tocara.
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
    render(<Host />);
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
  render(<Host />);
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
  render(<Host />);
  await screen.findByText("Primaria");

  await user.click(screen.getByRole("button", { name: /Generar cargos del mes/ }));

  expect(await screen.findByRole("status")).not.toHaveTextContent(/fecha límite/);
});

describe("la escalera de la inscripción", () => {
  const ladder: EnrollmentConcept[] = [
    {
      name: "INSCRIPCIÓN",
      tiers: [
        { effectiveFrom: "2027-01-01", effectiveTo: "", amountCents: 220000 },
        { effectiveFrom: "2027-03-01", effectiveTo: "", amountCents: 330000 },
        { effectiveFrom: "2027-04-01", effectiveTo: "", amountCents: 440000 },
      ],
    },
    { name: "Gastos adm y seguro", tiers: [{ effectiveFrom: "2027-01-01", effectiveTo: "", amountCents: 470000 }] },
    {
      name: "Cuota Asociación Padres de Familia",
      tiers: [{ effectiveFrom: "2027-01-01", effectiveTo: "", amountCents: 85000 }],
    },
  ];

  // The flyer's own totals. The director's only real question is "¿cuánto se le
  // cobra si se inscribe en marzo?", and the screen has to answer it before
  // anything is saved — otherwise the arithmetic is a promise, not a fact.
  it("suma lo que el folleto dice, por tramo", () => {
    expect(inscriptionTotals(ladder)).toEqual([
      { effectiveFrom: "2027-01-01", totalCents: 775000 },
      { effectiveFrom: "2027-03-01", totalCents: 885000 },
      { effectiveFrom: "2027-04-01", totalCents: 995000 },
    ]);
  });

  // A concept that does not move still counts in every tier. Reading only the
  // steps that were typed would price March at $3,300 instead of $8,850.
  it("los conceptos planos cuentan en todos los tramos", () => {
    const flat: EnrollmentConcept[] = [
      { name: "Cuota", tiers: [{ effectiveFrom: "2027-01-01", effectiveTo: "", amountCents: 85000 }] },
    ];
    expect(inscriptionTotals(flat)).toEqual([{ effectiveFrom: "2027-01-01", totalCents: 85000 }]);
  });

  it("sin conceptos no hay tramos que mostrar", () => {
    expect(inscriptionTotals([])).toEqual([]);
  });

  // The ladder is a one-way ratchet server-side (ADR 0011). The screen must say
  // so before the save fails, not after.
  it("avisa cuando un escalón cuesta menos que el anterior", () => {
    const down: EnrollmentConcept[] = [
      {
        name: "INSCRIPCIÓN",
        tiers: [
          { effectiveFrom: "2027-01-01", effectiveTo: "", amountCents: 330000 },
          { effectiveFrom: "2027-03-01", effectiveTo: "", amountCents: 33000 },
        ],
      },
    ];
    expect(ladderProblem(down)).toMatch(/menos/i);
    expect(ladderProblem(ladder)).toBe("");
  });

  it("avisa cuando dos conceptos se llaman igual", () => {
    const twins: EnrollmentConcept[] = [
      { name: "Cuota APF", tiers: [{ effectiveFrom: "2027-01-01", effectiveTo: "", amountCents: 85000 }] },
      { name: "Cuota A.P.F.", tiers: [{ effectiveFrom: "2027-01-01", effectiveTo: "", amountCents: 85000 }] },
    ];
    expect(ladderProblem(twins)).toMatch(/nombre/i);
    // T1: sin acento es el MISMO renglón para el cobro.
    const accents: EnrollmentConcept[] = [
      { name: "INSCRIPCIÓN", tiers: [{ effectiveFrom: "2027-01-01", effectiveTo: "", amountCents: 85000 }] },
      { name: "INSCRIPCION", tiers: [{ effectiveFrom: "2027-01-01", effectiveTo: "", amountCents: 85000 }] },
    ];
    expect(ladderProblem(accents)).toMatch(/nombre/i);
  });

  it("avisa cuando un escalón no tiene fecha o no tiene monto", () => {
    expect(
      ladderProblem([{ name: "INSCRIPCIÓN", tiers: [{ effectiveFrom: "", effectiveTo: "", amountCents: 220000 }] }]),
    ).toMatch(/fecha/i);
    expect(
      ladderProblem([{ name: "INSCRIPCIÓN", tiers: [{ effectiveFrom: "2027-01-01", effectiveTo: "", amountCents: 0 }] }]),
    ).toMatch(/monto/i);
    expect(ladderProblem([{ name: "", tiers: [] }])).toMatch(/nombre/i);
  });
});

describe("la inscripción con vigencias", () => {
  const withRanges: EnrollmentConcept[] = [
    {
      name: "INSCRIPCIÓN",
      tiers: [
        { effectiveFrom: "2027-01-01", effectiveTo: "2027-02-28", amountCents: 220000 },
        { effectiveFrom: "2027-03-01", effectiveTo: "2027-03-31", amountCents: 330000 },
        { effectiveFrom: "2027-04-01", effectiveTo: "2027-09-30", amountCents: 440000 },
      ],
    },
    {
      name: "Gastos adm y seguro",
      tiers: [{ effectiveFrom: "2027-01-01", effectiveTo: "", amountCents: 470000 }],
    },
  ];

  it("avisa cuando una vigencia termina antes de empezar", () => {
    expect(
      ladderProblem([
        {
          name: "INSCRIPCIÓN",
          tiers: [{ effectiveFrom: "2027-03-01", effectiveTo: "2027-01-01", amountCents: 220000 }],
        },
      ]),
    ).toMatch(/termina antes/i);
  });

  it("avisa cuando dos vigencias del mismo concepto se enciman", () => {
    expect(
      ladderProblem([
        {
          name: "INSCRIPCIÓN",
          tiers: [
            { effectiveFrom: "2027-01-01", effectiveTo: "2027-03-15", amountCents: 220000 },
            { effectiveFrom: "2027-03-01", effectiveTo: "2027-03-31", amountCents: 330000 },
          ],
        },
      ]),
    ).toMatch(/encim/i);
  });

  it("una vigencia sin fecha de fin es válida", () => {
    expect(ladderProblem(withRanges)).toBe("");
  });

  // La regla del dueño: fuera de todo rango se cobra el último monto vigente.
  it("los totales siguen la regla del último monto vigente", () => {
    expect(inscriptionTotals(withRanges)).toEqual([
      { effectiveFrom: "2027-01-01", totalCents: 690000 },
      { effectiveFrom: "2027-03-01", totalCents: 800000 },
      { effectiveFrom: "2027-04-01", totalCents: 910000 },
    ]);
  });
});

describe("la captura del tabulador", () => {
  it("la fecha límite de inscripción se elige en calendario", async () => {
    // Se tecleaba como texto con placeholder "2026-06-30", y había un mensaje
    // de error explicando el formato aaaa-mm-dd. Con calendario, ese error deja
    // de poder ocurrir.
    const user = userEvent.setup();
    render(<Host />);
    await screen.findByRole("row", { name: /Kínder/ });
    await user.click(within(screen.getByRole("row", { name: /Kínder/ })).getByRole("button", { name: /Definir|Editar/ }));

    expect(await screen.findByLabelText("Fecha límite de inscripción")).toHaveAttribute("type", "date");
  });

  it("los montos se capturan en pesos y los conteos no", async () => {
    const user = userEvent.setup();
    render(<Host />);
    await screen.findByRole("row", { name: /Kínder/ });
    await user.click(within(screen.getByRole("row", { name: /Kínder/ })).getByRole("button", { name: /Definir|Editar/ }));

    await screen.findByLabelText("Colegiatura mensual");
    // El dinero lleva su signo; las mensualidades y el día límite son conteos.
    expect(screen.getAllByText("$").length).toBeGreaterThan(0);
    expect(screen.getByLabelText("Mensualidades")).toHaveAttribute("type", "number");
    expect(screen.getByLabelText("Día límite")).toHaveAttribute("type", "number");
  });

  it("el nombre de un concepto se captura en mayúsculas", async () => {
    const user = userEvent.setup();
    render(<Host />);
    await screen.findByRole("row", { name: /Kínder/ });
    await user.click(within(screen.getByRole("row", { name: /Kínder/ })).getByRole("button", { name: /Definir|Editar/ }));

    const nombre = await screen.findByLabelText("Nuevo concepto");
    await user.type(nombre, "gastos adm");
    expect(nombre).toHaveValue("GASTOS ADM");
  });
});

/**
 * La columna de inscripción de la tabla.
 *
 * Desde que la inscripción se captura por conceptos (ADR 0011), `enrollmentCents`
 * viaja SIEMPRE en cero: el monto vive en los tramos de cada concepto. La tabla
 * seguía leyendo ese cero, así que un nivel con inscripción de $4,500 se anunciaba
 * como $0.00 — y este archivo advierte contra exactamente eso: "una escuela
 * leyendo $0.00 en una columna no distingue 'gratis' de 'nadie lo llenó', y la
 * diferencia es un año de colegiaturas sin cobrar".
 */
describe("lo que la tabla dice que cuesta la inscripción", () => {
  it("suma los conceptos en vez de leer el cero que ya no significa nada", async () => {
    finance.listTuitionPlans.mockResolvedValue([
      {
        id: "p1", cycleId: "c1", levelId: "lv1",
        // Cero a propósito: es lo que el servidor guarda desde ADR 0011.
        enrollmentCents: 0,
        monthlyCents: 280000, months: 10, firstBilledMonth: 9, dueDay: 10,
        lateFeeCents: 0, active: true, monthlyDiscountCents: 0, schedule: [], fees: [],
        enrollmentDueOn: "",
        enrollmentStartsOn: "",
        enrollmentConcepts: [
          { name: "INSCRIPCIÓN", tiers: [{ effectiveFrom: "2026-08-01", effectiveTo: "", amountCents: 450000 }] },
          { name: "SEGURO", tiers: [{ effectiveFrom: "2026-08-01", effectiveTo: "", amountCents: 50000 }] },
        ],
        policy: { dailyMode: false, lateFeeDailyCents: 0, surchargeCountsAllDays: true, earlyPaymentCents: 0, earlyPaymentDays: 0 },
      },
    ]);

    render(<Host />);

    const fila = await screen.findByRole("row", { name: /Kínder/ });
    expect(fila).toHaveTextContent("5000.00");
    expect(fila, "sigue anunciando la inscripción como gratis").not.toHaveTextContent("$0.00");
  });
});

/**
 * ISSUE-002 — la inscripción de un plan VIEJO se anunciaba como $0.00.
 *
 * Al arreglar que la columna leyera los conceptos (ADR 0011) se rompió el otro
 * lado: un plan escrito antes de los conceptos guarda UN `enrollmentCents` y la
 * lista vacía, así que la columna pasó a decir cero. Los cuatro tabuladores del
 * colegio sembrado son de ese tipo: la pantalla anunciaba las cuatro
 * inscripciones como gratis.
 */
describe("la inscripción de un plan sin conceptos", () => {
  it("cae al monto único en vez de decir cero", async () => {
    finance.listTuitionPlans.mockResolvedValue([
      {
        id: "p1", cycleId: "c1", levelId: "lv1",
        // Como lo guarda el seed y como lo guardaban todos los planes viejos.
        enrollmentCents: 450000,
        enrollmentConcepts: [],
        monthlyCents: 280000, months: 10, firstBilledMonth: 9, dueDay: 10,
        lateFeeCents: 0, active: true, monthlyDiscountCents: 0, schedule: [], fees: [],
        enrollmentDueOn: "",
        enrollmentStartsOn: "",
        policy: { dailyMode: false, lateFeeDailyCents: 0, surchargeCountsAllDays: true, earlyPaymentCents: 0, earlyPaymentDays: 0 },
      },
    ]);

    render(<Host />);

    const fila = await screen.findByRole("row", { name: /Kínder/ });
    expect(fila).toHaveTextContent("4500.00");
    expect(fila, "anuncia la inscripción como gratis").not.toHaveTextContent("$0.00");
  });
});

/**
 * ISSUE-001 — "Sin tabulador" dicho antes de saberlo.
 *
 * La tabla se dibuja de inmediato con la lista de planes vacía, así que durante
 * la carga TODOS los niveles anuncian "Sin tabulador · este nivel no se cobra".
 * Es la misma falla contra la que advierte el comentario de este archivo, en su
 * otra forma: un colegio que lee eso no distingue "no se cobra" de "todavía no
 * sabemos", y la frase está escrita como un hecho.
 */
describe("mientras los tabuladores cargan", () => {
  it("no afirma que el nivel no se cobra", async () => {
    let resolver: ((planes: never[]) => void) | undefined;
    finance.listTuitionPlans.mockImplementation(
      () => new Promise((resolve) => { resolver = resolve as (p: never[]) => void; }),
    );

    render(<Host />);

    const fila = await screen.findByRole("row", { name: /Kínder/ });
    expect(fila).not.toHaveTextContent("este nivel no se cobra");

    resolver?.([]);
    // Ya con la respuesta en mano, decirlo es correcto.
    await waitFor(() => expect(screen.getByRole("row", { name: /Kínder/ })).toHaveTextContent("Sin tabulador"));
  });
});
