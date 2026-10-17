import { screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../../config/api";
import { ConfigError, type Level } from "../../../config/api";
import { freshSchool, installSchool, openStep, renderSetup, settled, type User } from "./testHarness";

/*
 * T8 in the live view (module_config_setup): grades and groups are corrected
 * where they stand, and the levels are put in school order — all only for
 * config:write. The gateway is the boundary; this is UX.
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
const API = expect.any(String);

beforeEach(() => {
  window.sessionStorage.clear();
  installSchool(freshSchool());
});
afterEach(() => vi.clearAllMocks());

const orderList = (step: HTMLElement): HTMLElement => within(step).getByRole("list", { name: "Orden de los niveles" });
const orderNames = (step: HTMLElement): string[] => within(orderList(step)).getAllByRole("listitem").map((li) => li.textContent ?? "");

async function levelsStep(user: User, level = "PRIMARIA"): Promise<HTMLElement> {
  const step = await openStep(user, "Niveles y grados");
  await user.click(within(step).getByRole("button", { name: level }));
  return step;
}

async function groupsStep(user: User): Promise<HTMLElement> {
  const step = await openStep(user, "Grupos");
  await user.click(within(step).getByRole("button", { name: "PRIMARIA" }));
  await user.click(within(step).getByRole("button", { name: /^1° PRIMARIA/ }));
  await within(step).findByText(/A · 30 lugares/);
  return step;
}

describe("without config:write", () => {
  it("offers no reorder, rename or delete", async () => {
    const { user } = renderSetup({ canWriteConfig: false });
    await settled();
    const levels = await levelsStep(user);
    await within(levels).findByRole("button", { name: "1° PRIMARIA" });
    expect(within(levels).queryByRole("button", { name: /^Subir|^Bajar/ })).toBeNull();
    expect(within(levels).queryByRole("button", { name: /^Renombrar|^Borrar/ })).toBeNull();

    const groups = await groupsStep(user);
    expect(within(groups).queryByRole("button", { name: /^Renombrar|^Borrar/ })).toBeNull();
  });
});

describe("Niveles y grados · school order", () => {
  it("shows the levels in the server's order and Subir sends the whole new order", async () => {
    const { user } = renderSetup({ canWriteConfig: true });
    await settled();
    const step = await levelsStep(user);
    expect(orderNames(step)).toEqual([expect.stringContaining("PRIMARIA"), expect.stringContaining("SECUNDARIA")]);
    expect(within(step).queryByRole("button", { name: "Subir PRIMARIA" })).toBeNull();
    expect(within(step).queryByRole("button", { name: "Bajar SECUNDARIA" })).toBeNull();

    await user.click(within(step).getByRole("button", { name: "Subir SECUNDARIA" }));
    expect(config.reorderLevels).toHaveBeenCalledWith(API, ["l2", "l1"]);
    await waitFor(() => expect(orderNames(step)).toEqual([expect.stringContaining("SECUNDARIA"), expect.stringContaining("PRIMARIA")]));
  });

  it("moves at once, and a refusal puts the old order back with the server's sentence", async () => {
    let refuse: (e: Error) => void = () => {};
    config.reorderLevels.mockImplementation(() => new Promise<Level[]>((_, reject) => { refuse = reject; }));
    const { user } = renderSetup({ canWriteConfig: true });
    await settled();
    const step = await levelsStep(user);

    await user.click(within(step).getByRole("button", { name: "Bajar PRIMARIA" }));
    // Optimistic: the list already reads the new order while the server thinks.
    expect(orderNames(step)).toEqual([expect.stringContaining("SECUNDARIA"), expect.stringContaining("PRIMARIA")]);

    refuse(new ConfigError(400, "La lista de niveles no coincide con los niveles del colegio.", "LEVEL_ORDER_MISMATCH"));
    expect(await within(step).findByRole("alert")).toHaveTextContent("La lista de niveles no coincide con los niveles del colegio.");
    expect(orderNames(step)).toEqual([expect.stringContaining("PRIMARIA"), expect.stringContaining("SECUNDARIA")]);
  });
});

describe("Niveles y grados · grades", () => {
  it("renames a grade in place", async () => {
    const { user } = renderSetup({ canWriteConfig: true });
    await settled();
    const step = await levelsStep(user);
    await user.click(await within(step).findByRole("button", { name: "Renombrar grado 2° PRIMARIA" }));
    const input = within(step).getByRole("textbox", { name: "Nuevo nombre de grado 2° PRIMARIA" });
    await user.clear(input);
    await user.type(input, "SEGUNDO{Enter}");
    expect(config.renameGradeLevel).toHaveBeenCalledWith(API, "g2", "SEGUNDO");
    expect(await within(step).findByRole("button", { name: "SEGUNDO" })).toBeInTheDocument();
  });

  it("deletes a grade after confirming, and the list loses it", async () => {
    const { user } = renderSetup({ canWriteConfig: true });
    await settled();
    const step = await levelsStep(user);
    await user.click(await within(step).findByRole("button", { name: "Borrar grado 2° PRIMARIA" }));
    const dialog = await screen.findByRole("dialog", { name: "¿Borrar el grado 2° PRIMARIA?" });
    expect(dialog).toHaveTextContent("PRIMARIA");
    await user.click(within(dialog).getByRole("button", { name: "Borrar grado" }));
    expect(config.deleteGradeLevel).toHaveBeenCalledWith(API, "g2");
    await waitFor(() => expect(within(step).queryByRole("button", { name: "2° PRIMARIA" })).toBeNull());
  });

  it("a grade in use stays, and the server says what blocks it", async () => {
    const sentence = "No se puede borrar el grado 1° PRIMARIA: tiene 1 grupo y 1 materia. Bórralos primero.";
    config.deleteGradeLevel.mockRejectedValue(new ConfigError(409, sentence, "GRADE_LEVEL_IN_USE"));
    const { user } = renderSetup({ canWriteConfig: true });
    await settled();
    const step = await levelsStep(user);
    await user.click(await within(step).findByRole("button", { name: "Borrar grado 1° PRIMARIA" }));
    await user.click(await screen.findByRole("button", { name: "Borrar grado" }));
    expect(await within(step).findByRole("alert")).toHaveTextContent(sentence);
    expect(within(step).getByRole("button", { name: "1° PRIMARIA" })).toBeInTheDocument();
  });
});

describe("Grupos", () => {
  it("renames a group in place; a taken name shows the server's sentence", async () => {
    config.renameGroup.mockRejectedValueOnce(new ConfigError(409, "Ya existe un grupo con ese nombre en este grado y ciclo.", "GROUP_NAME_TAKEN"));
    const { user } = renderSetup({ canWriteConfig: true });
    await settled();
    const step = await groupsStep(user);
    await user.click(within(step).getByRole("button", { name: "Renombrar grupo A" }));
    const input = within(step).getByRole("textbox", { name: "Nuevo nombre de grupo A" });
    await user.clear(input);
    await user.type(input, "B{Enter}");
    expect(await within(step).findByRole("alert")).toHaveTextContent("Ya existe un grupo con ese nombre en este grado y ciclo.");

    await user.clear(input);
    await user.type(input, "C{Enter}");
    expect(config.renameGroup).toHaveBeenLastCalledWith(API, "gr1", "C");
    expect(await within(step).findByText(/C · 30 lugares/)).toBeInTheDocument();
  });

  it("deletes an unused group after confirming; the grade's count follows", async () => {
    const { user } = renderSetup({ canWriteConfig: true });
    await settled();
    const step = await groupsStep(user);
    await user.click(within(step).getByRole("button", { name: "Borrar grupo A" }));
    await user.click(await screen.findByRole("button", { name: "Borrar grupo" }));
    expect(config.deleteGroup).toHaveBeenCalledWith(API, "gr1");
    await waitFor(() => expect(within(step).queryByText(/A · 30 lugares/)).toBeNull());
    await waitFor(() => expect(within(step).getByRole("button", { name: /^1° PRIMARIA/ })).toHaveTextContent("sin grupos"));
  });

  it("a group in use stays, with the server's list of what keeps it", async () => {
    const sentence = "No se puede borrar el grupo A: tiene 25 inscripciones y 40 listas de asistencia. Ese historial se conserva; puedes cambiarle el nombre.";
    config.deleteGroup.mockRejectedValue(new ConfigError(409, sentence, "GROUP_IN_USE"));
    const { user } = renderSetup({ canWriteConfig: true });
    await settled();
    const step = await groupsStep(user);
    await user.click(within(step).getByRole("button", { name: "Borrar grupo A" }));
    await user.click(await screen.findByRole("button", { name: "Borrar grupo" }));
    expect(await within(step).findByRole("alert")).toHaveTextContent(sentence);
    expect(within(step).getByText(/A · 30 lugares/)).toBeInTheDocument();
  });
});
