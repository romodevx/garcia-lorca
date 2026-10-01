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
  enrollmentStartsOn: "",
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

/**
 * Dinero que se perdía sin decir nada, y un tope prestado de otro campo.
 */
describe("los montos de cobranza no se tragan en silencio", () => {
  async function abrirCobranza(user: ReturnType<typeof userEvent.setup>): Promise<void> {
    open();
    await user.click(screen.getByRole("button", { name: /Cobranza/ }));
  }

  /**
   * `pesosToCents` devuelve null para "100.oo", "$100", "1,00". El código hacía
   * `?? 0`: se guardaba cero sin una palabra, la pantalla volvía a mostrar el
   * campo vacío, y TODAS las familias perdían el pronto pago el ciclo entero.
   */
  it("reclama un pronto pago que no se puede leer, en vez de guardarlo en cero", async () => {
    const user = userEvent.setup();
    await abrirCobranza(user);
    await user.type(screen.getByLabelText("Colegiatura mensual"), "2800");
    await user.type(screen.getByLabelText("Nuevo concepto"), "INSCRIPCIÓN");
    await user.click(screen.getByRole("button", { name: /Agregar concepto/ }));
    await user.type(screen.getByLabelText("Monto"), "4500");
    await user.type(screen.getByLabelText("Vigente desde"), "2026-08-01");
    await user.click(screen.getByRole("button", { name: /Agregar monto/ }));

    await user.type(screen.getByLabelText("Pronto pago (monto)"), "100.oo");
    await user.click(screen.getByRole("button", { name: "Guardar tabulador" }));

    expect(screen.getByRole("alert")).toHaveTextContent(/pronto pago/i);
    expect(finance.upsertTuitionPlan).not.toHaveBeenCalled();
  });

  /**
   * El tope de 28 es "un día que también existe en febrero". Los días de pronto
   * pago son un CONTEO de días hábiles: no tienen por qué caber en un mes.
   * Prestarle el tope volvía inguardable cualquier plan con 30.
   */
  it("acepta una ventana de pronto pago mayor a 28 días", async () => {
    const user = userEvent.setup();
    await abrirCobranza(user);
    await user.type(screen.getByLabelText("Colegiatura mensual"), "2800");
    await user.type(screen.getByLabelText("Nuevo concepto"), "INSCRIPCIÓN");
    await user.click(screen.getByRole("button", { name: /Agregar concepto/ }));
    await user.type(screen.getByLabelText("Monto"), "4500");
    await user.type(screen.getByLabelText("Vigente desde"), "2026-08-01");
    await user.click(screen.getByRole("button", { name: /Agregar monto/ }));

    await user.clear(screen.getByLabelText("Pronto pago (días hábiles)"));
    await user.type(screen.getByLabelText("Pronto pago (días hábiles)"), "30");
    await user.click(screen.getByRole("button", { name: "Guardar tabulador" }));

    expect(finance.upsertTuitionPlan).toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({ policy: expect.objectContaining({ earlyPaymentDays: 30 }) }),
    );
  });
});

/**
 * `REGLAS-modales.md` §3: "Al enviar se dice qué falta y el foco va al primer
 * campo con problema." Con Cobranza y Cuotas plegadas, el mensaje señalaba un
 * campo que ni siquiera estaba en el DOM: quien captura apretaba Guardar, no veía
 * cambiar nada y concluía que el botón estaba roto.
 */
describe("un error dentro de una sección plegada", () => {
  it("abre la sección que lo contiene", async () => {
    const user = userEvent.setup();
    open();

    // Se enciende el recargo por día y se vuelve a plegar la sección.
    await user.click(screen.getByRole("button", { name: /Cobranza/ }));
    await user.click(screen.getByLabelText("Cobrar recargo por día"));
    await user.click(screen.getByRole("button", { name: /Cobranza/ }));
    expect(screen.getByRole("button", { name: /Cobranza/ })).toHaveAttribute("aria-expanded", "false");

    await user.type(screen.getByLabelText("Colegiatura mensual"), "2800");
    await user.click(screen.getByRole("button", { name: "Guardar tabulador" }));

    expect(screen.getByRole("alert")).toHaveTextContent(/recargo diario/i);
    expect(screen.getByRole("button", { name: /Cobranza/ })).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByLabelText("Recargo por día")).toBeInTheDocument();
  });
});

/**
 * Lo tecleado en un tramo o una cuota que nadie "agregó".
 *
 * El monto y la fecha viven en el estado local de la tarjeta hasta que se aprieta
 * "Agregar monto con su vigencia". Con el botón de guardar SIEMPRE a la vista en
 * la barra de abajo, escribir el precio y guardar directo es el gesto natural — y
 * el precio se iba a la basura sin una palabra, dejando un tabulador con un
 * escalón de menos que cobra mal todo el año.
 */
describe("un monto escrito y no agregado", () => {
  it("no se guarda el tabulador tirando el tramo a medio capturar", async () => {
    const user = userEvent.setup();
    open();
    await user.type(screen.getByLabelText("Colegiatura mensual"), "2800");
    await user.type(screen.getByLabelText("Nuevo concepto"), "INSCRIPCIÓN");
    await user.click(screen.getByRole("button", { name: /Agregar concepto/ }));
    await user.type(screen.getByLabelText("Monto"), "4500");
    await user.type(screen.getByLabelText("Vigente desde"), "2026-08-01");
    await user.click(screen.getByRole("button", { name: /Agregar monto/ }));

    // Un SEGUNDO escalón, tecleado y no agregado.
    await user.type(screen.getByLabelText("Monto"), "5200");
    await user.type(screen.getByLabelText("Vigente desde"), "2026-10-01");
    await user.click(screen.getByRole("button", { name: "Guardar tabulador" }));

    expect(screen.getByRole("alert")).toHaveTextContent(/INSCRIPCIÓN/);
    expect(finance.upsertTuitionPlan).not.toHaveBeenCalled();
  });

  it("tampoco tira una cuota a medio capturar", async () => {
    const user = userEvent.setup();
    open();
    await user.type(screen.getByLabelText("Colegiatura mensual"), "2800");
    await user.type(screen.getByLabelText("Nuevo concepto"), "INSCRIPCIÓN");
    await user.click(screen.getByRole("button", { name: /Agregar concepto/ }));
    await user.type(screen.getByLabelText("Monto"), "4500");
    await user.type(screen.getByLabelText("Vigente desde"), "2026-08-01");
    await user.click(screen.getByRole("button", { name: /Agregar monto/ }));

    await user.click(screen.getByRole("button", { name: /Cuotas/ }));
    await user.type(screen.getByLabelText("Nombre de la cuota"), "CUOTA DE JULIO");
    await user.type(screen.getByLabelText("Monto total"), "1600");
    await user.click(screen.getByRole("button", { name: "Guardar tabulador" }));

    expect(screen.getByRole("alert")).toHaveTextContent(/cuota/i);
    expect(finance.upsertTuitionPlan).not.toHaveBeenCalled();
  });
});

/**
 * Plegar "Cuotas" después de teclear en ella.
 *
 * `FormSection` sólo dibuja sus hijos mientras está abierta, así que plegarla
 * desmonta `FeesSection` y dejaba en el ref un cierre viejo sobre lo tecleado:
 * el siguiente "Guardar tabulador" se quejaba de una cuota que ya no existía, y
 * al reabrir la sección los campos estaban vacíos. El segundo clic sí guardaba,
 * así que se leía como un botón que a veces no hace nada.
 */
describe("plegar una sección después de escribir en ella", () => {
  it("no bloquea el guardado con una queja sobre algo que ya no está", async () => {
    const user = userEvent.setup();
    open();
    await user.type(screen.getByLabelText("Colegiatura mensual"), "2800");
    await user.type(screen.getByLabelText("Nuevo concepto"), "INSCRIPCIÓN");
    await user.click(screen.getByRole("button", { name: /Agregar concepto/ }));
    await user.type(screen.getByLabelText("Monto"), "4500");
    await user.type(screen.getByLabelText("Vigente desde"), "2026-08-01");
    await user.click(screen.getByRole("button", { name: /Agregar monto/ }));

    await user.click(screen.getByRole("button", { name: /Cuotas/ }));
    await user.type(screen.getByLabelText("Nombre de la cuota"), "CUOTA DE JULIO");
    await user.click(screen.getByRole("button", { name: /Cuotas/ }));

    await user.click(screen.getByRole("button", { name: "Guardar tabulador" }));

    expect(finance.upsertTuitionPlan).toHaveBeenCalled();
  });
});

/**
 * T1 (revisión 2026-09-30): el cobro reconoce un renglón por su nombre SIN
 * acentos ni mayúsculas, así que "INSCRIPCION" e "INSCRIPCIÓN" son el mismo
 * renglón. La forma lo tiene que decir antes del servidor, que lo rechaza.
 */
describe("dos renglones que sólo difieren en acentos", () => {
  it("no deja agregar un concepto que ya existe con otro acento", async () => {
    const user = userEvent.setup();
    open();
    await user.type(screen.getByLabelText("Nuevo concepto"), "INSCRIPCIÓN");
    await user.click(screen.getByRole("button", { name: /Agregar concepto/ }));
    await user.type(screen.getByLabelText("Nuevo concepto"), "INSCRIPCION");
    await user.click(screen.getByRole("button", { name: /Agregar concepto/ }));

    // The concept still has no price and raises its own alert; one of them must name it.
    expect(screen.getAllByRole("alert").map((a) => a.textContent).join(" ")).toMatch(/casi igual.*INSCRIPCION/);
    expect(screen.getAllByRole("button", { name: /^Quitar INSCRIPCI/ })).toHaveLength(1);
  });

  it("no deja agregar una cuota que ya existe con otro acento", async () => {
    const user = userEvent.setup();
    open();
    await user.click(screen.getByRole("button", { name: /Cuotas/ }));
    const addFee = async (name: string): Promise<void> => {
      await user.type(screen.getByLabelText("Nombre de la cuota"), name);
      await user.type(screen.getByLabelText("Monto total"), "1600");
      await user.click(screen.getAllByRole("button", { pressed: false })[0]!);
      await user.click(screen.getByRole("button", { name: /Agregar cuota/ }));
    };
    await addFee("GRADUACIÓN");
    await addFee("GRADUACION");

    expect(screen.getByRole("alert")).toHaveTextContent(/GRADUACION/);
    expect(screen.getAllByRole("button", { name: /^Quitar GRADUACI/ })).toHaveLength(1);
  });

  it("sí deja la misma cuota en otro mes", async () => {
    const user = userEvent.setup();
    open();
    await user.click(screen.getByRole("button", { name: /Cuotas/ }));
    const addFee = async (name: string, monthIndex: number): Promise<void> => {
      await user.type(screen.getByLabelText("Nombre de la cuota"), name);
      await user.type(screen.getByLabelText("Monto total"), "1600");
      await user.click(screen.getAllByRole("button", { pressed: false })[monthIndex]!);
      await user.click(screen.getByRole("button", { name: /Agregar cuota/ }));
    };
    await addFee("MATERIAL", 0);
    await addFee("MATERIAL", 0); // refused: same month
    await user.click(screen.getByRole("button", { name: /septiembre/ }));
    await user.click(screen.getByRole("button", { name: /diciembre/ }));
    await user.click(screen.getByRole("button", { name: /Agregar cuota/ }));

    expect(screen.getAllByRole("button", { name: /^Quitar MATERIAL/ })).toHaveLength(2);
    // Regression: ISSUE-001 — the refusal stayed on screen after the fee was accepted
    // Found by /qa on 2026-09-30
    // Report: .gstack/qa-reports/run-20260930T170040Z/qa-report-localhost-2026-09-30.md
    expect(screen.queryByRole("alert")).toBeNull();
  });
});

/**
 * T5: la inscripción tiene un rango — desde cuándo se le muestra y se le pide a
 * la familia, y hasta cuándo se paga sin recargo.
 */
describe("el rango de fechas de la inscripción", () => {
  async function capturaMinima(user: ReturnType<typeof userEvent.setup>): Promise<void> {
    await user.type(screen.getByLabelText("Colegiatura mensual"), "2800");
    await user.type(screen.getByLabelText("Nuevo concepto"), "INSCRIPCIÓN");
    await user.click(screen.getByRole("button", { name: /Agregar concepto/ }));
    await user.type(screen.getByLabelText("Monto"), "4500");
    await user.type(screen.getByLabelText("Vigente desde"), "2026-08-01");
    await user.click(screen.getByRole("button", { name: /Agregar monto/ }));
  }

  it("guarda la fecha de inicio junto con la fecha límite", async () => {
    const user = userEvent.setup();
    open();
    await capturaMinima(user);

    await user.type(screen.getByLabelText("Fecha de inicio de inscripción"), "2026-06-01");
    await user.type(screen.getByLabelText("Fecha límite de inscripción"), "2026-06-30");
    await user.click(screen.getByRole("button", { name: "Guardar tabulador" }));

    expect(finance.upsertTuitionPlan).toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({ enrollmentStartsOn: "2026-06-01", enrollmentDueOn: "2026-06-30" }),
    );
  });

  it("abre con las fechas que ya tenía el tabulador", () => {
    open({ plan: { ...PREVIOUS, enrollmentStartsOn: "2025-06-01" } });

    expect(screen.getByLabelText("Fecha de inicio de inscripción")).toHaveValue("2025-06-01");
    expect(screen.getByLabelText("Fecha límite de inscripción")).toHaveValue("2025-06-30");
  });

  it("al salir del campo avisa que el inicio no puede ser posterior al límite", async () => {
    const user = userEvent.setup();
    open();

    await user.type(screen.getByLabelText("Fecha límite de inscripción"), "2026-06-30");
    await user.type(screen.getByLabelText("Fecha de inicio de inscripción"), "2026-07-15");
    // Mientras se escribe no se regaña: el aviso llega al salir del campo.
    expect(screen.queryByText(/no puede ser posterior/i)).toBeNull();
    await user.tab();

    expect(screen.getByText(/no puede ser posterior a la fecha límite/i)).toBeInTheDocument();
  });

  it("al guardar no envía un rango al revés y lleva el foco al inicio", async () => {
    const user = userEvent.setup();
    open();
    await capturaMinima(user);

    await user.type(screen.getByLabelText("Fecha de inicio de inscripción"), "2026-07-15");
    await user.type(screen.getByLabelText("Fecha límite de inscripción"), "2026-06-30");
    await user.click(screen.getByRole("button", { name: "Guardar tabulador" }));

    expect(finance.upsertTuitionPlan).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Fecha de inicio de inscripción")).toHaveFocus();
    expect(screen.getByText(/no puede ser posterior a la fecha límite/i)).toBeInTheDocument();
  });

  it("pide la fecha límite cuando sólo hay inicio", async () => {
    const user = userEvent.setup();
    open();
    await capturaMinima(user);

    await user.type(screen.getByLabelText("Fecha de inicio de inscripción"), "2026-06-01");
    await user.click(screen.getByRole("button", { name: "Guardar tabulador" }));

    expect(finance.upsertTuitionPlan).not.toHaveBeenCalled();
    expect(screen.getByText(/necesita su fecha límite/i)).toBeInTheDocument();
  });

  it("el aviso se quita en cuanto el rango queda bien", async () => {
    const user = userEvent.setup();
    open();
    await user.type(screen.getByLabelText("Fecha límite de inscripción"), "2026-06-30");
    await user.type(screen.getByLabelText("Fecha de inicio de inscripción"), "2026-07-15");
    await user.tab();
    expect(screen.getByText(/no puede ser posterior/i)).toBeInTheDocument();

    await user.clear(screen.getByLabelText("Fecha de inicio de inscripción"));
    await user.type(screen.getByLabelText("Fecha de inicio de inscripción"), "2026-06-01");

    expect(screen.queryByText(/no puede ser posterior/i)).toBeNull();
  });
});
