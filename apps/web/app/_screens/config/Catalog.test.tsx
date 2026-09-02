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

    // Creating is a drawer now — the same one a new pupil opens — so the screen
    // shows a choice OR a form, never both at once.
    await user.click(await screen.findByRole("button", { name: "Nuevo ciclo" }));
    const drawer = await screen.findByRole("dialog", { name: "Nuevo ciclo escolar" });
    await user.type(within(drawer).getByLabelText("Nombre"), "2026-2027");
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
      name: "Primaria",
      sepCode: "PRI",
      enrollmentGranularity: "cycle",
    });
    mocked.createGradeLevel.mockResolvedValue({ id: "g1", levelId: "l1", name: "1° primaria", ordinal: 1 });

    render(<Catalog onBack={() => {}} />);

    // Create a level, in its drawer. "Por ciclo" comes pre-selected: it is what
    // nearly every level is, and a required choice with no default was one more
    // click on every single one.
    await user.click(await screen.findByRole("button", { name: "Nuevo nivel" }));
    const levelDrawer = await screen.findByRole("dialog", { name: "Nuevo nivel educativo" });
    await user.type(within(levelDrawer).getByLabelText("Nombre"), "Primaria");
    await user.type(within(levelDrawer).getByLabelText("Clave SEP"), "PRI");
    await user.click(within(levelDrawer).getByRole("button", { name: "Crear nivel" }));

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

    await user.click(await screen.findByRole("button", { name: "Nuevo grado" }));
    const gradeDrawer = await screen.findByRole("dialog", { name: /Nuevo grado en Primaria/ });
    await user.type(within(gradeDrawer).getByLabelText("Nombre del grado"), "1° primaria");
    await user.click(within(gradeDrawer).getByRole("button", { name: "Crear grado" }));

    await waitFor(() =>
      expect(mocked.createGradeLevel).toHaveBeenCalledWith(expect.any(String), {
        levelId: "l1",
        name: "1° primaria",
        ordinal: 1,
      }),
    );
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
      { id: "l1", name: "Primaria", sepCode: "PRI", enrollmentGranularity: "cycle" },
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
  it("says what is missing rather than disabling the button", async () => {
    const user = userEvent.setup();
    mocked.listCycles.mockResolvedValue([]);

    render(<Catalog onBack={() => {}} />);
    await user.click(await screen.findByRole("button", { name: "Nuevo ciclo" }));
    const drawer = await screen.findByRole("dialog", { name: "Nuevo ciclo escolar" });

    await user.click(within(drawer).getByRole("button", { name: "Crear ciclo" }));

    expect(await within(drawer).findByRole("alert")).toHaveTextContent(/nombre del ciclo y sus dos fechas/i);
    expect(mocked.createCycle).not.toHaveBeenCalled();
  });
});
