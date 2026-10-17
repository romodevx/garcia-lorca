import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../../config/api";
import type { Term } from "../../../config/api";
import { cycleMonths } from "../../../config/termMonths";
import * as financeApi from "../../../finance/api";
import {
  ACTIVE,
  DRAFT,
  HOLIDAY,
  T1,
  T9,
  TRASHED,
  freshSchool,
  installSchool,
  openStep,
  renderSetup,
  settled,
  stepButton,
  stepRegion,
  type User,
} from "./testHarness";

/*
 * behavior.md A1–A56 in the setup view: every action reaches the SAME function
 * with the SAME arguments as in the old view. A1, A2/A12 and A53 live in
 * CatalogSetup.test.tsx; A15–A24 are TuitionPage's own and it is reached here
 * through A13 (the page itself is unchanged and keeps its own tests).
 */

vi.mock("../../../config/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../../config/api")>()),
  listCycles: vi.fn(), createCycle: vi.fn(), activateCycle: vi.fn(), listLevels: vi.fn(), createLevel: vi.fn(),
  updateLevelAttendanceMode: vi.fn(), updateLevelGrading: vi.fn(), listGradeLevels: vi.fn(), createGradeLevel: vi.fn(),
  listSubjects: vi.fn(), createSubject: vi.fn(), createSubjectInGrades: vi.fn(), renameSubject: vi.fn(), deleteSubject: vi.fn(),
  listGroups: vi.fn(), createGroup: vi.fn(), listTerms: vi.fn(), createTerm: vi.fn(), setTermMonths: vi.fn(), copyTermMonths: vi.fn(),
  listNonWorkingPeriods: vi.fn(), createNonWorkingPeriod: vi.fn(), deleteNonWorkingPeriod: vi.fn(),
  listSchoolEvents: vi.fn(), createSchoolEvent: vi.fn(), deleteSchoolEvent: vi.fn(),
  getSchoolProfile: vi.fn(), saveSchoolProfile: vi.fn(), listLevelTrash: vi.fn(), restoreLevel: vi.fn(),
}));
vi.mock("../../../finance/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../../finance/api")>()),
  listTuitionPlans: vi.fn(), upsertTuitionPlan: vi.fn(), runBilling: vi.fn(),
}));

const config = vi.mocked(configApi);
const finance = vi.mocked(financeApi);
const API = expect.any(String);

beforeEach(() => {
  window.sessionStorage.clear();
  installSchool(freshSchool());
});
afterEach(() => vi.clearAllMocks());

async function pickLevel(user: User, name: string): Promise<void> {
  const levels = await openStep(user, "Niveles y grados");
  await user.click(within(levels).getByRole("button", { name }));
}

describe("Paso 1 · Datos del colegio (A3–A5)", () => {
  it("A3/A4 · edits and saves through saveSchoolProfile, and A5 closes without saving", async () => {
    const { user } = renderSetup();
    await settled();
    const step = await openStep(user, "Datos del colegio");

    await user.click(await within(step).findByRole("button", { name: "Editar datos" }));
    await user.click(within(screen.getByRole("dialog", { name: "Datos del colegio" })).getByRole("button", { name: "Cancelar" }));
    expect(config.saveSchoolProfile).not.toHaveBeenCalled();

    await user.click(within(step).getByRole("button", { name: "Editar datos" }));
    await user.click(within(screen.getByRole("dialog", { name: "Datos del colegio" })).getByRole("button", { name: "Guardar datos" }));
    await waitFor(() => expect(config.saveSchoolProfile).toHaveBeenCalledWith(API, expect.objectContaining({ name: "GARCÍA LORCA" })));
  });
});

describe("Paso 2 · Ciclo escolar (A7–A11)", () => {
  beforeEach(() => installSchool(freshSchool({ cycles: [ACTIVE, DRAFT] })));

  it("A7 · choosing a cycle re-targets the whole screen", async () => {
    const { user } = renderSetup();
    await settled();
    const step = await openStep(user, "Ciclo escolar");
    await user.click(within(step).getByRole("button", { name: "2027-2028 · Borrador" }));
    await waitFor(() => expect(config.listNonWorkingPeriods).toHaveBeenCalledWith(API, "c2"));
    expect(finance.listTuitionPlans).toHaveBeenCalledWith(API, "c2");
    expect(config.listTerms).toHaveBeenCalledWith(API, "c2");
  });

  it("A8–A10 · Nuevo ciclo creates through createCycle; Escape closes", async () => {
    const { user } = renderSetup();
    await settled();
    const step = await openStep(user, "Ciclo escolar");
    await user.click(within(step).getByRole("button", { name: "Nuevo ciclo" }));
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();

    await user.click(within(step).getByRole("button", { name: "Nuevo ciclo" }));
    const drawer = screen.getByRole("dialog", { name: "Nuevo ciclo escolar" });
    await user.clear(within(drawer).getByLabelText("Nombre del ciclo"));
    await user.type(within(drawer).getByLabelText("Nombre del ciclo"), "2028-2029");
    fireEvent.change(within(drawer).getByLabelText("Inicia"), { target: { value: "2028-08-01" } });
    fireEvent.change(within(drawer).getByLabelText("Termina"), { target: { value: "2029-06-30" } });
    await user.click(within(drawer).getByRole("button", { name: "Crear ciclo" }));

    await waitFor(() => expect(config.createCycle).toHaveBeenCalledWith(API, { name: "2028-2029", startsOn: "2028-08-01", endsOn: "2029-06-30" }));
  });

  it("A8 · a new cycle's status is read for the new cycle only, never the one it replaced (N1)", async () => {
    const { user } = renderSetup();
    await settled();
    const step = await openStep(user, "Ciclo escolar");
    await user.click(within(step).getByRole("button", { name: "Nuevo ciclo" }));
    const drawer = screen.getByRole("dialog", { name: "Nuevo ciclo escolar" });
    await user.clear(within(drawer).getByLabelText("Nombre del ciclo"));
    await user.type(within(drawer).getByLabelText("Nombre del ciclo"), "2028-2029");
    fireEvent.change(within(drawer).getByLabelText("Inicia"), { target: { value: "2028-08-01" } });
    fireEvent.change(within(drawer).getByLabelText("Termina"), { target: { value: "2029-06-30" } });
    config.listTerms.mockClear();
    config.listGroups.mockClear();
    config.listGradeLevels.mockClear();
    await user.click(within(drawer).getByRole("button", { name: "Crear ciclo" }));

    const made = await waitFor(() => {
      const id = config.createCycle.mock.results[0]?.value as Promise<{ id: string }> | undefined;
      expect(id).toBeDefined();
      return id!;
    });
    const { id } = await made;
    await waitFor(() => expect(config.listTerms).toHaveBeenCalledWith(API, id));
    expect(config.listTerms).not.toHaveBeenCalledWith(API, ACTIVE.id);
    expect(config.listGroups).not.toHaveBeenCalledWith(API, ACTIVE.id);
    expect(config.listGradeLevels).not.toHaveBeenCalledWith(API);
  });

  it("A11 · Activar ciclo goes through activateCycle and re-reads the cycle's terms and groups, not the grades (N1)", async () => {
    const { user } = renderSetup();
    await settled();
    const step = await openStep(user, "Ciclo escolar");
    await user.click(within(step).getByRole("button", { name: "2027-2028 · Borrador" }));
    await waitFor(() => expect(config.listGroups).toHaveBeenCalledWith(API, "c2"));
    config.listGradeLevels.mockClear();
    config.listTerms.mockClear();
    config.listGroups.mockClear();
    await user.click(within(step).getByRole("button", { name: "Activar ciclo 2027-2028" }));
    // A live year is paused, so it asks first (QA CC-102).
    await user.click(await screen.findByRole("button", { name: "Activar 2027-2028" }));

    await waitFor(() => expect(config.activateCycle).toHaveBeenCalledWith(API, "c2"));
    await waitFor(() => expect(config.listTerms).toHaveBeenCalledWith(API, "c2"));
    await waitFor(() => expect(config.listGroups).toHaveBeenCalledWith(API, "c2"));
    expect(config.listGradeLevels).not.toHaveBeenCalledWith(API);
  });
});

describe("Paso 3 · Niveles y grados (A33–A42)", () => {
  it("A33 · a level chip loads its grades", async () => {
    const { user } = renderSetup();
    await settled();
    await pickLevel(user, "SECUNDARIA");
    await waitFor(() => expect(config.listGradeLevels).toHaveBeenCalledWith(API, "l2"));
  });

  it("A34–A36 · attendance and grading go through the same PATCHes", async () => {
    const { user } = renderSetup();
    await settled();
    await pickLevel(user, "PRIMARIA");
    const step = stepRegion("Niveles y grados");

    await user.click(within(step).getByRole("radio", { name: "Por clase" }));
    await waitFor(() => expect(config.updateLevelAttendanceMode).toHaveBeenCalledWith(API, "l1", "per_class"));

    await user.click(within(step).getByRole("button", { name: "Guardar calificación" }));
    await waitFor(() => expect(config.updateLevelGrading).toHaveBeenCalledWith(API, "l1", { enabled: true, min: 5, max: 10, decimals: 0, passing: 6 }));

    await user.click(within(step).getByRole("checkbox", { name: "Lleva calificación numérica" }));
    await waitFor(() => expect(config.updateLevelGrading).toHaveBeenLastCalledWith(API, "l1", { enabled: false, min: 5, max: 10, decimals: 0, passing: 6 }));
  });

  it("puts the level chips and Nuevo nivel first, then the grades, then the level's settings (N6)", async () => {
    const { user } = renderSetup({ canDeleteLevels: true, onDeleteLevel: vi.fn() });
    await settled();
    await pickLevel(user, "SECUNDARIA");
    const step = stepRegion("Niveles y grados");
    const inOrder = [
      within(step).getByRole("button", { name: "SECUNDARIA" }),
      within(step).getByRole("button", { name: "Nuevo nivel" }),
      await within(step).findByRole("heading", { name: "Grados de SECUNDARIA" }),
      within(step).getByRole("group", { name: "Asistencia de SECUNDARIA" }),
      within(step).getByRole("button", { name: "Eliminar nivel SECUNDARIA" }),
    ];
    inOrder.slice(1).forEach((el, i) => {
      expect(inOrder[i]!.compareDocumentPosition(el) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    });
  });

  it("A37 · Eliminar nivel opens the deletion page, only for the owner", async () => {
    const onDeleteLevel = vi.fn();
    const { user } = renderSetup({ canDeleteLevels: true, onDeleteLevel });
    await settled();
    await pickLevel(user, "PRIMARIA");
    await user.click(within(stepRegion("Niveles y grados")).getByRole("button", { name: "Eliminar nivel PRIMARIA" }));
    expect(onDeleteLevel).toHaveBeenCalledWith("l1");
  });

  it("A38–A40 · Nuevo nivel creates through createLevel, closing or staying open", async () => {
    const { user } = renderSetup();
    await settled();
    const step = await openStep(user, "Niveles y grados");

    await user.click(within(step).getByRole("button", { name: "Nuevo nivel" }));
    let drawer = screen.getByRole("dialog", { name: "Nuevo nivel educativo" });
    await user.type(within(drawer).getByLabelText("Nombre del nivel"), "Maternal");
    await user.click(within(drawer).getByRole("button", { name: "Guardar y crear otro" }));
    await waitFor(() => expect(config.createLevel).toHaveBeenCalledWith(API, { name: "MATERNAL", sepCode: "", enrollmentGranularity: "cycle" }));
    drawer = screen.getByRole("dialog", { name: "Nuevo nivel educativo" });

    await user.type(within(drawer).getByLabelText("Nombre del nivel"), "Kínder");
    await user.click(within(drawer).getByRole("button", { name: "Crear nivel" }));
    await waitFor(() => expect(config.createLevel).toHaveBeenLastCalledWith(API, { name: "KÍNDER", sepCode: "", enrollmentGranularity: "cycle" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("A41/A42 · a grade chip is picked, and Nuevo grado creates through createGradeLevel", async () => {
    const { user } = renderSetup();
    await settled();
    await pickLevel(user, "PRIMARIA");
    const step = stepRegion("Niveles y grados");
    await user.click(await within(step).findByRole("button", { name: "2° PRIMARIA" }));
    expect(within(step).getByRole("button", { name: "2° PRIMARIA" })).toHaveAttribute("aria-pressed", "true");

    await user.click(within(step).getByRole("button", { name: "Nuevo grado" }));
    await user.type(within(step).getByLabelText("Nombre del grado"), "3° primaria{Enter}");
    await waitFor(() => expect(config.createGradeLevel).toHaveBeenCalledWith(API, { levelId: "l1", name: "3° PRIMARIA", ordinal: 3 }));
  });
});

describe("Paso 4 · Periodos (A43–A48)", () => {
  it("A43/A44 · switches the view and saves a tapped month through setTermMonths", async () => {
    installSchool(freshSchool({ terms: [{ ...T1, months: [] }, T9] }));
    const { user } = renderSetup();
    await settled();
    await pickLevel(user, "PRIMARIA");
    const step = await openStep(user, "Periodos");
    await user.click(await within(step).findByRole("button", { name: "septiembre de 2026 en 1.er trimestre" }));
    await waitFor(() => expect(config.setTermMonths).toHaveBeenCalledWith(API, "t1", ["2026-09"]));

    await user.click(within(step).getByRole("button", { name: "Lista" }));
    expect(within(step).getByRole("button", { name: "Lista" })).toHaveAttribute("aria-pressed", "true");
  });

  it("A43 · the tap that gives the last month a period turns the step done without a reload", async () => {
    const months = cycleMonths(ACTIVE.startsOn, ACTIVE.endsOn);
    const first: Term = { ...T1, months: months.slice(0, 6).filter((m) => m !== "2026-09") };
    const second: Term = { ...T1, id: "t2", name: "2.º trimestre", ordinal: 2, months: months.slice(6) };
    installSchool(freshSchool({ terms: [first, second, T9] }));
    const { user } = renderSetup();
    await settled();
    await pickLevel(user, "PRIMARIA");
    const step = await openStep(user, "Periodos");
    await waitFor(() => expect(stepButton("Periodos")).toHaveTextContent("Meses sin periodo en PRIMARIA"));

    await user.click(await within(step).findByRole("button", { name: "septiembre de 2026 en 1.er trimestre" }));

    await waitFor(() => expect(stepButton("Periodos")).toHaveTextContent("2 niveles con periodos"));
  });

  it("A45 · Crear trimestres creates three terms through createTerm, and refreshes the terms read", async () => {
    installSchool(freshSchool({ terms: [T1] }));
    const { user } = renderSetup();
    await settled();
    await pickLevel(user, "SECUNDARIA");
    const step = await openStep(user, "Periodos");
    await user.click(await within(step).findByRole("button", { name: "Crear trimestres" }));

    await waitFor(() => expect(config.createTerm).toHaveBeenCalledTimes(3));
    expect(config.createTerm).toHaveBeenCalledWith(API, expect.objectContaining({ cycleId: "c1", levelId: "l2", ordinal: 1 }));
    // The new terms grade no month yet: the step moves on from "Sin periodos" but is not done.
    await waitFor(() => expect(stepButton("Periodos")).toHaveTextContent("Meses sin periodo en SECUNDARIA"));
  });

  it("A46 · Copiar meses goes through copyTermMonths", async () => {
    const { user } = renderSetup();
    await settled();
    await pickLevel(user, "PRIMARIA");
    const step = await openStep(user, "Periodos");
    await user.click(await within(step).findByRole("button", { name: "Copiar a otros niveles" }));
    await user.click(within(within(step).getByRole("group", { name: "Niveles destino" })).getByRole("button", { name: "SECUNDARIA" }));
    await user.click(within(step).getByRole("button", { name: "Copiar meses" }));
    await waitFor(() => expect(config.copyTermMonths).toHaveBeenCalledWith(API, { cycleId: "c1", fromLevelId: "l1", toLevelIds: ["l2"] }));
  });

  it("A47 · Nuevo periodo creates through createTerm", async () => {
    const { user } = renderSetup();
    await settled();
    await pickLevel(user, "PRIMARIA");
    const step = await openStep(user, "Periodos");
    await user.click(await within(step).findByRole("button", { name: "Nuevo periodo" }));
    await user.type(within(step).getByLabelText("Nombre del periodo"), "Extra");
    fireEvent.change(within(step).getByLabelText("Inicio del periodo"), { target: { value: "2026-12-01" } });
    fireEvent.change(within(step).getByLabelText("Fin del periodo"), { target: { value: "2027-02-28" } });
    await user.click(within(step).getByLabelText("Nombre del periodo"));
    await user.keyboard("{Enter}");
    await waitFor(() => expect(config.createTerm).toHaveBeenCalledWith(API, { cycleId: "c1", levelId: "l1", name: "EXTRA", ordinal: 2, startsOn: "2026-12-01", endsOn: "2027-02-28" }));
  });

  it("A48 · Periodos por grado opens the level's page", async () => {
    const onOpenLevelPeriods = vi.fn();
    const { user } = renderSetup({ onOpenLevelPeriods });
    await settled();
    await pickLevel(user, "PRIMARIA");
    const step = await openStep(user, "Periodos");
    await user.click(await within(step).findByRole("button", { name: "Periodos por grado de PRIMARIA" }));
    expect(onOpenLevelPeriods).toHaveBeenCalledWith("l1");
  });
});

describe("Paso 5 · Materias (A49–A52)", () => {
  it("A49 · Todos los grados creates the subject in every grade of the level", async () => {
    const { user } = renderSetup();
    await settled();
    await pickLevel(user, "PRIMARIA");
    const step = await openStep(user, "Materias");
    await user.type(await within(step).findByRole("textbox", { name: /nombre de la materia/i }), "Artes");
    await user.click(within(step).getByRole("button", { name: /agregar materia/i }));
    await waitFor(() => expect(config.createSubjectInGrades).toHaveBeenCalledWith(API, "ARTES", ["g1", "g2"]));
  });

  it("A50 · Renombrar goes through renameSubject", async () => {
    const { user } = renderSetup();
    await settled();
    await pickLevel(user, "PRIMARIA");
    const step = await openStep(user, "Materias");
    await user.click(await within(step).findByRole("button", { name: "Renombrar MATEMÁTICAS" }));
    const input = within(step).getByLabelText("Nuevo nombre de MATEMÁTICAS");
    await user.clear(input);
    await user.type(input, "Cálculo{Enter}");
    await waitFor(() => expect(config.renameSubject).toHaveBeenCalledWith(API, "s1", "CÁLCULO"));
  });

  it("A51/A52 · the undo is still in reach after another step opens, and it recreates the subject", async () => {
    const { user } = renderSetup();
    await settled();
    await pickLevel(user, "PRIMARIA");
    const step = await openStep(user, "Materias");
    await user.click(await within(step).findByRole("button", { name: "Eliminar MATEMÁTICAS" }));
    await waitFor(() => expect(config.deleteSubject).toHaveBeenCalledWith(API, "s1"));

    await openStep(user, "Calendario");
    const undo = await screen.findByRole("button", { name: /deshacer/i });
    expect(undo).toBeVisible();
    await user.click(undo);
    await waitFor(() => expect(config.createSubject).toHaveBeenCalledWith(API, { gradeLevelId: "g1", name: "MATEMÁTICAS" }));
  });
});

describe("Paso 7 · Colegiaturas (A13, A14, A25, A26)", () => {
  it("A13/A26 · Editar arrives with the saved values; Cancelar comes back to step 7 with the selection and a fresh status", async () => {
    const { user } = renderSetup();
    await settled();
    await pickLevel(user, "SECUNDARIA");
    const step = await openStep(user, "Colegiaturas");
    await user.click(await within(step).findByRole("button", { name: "Editar" }));

    await screen.findByRole("heading", { name: "Precios de PRIMARIA · 2026-2027" });
    expect(screen.queryByRole("navigation")).toBeNull();
    expect(screen.queryByRole("heading", { level: 1, name: "Configuración académica" })).toBeNull();
    expect(screen.getByLabelText("Colegiatura mensual")).toHaveValue("2,800.00");
    expect(screen.getByLabelText("Mensualidades")).toHaveValue(11);
    expect(screen.getByLabelText("Día límite")).toHaveValue(7);

    await user.click(screen.getByRole("button", { name: "Cancelar" }));
    await screen.findByRole("navigation");
    expect(stepButton("Colegiaturas")).toHaveAttribute("aria-current", "step");
    expect(within(stepRegion("Niveles y grados")).getByRole("button", { name: "SECUNDARIA", pressed: true, hidden: true })).toBeDefined();
  });

  it("A25 · saving the tabulador goes through upsertTuitionPlan and the step re-reads its plans", async () => {
    const { user } = renderSetup();
    await settled();
    const step = await openStep(user, "Colegiaturas");
    await user.click(await within(step).findByRole("button", { name: "Editar" }));
    await screen.findByRole("heading", { name: /^Precios de / });
    finance.listTuitionPlans.mockClear();

    await user.click(screen.getByRole("button", { name: "Guardar tabulador" }));
    await waitFor(() => expect(finance.upsertTuitionPlan).toHaveBeenCalledWith(API, expect.objectContaining({ cycleId: "c1", levelId: "l1", monthlyCents: 280000 })));
    await waitFor(() => expect(finance.listTuitionPlans).toHaveBeenCalledWith(API, "c1"));
    expect(await screen.findByRole("navigation")).toBeInTheDocument();
    expect(stepButton("Colegiaturas")).toHaveAttribute("aria-current", "step");
  });

  it("A13 · Definir opens a level with no tabulador", async () => {
    const { user } = renderSetup();
    await settled();
    const step = await openStep(user, "Colegiaturas");
    await user.click(await within(step).findByRole("button", { name: "Definir" }));
    expect(await screen.findByRole("heading", { name: "Precios de SECUNDARIA · 2026-2027" })).toBeInTheDocument();
  });

  it("A14 · Generar cargos del mes goes through runBilling", async () => {
    const { user } = renderSetup();
    await settled();
    const step = await openStep(user, "Colegiaturas");
    await user.click(within(step).getByRole("button", { name: "Generar cargos del mes" }));
    await waitFor(() => expect(finance.runBilling).toHaveBeenCalledWith(API, "c1"));
  });
});

describe("Paso 8 · Calendario (A27–A32)", () => {
  it("A27/A30 · adds a closed day and an activity through the same calls", async () => {
    const { user } = renderSetup();
    await settled();
    const step = await openStep(user, "Calendario");

    await user.click(within(step).getByRole("button", { name: "Nuevo día cerrado" }));
    await user.type(within(step).getByLabelText("Nombre"), "Patria");
    await user.type(within(step).getByLabelText("Del día"), "2026-09-16");
    await user.keyboard("{Enter}");
    await waitFor(() => expect(config.createNonWorkingPeriod).toHaveBeenCalledWith(API, { cycleId: "c1", label: "PATRIA", kind: "holiday", startsOn: "2026-09-16" }));
    await waitFor(() => expect(stepButton("Calendario")).toHaveTextContent("1 día cerrado"));

    await user.keyboard("{Escape}");
    await user.click(within(step).getByRole("button", { name: "Nueva actividad" }));
    await user.type(within(step).getByLabelText("Actividad"), "Festival");
    await user.type(within(step).getByLabelText("Fecha"), "2026-12-12");
    await user.keyboard("{Enter}");
    await waitFor(() => expect(config.createSchoolEvent).toHaveBeenCalledWith(API, { cycleId: "c1", title: "FESTIVAL", startsOn: "2026-12-12" }));
  });

  describe("removals", () => {
    beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }));
    afterEach(() => vi.useRealTimers());

    it("A28/A29 · Deshacer is still in reach after another step opens, and it cancels the DELETE", async () => {
      installSchool(freshSchool({ closedDays: [HOLIDAY] }));
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderSetup();
      await settled();
      const step = await openStep(user, "Calendario");
      await user.click(await within(step).findByRole("button", { name: "Quitar PATRIA" }));

      await openStep(user, "Grupos");
      const undo = screen.getByRole("button", { name: /deshacer/i });
      expect(undo).toBeVisible();
      await user.click(undo);
      await act(async () => {
        vi.advanceTimersByTime(UNDO_WINDOW_MS);
      });

      expect(config.deleteNonWorkingPeriod).not.toHaveBeenCalled();
      expect(stepButton("Calendario")).toHaveTextContent("1 día cerrado");
    });

    it("A28 · with nobody undoing, the DELETE reaches the server once the offer is closed", async () => {
      installSchool(freshSchool({ closedDays: [HOLIDAY] }));
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderSetup();
      await settled();
      const step = await openStep(user, "Calendario");
      await user.click(await within(step).findByRole("button", { name: "Quitar PATRIA" }));
      await user.click(screen.getByRole("button", { name: "Cerrar aviso" }));
      await waitFor(() => expect(config.deleteNonWorkingPeriod).toHaveBeenCalledWith(API, "d1", { keepalive: false }));
    });
  });
});

describe("Herramientas · Papelera (A54–A56)", () => {
  it("opens in the right column; Cancelar sends nothing, Restaurar goes through restoreLevel and the status re-reads", async () => {
    installSchool(freshSchool({ trash: [TRASHED] }));
    const { user } = renderSetup({ canDeleteLevels: true });
    await settled();
    await user.click(screen.getByRole("button", { name: "Papelera" }));
    const trash = stepRegion("Papelera");
    expect(within(trash).getByRole("heading", { name: "Papelera" })).toBeVisible();
    for (const step of within(screen.getByRole("navigation")).getAllByRole("button")) expect(step).not.toHaveAttribute("aria-current");

    await user.click(await within(trash).findByRole("button", { name: "Restaurar MATERNAL" }));
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancelar" }));
    expect(config.restoreLevel).not.toHaveBeenCalled();

    config.listGradeLevels.mockClear();
    config.listTerms.mockClear();
    config.listGroups.mockClear();
    await user.click(within(trash).getByRole("button", { name: "Restaurar MATERNAL" }));
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Restaurar nivel" }));
    await waitFor(() => expect(config.restoreLevel).toHaveBeenCalledWith(API, "b1"));
    await waitFor(() => expect(config.listLevels).toHaveBeenCalledTimes(2));
    expect(config.listGradeLevels).toHaveBeenCalledWith(API);
    // The restore brings the level's terms and groups back too (S1).
    expect(config.listTerms).toHaveBeenCalledWith(API, ACTIVE.id);
    expect(config.listGroups).toHaveBeenCalledWith(API, ACTIVE.id);
  });
});

/** useUndoableRemoval's grace window, plus a beat. */
const UNDO_WINDOW_MS = 8_100;
