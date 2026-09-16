import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Cycle, Level } from "../../config/api";
import * as financeApi from "../../finance/api";
import type { TuitionPlan } from "../../finance/api";
import { TuitionPage } from "./TuitionPage";

vi.mock("../../finance/api", () => ({ upsertTuitionPlan: vi.fn() }));
const finance = vi.mocked(financeApi);

const CYCLE: Cycle = {
  id: "c2",
  name: "2026-2027",
  status: "draft",
  startsOn: "2026-08-01",
  endsOn: "2027-07-31",
};
const LEVEL: Level = { id: "l1", name: "KÍNDER", sepCode: "KIN", enrollmentGranularity: "cycle" };

const PREVIOUS: TuitionPlan = {
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
  policy: {
    dailyMode: false,
    lateFeeDailyCents: 0,
    surchargeCountsAllDays: true,
    earlyPaymentCents: 0,
    earlyPaymentDays: 0,
  },
  schedule: [],
  monthlyDiscountCents: 0,
  fees: [],
  enrollmentDueOn: "2025-06-30",
  enrollmentConcepts: [
    {
      name: "INSCRIPCIÓN",
      tiers: [{ effectiveFrom: "2025-08-31", effectiveTo: "", amountCents: 450000 }],
    },
  ],
};

function open(props: Partial<React.ComponentProps<typeof TuitionPage>> = {}): void {
  render(
    <TuitionPage
      cycle={CYCLE}
      level={LEVEL}
      plan={undefined}
      onClose={vi.fn()}
      onSaved={vi.fn()}
      {...props}
    />,
  );
}

/*
 * Cuerpo con llaves, no una flecha de expresión.
 *
 * `mockResolvedValue` DEVUELVE el mock, y lo que un `beforeEach` devuelve Vitest
 * lo toma por su rutina de limpieza y lo EJECUTA. Con la flecha corta, cada
 * prueba dejaba una llamada fantasma —sin argumentos— registrada en el mock, y
 * la siguiente creía que alguien había guardado el tabulador.
 */
beforeEach(() => {
  finance.upsertTuitionPlan.mockResolvedValue({} as TuitionPlan);
});
afterEach(() => vi.clearAllMocks());

describe("el tabulador es una página", () => {
  /**
   * Que no sea un diálogo no es cosmético: es lo que permite que catorce campos
   * quepan sin el flujo en dos columnas que reacomodaba la forma mientras
   * alguien escribía.
   */
  it("no es un cajón", () => {
    open();

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByRole("heading", { name: "Precios de KÍNDER · 2026-2027" })).toBeInTheDocument();
  });

  // Divulgación progresiva: lo que casi ningún nivel cambia no ocupa espacio de
  // entrada, pero dice en una línea cómo está.
  it("Cobranza y Cuotas nacen plegadas, con su estado a la vista", () => {
    open();

    expect(screen.getByRole("button", { name: /Cobranza/ })).toHaveAttribute("aria-expanded", "false");
    expect(screen.getByRole("button", { name: /Cuotas/ })).toHaveAttribute("aria-expanded", "false");
    expect(screen.getByRole("button", { name: /Cuotas/ })).toHaveTextContent("ninguna");
  });

  it("Colegiatura e Inscripción están abiertas de entrada", () => {
    open();

    expect(screen.getByLabelText("Colegiatura mensual")).toBeInTheDocument();
    expect(screen.getByLabelText("Nuevo concepto")).toBeInTheDocument();
  });

  // El botón primario NUNCA se apaga por una forma incompleta: al enviar dice
  // qué falta. Un control muerto que no explica nada se lee como una app rota.
  it("el botón de guardar no se apaga, explica", async () => {
    const user = userEvent.setup();
    open();

    const save = screen.getByRole("button", { name: "Guardar tabulador" });
    expect(save).not.toBeDisabled();

    await user.click(save);

    expect(screen.getByRole("alert")).toHaveTextContent(/colegiatura mensual/i);
    expect(finance.upsertTuitionPlan).not.toHaveBeenCalled();
  });
});

describe("el año completo, mientras se captura", () => {
  it("se recalcula al teclear la colegiatura", async () => {
    const user = userEvent.setup();
    open();

    await user.type(screen.getByLabelText("Colegiatura mensual"), "2800");

    // 2800 × 10 mensualidades, con la inscripción todavía en cero.
    expect(screen.getByTestId("create-page-aside")).toHaveTextContent("$28,000.00");
  });

  it("no compara contra nada cuando no hay ciclo anterior", () => {
    open();

    expect(screen.getByTestId("create-page-aside")).not.toHaveTextContent(/vs\./);
  });
});

describe("copiar del ciclo anterior", () => {
  it("no ofrece el atajo cuando no hay de dónde copiar", () => {
    open();

    expect(screen.queryByRole("button", { name: "Copiar" })).toBeNull();
  });

  /**
   * El caso real del colegio: los precios del año pasado, con el monto más alto.
   * Tres clics en vez de catorce campos en blanco.
   */
  it("trae los montos tal cual y las fechas recorridas un año", async () => {
    const user = userEvent.setup();
    open({ previousPlan: PREVIOUS, previousCycleName: "2025-2026" });

    await user.click(screen.getByRole("button", { name: "Copiar" }));

    expect(screen.getByLabelText("Colegiatura mensual")).toHaveValue("2800.00");
    expect(screen.getByLabelText("Mensualidades")).toHaveValue(10);
    // La fecha límite de inscripción cae FUERA del ciclo por diseño — se cobra
    // en junio para un año que abre en agosto — y aun así se conserva.
    expect(screen.getByLabelText("Fecha límite de inscripción")).toHaveValue("2026-06-30");
  });

  it("dice contra qué ciclo se compara el año", async () => {
    const user = userEvent.setup();
    open({ previousPlan: PREVIOUS, previousCycleName: "2025-2026" });

    await user.click(screen.getByRole("button", { name: "Copiar" }));

    expect(screen.getByTestId("create-page-aside")).toHaveTextContent("Igual que 2025-2026");
  });
});
