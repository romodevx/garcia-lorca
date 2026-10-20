import { act, screen, waitFor, within } from "@testing-library/react";
import { flushSync } from "react-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../../config/api";
import {
  ACTIVE,
  GROUP_A,
  G1,
  G9,
  PLAN,
  freshSchool,
  installSchool,
  isLime,
  openStep,
  renderSetup,
  settled,
  stepButton,
  stepRegion,
} from "./testHarness";

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
const STORAGE_KEY = "config-setup-step";

beforeEach(() => {
  window.sessionStorage.clear();
  installSchool(freshSchool());
});
afterEach(() => vi.clearAllMocks());

/** A school whose six measured steps are all done. */
const readySchool = () => freshSchool({
  groups: [GROUP_A, { ...GROUP_A, id: "gr2", gradeLevelId: "g2" }, { ...GROUP_A, id: "gr9", gradeLevelId: "g9" }],
  plans: [PLAN, { ...PLAN, id: "p2", levelId: "l2" }],
});

const visibleLimes = (region: HTMLElement): HTMLElement[] => within(region).queryAllByRole("button").filter(isLime);

describe("CatalogSetup — boot (S1.1, S1.2, S1.3)", () => {
  it("says it is loading, with the header and the way back", () => {
    config.listCycles.mockReturnValue(new Promise(() => {}));
    renderSetup();
    expect(screen.getByText("Cargando catálogo…")).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: "Configuración académica" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Volver al inicio" })).toBeInTheDocument();
    expect(screen.queryByRole("navigation")).toBeNull();
  });

  it("says the catalog could not load and paints no step", async () => {
    config.listLevels.mockRejectedValue(new Error("down"));
    renderSetup();
    expect(await screen.findByRole("alert")).toHaveTextContent("No se pudo cargar el catálogo.");
    expect(screen.queryByRole("navigation")).toBeNull();
    expect(screen.queryByRole("heading", { name: "Datos del colegio" })).toBeNull();
  });

  it("shows a notice left by another screen", async () => {
    renderSetup({ notice: "PRIMARIA se eliminó." });
    expect(await screen.findByRole("status")).toHaveTextContent("PRIMARIA se eliminó.");
  });

  it("A1 · Volver al inicio calls onBack", async () => {
    const { user, props } = renderSetup();
    await user.click(screen.getByRole("button", { name: "Volver al inicio" }));
    expect(props.onBack).toHaveBeenCalledTimes(1);
  });

  it("names the selected cycle and what is missing, and drops the count cards (§1.2)", async () => {
    renderSetup();
    await settled();
    expect(screen.getByText("Ciclo 2026-2027 · faltan Grupos y Colegiaturas")).toBeInTheDocument();
    expect(screen.queryByText("Ciclos")).toBeNull();
  });

  it("A2/A12 · offers Migrar alumnos exactly once", async () => {
    const onOpenStudentImport = vi.fn();
    const { user } = renderSetup({ onOpenStudentImport });
    await settled();
    const all = screen.getAllByRole("button", { name: /Migrar alumnos/ });
    expect(all).toHaveLength(1);
    await user.click(all[0] as HTMLElement);
    expect(onOpenStudentImport).toHaveBeenCalledTimes(1);
  });
});

describe("CatalogSetup — the step that opens (§1.4)", () => {
  it("opens the first pending step", async () => {
    renderSetup();
    await settled();
    expect(stepButton("Grupos")).toHaveAttribute("aria-current", "step");
  });

  it("reopens the step this session last had open", async () => {
    window.sessionStorage.setItem(STORAGE_KEY, "calendar");
    renderSetup();
    await settled();
    expect(stepButton("Calendario")).toHaveAttribute("aria-current", "step");
  });

  it("opens Niveles y grados when everything is done, and calls the card Configuración", async () => {
    installSchool(readySchool());
    renderSetup();
    await settled();
    expect(stepButton("Niveles y grados")).toHaveAttribute("aria-current", "step");
    expect(screen.getByRole("navigation", { name: "Configuración" })).toBeInTheDocument();
    expect(screen.getByText("Ciclo 2026-2027 · listo para inscribir")).toBeInTheDocument();
  });

  it("remembers the step the user opens for this session", async () => {
    const { user } = renderSetup();
    await settled();
    await openStep(user, "Materias");
    expect(window.sessionStorage.getItem(STORAGE_KEY)).toBe("subjects");
  });

  it("never moves the step the user picked while the status was still being read", async () => {
    let answer: (v: Awaited<ReturnType<typeof configApi.getSchoolProfile>>) => void = () => {};
    config.getSchoolProfile.mockReturnValue(new Promise((resolve) => { answer = resolve; }));
    const { user } = renderSetup();
    await screen.findByRole("navigation", { name: "Puesta en marcha" });

    await user.click(stepButton("Calendario"));
    await act(async () => answer({ found: false, profile: { name: "", legalName: "", rfc: "", cct: "", address: "", phone: "", legend: "", clabe: "" } }));
    await settled();

    expect(stepButton("Calendario")).toHaveAttribute("aria-current", "step");
  });
});

describe("CatalogSetup — steps stay mounted and hidden (§3.1, §1.7)", () => {
  it("shows only the open step; the others are hidden, not gone", async () => {
    renderSetup();
    await settled();
    expect(screen.getByRole("heading", { level: 2, name: "Grupos", hidden: true })).toBeDefined();
    expect(screen.queryByRole("heading", { name: "Calendario escolar" })).toBeNull();
    expect(screen.getByRole("heading", { name: "Calendario escolar", hidden: true })).not.toBeVisible();
    expect(screen.getByRole("heading", { name: "Datos del colegio", hidden: true })).not.toBeVisible();
  });

  it("keeps the level picked in step 3 when step 4 opens", async () => {
    const { user } = renderSetup();
    await settled();
    const levels = await openStep(user, "Niveles y grados");
    await user.click(within(levels).getByRole("button", { name: "SECUNDARIA" }));

    const terms = await openStep(user, "Periodos");
    expect(within(terms).getByRole("button", { name: "SECUNDARIA", pressed: true })).toBeInTheDocument();
    expect(await within(terms).findByRole("heading", { name: "Periodos de SECUNDARIA" })).toBeVisible();
  });

  it("moves focus to the step's h2 and announces it", async () => {
    const { user } = renderSetup();
    await settled();
    await openStep(user, "Niveles y grados");
    expect(screen.getByRole("heading", { name: "Niveles educativos" })).toHaveFocus();
    expect(screen.getByText("Paso 3 de 8, Niveles y grados")).toBeInTheDocument();
  });

  it("Siguiente opens the next step the same way", async () => {
    const { user } = renderSetup();
    await settled();
    const groups = stepRegion("Grupos");
    await user.click(within(groups).getByRole("button", { name: "Siguiente: Colegiaturas" }));
    expect(stepButton("Colegiaturas")).toHaveAttribute("aria-current", "step");
    expect(await screen.findByRole("heading", { name: "Colegiaturas" })).toHaveFocus();
  });
});

describe("CatalogSetup — one lime per step (§1.3)", () => {
  it.each(["Datos del colegio", "Ciclo escolar", "Niveles y grados", "Periodos", "Materias", "Grupos", "Colegiaturas", "Calendario"])(
    "%s never shows two limes, so Siguiente is never a second one",
    async (title) => {
      const { user } = renderSetup();
      await settled();
      const levels = await openStep(user, "Niveles y grados");
      await user.click(within(levels).getByRole("button", { name: "PRIMARIA" }));
      const region = await openStep(user, title);
      await waitFor(() => expect(within(region).queryByText("Cargando periodos…")).toBeNull());
      expect(visibleLimes(region).length).toBeLessThanOrEqual(1);
    },
  );

  it("gives Siguiente the lime where the step has none of its own (Grupos)", async () => {
    renderSetup();
    await settled();
    expect(isLime(within(stepRegion("Grupos")).getByRole("button", { name: "Siguiente: Colegiaturas" }))).toBe(true);
  });
});

describe("CatalogSetup — each state of a step (§4.3)", () => {
  it("says 'Revisando…' while a status read is in flight", async () => {
    config.listGroups.mockImplementation((_, __, gradeId) => (gradeId === undefined ? new Promise(() => {}) : Promise.resolve([])));
    renderSetup();
    await screen.findByRole("navigation");
    await waitFor(() => expect(stepButton("Grupos")).toHaveTextContent("Revisando…"));
  });

  it("leaves a step unmarked and silent when its read fails", async () => {
    config.listGroups.mockImplementation((_, __, gradeId) => (gradeId === undefined ? Promise.reject(new Error("x")) : Promise.resolve([])));
    renderSetup();
    await settled();
    expect(stepButton("Grupos").querySelector("[data-line]")).toBeNull();
    expect(stepButton("Grupos").querySelector("[data-mark='done']")).toBeNull();
  });

  it("marks a done step with its check and line, and a pending one with what is missing", async () => {
    renderSetup();
    await settled();
    expect(stepButton("Datos del colegio").querySelector("[data-mark='done']")).not.toBeNull();
    expect(stepButton("Colegiaturas")).toHaveTextContent("Sin precio en SECUNDARIA");
  });

  it.each(["Periodos", "Materias", "Grupos", "Colegiaturas", "Calendario"])("%s without a cycle points at step 2", async (title) => {
    installSchool(freshSchool({ cycles: [] }));
    const { user } = renderSetup();
    await settled();
    const region = await openStep(user, title);
    expect(within(region).getByText("Primero crea el ciclo escolar.")).toBeVisible();
    await user.click(within(region).getByRole("button", { name: "Ir al paso 2" }));
    expect(stepButton("Ciclo escolar")).toHaveAttribute("aria-current", "step");
  });

  it.each(["Periodos", "Materias", "Grupos"])("%s without levels points at step 3", async (title) => {
    installSchool(freshSchool({ levels: [], grades: [] }));
    const { user } = renderSetup();
    await settled();
    const region = await openStep(user, title);
    expect(within(region).getByText("Primero agrega un nivel.")).toBeVisible();
    await user.click(within(region).getByRole("button", { name: "Ir al paso 3" }));
    expect(stepButton("Niveles y grados")).toHaveAttribute("aria-current", "step");
  });

  it.each([["Periodos", "Elige un nivel para ver sus periodos."], ["Materias", "Elige un nivel para ver sus materias."]])(
    "%s without a level picked offers the level chips and says so",
    async (title, hint) => {
      const { user } = renderSetup();
      await settled();
      const region = await openStep(user, title);
      expect(within(region).getByRole("group", { name: "Nivel" })).toBeVisible();
      expect(within(region).getByText(hint)).toBeVisible();
      expect(within(region).getByRole("heading", { level: 2, name: title })).toBeVisible();
    },
  );

  it.each([
    ["Periodos", "Cómo se parte el año para calificar.", "Periodos de SECUNDARIA"],
    ["Materias", "Las materias de cada grado.", undefined],
    ["Grupos", "Cada grado necesita al menos un grupo para inscribir alumnos.", undefined],
  ])("%s keeps its title and purpose above the chips once a level is picked", async (title, purpose, subheading) => {
    const { user } = renderSetup();
    await settled();
    const region = await openStep(user, title);
    await user.click(within(region).getByRole("button", { name: "SECUNDARIA" }));
    if (title === "Grupos") await user.click(within(within(region).getByRole("group", { name: "Grado" })).getByRole("button", { name: /^1°/ }));

    const headings = await waitFor(() => {
      const found = within(region).getAllByRole("heading");
      expect(found.length).toBeGreaterThan(0);
      return found;
    });
    const h2 = within(region).getByRole("heading", { level: 2 });
    expect(h2).toHaveTextContent(title);
    expect(headings[0]).toBe(h2);
    expect(within(region).getByText(purpose)).toBeVisible();
    const chips = within(region).getByRole("group", { name: "Nivel" });
    expect(h2.compareDocumentPosition(chips) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    if (subheading !== undefined) expect(await within(region).findByRole("heading", { level: 3, name: subheading })).toBeVisible();
  });

  it("Grupos without a grade offers level and grade chips, with each grade's count", async () => {
    const { user } = renderSetup();
    await settled();
    const region = stepRegion("Grupos");
    expect(within(region).getByText("Elige un grado para ver sus grupos.")).toBeVisible();
    await user.click(within(region).getByRole("button", { name: "PRIMARIA" }));
    const grades = await within(region).findByRole("group", { name: "Grado" });
    expect(within(grades).getByRole("button", { name: /1° PRIMARIA/ })).toHaveTextContent("1 grupo");
    expect(within(grades).getByRole("button", { name: /2° PRIMARIA/ })).toHaveTextContent("sin grupos");
  });
});

describe("CatalogSetup — the status follows the work (§4.2)", () => {
  it("the first grade of the last empty level turns step 3 done without a reload, and the open step stays", async () => {
    installSchool(freshSchool({ grades: [G1] }));
    const { user } = renderSetup();
    await settled();
    expect(stepButton("Niveles y grados")).toHaveTextContent("Faltan grados en SECUNDARIA");

    const levels = await openStep(user, "Niveles y grados");
    await user.click(within(levels).getByRole("button", { name: "SECUNDARIA" }));
    await user.click(await within(levels).findByRole("button", { name: "Nuevo grado" }));
    await user.type(within(levels).getByLabelText("Nombre del grado"), "1° secundaria{Enter}");

    await waitFor(() => expect(stepButton("Niveles y grados").querySelector("[data-mark='done']")).not.toBeNull());
    expect(stepButton("Niveles y grados")).toHaveTextContent("2 niveles · 2 grados");
    expect(stepButton("Niveles y grados")).toHaveAttribute("aria-current", "step");
    expect(config.createGradeLevel).toHaveBeenCalledWith(expect.any(String), { levelId: "l2", name: "1° SECUNDARIA", ordinal: 1 });
  });

  it("re-reads only the groups after a group is created", async () => {
    const { user } = renderSetup();
    await settled();
    const region = stepRegion("Grupos");
    await user.click(within(region).getByRole("button", { name: "SECUNDARIA" }));
    await user.click(await within(region).findByRole("button", { name: /1° SECUNDARIA/ }));
    config.listGradeLevels.mockClear();
    await user.click(await within(region).findByRole("button", { name: "Nuevo grupo" }));
    await user.keyboard("{Enter}");

    await waitFor(() => expect(within(stepRegion("Grupos")).getByRole("button", { name: /1° SECUNDARIA/ })).toHaveTextContent("1 grupo"));
    expect(config.createGroup).toHaveBeenCalledWith(expect.any(String), { cycleId: "c1", gradeLevelId: G9.id, name: "A", capacity: 30 });
    expect(config.listGroups).toHaveBeenCalledWith(expect.any(String), ACTIVE.id);
    expect(config.listGradeLevels).not.toHaveBeenCalledWith(expect.any(String));
  });
});

describe("CatalogSetup — the remembered step (§1.4)", () => {
  afterEach(() => vi.restoreAllMocks());

  it("reads this session's remembered step once (N3)", async () => {
    window.sessionStorage.setItem(STORAGE_KEY, "terms");
    const getItem = vi.spyOn(Storage.prototype, "getItem");
    renderSetup();
    await settled();
    expect(getItem.mock.calls.filter(([key]) => key === STORAGE_KEY)).toHaveLength(1);
    expect(stepButton("Periodos")).toHaveAttribute("aria-current", "step");
  });
});

describe("CatalogSetup — what the old view never does", () => {
  it("only the setup view reads every grade, term and group (§4.1)", async () => {
    renderSetup();
    await settled();
    expect(config.listGradeLevels).toHaveBeenCalledWith(expect.any(String));
    expect(config.listTerms).toHaveBeenCalledWith(expect.any(String), "c1");
    expect(config.listGroups).toHaveBeenCalledWith(expect.any(String), "c1");
  });
});

/**
 * A viewport this wide, as matchMedia answers `(max-width: Npx)` queries.
 * Returns a resize: the new width, then a "change" to every listening query.
 */
function viewport(initialWidth: number): (width: number) => void {
  let width = initialWidth;
  const listeners = new Set<() => void>();
  vi.stubGlobal("matchMedia", (query: string) => {
    const max = Number(/max-width:\s*(\d+)px/.exec(query)?.[1] ?? Infinity);
    return {
      get matches() { return width <= max; },
      media: query,
      addEventListener: (_: string, listener: () => void) => listeners.add(listener),
      removeEventListener: (_: string, listener: () => void) => listeners.delete(listener),
    };
  });
  return (next: number) => {
    width = next;
    act(() => listeners.forEach((listener) => listener()));
  };
}

describe("CatalogSetup — screen sizes (§1.6)", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("below 900 px folds the list into the 'Paso N de 8' bar", async () => {
    viewport(820);
    renderSetup();
    const bar = await screen.findByRole("button", { name: "Paso 6 de 8 · Grupos" });
    expect(bar).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByRole("navigation")).toBeNull();
  });

  it("a step chosen from the bar opens, the menu closes and focus lands on the step's h2 (§1.6, §1.7)", async () => {
    viewport(390);
    const { user } = renderSetup();
    const bar = await screen.findByRole("button", { name: "Paso 6 de 8 · Grupos" });
    await user.click(bar);
    const row = within(screen.getByRole("navigation")).getByRole("button", { name: /^Ciclo escolar/ });
    // A real tap: browsers run a microtask checkpoint between React's root
    // capture and bubble listeners, so updates queued in capture commit first.
    row.addEventListener("click", () => flushSync(() => {}));

    await user.click(row);

    expect(screen.getByRole("button", { name: "Paso 2 de 8 · Ciclo escolar" })).toHaveAttribute("aria-expanded", "false");
    expect(await screen.findByRole("heading", { level: 2, name: "Ciclo escolar" })).toHaveFocus();
  });

  it("between 900 and 1199 px keeps the list and opens the months as Lista", async () => {
    viewport(1024);
    window.sessionStorage.setItem(STORAGE_KEY, "terms");
    const { user } = renderSetup();
    await settled();
    const terms = stepRegion("Periodos");
    await user.click(within(terms).getByRole("button", { name: "PRIMARIA" }));
    expect(await within(terms).findByRole("button", { name: "Lista", pressed: true })).toBeVisible();
  });

  it("a window narrowed to 1024 px switches the months to Lista, with no sideways scroll", async () => {
    const resize = viewport(1440);
    window.sessionStorage.setItem(STORAGE_KEY, "terms");
    const { user } = renderSetup();
    await settled();
    const terms = stepRegion("Periodos");
    await user.click(within(terms).getByRole("button", { name: "PRIMARIA" }));
    expect(await within(terms).findByRole("button", { name: "Tabla", pressed: true })).toBeVisible();

    resize(1024);

    expect(within(terms).getByRole("button", { name: "Lista", pressed: true })).toBeVisible();
  });

  it("a view the user picked stays picked when the window changes size", async () => {
    const resize = viewport(1440);
    window.sessionStorage.setItem(STORAGE_KEY, "terms");
    const { user } = renderSetup();
    await settled();
    const terms = stepRegion("Periodos");
    await user.click(within(terms).getByRole("button", { name: "PRIMARIA" }));
    await user.click(await within(terms).findByRole("button", { name: "Lista" }));

    resize(1300);

    expect(within(terms).getByRole("button", { name: "Lista", pressed: true })).toBeVisible();
  });

  it("from 1200 px the months open as Tabla", async () => {
    viewport(1440);
    window.sessionStorage.setItem(STORAGE_KEY, "terms");
    const { user } = renderSetup();
    await settled();
    const terms = stepRegion("Periodos");
    await user.click(within(terms).getByRole("button", { name: "PRIMARIA" }));
    expect(await within(terms).findByRole("button", { name: "Tabla", pressed: true })).toBeVisible();
  });
});

// Owner report 2026-10-10: after an import, picking a grade in step 3 showed
// nothing, and its subtitle promised "sus materias y grupos". The groups live
// in step 6; step 3 now names them and takes you there with the grade picked.
describe("CatalogSetup — a grade picked in step 3 shows its groups", () => {
  it("names the grade's groups and opens Grupos on that grade", async () => {
    installSchool(freshSchool({ groups: [GROUP_A, { ...GROUP_A, id: "gr1b", name: "B" }] }));
    const { user } = renderSetup();
    await settled();
    const levels = await openStep(user, "Niveles y grados");
    await user.click(within(levels).getByRole("button", { name: "PRIMARIA" }));
    await user.click(within(levels).getByRole("button", { name: "1° PRIMARIA" }));

    expect(within(levels).queryByText(/materias y grupos/)).toBeNull();
    expect(within(levels).getByText("Grupos de 1° PRIMARIA: A · B")).toBeVisible();
    await user.click(within(levels).getByRole("button", { name: "Editar grupos de 1° PRIMARIA" }));

    expect(stepButton("Grupos")).toHaveAttribute("aria-current", "step");
    const groups = stepRegion("Grupos");
    expect(within(groups).getByRole("button", { name: /^1° PRIMARIA/, pressed: true })).toBeInTheDocument();
  });

  it("says when the grade has no groups yet, and the way to create them", async () => {
    const { user } = renderSetup();
    await settled();
    const levels = await openStep(user, "Niveles y grados");
    await user.click(within(levels).getByRole("button", { name: "PRIMARIA" }));
    await user.click(within(levels).getByRole("button", { name: "2° PRIMARIA" }));

    expect(within(levels).getByText("2° PRIMARIA aún no tiene grupos.")).toBeVisible();
    await user.click(within(levels).getByRole("button", { name: "Crear grupos de 2° PRIMARIA" }));
    expect(stepButton("Grupos")).toHaveAttribute("aria-current", "step");
  });
});
