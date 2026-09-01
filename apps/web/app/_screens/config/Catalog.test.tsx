import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as api from "../../config/api";
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

const mocked = vi.mocked(api);

beforeEach(() => {
  mocked.listCycles.mockResolvedValue([]);
  mocked.listLevels.mockResolvedValue([]);
  mocked.listGradeLevels.mockResolvedValue([]);
  mocked.listNonWorkingPeriods.mockResolvedValue([]);
  mocked.listSchoolEvents.mockResolvedValue([]);
  mocked.listSubjects.mockResolvedValue([]);
  mocked.listGroups.mockResolvedValue([]);
  mocked.listTerms.mockResolvedValue([]);
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

    // Initial load settles into the cycle form (no cycles yet).
    const nameInput = await screen.findByLabelText("Nombre del ciclo");
    await user.type(nameInput, "2026-2027");
    fireEvent.change(screen.getByLabelText("Inicio del ciclo"), { target: { value: "2026-08-01" } });
    fireEvent.change(screen.getByLabelText("Fin del ciclo"), { target: { value: "2027-06-30" } });

    await user.click(screen.getByRole("button", { name: "Crear ciclo" }));

    await waitFor(() =>
      expect(mocked.createCycle).toHaveBeenCalledWith(expect.any(String), {
        name: "2026-2027",
        startsOn: "2026-08-01",
        endsOn: "2027-06-30",
      }),
    );
    // The freshly created cycle now appears in the picker (as a selectable chip).
    // Scope to the chip button — the cycle name also appears as an <option> in
    // the assignments cycle dropdown, so a bare text query would be ambiguous.
    expect(await screen.findByRole("button", { name: /2026-2027/ })).toBeInTheDocument();
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
      name: "Primaria",
      sepCode: "PRI",
      enrollmentGranularity: "cycle",
    });
    mocked.createGradeLevel.mockResolvedValue({ id: "g1", levelId: "l1", name: "1° primaria", ordinal: 1 });

    render(<Catalog onBack={() => {}} />);

    // Create a level. Granularity is a required, defaultless choice.
    await user.type(await screen.findByLabelText("Nombre del nivel"), "Primaria");
    await user.type(screen.getByLabelText("Clave SEP"), "PRI");
    await user.click(screen.getByRole("radio", { name: "Por ciclo" }));
    await user.click(screen.getByRole("button", { name: "Crear nivel" }));

    await waitFor(() =>
      expect(mocked.createLevel).toHaveBeenCalledWith(expect.any(String), {
        name: "Primaria",
        sepCode: "PRI",
        enrollmentGranularity: "cycle",
      }),
    );

    // Creating the level selects it, which loads its (empty) grade levels and
    // reveals the grade-level form with the next ordinal (1) pre-filled.
    await waitFor(() => expect(mocked.listGradeLevels).toHaveBeenCalledWith(expect.any(String), "l1"));

    const gradeForm = await screen.findByRole("form", { name: "Agregar grado" });
    await user.type(within(gradeForm).getByLabelText("Nombre del grado"), "1° primaria");
    await user.click(within(gradeForm).getByRole("button", { name: "Crear grado" }));

    await waitFor(() =>
      expect(mocked.createGradeLevel).toHaveBeenCalledWith(expect.any(String), {
        levelId: "l1",
        name: "1° primaria",
        ordinal: 1,
      }),
    );
  });
});
