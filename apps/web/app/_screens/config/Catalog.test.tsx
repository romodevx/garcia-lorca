import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as api from "../../config/api";
import * as financeApi from "../../finance/api";
import { Catalog } from "./Catalog";

// The screen talks to the gateway exclusively through the config api client, so
// we mock it wholesale and assert the calls (the fetch itself is covered by
// app/config/api.test.ts). Neutral literals only.
vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  createCycle: vi.fn(),
  listLevels: vi.fn(),
  createLevel: vi.fn(),
  listGradeLevels: vi.fn(),
  createGradeLevel: vi.fn(),
  listSubjects: vi.fn(),
  createSubject: vi.fn(),
  listGroups: vi.fn(),
  createGroup: vi.fn(),
  listTerms: vi.fn(),
  createTerm: vi.fn(),
  listNonWorkingPeriods: vi.fn(),
  createNonWorkingPeriod: vi.fn(),
  deleteNonWorkingPeriod: vi.fn(),
  listSchoolEvents: vi.fn(),
  createSchoolEvent: vi.fn(),
  deleteSchoolEvent: vi.fn(),
}));

/*
 * El tabulador vive en finance, no en config. Sin este mock, `listTuitionPlans`
 * sale a la red de verdad, falla, y el panel se queda sin planes — que es
 * exactamente el estado en el que "Copiar de…" no debe aparecer, así que la
 * prueba pasaría por la razón equivocada.
 */
vi.mock("../../finance/api", () => ({
  listTuitionPlans: vi.fn(),
  upsertTuitionPlan: vi.fn(),
  runBilling: vi.fn(),
}));

const mocked = vi.mocked(api);
const finance = vi.mocked(financeApi);

/** Un tabulador mínimo del ciclo anterior: lo que hace que haya algo que copiar. */
const PLAN_ANTERIOR = {
  id: "p1", cycleId: "c1", levelId: "l1", enrollmentCents: 0, monthlyCents: 280000,
  months: 10, firstBilledMonth: 9, dueDay: 10, lateFeeCents: 0, active: true,
  policy: { dailyMode: false, lateFeeDailyCents: 0, surchargeCountsAllDays: true, earlyPaymentCents: 0, earlyPaymentDays: 0 },
  schedule: [], monthlyDiscountCents: 0, fees: [], enrollmentDueOn: "",
  enrollmentConcepts: [{ name: "INSCRIPCIÓN", tiers: [{ effectiveFrom: "2025-08-31", effectiveTo: "", amountCents: 450000 }] }],
};

beforeEach(() => {
  mocked.listCycles.mockResolvedValue([]);
  mocked.listLevels.mockResolvedValue([]);
  mocked.listGradeLevels.mockResolvedValue([]);
  mocked.listNonWorkingPeriods.mockResolvedValue([]);
  mocked.listSchoolEvents.mockResolvedValue([]);
  mocked.listSubjects.mockResolvedValue([]);
  mocked.listGroups.mockResolvedValue([]);
  mocked.listTerms.mockResolvedValue([]);
  finance.listTuitionPlans.mockResolvedValue([]);
});

afterEach(() => vi.clearAllMocks());

describe("Catalog — cycles", () => {
  it("creates a cycle and lists it", async () => {
    const user = userEvent.setup();
    mocked.createCycle.mockResolvedValue({
      id: "c1",
      name: "2026-2027",
      status: "draft",
      startsOn: "2026-08-01",
      endsOn: "2027-06-30",
    });

    render(<Catalog onBack={() => {}} />);

    // Creating is a drawer now — the same one a new pupil opens — so the screen
    // shows a choice OR a form, never both at once.
    await user.click(await screen.findByRole("button", { name: "Nuevo ciclo" }));
    const drawer = await screen.findByRole("dialog", { name: "Nuevo ciclo escolar" });
    await user.type(within(drawer).getByLabelText(/^Nombre del (ciclo|nivel)$/), "2026-2027");
    fireEvent.change(within(drawer).getByLabelText("Inicia"), { target: { value: "2026-08-01" } });
    fireEvent.change(within(drawer).getByLabelText("Termina"), { target: { value: "2027-06-30" } });

    await user.click(within(drawer).getByRole("button", { name: "Crear ciclo" }));

    await waitFor(() =>
      expect(mocked.createCycle).toHaveBeenCalledWith(expect.any(String), {
        name: "2026-2027",
        startsOn: "2026-08-01",
        endsOn: "2027-06-30",
      }),
    );
    // The freshly created cycle appears in the picker as a selectable chip —
    // and, because it is a draft, as the "Activar ciclo 2026-2027" action. Two
    // buttons name it, which is the screen doing its job: pick it, then put it
    // into service.
    expect((await screen.findAllByRole("button", { name: /2026-2027/ })).length).toBeGreaterThan(0);
    expect(await screen.findByRole("button", { name: /Activar ciclo 2026-2027/ })).toBeInTheDocument();
  });
});

describe("Catalog — nested level → grade level", () => {
  it("adds a level, then a grade level under it with an auto-suggested ordinal", async () => {
    const user = userEvent.setup();
    mocked.listCycles.mockResolvedValue([
      { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" },
    ]);
    mocked.createLevel.mockResolvedValue({
      id: "l1",
      name: "PRIMARIA",
      sepCode: "PRI",
      enrollmentGranularity: "cycle",
    });
    mocked.createGradeLevel.mockResolvedValue({ id: "g1", levelId: "l1", name: "1° PRIMARIA", ordinal: 1 });

    render(<Catalog onBack={() => {}} />);

    // Create a level, in its drawer. "Por ciclo" comes pre-selected: it is what
    // nearly every level is, and a required choice with no default was one more
    // click on every single one.
    await user.click(await screen.findByRole("button", { name: "Nuevo nivel" }));
    const levelDrawer = await screen.findByRole("dialog", { name: "Nuevo nivel educativo" });
    await user.type(within(levelDrawer).getByLabelText(/^Nombre del (ciclo|nivel)$/), "Primaria");
    await user.type(within(levelDrawer).getByLabelText("Clave SEP"), "PRI");
    await user.click(within(levelDrawer).getByRole("button", { name: "Crear nivel" }));

    await waitFor(() =>
      expect(mocked.createLevel).toHaveBeenCalledWith(expect.any(String), {
        name: "PRIMARIA",
        sepCode: "PRI",
        enrollmentGranularity: "cycle",
      }),
    );

    // Creating the level selects it, which loads its (empty) grade levels and
    // reveals the grade-level form with the next ordinal (1) pre-filled.
    await waitFor(() => expect(mocked.listGradeLevels).toHaveBeenCalledWith(expect.any(String), "l1"));

    // El grado NO abre cajón: se captura en su propio renglón, con el orden que
    // sigue ya escrito, así que basta el nombre y Enter.
    await user.click(await screen.findByRole("button", { name: "Nuevo grado" }));
    await user.type(screen.getByLabelText("Nombre del grado"), "1° primaria{Enter}");

    await waitFor(() =>
      expect(mocked.createGradeLevel).toHaveBeenCalledWith(expect.any(String), {
        levelId: "l1",
        name: "1° PRIMARIA",
        ordinal: 1,
      }),
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

describe("Catalog — todo se crea en modal", () => {
  // What the school asked for: the catalogue looked and behaved unlike the rest
  // of the system. Every panel carried a form open underneath its list, so the
  // screen showed a choice and a creation form at the same time, six times over.
  //
  // Now the panels list, and creating opens the same drawer a new pupil does.
  it("shows no form until somebody asks to create something", async () => {
    mocked.listCycles.mockResolvedValue([
      { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" },
    ]);
    mocked.listLevels.mockResolvedValue([
      { id: "l1", name: "PRIMARIA", sepCode: "PRI", enrollmentGranularity: "cycle" },
    ]);

    render(<Catalog onBack={() => {}} />);
    await screen.findByRole("button", { name: "Nuevo ciclo" });

    // No drawer, and none of the fields that live in one.
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Clave SEP")).not.toBeInTheDocument();
  });

  // Escape closes it, like every other drawer in the system, and nothing is
  // created on the way out.
  it("closes the drawer on Escape without creating anything", async () => {
    const user = userEvent.setup();
    mocked.listCycles.mockResolvedValue([]);

    render(<Catalog onBack={() => {}} />);
    await user.click(await screen.findByRole("button", { name: "Nuevo ciclo" }));
    await screen.findByRole("dialog", { name: "Nuevo ciclo escolar" });

    await user.keyboard("{Escape}");

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(mocked.createCycle).not.toHaveBeenCalled();
  });

  // An incomplete form says which field is missing instead of presenting a dead
  // button that explains nothing.
  it("marca en el propio campo lo que falta, en vez de apagar el botón", async () => {
    // Antes esto era una frase: "Escribe el nombre del ciclo y sus dos fechas".
    // Decía que faltaba algo sin decir CUÁL, y en una forma de varios campos eso
    // obliga a revisarlos todos. Ahora cada campo que falta se marca en sí mismo.
    const user = userEvent.setup();
    mocked.listCycles.mockResolvedValue([]);

    render(<Catalog onBack={() => {}} />);
    await user.click(await screen.findByRole("button", { name: "Nuevo ciclo" }));
    const drawer = await screen.findByRole("dialog", { name: "Nuevo ciclo escolar" });

    await user.click(within(drawer).getByRole("button", { name: "Crear ciclo" }));

    expect(within(drawer).getByLabelText(/^Nombre del (ciclo|nivel)$/)).toHaveAttribute("aria-invalid", "true");
    expect(within(drawer).getByLabelText("Inicia")).toHaveAttribute("aria-invalid", "true");
    expect(within(drawer).getByLabelText("Termina")).toHaveAttribute("aria-invalid", "true");
    expect(mocked.createCycle).not.toHaveBeenCalled();
  });
});

/**
 * Save & New donde de verdad sirve.
 *
 * Una escuela captura maternal, kínder y primaria de corrido. El nivel se queda
 * en cajón porque elegir "por periodo" cambia cómo se inscribe el alumno todo el
 * año y esa decisión necesita su párrafo — pero el cajón no tiene por qué
 * cerrarse entre uno y otro.
 */
describe("crear varios niveles de corrido", () => {
  it("guarda sin cerrar y deja el cajón listo para el siguiente", async () => {
    const user = userEvent.setup();
    mocked.listCycles.mockResolvedValue([
      { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" },
    ]);
    mocked.createLevel.mockResolvedValue({
      id: "l1", name: "MATERNAL", sepCode: "MAT", enrollmentGranularity: "cycle",
    });

    render(<Catalog onBack={() => {}} />);
    await user.click(await screen.findByRole("button", { name: "Nuevo nivel" }));
    const drawer = await screen.findByRole("dialog", { name: "Nuevo nivel educativo" });
    await user.type(within(drawer).getByLabelText(/^Nombre del (ciclo|nivel)$/), "Maternal");
    await user.type(within(drawer).getByLabelText("Clave SEP"), "MAT");

    await user.click(within(drawer).getByRole("button", { name: "Guardar y crear otro" }));

    await waitFor(() => expect(mocked.createLevel).toHaveBeenCalled());
    expect(screen.getByRole("dialog", { name: "Nuevo nivel educativo" })).toBeInTheDocument();
    expect(within(drawer).getByLabelText(/^Nombre del (ciclo|nivel)$/)).toHaveValue("");
  });
});

/**
 * El atajo que convierte la captura del tabulador en tres clics.
 *
 * `TuitionPanel` sabe ofrecerlo desde que el tabulador se volvió página, pero
 * durante un rato nadie le pasó el ciclo anterior — así que "Copiar de…" no
 * aparecía NUNCA en la app real, exactamente como el buscador de alumno que
 * terminó borrándose por lo mismo. Un atajo que nadie puede alcanzar no existe.
 */
describe("el tabulador sabe de qué ciclo copiar", () => {
  it("le pasa el ciclo inmediatamente anterior al seleccionado", async () => {
    const user = userEvent.setup();
    mocked.listCycles.mockResolvedValue([
      { id: "c1", name: "2025-2026", status: "closed", startsOn: "2025-08-01", endsOn: "2026-06-30" },
      { id: "c2", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" },
    ]);
    mocked.listLevels.mockResolvedValue([
      { id: "l1", name: "KÍNDER", sepCode: "KIN", enrollmentGranularity: "cycle" },
    ]);

    // Sólo el ciclo anterior tiene tabulador: es lo que hace que haya algo que
    // copiar, y de paso prueba que se pide el plan del ciclo CORRECTO.
    finance.listTuitionPlans.mockImplementation(async (_url: string, cycleId: string) =>
      cycleId === "c1" ? [PLAN_ANTERIOR] : [],
    );

    render(<Catalog onBack={() => {}} />);
    await screen.findByRole("button", { name: /Definir|Editar/ });
    await user.click(screen.getByRole("button", { name: /Definir|Editar/ }));

    // El ciclo anterior es 2025-2026, no "el primero de la lista".
    expect(await screen.findByRole("button", { name: "Copiar" })).toBeInTheDocument();
    expect(screen.getByText(/Copiar de 2025-2026/)).toBeInTheDocument();
  });

  it("no lo ofrece cuando el ciclo seleccionado es el primero que hubo", async () => {
    const user = userEvent.setup();
    mocked.listCycles.mockResolvedValue([
      { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" },
    ]);
    mocked.listLevels.mockResolvedValue([
      { id: "l1", name: "KÍNDER", sepCode: "KIN", enrollmentGranularity: "cycle" },
    ]);

    render(<Catalog onBack={() => {}} />);
    await screen.findByRole("button", { name: /Definir|Editar/ });
    await user.click(screen.getByRole("button", { name: /Definir|Editar/ }));

    await screen.findByRole("heading", { name: /^Precios de / });
    expect(screen.queryByRole("button", { name: "Copiar" })).toBeNull();
  });
});

/**
 * El tabulador OCUPA la pantalla mientras se captura.
 *
 * Nació renderizándose entre la barra de ciclos y el calendario, con todo el
 * catálogo alrededor: no era una página, era un panel con migaja de página. Y no
 * era sólo feo — el chip del ciclo seguía clicable, así que cambiar de año a
 * media captura re-apuntaba el guardado SIN limpiar lo escrito: los precios del
 * año viejo se escribían encima del nuevo.
 */
describe("capturar precios ocupa la pantalla", () => {
  async function abrirTabulador(user: ReturnType<typeof userEvent.setup>): Promise<void> {
    mocked.listCycles.mockResolvedValue([
      { id: "c1", name: "2025-2026", status: "closed", startsOn: "2025-08-01", endsOn: "2026-06-30" },
      { id: "c2", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" },
    ]);
    mocked.listLevels.mockResolvedValue([
      { id: "l1", name: "KÍNDER", sepCode: "KIN", enrollmentGranularity: "cycle" },
    ]);
    render(<Catalog onBack={() => {}} />);
    await screen.findByRole("button", { name: /Definir|Editar/ });
    await user.click(screen.getByRole("button", { name: /Definir|Editar/ }));
    await screen.findByRole("heading", { name: /^Precios de / });
  }

  it("esconde el catálogo detrás mientras se captura", async () => {
    const user = userEvent.setup();
    await abrirTabulador(user);

    // Nada del catálogo sigue a la vista: ni el encabezado, ni las tarjetas,
    // ni los otros paneles.
    expect(screen.queryByRole("heading", { name: "Configuración académica" })).toBeNull();
    expect(screen.queryByText("Ciclo escolar")).toBeNull();
    expect(screen.queryByText("Calendario escolar")).toBeNull();
  });

  // El defecto de dinero: sin la barra de ciclos no hay manera de re-apuntar el
  // guardado a otro año a media captura.
  it("no deja cambiar de ciclo con el tabulador abierto", async () => {
    const user = userEvent.setup();
    await abrirTabulador(user);

    expect(screen.queryByRole("button", { name: /2026-2027/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /Nuevo ciclo/ })).toBeNull();
  });

  it("Cancelar devuelve el catálogo entero", async () => {
    const user = userEvent.setup();
    await abrirTabulador(user);

    await user.click(screen.getByRole("button", { name: "Cancelar" }));

    expect(await screen.findByRole("heading", { name: "Configuración académica" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /^Precios de / })).toBeNull();
  });
});

/**
 * Editar un tabulador que YA existe.
 *
 * Al subir el estado a `Catalog` para que la página ocupe la pantalla, el retorno
 * temprano puso `<TuitionPanel>` en OTRA posición del árbol: React lo desmonta y
 * lo vuelve a montar, su lista de planes se reinicia a vacía, y `TuitionPage`
 * siembra todos sus campos en inicializadores de `useState` que ya no se vuelven
 * a correr. Resultado: "Editar" abría el tabulador EN BLANCO, y guardar
 * sobrescribía cuotas, cobranza, día límite y fecha de inscripción con los
 * valores por omisión de la forma.
 */
describe("editar un tabulador existente", () => {
  it("llega con los valores del plan guardado, no en blanco", async () => {
    const user = userEvent.setup();
    mocked.listCycles.mockResolvedValue([
      { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" },
    ]);
    mocked.listLevels.mockResolvedValue([
      { id: "l1", name: "KÍNDER", sepCode: "KIN", enrollmentGranularity: "cycle" },
    ]);
    finance.listTuitionPlans.mockResolvedValue([
      {
        ...PLAN_ANTERIOR,
        cycleId: "c1",
        monthlyCents: 280000,
        months: 11,
        dueDay: 7,
        enrollmentDueOn: "2026-06-30",
        fees: [{ name: "CUOTA DE JULIO", amountCents: 160000, months: [9, 10] }],
      },
    ]);

    render(<Catalog onBack={() => {}} />);
    await user.click(await screen.findByRole("button", { name: /Editar/ }));
    await screen.findByRole("heading", { name: /^Precios de / });

    expect(screen.getByLabelText("Colegiatura mensual")).toHaveValue("2800.00");
    expect(screen.getByLabelText("Mensualidades")).toHaveValue(11);
    expect(screen.getByLabelText("Día límite")).toHaveValue(7);
    expect(screen.getByLabelText("Fecha límite de inscripción")).toHaveValue("2026-06-30");
    // Las cuotas guardadas siguen ahí: guardar no puede borrarlas.
    expect(screen.getByRole("button", { name: /Cuotas/ })).toHaveTextContent("1");
  });
});
