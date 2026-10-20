import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as api from "../../../config/api";
import type { Cycle, EvaluationPeriod, GradeLevel, Level } from "../../../config/api";
import { LevelPeriodsScreen } from "./LevelPeriodsScreen";

vi.mock("../../../config/api", () => {
  class ConfigError extends Error {
    readonly status: number;
    readonly detail: string;
    readonly code: string | undefined;
    constructor(status: number, detail = "", code?: string) {
      super(detail);
      this.status = status;
      this.detail = detail;
      this.code = code;
    }
  }
  return {
    ConfigError,
    listCycles: vi.fn(),
    listLevels: vi.fn(),
    listGradeLevels: vi.fn(),
    listEvaluationPeriods: vi.fn(),
    listTerms: vi.fn(),
    saveGradeEvaluationPeriods: vi.fn(),
    copyEvaluationPeriods: vi.fn(),
  };
});

const mocked = vi.mocked(api);

const CYCLE: Cycle = { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-24", endsOn: "2027-07-09" };
const OLD: Cycle = { id: "c0", name: "2025-2026", status: "closed", startsOn: "2025-08-25", endsOn: "2026-07-10" };
const LEVEL = { id: "l1", name: "PRIMARIA" } as Level;
const G1: GradeLevel = { id: "g1", levelId: "l1", name: "1° PRIMARIA", ordinal: 1 };
const G2: GradeLevel = { id: "g2", levelId: "l1", name: "2° PRIMARIA", ordinal: 2 };
const G3: GradeLevel = { id: "g3", levelId: "l1", name: "3° PRIMARIA", ordinal: 3 };

const period = (id: string, grade: string, ordinal: number, name: string, startsOn: string, endsOn: string): EvaluationPeriod => ({
  id, cycleId: "c1", gradeLevelId: grade, ordinal, name, startsOn, endsOn, termId: null,
});

let store: EvaluationPeriod[];

beforeEach(() => {
  store = [
    period("p1", "g1", 1, "1.ER TRIMESTRE", "2026-08-24", "2026-11-30"),
    period("p2", "g1", 2, "2.º TRIMESTRE", "2026-12-01", "2027-03-31"),
    period("q1", "g2", 1, "BIMESTRE 1", "2026-08-24", "2026-10-31"),
  ];
  mocked.listCycles.mockResolvedValue([OLD, CYCLE]);
  mocked.listLevels.mockResolvedValue([LEVEL]);
  mocked.listGradeLevels.mockResolvedValue([G3, G1, G2]);
  mocked.listTerms.mockResolvedValue([]);
  mocked.listEvaluationPeriods.mockImplementation(async (_b, scope) => store.filter((p) => p.cycleId === scope.cycleId));
  mocked.saveGradeEvaluationPeriods.mockImplementation(async (_b, grade, cycleId, inputs) =>
    inputs.map((p, i) => period(`s${i}`, grade, p.ordinal, p.name, p.startsOn, p.endsOn)).map((p) => ({ ...p, cycleId })),
  );
});

afterEach(() => {
  vi.clearAllMocks();
  vi.useRealTimers();
});

function renderScreen(canWrite = true): { onBack: ReturnType<typeof vi.fn> } {
  const onBack = vi.fn();
  render(<LevelPeriodsScreen levelId="l1" canWrite={canWrite} onBack={onBack} />);
  return { onBack };
}

const gradeBlock = (name: string): HTMLElement => screen.getByRole("region", { name: `Periodos de ${name}` });
const row = (grade: string, n: number): HTMLElement => within(gradeBlock(grade)).getByRole("group", { name: `Periodo ${n}` });
const field = (grade: string, n: number, label: "Nombre" | "Inicio" | "Fin"): HTMLInputElement =>
  within(row(grade, n)).getByLabelText(label) as HTMLInputElement;

describe("LevelPeriodsScreen — a new row proposes the level's own periods", () => {
  it("offers the level's semesters, not trimesters, for a grade with no periods", async () => {
    mocked.listTerms.mockResolvedValue([
      { id: "t1", cycleId: "c1", levelId: "l1", name: "1.ER SEMESTRE", ordinal: 1, startsOn: "2026-08-24", endsOn: "2027-01-22", months: [] },
      { id: "t2", cycleId: "c1", levelId: "l1", name: "2.º SEMESTRE", ordinal: 2, startsOn: "2027-01-25", endsOn: "2027-07-09", months: [] },
    ]);
    renderScreen();
    await screen.findByRole("region", { name: "Periodos de 3° PRIMARIA" });
    await waitFor(() => expect(mocked.listTerms).toHaveBeenCalledWith(expect.anything(), "c1", "l1"));
    const add = within(gradeBlock("3° PRIMARIA")).getByRole("button", { name: "Agregar periodo" });
    await userEvent.click(add);
    await userEvent.click(add);
    expect([field("3° PRIMARIA", 1, "Nombre").value, field("3° PRIMARIA", 1, "Fin").value]).toEqual(["1.ER SEMESTRE", "2027-01-22"]);
    expect([field("3° PRIMARIA", 2, "Nombre").value, field("3° PRIMARIA", 2, "Inicio").value]).toEqual(["2.º SEMESTRE", "2027-01-25"]);
  });

  it("still offers trimesters when the level's periods cannot be read", async () => {
    mocked.listTerms.mockRejectedValue(new Error("down"));
    renderScreen();
    await screen.findByRole("region", { name: "Periodos de 3° PRIMARIA" });
    await userEvent.click(within(gradeBlock("3° PRIMARIA")).getByRole("button", { name: "Agregar periodo" }));
    expect(field("3° PRIMARIA", 1, "Nombre").value).toBe("1.er trimestre");
    expect(screen.queryByRole("alert")).toBeNull();
  });
});

describe("LevelPeriodsScreen — a level's grades with their evaluation periods (T14)", () => {
  it("shows every grade of the level in school order with its own periods, for the active cycle", async () => {
    renderScreen();

    expect(await screen.findByRole("heading", { name: "Periodos de evaluación · PRIMARIA" })).toBeInTheDocument();
    await waitFor(() => expect(field("1° PRIMARIA", 1, "Nombre").value).toBe("1.ER TRIMESTRE"));
    expect(field("1° PRIMARIA", 2, "Fin").value).toBe("2027-03-31");
    expect(field("2° PRIMARIA", 1, "Inicio").value).toBe("2026-08-24");
    expect(within(gradeBlock("3° PRIMARIA")).getByText(/aún no tiene periodos/i)).toBeInTheDocument();
    expect(screen.getAllByRole("region").map((r) => r.getAttribute("aria-label"))).toEqual([
      "Periodos de 1° PRIMARIA", "Periodos de 2° PRIMARIA", "Periodos de 3° PRIMARIA",
    ]);
    expect(mocked.listEvaluationPeriods).toHaveBeenCalledWith(expect.anything(), { cycleId: "c1", levelId: "l1" });
  });

  it("checks a date when the field is left and clears the error once it is valid", async () => {
    renderScreen();
    await waitFor(() => expect(field("1° PRIMARIA", 1, "Fin").value).toBe("2026-11-30"));

    fireEvent.change(field("1° PRIMARIA", 1, "Fin"), { target: { value: "2026-08-01" } });
    expect(within(row("1° PRIMARIA", 1)).queryByText("El fin es antes que el inicio.")).toBeNull();
    fireEvent.blur(field("1° PRIMARIA", 1, "Fin"));
    expect(within(row("1° PRIMARIA", 1)).getByText("El fin es antes que el inicio.")).toBeInTheDocument();

    fireEvent.change(field("1° PRIMARIA", 1, "Fin"), { target: { value: "2026-11-30" } });
    expect(within(row("1° PRIMARIA", 1)).queryByText("El fin es antes que el inicio.")).toBeNull();
  });

  it("on save refuses an overlap without calling the server and focuses the offending field; the button stays enabled", async () => {
    renderScreen();
    await waitFor(() => expect(field("1° PRIMARIA", 2, "Inicio").value).toBe("2026-12-01"));

    fireEvent.change(field("1° PRIMARIA", 2, "Inicio"), { target: { value: "2026-11-15" } });
    const save = within(gradeBlock("1° PRIMARIA")).getByRole("button", { name: "Guardar periodos" });
    expect(save).toBeEnabled();
    await userEvent.click(save);

    expect(mocked.saveGradeEvaluationPeriods).not.toHaveBeenCalled();
    expect(within(row("1° PRIMARIA", 2)).getByText("Se encima con «1.ER TRIMESTRE» (termina el 30/11/2026).")).toBeInTheDocument();
    expect(field("1° PRIMARIA", 2, "Inicio")).toHaveFocus();
  });

  it("saves the grade's whole set, numbered in date order, with a new row prefilled after the last period", async () => {
    renderScreen();
    await waitFor(() => expect(field("1° PRIMARIA", 2, "Fin").value).toBe("2027-03-31"));

    await userEvent.click(within(gradeBlock("1° PRIMARIA")).getByRole("button", { name: "Agregar periodo" }));
    expect(field("1° PRIMARIA", 3, "Nombre").value).toBe("3.er trimestre");
    expect(field("1° PRIMARIA", 3, "Inicio").value).toBe("2027-04-01");
    expect(field("1° PRIMARIA", 3, "Fin").value).toBe("2027-07-09");
    expect(field("1° PRIMARIA", 3, "Nombre")).toHaveFocus();

    await userEvent.click(within(gradeBlock("1° PRIMARIA")).getByRole("button", { name: "Guardar periodos" }));

    await waitFor(() => expect(mocked.saveGradeEvaluationPeriods).toHaveBeenCalledTimes(1));
    expect(mocked.saveGradeEvaluationPeriods).toHaveBeenCalledWith(expect.anything(), "g1", "c1", [
      { id: "p1", ordinal: 1, name: "1.ER TRIMESTRE", startsOn: "2026-08-24", endsOn: "2026-11-30" },
      { id: "p2", ordinal: 2, name: "2.º TRIMESTRE", startsOn: "2026-12-01", endsOn: "2027-03-31" },
      { ordinal: 3, name: "3.er trimestre", startsOn: "2027-04-01", endsOn: "2027-07-09" },
    ]);
    expect(await screen.findByText("Periodos de 1° PRIMARIA guardados.")).toBeInTheDocument();
  });

  it("removes a period from the set before saving", async () => {
    renderScreen();
    await waitFor(() => expect(field("1° PRIMARIA", 2, "Fin").value).toBe("2027-03-31"));

    // Removing the FIRST period removes it: the second keeps its own id
    // although it becomes period 1.
    await userEvent.click(within(row("1° PRIMARIA", 1)).getByRole("button", { name: "Quitar periodo 1" }));
    await userEvent.click(within(gradeBlock("1° PRIMARIA")).getByRole("button", { name: "Guardar periodos" }));

    await waitFor(() => expect(mocked.saveGradeEvaluationPeriods).toHaveBeenCalledWith(expect.anything(), "g1", "c1", [
      { id: "p2", ordinal: 1, name: "2.º TRIMESTRE", startsOn: "2026-12-01", endsOn: "2027-03-31" },
    ]));
  });

  it("shows the server's refusal when a concurrent change wins", async () => {
    mocked.saveGradeEvaluationPeriods.mockRejectedValue(new api.ConfigError(409, "Los periodos de un grado no pueden encimarse", "PERIOD_OVERLAP"));
    renderScreen();
    await waitFor(() => expect(field("1° PRIMARIA", 1, "Nombre").value).toBe("1.ER TRIMESTRE"));

    await userEvent.click(within(gradeBlock("1° PRIMARIA")).getByRole("button", { name: "Guardar periodos" }));

    expect(await within(gradeBlock("1° PRIMARIA")).findByRole("alert")).toHaveTextContent("Los periodos de un grado no pueden encimarse");
  });

  it("says which period already holds grades when a save would remove it (T15)", async () => {
    const text = "«1.ER TRIMESTRE» ya tiene calificaciones; no se puede quitar";
    mocked.saveGradeEvaluationPeriods.mockRejectedValue(new api.ConfigError(409, text, "PERIOD_HAS_GRADES"));
    renderScreen();
    await waitFor(() => expect(field("1° PRIMARIA", 1, "Nombre").value).toBe("1.ER TRIMESTRE"));

    await userEvent.click(within(row("1° PRIMARIA", 1)).getByRole("button", { name: "Quitar periodo 1" }));
    await userEvent.click(within(gradeBlock("1° PRIMARIA")).getByRole("button", { name: "Guardar periodos" }));

    expect(await within(gradeBlock("1° PRIMARIA")).findByRole("alert")).toHaveTextContent(text);
  });

  it("says which target period already holds grades when a copy would remove it (T15)", async () => {
    const text = "«BIMESTRE 1» ya tiene calificaciones; no se puede quitar";
    mocked.copyEvaluationPeriods.mockRejectedValue(new api.ConfigError(409, text, "PERIOD_HAS_GRADES"));
    renderScreen();
    await waitFor(() => expect(field("1° PRIMARIA", 1, "Nombre").value).toBe("1.ER TRIMESTRE"));

    const block = gradeBlock("1° PRIMARIA");
    await userEvent.click(within(block).getByRole("button", { name: "Copiar a…" }));
    await userEvent.click(within(within(block).getByRole("group", { name: "Grados destino" })).getByRole("button", { name: "2° PRIMARIA" }));
    await userEvent.click(within(block).getByRole("button", { name: "Copiar periodos" }));

    expect(await within(gradeBlock("1° PRIMARIA")).findByRole("alert")).toHaveTextContent(text);
  });

  it("Deshacer says which period already holds grades when it cannot put the old ones back (T15)", async () => {
    const text = "«1.ER TRIMESTRE» ya tiene calificaciones; no se puede quitar";
    mocked.copyEvaluationPeriods.mockResolvedValue([period("r1", "g3", 1, "1.ER TRIMESTRE", "2026-08-24", "2026-11-30")]);
    renderScreen();
    await waitFor(() => expect(field("1° PRIMARIA", 1, "Nombre").value).toBe("1.ER TRIMESTRE"));
    const block = gradeBlock("1° PRIMARIA");
    await userEvent.click(within(block).getByRole("button", { name: "Copiar a…" }));
    await userEvent.click(within(within(block).getByRole("group", { name: "Grados destino" })).getByRole("button", { name: "3° PRIMARIA" }));
    await userEvent.click(within(block).getByRole("button", { name: "Copiar periodos" }));
    await waitFor(() => expect(field("3° PRIMARIA", 1, "Nombre").value).toBe("1.ER TRIMESTRE"));

    mocked.saveGradeEvaluationPeriods.mockRejectedValue(new api.ConfigError(409, text, "PERIOD_HAS_GRADES"));
    await userEvent.click(await screen.findByRole("button", { name: "Deshacer" }));

    expect(await within(gradeBlock("3° PRIMARIA")).findByRole("alert")).toHaveTextContent(text);
  });

  it("copies a grade's periods to the chosen grades, and Deshacer puts their previous periods back", async () => {
    mocked.copyEvaluationPeriods.mockResolvedValue([
      period("r1", "g3", 1, "1.ER TRIMESTRE", "2026-08-24", "2026-11-30"),
      period("r2", "g3", 2, "2.º TRIMESTRE", "2026-12-01", "2027-03-31"),
    ]);
    renderScreen();
    await waitFor(() => expect(field("1° PRIMARIA", 1, "Nombre").value).toBe("1.ER TRIMESTRE"));

    const block = gradeBlock("1° PRIMARIA");
    await userEvent.click(within(block).getByRole("button", { name: "Copiar a…" }));
    const targets = within(block).getByRole("group", { name: "Grados destino" });
    expect(within(targets).queryByRole("button", { name: "1° PRIMARIA" })).toBeNull();
    await userEvent.click(within(targets).getByRole("button", { name: "3° PRIMARIA" }));
    await userEvent.click(within(block).getByRole("button", { name: "Copiar periodos" }));

    await waitFor(() => expect(mocked.copyEvaluationPeriods).toHaveBeenCalledWith(expect.anything(), {
      cycleId: "c1", fromGradeLevelId: "g1", toGradeLevelIds: ["g3"],
    }));
    await waitFor(() => expect(field("3° PRIMARIA", 2, "Nombre").value).toBe("2.º TRIMESTRE"));

    await userEvent.click(await screen.findByRole("button", { name: "Deshacer" }));
    await waitFor(() => expect(mocked.saveGradeEvaluationPeriods).toHaveBeenCalledWith(expect.anything(), "g3", "c1", []));
    await waitFor(() => expect(within(gradeBlock("3° PRIMARIA")).getByText(/aún no tiene periodos/i)).toBeInTheDocument());
  });

  it("applies a grade's periods to the whole level in one call", async () => {
    mocked.copyEvaluationPeriods.mockResolvedValue([]);
    renderScreen();
    await waitFor(() => expect(field("1° PRIMARIA", 1, "Nombre").value).toBe("1.ER TRIMESTRE"));

    await userEvent.click(within(gradeBlock("1° PRIMARIA")).getByRole("button", { name: "Aplicar a todo el nivel" }));

    await waitFor(() => expect(mocked.copyEvaluationPeriods).toHaveBeenCalledWith(expect.anything(), {
      cycleId: "c1", fromGradeLevelId: "g1", toGradeLevelIds: ["g2", "g3"],
    }));
  });

  it("copying with no grade chosen says so instead of doing nothing", async () => {
    renderScreen();
    await waitFor(() => expect(field("1° PRIMARIA", 1, "Nombre").value).toBe("1.ER TRIMESTRE"));
    const block = gradeBlock("1° PRIMARIA");

    await userEvent.click(within(block).getByRole("button", { name: "Copiar a…" }));
    await userEvent.click(within(block).getByRole("button", { name: "Copiar periodos" }));

    expect(within(block).getByText("Elige al menos un grado.")).toBeInTheDocument();
    expect(mocked.copyEvaluationPeriods).not.toHaveBeenCalled();
  });

  // C2-001: the button used to refuse unsaved values; it now applies what is on screen.
  it("applies the unsaved values on screen: saves this grade first, then copies them to the level", async () => {
    mocked.copyEvaluationPeriods.mockResolvedValue([]);
    renderScreen();
    await waitFor(() => expect(field("1° PRIMARIA", 1, "Nombre").value).toBe("1.ER TRIMESTRE"));
    fireEvent.change(field("1° PRIMARIA", 1, "Fin"), { target: { value: "2026-11-27" } });

    await userEvent.click(within(gradeBlock("1° PRIMARIA")).getByRole("button", { name: "Aplicar a todo el nivel" }));

    await waitFor(() => expect(mocked.copyEvaluationPeriods).toHaveBeenCalledWith(expect.anything(), {
      cycleId: "c1", fromGradeLevelId: "g1", toGradeLevelIds: ["g2", "g3"],
    }));
    expect(mocked.saveGradeEvaluationPeriods).toHaveBeenCalledWith(expect.anything(), "g1", "c1", [
      expect.objectContaining({ name: "1.ER TRIMESTRE", endsOn: "2026-11-27" }),
      expect.objectContaining({ name: "2.º TRIMESTRE" }),
    ]);
    expect(mocked.saveGradeEvaluationPeriods.mock.invocationCallOrder[0]).toBeLessThan(
      mocked.copyEvaluationPeriods.mock.invocationCallOrder[0] ?? 0,
    );
    expect(await screen.findByRole("button", { name: "Deshacer" })).toBeInTheDocument();
  });

  it("unsaved values that are wrong are not applied: the field says why and gets the focus", async () => {
    renderScreen();
    await waitFor(() => expect(field("1° PRIMARIA", 1, "Nombre").value).toBe("1.ER TRIMESTRE"));
    fireEvent.change(field("1° PRIMARIA", 1, "Fin"), { target: { value: "2026-08-01" } });

    await userEvent.click(within(gradeBlock("1° PRIMARIA")).getByRole("button", { name: "Aplicar a todo el nivel" }));

    expect(await within(row("1° PRIMARIA", 1)).findByText("El fin es antes que el inicio.")).toBeInTheDocument();
    await waitFor(() => expect(field("1° PRIMARIA", 1, "Fin")).toHaveFocus());
    expect(mocked.saveGradeEvaluationPeriods).not.toHaveBeenCalled();
    expect(mocked.copyEvaluationPeriods).not.toHaveBeenCalled();
  });

  it("switching the cycle shows that cycle's periods", async () => {
    renderScreen();
    await waitFor(() => expect(field("1° PRIMARIA", 1, "Nombre").value).toBe("1.ER TRIMESTRE"));

    await userEvent.click(screen.getByRole("button", { name: "2025-2026" }));

    await waitFor(() => expect(mocked.listEvaluationPeriods).toHaveBeenLastCalledWith(expect.anything(), { cycleId: "c0", levelId: "l1" }));
    await waitFor(() => expect(within(gradeBlock("1° PRIMARIA")).getByText(/aún no tiene periodos/i)).toBeInTheDocument());
  });

  it("Reintentar reads the periods again after a failed load", async () => {
    mocked.listEvaluationPeriods.mockRejectedValueOnce(new Error("offline"));
    renderScreen();

    await userEvent.click(await screen.findByRole("button", { name: "Reintentar" }));

    await waitFor(() => expect(field("1° PRIMARIA", 1, "Nombre").value).toBe("1.ER TRIMESTRE"));
  });

  it("a save that answers after the cycle changed does not land in the new cycle", async () => {
    let answer: (v: EvaluationPeriod[]) => void = () => {};
    mocked.saveGradeEvaluationPeriods.mockImplementationOnce(() => new Promise((resolve) => { answer = resolve; }));
    renderScreen();
    await waitFor(() => expect(field("1° PRIMARIA", 1, "Nombre").value).toBe("1.ER TRIMESTRE"));

    await userEvent.click(within(gradeBlock("1° PRIMARIA")).getByRole("button", { name: "Guardar periodos" }));
    await userEvent.click(screen.getByRole("button", { name: "2025-2026" }));
    await waitFor(() => expect(within(gradeBlock("1° PRIMARIA")).getByText(/aún no tiene periodos/i)).toBeInTheDocument());
    await act(async () => answer([period("p1", "g1", 1, "1.ER TRIMESTRE", "2026-08-24", "2026-11-30")]));

    expect(within(gradeBlock("1° PRIMARIA")).getByText(/aún no tiene periodos/i)).toBeInTheDocument();
  });

  it("without config:write the periods are read-only", async () => {
    renderScreen(false);
    await waitFor(() => expect(field("1° PRIMARIA", 1, "Nombre").value).toBe("1.ER TRIMESTRE"));

    expect(field("1° PRIMARIA", 1, "Nombre")).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Guardar periodos" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Copiar a…" })).toBeNull();
  });

  it("goes back to Configuración", async () => {
    const { onBack } = renderScreen();
    await userEvent.click(await screen.findByRole("button", { name: /volver a configuración/i }));
    expect(onBack).toHaveBeenCalled();
  });

  // ISSUE-CC-105: the undo offer never leaves on its own; the person closes it.
  it("the undo offer stays until the person closes it", async () => {
    mocked.copyEvaluationPeriods.mockResolvedValue([]);
    renderScreen();
    await waitFor(() => expect(field("1° PRIMARIA", 1, "Nombre").value).toBe("1.ER TRIMESTRE"));
    // Only the undo's own clock is faked: the load above ran on real time.
    vi.useFakeTimers({ shouldAdvanceTime: true });

    fireEvent.click(within(gradeBlock("1° PRIMARIA")).getByRole("button", { name: "Aplicar a todo el nivel" }));
    expect(await screen.findByRole("button", { name: "Deshacer" })).toBeInTheDocument();
    await act(async () => {}); // the effect that starts the clock has run
    await act(async () => {
      vi.advanceTimersByTime(60_000);
    });
    expect(screen.getByRole("button", { name: "Deshacer" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Cerrar aviso" }));
    expect(screen.queryByRole("button", { name: "Deshacer" })).toBeNull();
  });
});
