import { screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../../config/api";
import { OPEN_STEP_KEY } from "./useOpenStep";
import { G1, G2, G9, PRIMARIA, SECUNDARIA, T1, freshSchool, installSchool, openStep, renderSetup, settled, stepsNav, type User } from "./testHarness";

const config = vi.mocked(configApi);

async function openStructure(props: Parameters<typeof renderSetup>[0] = {}): Promise<{ user: User; step: HTMLElement }> {
  const { user } = renderSetup({ structureView: true, ...props });
  await settled();
  const step = await openStep(user, "Estructura de la escuela");
  return { user, step };
}

/** The card of one level: the region its h3 names. */
const card = (step: HTMLElement, level: string): HTMLElement => within(step).getByRole("region", { name: level });
/** One grade's row inside its level's card: the group its name labels. */
const row = (scope: HTMLElement, grade: string): HTMLElement => within(scope).getByRole("group", { name: grade });

/*
 * module_school_structure ON (PLAN-redesign-estructura): «Estructura de la
 * escuela» is step 3 of 7 and «Grupos» is gone. With the flag OFF every other
 * CatalogSetup test keeps the eight steps.
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
  reorderLevels: vi.fn(), renameGradeLevel: vi.fn(), deleteGradeLevel: vi.fn(), renameGroup: vi.fn(), deleteGroup: vi.fn(),
}));
vi.mock("../../../finance/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../../finance/api")>()),
  listTuitionPlans: vi.fn(), upsertTuitionPlan: vi.fn(), runBilling: vi.fn(),
}));

beforeEach(() => {
  window.sessionStorage.clear();
  installSchool(freshSchool());
});
afterEach(() => vi.clearAllMocks());

describe("CatalogSetup — Estructura de la escuela (flag ON)", () => {
  it("lists seven steps, with Estructura de la escuela third and no Grupos", async () => {
    renderSetup({ structureView: true });
    await settled();
    expect(within(stepsNav()).getByRole("button", { name: /^Estructura de la escuela/ })).toBeInTheDocument();
    expect(within(stepsNav()).queryByRole("button", { name: /^Grupos/ })).toBeNull();
  });

  it("numbers the merged step 3 of 7, and Periodos steps back to it", async () => {
    const { user } = renderSetup({ structureView: true });
    await settled();
    const step = await openStep(user, "Estructura de la escuela");
    expect(within(step).getByText("Paso 3 de 7")).toBeInTheDocument();
    const terms = await openStep(user, "Periodos");
    await user.click(within(terms).getByRole("button", { name: /Anterior: Estructura de la escuela/ }));
    await waitFor(() => expect(screen.getByRole("heading", { level: 2, name: "Estructura de la escuela" })).toBeVisible());
  });

  it("opens the merged step for a session that last had Grupos open", async () => {
    window.sessionStorage.setItem(OPEN_STEP_KEY, "groups");
    renderSetup({ structureView: true });
    await settled();
    expect(await screen.findByRole("heading", { level: 2, name: "Estructura de la escuela" })).toBeVisible();
  });
});

describe("Periodos (flag ON) — never an empty view", () => {
  it("opens on the first level still without periods and says how each level goes", async () => {
    installSchool(freshSchool({ terms: [T1] }));
    const { user } = renderSetup({ structureView: true });
    await settled();
    const step = await openStep(user, "Periodos");
    const levels = within(step).getByRole("group", { name: "Nivel" });
    await waitFor(() => expect(within(levels).getByRole("button", { name: "SECUNDARIA, Sin periodos" })).toHaveAttribute("aria-pressed", "true"));
    expect(within(levels).getByRole("button", { name: "PRIMARIA, 1 periodo" })).toHaveAttribute("aria-pressed", "false");
    expect(within(step).queryByText("Elige un nivel para ver sus periodos.")).toBeNull();
  });

  it("opens on the first level when every level is complete", async () => {
    const { user } = renderSetup({ structureView: true });
    await settled();
    const step = await openStep(user, "Periodos");
    const levels = within(step).getByRole("group", { name: "Nivel" });
    await waitFor(() => expect(within(levels).getByRole("button", { name: "PRIMARIA, 1 periodo" })).toHaveAttribute("aria-pressed", "true"));
  });
});

describe("Estructura de la escuela — the tree", () => {
  it("shows every level as a card, each grade as a row and its groups as pills", async () => {
    const { step } = await openStructure();
    expect(within(step).getByRole("heading", { level: 3, name: "PRIMARIA" })).toBeInTheDocument();
    expect(within(step).getByRole("heading", { level: 3, name: "SECUNDARIA" })).toBeInTheDocument();
    const prim = card(step, "PRIMARIA");
    expect(within(row(prim, G1.name)).getByText("A · 30 lugares")).toBeInTheDocument();
    expect(within(row(prim, G2.name)).getByText("Sin grupos")).toBeInTheDocument();
    expect(within(row(card(step, "SECUNDARIA"), G9.name)).getByText("Sin grupos")).toBeInTheDocument();
    expect(within(prim).getByText("2 grados · 1 grupo · 1 sin grupos")).toBeInTheDocument();
  });

  it("keeps rename and delete visible beside every grade and group", async () => {
    const { step } = await openStructure();
    const first = row(card(step, "PRIMARIA"), G1.name);
    // Every level has a 1°: the names say which one (design review MUST 3).
    expect(within(first).getByRole("button", { name: `Renombrar grado ${G1.name} de PRIMARIA` })).toBeInTheDocument();
    expect(within(first).getByRole("button", { name: `Borrar grado ${G1.name} de PRIMARIA` })).toBeInTheDocument();
    expect(within(first).getByRole("button", { name: `Renombrar grupo A de ${G1.name} de PRIMARIA` })).toBeInTheDocument();
    expect(within(first).getByRole("button", { name: `Borrar grupo A de ${G1.name} de PRIMARIA` })).toBeInTheDocument();
  });

  it("creates a group in the grade of its row, with the same call and defaults, and proposes the next letter at once", async () => {
    const { user, step } = await openStructure();
    const second = row(card(step, "PRIMARIA"), G2.name);
    await user.click(within(second).getByRole("button", { name: `Agregar grupo a ${G2.name} de PRIMARIA` }));
    await user.click(within(second).getByRole("textbox", { name: /Nombre del grupo/ }));
    await user.keyboard("{Enter}");
    await waitFor(() => expect(config.createGroup).toHaveBeenCalledWith(expect.any(String), { cycleId: "c1", gradeLevelId: G2.id, name: "A", capacity: 30 }));
    await waitFor(() => expect(within(second).getByRole("textbox", { name: /Nombre del grupo/ })).toHaveValue("B"));
    await user.keyboard("{Enter}");
    await waitFor(() => expect(config.createGroup).toHaveBeenLastCalledWith(expect.any(String), expect.objectContaining({ gradeLevelId: G2.id, name: "B" })));
    await waitFor(() => expect(within(second).getByRole("textbox", { name: /Nombre del grupo/ })).toHaveValue("C"));
  });

  it("creates a grade in the level of its card without changing the selected level", async () => {
    const { user, step } = await openStructure();
    const sec = card(step, "SECUNDARIA");
    await user.click(within(sec).getByRole("button", { name: "Agregar grado a SECUNDARIA" }));
    await user.type(within(sec).getByRole("textbox", { name: /Nombre del grado/ }), "2° SECUNDARIA{Enter}");
    await waitFor(() => expect(config.createGradeLevel).toHaveBeenCalledWith(expect.any(String), { levelId: SECUNDARIA.id, name: "2° SECUNDARIA", ordinal: 2 }));
    expect(await within(sec).findByRole("group", { name: "2° SECUNDARIA" })).toBeInTheDocument();
  });

  it("moves a level with Subir / Bajar in its card", async () => {
    const { user, step } = await openStructure();
    await user.click(within(card(step, "PRIMARIA")).getByRole("button", { name: "Bajar PRIMARIA" }));
    await waitFor(() => expect(config.reorderLevels).toHaveBeenCalledWith(expect.any(String), [SECUNDARIA.id, PRIMARIA.id]));
  });

  it("opens the level's settings in a side panel, with the same controls", async () => {
    const { user, step } = await openStructure();
    await user.click(within(card(step, "PRIMARIA")).getByRole("button", { name: "Ajustes de PRIMARIA" }));
    const panel = await screen.findByRole("dialog", { name: /PRIMARIA/ });
    expect(within(panel).getByRole("radio", { name: "Por día" })).toBeInTheDocument();
    expect(within(panel).getByRole("checkbox", { name: "Lleva calificación numérica" })).toBeInTheDocument();
  });

  it("without a cycle, asks for one where the groups would be", async () => {
    installSchool(freshSchool({ cycles: [] }));
    const { step } = await openStructure();
    expect(within(step).getAllByText(/Primero crea el ciclo escolar/).length).toBeGreaterThan(0);
    expect(within(step).queryByRole("button", { name: /Agregar grupo/ })).toBeNull();
  });

  it("never says Sin grupos when the groups could not be read, and offers to retry", async () => {
    config.listGroups.mockRejectedValue(new Error("down"));
    const { step } = await openStructure();
    expect(within(step).queryByText("Sin grupos")).toBeNull();
    expect(within(step).getByRole("button", { name: "Reintentar" })).toBeInTheDocument();
  });

  it("starts an empty school with Nuevo nivel", async () => {
    installSchool(freshSchool({ levels: [], grades: [], groups: [] }));
    const { step } = await openStructure();
    expect(within(step).getByText("Aún no hay niveles. Agrega el primero.")).toBeInTheDocument();
    expect(within(step).getAllByRole("button", { name: "Nuevo nivel" }).length).toBeGreaterThan(0);
  });

  it("read-only: no Nuevo nivel, no rename or delete, no Subir / Bajar", async () => {
    const { step } = await openStructure({ canWriteConfig: false, canReadTuition: false });
    expect(within(step).queryByRole("button", { name: "Nuevo nivel" })).toBeNull();
    expect(within(step).queryByRole("button", { name: /Renombrar|Borrar/ })).toBeNull();
    expect(within(step).queryByRole("button", { name: /^Bajar|^Subir/ })).toBeNull();
    expect(within(row(card(step, "PRIMARIA"), G1.name)).getByText("A · 30 lugares")).toBeInTheDocument();
  });

  it("after deleting a group, the focus lands on its grade's create row and the deletion is said", async () => {
    const { user, step } = await openStructure();
    const first = row(card(step, "PRIMARIA"), G1.name);
    await user.click(within(first).getByRole("button", { name: `Borrar grupo A de ${G1.name} de PRIMARIA` }));
    await user.click(await screen.findByRole("button", { name: "Borrar grupo" }));
    await waitFor(() => expect(config.deleteGroup).toHaveBeenCalledWith(expect.any(String), "gr1"));
    await waitFor(() => expect(within(first).getByRole("button", { name: `Agregar grupo a ${G1.name} de PRIMARIA` })).toHaveFocus());
    expect(within(first).getByRole("status")).toHaveTextContent("Grupo A borrado");
    expect(within(first).getByText("Sin grupos")).toBeInTheDocument();
  });

  it("after deleting a grade, the focus moves to the next grade's rename control", async () => {
    const { user, step } = await openStructure();
    const prim = card(step, "PRIMARIA");
    await user.click(within(row(prim, G1.name)).getByRole("button", { name: `Borrar grado ${G1.name} de PRIMARIA` }));
    await user.click(await screen.findByRole("button", { name: "Borrar grado" }));
    await waitFor(() => expect(within(prim).getByRole("button", { name: `Renombrar grado ${G2.name} de PRIMARIA` })).toHaveFocus());
    expect(within(prim).queryByRole("group", { name: G1.name })).toBeNull();
  });

  // Tech-lead H2: a refresh that fails after a write keeps the tree and says so.
  it("offers Reintentar when the refresh after a write failed", async () => {
    const { user, step } = await openStructure();
    config.listGroups.mockRejectedValueOnce(new Error("down"));
    const second = row(card(step, "PRIMARIA"), G2.name);
    await user.click(within(second).getByRole("button", { name: `Agregar grupo a ${G2.name} de PRIMARIA` }));
    await user.click(within(second).getByRole("textbox", { name: /Nombre del grupo/ }));
    await user.keyboard("{Enter}");
    expect(await within(step).findByRole("button", { name: "Reintentar" })).toBeInTheDocument();
    expect(within(second).getByText("A · 30 lugares")).toBeInTheDocument();
  });

  // Owner, 20-oct (option 1): one colour per kind, the same everywhere, with a legend.
  it("paints every level, grade and group in its kind's colour, and says which is which", async () => {
    const { step } = await openStructure();
    const legend = within(step).getByRole("list", { name: "Colores" });
    expect(within(legend).getAllByRole("listitem").map((li) => li.textContent)).toEqual(["Nivel", "Grado", "Grupo"]);
    const prim = card(step, "PRIMARIA");
    expect(prim).toHaveAttribute("data-tier", "level");
    expect(within(row(prim, G1.name)).getByText(G1.name).closest("[data-tier]")).toHaveAttribute("data-tier", "grade");
    expect(within(row(prim, G1.name)).getByText("A · 30 lugares").closest("[data-tier]")).toHaveAttribute("data-tier", "group");
  });
});

