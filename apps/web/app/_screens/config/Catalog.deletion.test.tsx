import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as api from "../../config/api";
import * as financeApi from "../../finance/api";
import { Catalog } from "./Catalog";

vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  listLevels: vi.fn(),
  listGradeLevels: vi.fn(),
  listSubjects: vi.fn(),
  listGroups: vi.fn(),
  listTerms: vi.fn(),
  listNonWorkingPeriods: vi.fn(),
  listSchoolEvents: vi.fn(),
  getSchoolProfile: vi.fn(),
  listLevelTrash: vi.fn(),
  restoreLevel: vi.fn(),
  ConfigError: class ConfigError extends Error {},
}));
vi.mock("../../finance/api", () => ({ listTuitionPlans: vi.fn() }));

const mocked = vi.mocked(api);
const PRIMARIA = { id: "l1", name: "PRIMARIA", sepCode: "", enrollmentGranularity: "cycle" as const };

beforeEach(() => {
  mocked.listCycles.mockResolvedValue([]);
  mocked.listLevels.mockResolvedValue([PRIMARIA]);
  mocked.listGradeLevels.mockResolvedValue([]);
  mocked.listSubjects.mockResolvedValue([]);
  mocked.getSchoolProfile.mockResolvedValue({ profile: { name: "", legalName: "", rfc: "", cct: "", address: "", phone: "", legend: "", clabe: "" }, found: false });
  mocked.listLevelTrash.mockResolvedValue([]);
  vi.mocked(financeApi).listTuitionPlans.mockResolvedValue([]);
});
afterEach(() => vi.clearAllMocks());

describe("Catalog — deleting levels", () => {
  it("the owner gets the delete action on the selected level and the trash", async () => {
    const onDeleteLevel = vi.fn();
    const user = userEvent.setup();
    render(<Catalog onBack={vi.fn()} canDeleteLevels onDeleteLevel={onDeleteLevel} />);

    await user.click(await screen.findByRole("button", { name: /PRIMARIA/ }));
    await user.click(screen.getByRole("button", { name: "Eliminar nivel PRIMARIA" }));

    expect(onDeleteLevel).toHaveBeenCalledWith("l1");
    expect(screen.getByRole("heading", { name: "Papelera" })).toBeInTheDocument();
    expect(await screen.findByText("La papelera está vacía")).toBeInTheDocument();
  });

  it("anyone else sees neither", async () => {
    const user = userEvent.setup();
    render(<Catalog onBack={vi.fn()} />);

    await user.click(await screen.findByRole("button", { name: /PRIMARIA/ }));

    expect(screen.queryByRole("button", { name: /Eliminar nivel/ })).toBeNull();
    expect(screen.queryByRole("heading", { name: "Papelera" })).toBeNull();
    expect(mocked.listLevelTrash).not.toHaveBeenCalled();
  });

  it("shows the notice a finished deletion left behind", async () => {
    render(<Catalog onBack={vi.fn()} canDeleteLevels notice="Nivel eliminado. Puedes restaurarlo desde la Papelera hasta el 5 de octubre de 2026." />);
    expect(await screen.findByRole("status")).toHaveTextContent("Nivel eliminado. Puedes restaurarlo");
  });

  it("the notice leaves on its own after a while", () => {
    vi.useFakeTimers();
    try {
      render(<Catalog onBack={vi.fn()} notice="Nivel eliminado." />);
      expect(screen.getByRole("status")).toHaveTextContent("Nivel eliminado.");
      act(() => vi.advanceTimersByTime(8_000));
      expect(screen.queryByRole("status")).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it("a restored level comes back into the list", async () => {
    mocked.listLevelTrash.mockResolvedValue([{
      batchId: "b1", levelId: "l2", levelName: "KÍNDER", deletedAt: "2026-09-27T17:00:00Z", restorableUntil: "2099-10-04T17:00:00Z",
      deletedByName: "Luis", counts: { gradeLevels: 0, groups: 0, subjects: 0, terms: 0, teachingAssignments: 0, students: 0, enrollments: 0, attendanceMarks: 0, monthlyGrades: 0, reportCards: 0, tuitionPlans: 0, uniformItems: 0, schoolEvents: 0 }, status: "active",
    }]);
    mocked.restoreLevel.mockResolvedValue({ restored: {}, skipped: [] });
    const user = userEvent.setup();
    render(<Catalog onBack={vi.fn()} canDeleteLevels onDeleteLevel={vi.fn()} />);

    await user.click(await screen.findByRole("button", { name: "Restaurar KÍNDER" }));
    mocked.listLevels.mockResolvedValue([PRIMARIA, { ...PRIMARIA, id: "l2", name: "KÍNDER" }]);
    await user.click(screen.getByRole("button", { name: "Restaurar nivel" }));

    expect(await screen.findByRole("button", { name: /KÍNDER · por ciclo/ })).toBeInTheDocument();
  });
});
