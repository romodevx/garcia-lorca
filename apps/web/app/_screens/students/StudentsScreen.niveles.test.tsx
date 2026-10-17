import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import * as configApi from "../../config/api";
import { StudentsScreen } from "./StudentsScreen";

// Regression: ISSUE-T14-017 — the "Niveles" card said "Kínder a Preparatoria"
// whatever the school configured (its first level was Maternal).
// Report: .gstack/qa-reports/qa-report-t14-faseA-2026-10-17.md

vi.mock("../../students/api", async (importOriginal) => ({
  ...(await importOriginal<typeof studentsApi>()),
  listStudents: vi.fn(),
  listEnrollments: vi.fn(),
}));
vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  listGroups: vi.fn(),
  listLabelledGroups: vi.fn(),
  listLevels: vi.fn(),
  listGradeLevels: vi.fn(),
}));

const students = vi.mocked(studentsApi);
const config = vi.mocked(configApi);

const level = (id: string, name: string, sortOrder: number): configApi.Level => ({
  id, name, sepCode: id, enrollmentGranularity: "cycle", sortOrder,
});

beforeEach(() => {
  window.history.replaceState(null, "", "/alumnos");
  students.listStudents.mockResolvedValue([]);
  students.listEnrollments.mockResolvedValue([]);
  config.listCycles.mockResolvedValue([]);
  config.listGroups.mockResolvedValue([]);
  config.listLabelledGroups.mockResolvedValue([]);
  config.listGradeLevels.mockResolvedValue([]);
});
afterEach(() => vi.clearAllMocks());

/** The line under the card's count: the card is label, count, then that line. */
async function cardLine(): Promise<string> {
  const label = await screen.findByText("Niveles", { selector: "div" });
  return label.parentElement?.children[2]?.textContent ?? "";
}

describe("la tarjeta Niveles", () => {
  it("nombra el primer y el último nivel configurados, en el orden de la escuela", async () => {
    config.listLevels.mockResolvedValue([level("lv-b", "Bachillerato", 4), level("lv-m", "Maternal", 1), level("lv-p", "Primaria", 2)]);
    render(<StudentsScreen onBack={() => {}} />);

    await waitFor(async () => expect(await cardLine()).toBe("Maternal a Bachillerato"));
  });

  it("con un solo nivel dice sólo ese", async () => {
    config.listLevels.mockResolvedValue([level("lv-p", "Primaria", 1)]);
    render(<StudentsScreen onBack={() => {}} />);

    await waitFor(async () => expect(await cardLine()).toBe("Primaria"));
  });

  it("sin niveles no inventa un rango", async () => {
    config.listLevels.mockResolvedValue([]);
    render(<StudentsScreen onBack={() => {}} />);

    await screen.findByText("Niveles", { selector: "div" });
    await waitFor(() => expect(config.listLevels).toHaveBeenCalled());
    expect(await cardLine()).toBe("");
  });
});
