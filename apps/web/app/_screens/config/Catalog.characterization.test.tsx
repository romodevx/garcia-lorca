import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as api from "../../config/api";
import * as financeApi from "../../finance/api";
import { Catalog } from "./Catalog";

/*
 * Characterization tests (2026-10-09), written BEFORE the Configuración
 * académica redesign to freeze what the container does today. See
 * .gstack/redesign/configuracion-academica/behavior.md, "Caracterización añadida".
 *
 * They query by role + accessible name / visible text and assert API calls and
 * resulting visible state, so a re-layout should not break them; a changed
 * behavior should.
 */

vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  createCycle: vi.fn(),
  activateCycle: vi.fn(),
  listLevels: vi.fn(),
  createLevel: vi.fn(),
  updateLevelAttendanceMode: vi.fn(),
  updateLevelGrading: vi.fn(),
  listGradeLevels: vi.fn(),
  createGradeLevel: vi.fn(),
  listSubjects: vi.fn(),
  createSubject: vi.fn(),
  listGroups: vi.fn(),
  createGroup: vi.fn(),
  listTerms: vi.fn(),
  createTerm: vi.fn(),
  setTermMonths: vi.fn(),
  copyTermMonths: vi.fn(),
  listNonWorkingPeriods: vi.fn(),
  createNonWorkingPeriod: vi.fn(),
  deleteNonWorkingPeriod: vi.fn(),
  listSchoolEvents: vi.fn(),
  createSchoolEvent: vi.fn(),
  deleteSchoolEvent: vi.fn(),
  getSchoolProfile: vi.fn(),
  saveSchoolProfile: vi.fn(),
  listLevelTrash: vi.fn(),
  restoreLevel: vi.fn(),
  ConfigError: class ConfigError extends Error {},
}));

vi.mock("../../finance/api", () => ({
  listTuitionPlans: vi.fn(),
  upsertTuitionPlan: vi.fn(),
  runBilling: vi.fn(),
}));

const mocked = vi.mocked(api);
const finance = vi.mocked(financeApi);

const ACTIVE = { id: "c1", name: "2026-2027", status: "active" as const, startsOn: "2026-08-01", endsOn: "2027-06-30" };
const DRAFT = { id: "c2", name: "2027-2028", status: "draft" as const, startsOn: "2027-08-01", endsOn: "2028-06-30" };
const PRIMARIA = { id: "l1", name: "PRIMARIA", sepCode: "", enrollmentGranularity: "cycle" as const };
const SECUNDARIA = { id: "l2", name: "SECUNDARIA", sepCode: "", enrollmentGranularity: "cycle" as const };
const GRADES_BY_LEVEL: Record<string, api.GradeLevel[]> = {
  l1: [
    { id: "g1", levelId: "l1", name: "1° PRIMARIA", ordinal: 1 },
    { id: "g2", levelId: "l1", name: "2° PRIMARIA", ordinal: 2 },
  ],
  l2: [{ id: "g9", levelId: "l2", name: "1° SECUNDARIA", ordinal: 1 }],
};

beforeEach(() => {
  mocked.listCycles.mockResolvedValue([]);
  mocked.listLevels.mockResolvedValue([]);
  mocked.listGradeLevels.mockImplementation(async (_: string, levelId?: string) => GRADES_BY_LEVEL[levelId ?? ""] ?? []);
  mocked.listNonWorkingPeriods.mockResolvedValue([]);
  mocked.listSchoolEvents.mockResolvedValue([]);
  mocked.getSchoolProfile.mockResolvedValue({
    profile: { name: "", legalName: "", rfc: "", cct: "", address: "", phone: "", legend: "", clabe: "" },
    found: false,
  });
  mocked.listSubjects.mockResolvedValue([]);
  mocked.listGroups.mockResolvedValue([]);
  mocked.listTerms.mockResolvedValue([]);
  mocked.listLevelTrash.mockResolvedValue([]);
  finance.listTuitionPlans.mockResolvedValue([]);
});

afterEach(() => vi.clearAllMocks());

/**
 * The number shown on a stat card. The ONLY place in this file that leans on
 * markup: label and value share a parent. If the redesign moves the cards,
 * update this helper, not the assertions.
 */
function statValue(label: string): string | undefined {
  const pattern = new RegExp(`^${label}\\s*(\\d+)$`);
  for (const el of screen.queryAllByText(label)) {
    const match = pattern.exec(el.parentElement?.textContent?.trim() ?? "");
    if (match !== null) return match[1];
  }
  return undefined;
}

const levelChip = (name: string): HTMLElement => screen.getByRole("button", { name: new RegExp(`^${name} · `) });

describe("Catalog — boot states (S1.1, S1.2)", () => {
  it("says it is loading and shows neither panels nor stats until the catalog arrives", async () => {
    mocked.listCycles.mockReturnValue(new Promise(() => {}));

    render(<Catalog canWriteConfig canReadTuition onBack={() => {}} />);

    expect(screen.getByText("Cargando catálogo…")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Configuración académica" })).toBeInTheDocument();
    expect(screen.queryByText("Ciclo escolar")).toBeNull();
    expect(screen.queryByText("Niveles educativos")).toBeNull();
    expect(statValue("Ciclos")).toBeUndefined();
  });

  it("says the catalog could not load and paints no panel when the boot read fails", async () => {
    mocked.listLevels.mockRejectedValue(new Error("down"));

    render(<Catalog canWriteConfig canReadTuition onBack={() => {}} />);

    expect(await screen.findByRole("alert")).toHaveTextContent("No se pudo cargar el catálogo.");
    expect(screen.queryByText("Cargando catálogo…")).toBeNull();
    expect(screen.queryByText("Datos del colegio")).toBeNull();
    expect(screen.queryByText("Ciclo escolar")).toBeNull();
    expect(screen.queryByText("Niveles educativos")).toBeNull();
    expect(statValue("Ciclos")).toBeUndefined();
    // The way back stays.
    expect(screen.getByRole("button", { name: "Volver al inicio" })).toBeInTheDocument();
  });
});

describe("Catalog — header and stats", () => {
  it("'Volver al inicio' calls onBack", async () => {
    const onBack = vi.fn();
    const user = userEvent.setup();
    render(<Catalog canWriteConfig canReadTuition onBack={onBack} />);

    await user.click(await screen.findByRole("button", { name: "Volver al inicio" }));

    expect(onBack).toHaveBeenCalledTimes(1);
  });

  it("counts cycles and levels, and 'Grados' counts only the selected level's grades", async () => {
    mocked.listCycles.mockResolvedValue([ACTIVE, DRAFT]);
    mocked.listLevels.mockResolvedValue([PRIMARIA, SECUNDARIA]);
    const user = userEvent.setup();
    render(<Catalog canWriteConfig canReadTuition onBack={() => {}} />);

    await screen.findByText("Niveles educativos");
    expect(statValue("Ciclos")).toBe("2");
    expect(statValue("Niveles")).toBe("2");
    expect(statValue("Grados")).toBe("0");

    await user.click(levelChip("PRIMARIA"));
    await waitFor(() => expect(statValue("Grados")).toBe("2"));

    await user.click(levelChip("SECUNDARIA"));
    await waitFor(() => expect(statValue("Grados")).toBe("1"));
  });
});

describe("Catalog — selection dependencies (D2, D3, A7)", () => {
  it("choosing another level reloads its grades and drops the chosen grade, so Grupos disappears", async () => {
    mocked.listCycles.mockResolvedValue([ACTIVE]);
    mocked.listLevels.mockResolvedValue([PRIMARIA, SECUNDARIA]);
    const user = userEvent.setup();
    render(<Catalog canWriteConfig canReadTuition onBack={() => {}} />);

    await user.click(await screen.findByRole("button", { name: /^PRIMARIA · / }));
    await waitFor(() => expect(mocked.listGradeLevels).toHaveBeenCalledWith(expect.any(String), "l1"));
    expect(screen.queryByRole("heading", { name: "Grupos" })).toBeNull();

    await user.click(await screen.findByRole("button", { name: "1. 1° PRIMARIA" }));
    expect(await screen.findByRole("heading", { name: "Grupos" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "1. 1° PRIMARIA" })).toHaveAttribute("aria-pressed", "true");
    await waitFor(() => expect(mocked.listGroups).toHaveBeenCalledWith(expect.any(String), "c1", "g1"));

    await user.click(levelChip("SECUNDARIA"));
    await waitFor(() => expect(mocked.listGradeLevels).toHaveBeenCalledWith(expect.any(String), "l2"));
    expect(await screen.findByRole("button", { name: "1. 1° SECUNDARIA" })).toHaveAttribute("aria-pressed", "false");
    expect(screen.queryByRole("heading", { name: "Grupos" })).toBeNull();
  });

  it("choosing another cycle re-targets tuition, calendar, agenda, terms and groups to the new cycle id", async () => {
    mocked.listCycles.mockResolvedValue([ACTIVE, DRAFT]);
    mocked.listLevels.mockResolvedValue([PRIMARIA]);
    const user = userEvent.setup();
    render(<Catalog canWriteConfig canReadTuition onBack={() => {}} />);

    await user.click(await screen.findByRole("button", { name: /^PRIMARIA · / }));
    await user.click(await screen.findByRole("button", { name: "1. 1° PRIMARIA" }));
    await screen.findByRole("heading", { name: "Grupos" });
    await waitFor(() => expect(mocked.listGroups).toHaveBeenCalledWith(expect.any(String), "c1", "g1"));
    expect(finance.listTuitionPlans).toHaveBeenCalledWith(expect.any(String), "c1");
    expect(mocked.listNonWorkingPeriods).toHaveBeenCalledWith(expect.any(String), "c1");
    expect(mocked.listSchoolEvents).toHaveBeenCalledWith(expect.any(String), "c1");
    expect(mocked.listTerms).toHaveBeenCalledWith(expect.any(String), "c1", "l1");

    await user.click(screen.getByRole("button", { name: "2027-2028 · Borrador" }));

    expect(screen.getByRole("button", { name: "2027-2028 · Borrador" })).toHaveAttribute("aria-pressed", "true");
    await waitFor(() => expect(finance.listTuitionPlans).toHaveBeenCalledWith(expect.any(String), "c2"));
    await waitFor(() => expect(mocked.listNonWorkingPeriods).toHaveBeenCalledWith(expect.any(String), "c2"));
    await waitFor(() => expect(mocked.listSchoolEvents).toHaveBeenCalledWith(expect.any(String), "c2"));
    await waitFor(() => expect(mocked.listTerms).toHaveBeenCalledWith(expect.any(String), "c2", "l1"));
    await waitFor(() => expect(mocked.listGroups).toHaveBeenCalledWith(expect.any(String), "c2", "g1"));
    // The grade selection survives a cycle change; the groups panel now speaks of the new year.
    expect(await screen.findByText("De 1° PRIMARIA en 2027-2028.")).toBeInTheDocument();
  });

  it("creating a level selects it (its grades load and its grades panel appears)", async () => {
    mocked.listCycles.mockResolvedValue([ACTIVE]);
    mocked.createLevel.mockResolvedValue(SECUNDARIA);
    const user = userEvent.setup();
    render(<Catalog canWriteConfig canReadTuition onBack={() => {}} />);

    await user.click(await screen.findByRole("button", { name: "Nuevo nivel" }));
    await user.type(screen.getByLabelText(/^Nombre del nivel$/), "Secundaria");
    await user.click(screen.getByRole("button", { name: "Crear nivel" }));

    await waitFor(() => expect(mocked.listGradeLevels).toHaveBeenCalledWith(expect.any(String), "l2"));
    expect(await screen.findByRole("heading", { name: "Grados de SECUNDARIA" })).toBeInTheDocument();
    expect(levelChip("SECUNDARIA")).toHaveAttribute("aria-pressed", "true");
  });
});

describe("Catalog — which cycle is selected (D1, handleActivateCycle)", () => {
  it("with no active cycle, the first one in the list is selected", async () => {
    mocked.listCycles.mockResolvedValue([
      { ...DRAFT, id: "c0", name: "2025-2026", startsOn: "2025-08-01", endsOn: "2026-06-30" },
      DRAFT,
    ]);
    render(<Catalog canWriteConfig canReadTuition onBack={() => {}} />);

    expect(await screen.findByRole("button", { name: "2025-2026 · Borrador" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "2027-2028 · Borrador" })).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByRole("button", { name: "Activar ciclo 2025-2026" })).toBeInTheDocument();
  });

  it("activating a draft puts it in service and stands the previously active cycle down to draft, locally", async () => {
    mocked.listCycles.mockResolvedValue([ACTIVE, DRAFT]);
    mocked.activateCycle.mockResolvedValue({ ...DRAFT, status: "active" });
    const user = userEvent.setup();
    render(<Catalog canWriteConfig canReadTuition onBack={() => {}} />);

    // The active cycle is the one selected on boot.
    expect(await screen.findByText("2026-2027 es el ciclo en curso.")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "2027-2028 · Borrador" }));
    await user.click(screen.getByRole("button", { name: "Activar ciclo 2027-2028" }));
    // 2026-2027 is live, so it asks first (QA CC-102).
    await user.click(await screen.findByRole("button", { name: "Activar 2027-2028" }));

    expect(mocked.activateCycle).toHaveBeenCalledWith(expect.any(String), "c2");
    expect(await screen.findByText("2027-2028 es el ciclo en curso.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "2027-2028 · Activo" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "2026-2027 · Borrador" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Activar ciclo/ })).toBeNull();
    // No reload: the local list mirrors the server.
    expect(mocked.listCycles).toHaveBeenCalledTimes(1);
  });
});

describe("Catalog — pricing mode hides the rest and restores it (D6)", () => {
  it("hides header, Migrar alumnos (button + card), school profile, cycle bar, calendar, levels and Papelera; Cancelar brings them back", async () => {
    mocked.listCycles.mockResolvedValue([ACTIVE]);
    mocked.listLevels.mockResolvedValue([PRIMARIA]);
    const user = userEvent.setup();
    render(<Catalog canWriteConfig canReadTuition onBack={() => {}} canDeleteLevels onDeleteLevel={vi.fn()} onOpenStudentImport={vi.fn()} />);

    const visibleOutsidePricing = async (): Promise<void> => {
      expect(await screen.findByRole("heading", { name: "Configuración académica" })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Volver al inicio" })).toBeInTheDocument();
      expect(screen.getAllByRole("button", { name: "Migrar alumnos" })).toHaveLength(2);
      expect(screen.getByRole("region", { name: "Migrar alumnos del sistema anterior" })).toBeInTheDocument();
      expect(screen.getByText("Datos del colegio")).toBeInTheDocument();
      expect(screen.getByText("Ciclo escolar")).toBeInTheDocument();
      expect(await screen.findByText("Calendario escolar")).toBeInTheDocument();
      expect(screen.getByText("Niveles educativos")).toBeInTheDocument();
      expect(await screen.findByRole("heading", { name: "Papelera" })).toBeInTheDocument();
      expect(statValue("Ciclos")).toBe("1");
    };

    await visibleOutsidePricing();

    await user.click(await screen.findByRole("button", { name: "Definir" }));
    expect(await screen.findByRole("heading", { name: "Precios de PRIMARIA · 2026-2027" })).toBeInTheDocument();

    expect(screen.queryByRole("heading", { name: "Configuración académica" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Volver al inicio" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Migrar alumnos" })).toBeNull();
    expect(screen.queryByRole("region", { name: "Migrar alumnos del sistema anterior" })).toBeNull();
    expect(screen.queryByText("Datos del colegio")).toBeNull();
    expect(screen.queryByText("Ciclo escolar")).toBeNull();
    expect(screen.queryByText("Calendario escolar")).toBeNull();
    expect(screen.queryByText("Niveles educativos")).toBeNull();
    expect(screen.queryByRole("heading", { name: "Papelera" })).toBeNull();
    expect(statValue("Ciclos")).toBeUndefined();

    await user.click(screen.getByRole("button", { name: "Cancelar" }));

    await visibleOutsidePricing();
    expect(screen.queryByRole("heading", { name: /^Precios de / })).toBeNull();
  });

  it("keeps the level selection across pricing (it lives in Catalog)", async () => {
    mocked.listCycles.mockResolvedValue([ACTIVE]);
    mocked.listLevels.mockResolvedValue([PRIMARIA]);
    const user = userEvent.setup();
    render(<Catalog canWriteConfig canReadTuition onBack={() => {}} />);

    await user.click(await screen.findByRole("button", { name: /^PRIMARIA · / }));
    await screen.findByRole("heading", { name: "Grados de PRIMARIA" });
    await user.click(await screen.findByRole("button", { name: "Definir" }));
    await screen.findByRole("heading", { name: /^Precios de / });
    await user.click(screen.getByRole("button", { name: "Cancelar" }));

    expect(await screen.findByRole("heading", { name: "Grados de PRIMARIA" })).toBeInTheDocument();
    expect(levelChip("PRIMARIA")).toHaveAttribute("aria-pressed", "true");
  });

  /*
   * Side effect of today's implementation (behavior.md §1 D6, item 23): entering
   * pricing UNMOUNTS the calendar, and an unmounting calendar commits its pending
   * removals at once instead of waiting out the undo window. A redesign that
   * keeps the calendar mounted would change this — on purpose or not.
   */
  it("a pending calendar removal is committed immediately when the tuition page opens", async () => {
    mocked.listCycles.mockResolvedValue([ACTIVE]);
    mocked.listLevels.mockResolvedValue([PRIMARIA]);
    mocked.listNonWorkingPeriods.mockResolvedValue([
      { id: "d1", cycleId: "c1", label: "INDEPENDENCIA", kind: "holiday", startsOn: "2026-09-16", endsOn: "2026-09-16" },
    ]);
    mocked.deleteNonWorkingPeriod.mockResolvedValue(undefined);
    const user = userEvent.setup();
    render(<Catalog canWriteConfig canReadTuition onBack={() => {}} />);

    await user.click(await screen.findByRole("button", { name: "Quitar INDEPENDENCIA" }));
    expect(mocked.deleteNonWorkingPeriod).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "Definir" }));

    await waitFor(() => expect(mocked.deleteNonWorkingPeriod).toHaveBeenCalledWith(expect.any(String), "d1", { keepalive: false }));
  });
});
