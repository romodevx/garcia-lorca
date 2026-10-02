import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as api from "../../config/api";
import type { GradeLevel, Level, Subject } from "../../config/api";
import { SubjectsPanel } from "./SubjectsPanel";

vi.mock("../../config/api", () => {
  class ConfigError extends Error {
    readonly status: number;
    constructor(status: number, detail = "") {
      super(detail);
      this.status = status;
    }
    get isConflict(): boolean {
      return this.status === 409;
    }
  }
  return {
    ConfigError,
    listSubjects: vi.fn(),
    createSubjectInGrades: vi.fn(),
    createSubject: vi.fn(),
    renameSubject: vi.fn(),
    deleteSubject: vi.fn(),
  };
});

const mocked = vi.mocked(api);

const LEVEL: Level = { id: "l1", name: "PRIMARIA" } as Level;
const G1: GradeLevel = { id: "g1", levelId: "l1", name: "1° PRIMARIA", ordinal: 1 };
const G2: GradeLevel = { id: "g2", levelId: "l1", name: "2° PRIMARIA", ordinal: 2 };
const G3: GradeLevel = { id: "g3", levelId: "l1", name: "3° PRIMARIA", ordinal: 3 };
const GRADES = [G2, G1, G3];

const sub = (id: string, gradeLevelId: string, name: string): Subject => ({ id, gradeLevelId, name });

let store: Subject[];

beforeEach(() => {
  store = [sub("s1", "g1", "ESPAÑOL"), sub("s2", "g1", "MATEMÁTICAS"), sub("s3", "g2", "MATEMÁTICAS")];
  mocked.listSubjects.mockImplementation(async (_base, gradeLevelId) =>
    gradeLevelId === "" ? [...store, sub("x9", "other-level-grade", "AJENA")] : store.filter((s) => s.gradeLevelId === gradeLevelId),
  );
});

afterEach(() => {
  vi.clearAllMocks();
  vi.useRealTimers();
});

function renderPanel(selectedGradeLevelId?: string): void {
  render(<SubjectsPanel level={LEVEL} gradeLevels={GRADES} selectedGradeLevelId={selectedGradeLevelId} />);
}

const nameField = (): HTMLElement => screen.getByRole("textbox", { name: /nombre de la materia/i });
const gradeBlock = (name: string): HTMLElement => screen.getByRole("region", { name: `Materias de ${name}` });

describe("SubjectsPanel — the level's subjects, grade by grade (T13)", () => {
  it("shows every grade of the level in school order, each with its own subjects", async () => {
    renderPanel();

    expect(await within(gradeBlock("1° PRIMARIA")).findByText("ESPAÑOL")).toBeInTheDocument();
    expect(within(gradeBlock("1° PRIMARIA")).getByText("MATEMÁTICAS")).toBeInTheDocument();
    expect(within(gradeBlock("2° PRIMARIA")).getByText("MATEMÁTICAS")).toBeInTheDocument();
    expect(within(gradeBlock("2° PRIMARIA")).queryByText("ESPAÑOL")).toBeNull();
    expect(within(gradeBlock("3° PRIMARIA")).getByText(/aún no tiene materias/i)).toBeInTheDocument();

    const order = screen.getAllByRole("region").map((r) => r.getAttribute("aria-label"));
    expect(order).toEqual(["Materias de 1° PRIMARIA", "Materias de 2° PRIMARIA", "Materias de 3° PRIMARIA"]);
  });

  it("creates the subject in several grades at once — one row per grade — and says where it already was", async () => {
    mocked.createSubjectInGrades.mockResolvedValue({
      created: [sub("n1", "g1", "ARTES"), sub("n3", "g3", "ARTES")],
      existing: ["g2"],
      failed: [],
    });
    renderPanel();
    await screen.findAllByText("MATEMÁTICAS");

    await userEvent.type(nameField(), "artes");
    // Every grade starts chosen when none is selected on the page.
    for (const g of GRADES) expect(screen.getByRole("button", { name: g.name, pressed: true })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /agregar materia/i }));

    expect(mocked.createSubjectInGrades).toHaveBeenCalledWith(expect.any(String), "ARTES", ["g1", "g2", "g3"]);
    expect(await within(gradeBlock("1° PRIMARIA")).findByText("ARTES")).toBeInTheDocument();
    expect(within(gradeBlock("3° PRIMARIA")).getByText("ARTES")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent(/ARTES se agregó a 1° PRIMARIA y 3° PRIMARIA/);
    expect(screen.getByRole("status")).toHaveTextContent(/2° PRIMARIA ya la tenía/);
    // Save-and-create-another: the name clears and takes the focus; the grades stay.
    expect(nameField()).toHaveValue("");
    expect(nameField()).toHaveFocus();
    expect(screen.getByRole("button", { name: "2° PRIMARIA", pressed: true })).toBeInTheDocument();
  });

  it("starts with only the grade chosen on the page, and toggles grades and 'Todos'", async () => {
    mocked.createSubjectInGrades.mockResolvedValue({ created: [sub("n2", "g2", "INGLÉS")], existing: [], failed: [] });
    renderPanel("g2");
    await screen.findAllByText("MATEMÁTICAS");

    expect(screen.getByRole("button", { name: "2° PRIMARIA", pressed: true })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "1° PRIMARIA", pressed: false })).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Todos los grados" }));
    for (const g of GRADES) expect(screen.getByRole("button", { name: g.name, pressed: true })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Todos los grados" }));
    for (const g of GRADES) expect(screen.getByRole("button", { name: g.name, pressed: false })).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "2° PRIMARIA" }));
    await userEvent.type(nameField(), "inglés{Enter}");
    expect(mocked.createSubjectInGrades).toHaveBeenCalledWith(expect.any(String), "INGLÉS", ["g2"]);
  });

  it("the default grades follow grades that arrive after the panel (the catalog loads them late)", async () => {
    const { rerender } = render(<SubjectsPanel level={LEVEL} gradeLevels={[]} />);
    rerender(<SubjectsPanel level={LEVEL} gradeLevels={GRADES} />);
    await screen.findAllByText("MATEMÁTICAS");
    for (const g of GRADES) expect(screen.getByRole("button", { name: g.name, pressed: true })).toBeInTheDocument();
  });

  it("never sends without a name or a grade: marks the field and focuses it", async () => {
    renderPanel();
    await screen.findAllByText("MATEMÁTICAS");

    await userEvent.click(screen.getByRole("button", { name: /agregar materia/i }));
    expect(nameField()).toHaveAttribute("aria-invalid", "true");
    await waitFor(() => expect(nameField()).toHaveFocus());

    await userEvent.type(nameField(), "artes");
    await userEvent.click(screen.getByRole("button", { name: "Todos los grados" }));
    await userEvent.click(screen.getByRole("button", { name: /agregar materia/i }));
    expect(screen.getByText(/elige al menos un grado/i)).toBeInTheDocument();
    expect(mocked.createSubjectInGrades).not.toHaveBeenCalled();
  });

  it("renames a subject in its place; a name the grade already has is refused with the reason", async () => {
    mocked.renameSubject.mockResolvedValueOnce(sub("s2", "g1", "PENSAMIENTO MATEMÁTICO"));
    renderPanel();
    const block = await screen.findByRole("region", { name: "Materias de 1° PRIMARIA" });
    await within(block).findByText("MATEMÁTICAS");

    await userEvent.click(within(block).getByRole("button", { name: "Renombrar MATEMÁTICAS" }));
    const input = within(block).getByLabelText("Nuevo nombre de MATEMÁTICAS");
    expect(input).toHaveFocus();
    await userEvent.clear(input);
    await userEvent.type(input, "pensamiento matemático{Enter}");

    expect(mocked.renameSubject).toHaveBeenCalledWith(expect.any(String), "s2", "PENSAMIENTO MATEMÁTICO");
    expect(await within(block).findByText("PENSAMIENTO MATEMÁTICO")).toBeInTheDocument();
    // Only that grade's row changed.
    expect(within(gradeBlock("2° PRIMARIA")).getByText("MATEMÁTICAS")).toBeInTheDocument();

    mocked.renameSubject.mockRejectedValueOnce(new api.ConfigError(409, "exists"));
    await userEvent.click(within(block).getByRole("button", { name: "Renombrar ESPAÑOL" }));
    const again = within(block).getByLabelText("Nuevo nombre de ESPAÑOL");
    await userEvent.clear(again);
    await userEvent.type(again, "pensamiento matemático{Enter}");
    expect(await screen.findByRole("alert")).toHaveTextContent(/1° PRIMARIA ya tiene una materia con ese nombre/);
    expect(within(block).getByLabelText("Nuevo nombre de ESPAÑOL")).toHaveValue("PENSAMIENTO MATEMÁTICO");
  });

  it("Escape cancels a rename without sending it", async () => {
    renderPanel();
    const block = await screen.findByRole("region", { name: "Materias de 1° PRIMARIA" });
    await within(block).findByText("ESPAÑOL");
    await userEvent.click(within(block).getByRole("button", { name: "Renombrar ESPAÑOL" }));
    await userEvent.type(within(block).getByLabelText("Nuevo nombre de ESPAÑOL"), "X{Escape}");
    expect(within(block).getByText("ESPAÑOL")).toBeInTheDocument();
    expect(mocked.renameSubject).not.toHaveBeenCalled();
  });

  it("reads the level's subjects in ONE request and shows only its grades' rows", async () => {
    renderPanel();
    await screen.findAllByText("MATEMÁTICAS");
    expect(mocked.listSubjects).toHaveBeenCalledTimes(1);
    expect(screen.queryByText("AJENA")).toBeNull();
  });

  it("deletes at once and offers to undo, which puts the subject back in its grade", async () => {
    mocked.deleteSubject.mockResolvedValue(undefined);
    mocked.createSubject.mockResolvedValue(sub("s1b", "g1", "ESPAÑOL"));
    renderPanel();
    const block = await screen.findByRole("region", { name: "Materias de 1° PRIMARIA" });
    await within(block).findByText("ESPAÑOL");

    await userEvent.click(within(block).getByRole("button", { name: "Eliminar ESPAÑOL" }));
    expect(mocked.deleteSubject).toHaveBeenCalledWith(expect.any(String), "s1");
    await waitFor(() => expect(within(block).queryByText("ESPAÑOL")).toBeNull());

    await userEvent.click(screen.getByRole("button", { name: /deshacer/i }));
    expect(mocked.createSubject).toHaveBeenCalledWith(expect.any(String), { gradeLevelId: "g1", name: "ESPAÑOL" });
    expect(await within(block).findByText("ESPAÑOL")).toBeInTheDocument();
  });

  it("the undo offer leaves on its own after eight seconds", async () => {
    mocked.deleteSubject.mockResolvedValue(undefined);
    renderPanel();
    const block = await screen.findByRole("region", { name: "Materias de 1° PRIMARIA" });
    await within(block).findByText("ESPAÑOL");
    vi.useFakeTimers({ shouldAdvanceTime: true });

    fireEvent.click(within(block).getByRole("button", { name: "Eliminar ESPAÑOL" }));
    await act(async () => {
      await Promise.resolve();
    });
    expect(screen.getByRole("button", { name: /deshacer/i })).toBeInTheDocument();
    await act(async () => {
      vi.advanceTimersByTime(8_100);
    });
    expect(screen.queryByRole("button", { name: /deshacer/i })).toBeNull();
  });

  it("a subject that was ever in a teacher's load stays, with the reason", async () => {
    mocked.deleteSubject.mockRejectedValue(new api.ConfigError(409, "used"));
    renderPanel();
    const block = await screen.findByRole("region", { name: "Materias de 1° PRIMARIA" });
    await within(block).findByText("ESPAÑOL");

    await userEvent.click(within(block).getByRole("button", { name: "Eliminar ESPAÑOL" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/ESPAÑOL ya estuvo en una carga académica/);
    expect(within(block).getByText("ESPAÑOL")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /deshacer/i })).toBeNull();
  });
});
