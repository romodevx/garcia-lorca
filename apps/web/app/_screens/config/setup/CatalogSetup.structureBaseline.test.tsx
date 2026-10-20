import { screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../../config/api";
import { freshSchool, installSchool, openStep, renderSetup, settled } from "./testHarness";

/*
 * Characterization of «Niveles y grados» and «Grupos» before the «Estructura de
 * la escuela» redesign (PLAN-redesign-estructura §5.1): the behaviors
 * behavior.md listed with no test. They pin what the redesign must keep.
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

const config = vi.mocked(configApi);

beforeEach(() => {
  window.sessionStorage.clear();
  installSchool(freshSchool());
});
afterEach(() => vi.clearAllMocks());

describe("Estructura — baseline the redesign must keep", () => {
  // behavior.md gap #1: without config:write these stay offered (the gateway refuses).
  it("without config:write still offers Nuevo grado, attendance and grading, and Nuevo grupo", async () => {
    const { user } = renderSetup({ canWriteConfig: false, canReadTuition: false });
    await settled();
    const levels = await openStep(user, "Niveles y grados");
    await user.click(within(levels).getByRole("button", { name: /^PRIMARIA/ }));
    expect(await within(levels).findByRole("button", { name: /Nuevo grado/ })).toBeInTheDocument();
    expect(within(levels).getByRole("radio", { name: "Por día" })).toBeInTheDocument();
    expect(within(levels).getByRole("checkbox", { name: "Lleva calificación numérica" })).toBeInTheDocument();

    const groups = await openStep(user, "Grupos");
    await user.click(within(groups).getByRole("button", { name: /^1° PRIMARIA/ }));
    expect(await within(groups).findByRole("button", { name: /Nuevo grupo/ })).toBeInTheDocument();
  });

  // gap #3: the SEP key travels upper-cased and trimmed.
  it("sends the Clave SEP upper-cased and trimmed", async () => {
    const { user } = renderSetup();
    await settled();
    const levels = await openStep(user, "Niveles y grados");
    await user.click(within(levels).getByRole("button", { name: "Nuevo nivel" }));
    const drawer = await screen.findByRole("dialog");
    await user.type(within(drawer).getByRole("textbox", { name: /^Nombre del nivel/ }), "Maternal");
    await user.type(within(drawer).getByRole("textbox", { name: /^Clave SEP/ }), "  dja  ");
    await user.click(within(drawer).getByRole("button", { name: "Crear nivel" }));
    await waitFor(() => expect(config.createLevel).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({ name: "MATERNAL", sepCode: "DJA" })));
  });

  // gap #4: Esc closes the drawer; «Guardar y crear otro» puts the focus back on the first field.
  it("the level drawer closes on Esc, and saving another refocuses its first field", async () => {
    const { user } = renderSetup();
    await settled();
    const levels = await openStep(user, "Niveles y grados");
    await user.click(within(levels).getByRole("button", { name: "Nuevo nivel" }));
    let drawer = await screen.findByRole("dialog");
    await user.type(within(drawer).getByRole("textbox", { name: /^Nombre del nivel/ }), "Maternal");
    await user.click(within(drawer).getByRole("button", { name: "Guardar y crear otro" }));
    await waitFor(() => expect(config.createLevel).toHaveBeenCalledTimes(1));
    drawer = screen.getByRole("dialog");
    await waitFor(() => expect(within(drawer).getByRole("textbox", { name: /^Nombre del nivel/ })).toHaveFocus());
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  // gap #8: a refused create shows its sentence inside the setup view.
  it("a refused grade create shows its sentence in the step", async () => {
    config.createGradeLevel.mockRejectedValueOnce(new Error("conflict"));
    const { user } = renderSetup();
    await settled();
    const levels = await openStep(user, "Niveles y grados");
    await user.click(within(levels).getByRole("button", { name: /^PRIMARIA/ }));
    await user.click(await within(levels).findByRole("button", { name: /Nuevo grado/ }));
    await user.type(within(levels).getByRole("textbox", { name: /Nombre del grado/ }), "3°{Enter}");
    expect(await within(levels).findByText(/No se pudo crear el grado/)).toBeInTheDocument();
  });

  it("a refused group create shows its sentence in the step", async () => {
    config.createGroup.mockRejectedValueOnce(new Error("conflict"));
    const { user } = renderSetup();
    await settled();
    const groups = await openStep(user, "Grupos");
    await user.click(within(groups).getByRole("button", { name: /^PRIMARIA/ }));
    await user.click(await within(groups).findByRole("button", { name: /^2° PRIMARIA/ }));
    await user.click(await within(groups).findByRole("button", { name: /Nuevo grupo/ }));
    await user.click(within(groups).getByRole("textbox", { name: /Nombre del grupo/ }));
    await user.keyboard("{Enter}");
    expect(await within(groups).findByText(/No se pudo crear el grupo/)).toBeInTheDocument();
  });

  // gap #12: renaming to the same name closes the editor and calls nothing.
  it("renaming a grade to its own name calls nothing", async () => {
    const { user } = renderSetup();
    await settled();
    const levels = await openStep(user, "Niveles y grados");
    await user.click(within(levels).getByRole("button", { name: /^PRIMARIA/ }));
    await user.click(await within(levels).findByRole("button", { name: "Renombrar grado 1° PRIMARIA" }));
    await user.keyboard("{Enter}");
    await waitFor(() => expect(within(levels).queryByRole("textbox", { name: /Nuevo nombre/ })).not.toBeInTheDocument());
    expect(config.renameGradeLevel).not.toHaveBeenCalled();
  });
});
