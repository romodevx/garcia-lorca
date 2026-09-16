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
    await user.type(within(drawer).getByLabelText(/^Nombre del (ciclo|nivel)$/), "2026-2027");
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
      name: "PRIMARIA",
      sepCode: "PRI",
      enrollmentGranularity: "cycle",
    });
    mocked.createGradeLevel.mockResolvedValue({ id: "g1", levelId: "l1", name: "1° PRIMARIA", ordinal: 1 });

    render(<Catalog onBack={() => {}} />);

    // Create a level, in its drawer. "Por ciclo" comes pre-selected: it is what
    // nearly every level is, and a required choice with no default was one more
    // click on every single one.
    await user.click(await screen.findByRole("button", { name: "Nuevo nivel" }));
    const levelDrawer = await screen.findByRole("dialog", { name: "Nuevo nivel educativo" });
    await user.type(within(levelDrawer).getByLabelText(/^Nombre del (ciclo|nivel)$/), "Primaria");
    await user.type(within(levelDrawer).getByLabelText("Clave SEP"), "PRI");
    await user.click(within(levelDrawer).getByRole("button", { name: "Crear nivel" }));

    await waitFor(() =>
      expect(mocked.createLevel).toHaveBeenCalledWith(expect.any(String), {
        name: "PRIMARIA",
        sepCode: "PRI",
        enrollmentGranularity: "cycle",
      }),
    );

    // Creating the level selects it, which loads its (empty) grade levels and
    // reveals the grade-level form with the next ordinal (1) pre-filled.
    await waitFor(() => expect(mocked.listGradeLevels).toHaveBeenCalledWith(expect.any(String), "l1"));

    // El grado NO abre cajón: se captura en su propio renglón, con el orden que
    // sigue ya escrito, así que basta el nombre y Enter.
    await user.click(await screen.findByRole("button", { name: "Nuevo grado" }));
    await user.type(screen.getByLabelText("Nombre del grado"), "1° primaria{Enter}");

    await waitFor(() =>
      expect(mocked.createGradeLevel).toHaveBeenCalledWith(expect.any(String), {
        levelId: "l1",
        name: "1° PRIMARIA",
        ordinal: 1,
      }),
    );
    expect(screen.queryByRole("dialog")).toBeNull();
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
      { id: "l1", name: "PRIMARIA", sepCode: "PRI", enrollmentGranularity: "cycle" },
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
  it("marca en el propio campo lo que falta, en vez de apagar el botón", async () => {
    // Antes esto era una frase: "Escribe el nombre del ciclo y sus dos fechas".
    // Decía que faltaba algo sin decir CUÁL, y en una forma de varios campos eso
    // obliga a revisarlos todos. Ahora cada campo que falta se marca en sí mismo.
    const user = userEvent.setup();
    mocked.listCycles.mockResolvedValue([]);

    render(<Catalog onBack={() => {}} />);
    await user.click(await screen.findByRole("button", { name: "Nuevo ciclo" }));
    const drawer = await screen.findByRole("dialog", { name: "Nuevo ciclo escolar" });

    await user.click(within(drawer).getByRole("button", { name: "Crear ciclo" }));

    expect(within(drawer).getByLabelText(/^Nombre del (ciclo|nivel)$/)).toHaveAttribute("aria-invalid", "true");
    expect(within(drawer).getByLabelText("Inicia")).toHaveAttribute("aria-invalid", "true");
    expect(within(drawer).getByLabelText("Termina")).toHaveAttribute("aria-invalid", "true");
    expect(mocked.createCycle).not.toHaveBeenCalled();
  });
});
