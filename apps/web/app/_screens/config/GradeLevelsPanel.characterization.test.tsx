import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { GradeLevel, Level } from "../../config/api";
import { GradeLevelsPanel, type GradeLevelsPanelProps } from "./GradeLevelsPanel";

/*
 * Characterization tests (2026-10-09) for the grades panel, frozen before the
 * Configuración académica redesign (behavior.md §8, A41/A42, S8.1/S8.2).
 */

const PRIMARIA: Level = { id: "l1", name: "PRIMARIA", sepCode: "", enrollmentGranularity: "cycle" };
const grade = (id: string, ordinal: number, name: string): GradeLevel => ({ id, levelId: "l1", name, ordinal });

function setup(overrides: Partial<GradeLevelsPanelProps> = {}): GradeLevelsPanelProps {
  const props: GradeLevelsPanelProps = {
    level: PRIMARIA,
    gradeLevels: [grade("g1", 1, "1° PRIMARIA"), grade("g3", 3, "3° PRIMARIA")],
    selectedGradeLevelId: undefined,
    onSelect: vi.fn(),
    onCreate: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
  render(<GradeLevelsPanel {...props} />);
  return props;
}

afterEach(() => vi.clearAllMocks());

describe("GradeLevelsPanel", () => {
  it("is titled after the level and lists its grades as '{ordinal}. {name}' chips, marking the selected one", () => {
    setup({ selectedGradeLevelId: "g3" });

    expect(screen.getByRole("heading", { name: "Grados de PRIMARIA" })).toBeInTheDocument();
    expect(screen.getByText("Elige uno para sus materias y grupos, o agrega otro.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "1. 1° PRIMARIA" })).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByRole("button", { name: "3. 3° PRIMARIA" })).toHaveAttribute("aria-pressed", "true");
  });

  it("selecting a grade hands its id up", async () => {
    const user = userEvent.setup();
    const props = setup();

    await user.click(screen.getByRole("button", { name: "1. 1° PRIMARIA" }));

    expect(props.onSelect).toHaveBeenCalledWith("g1");
  });

  it("says so when the level has no grades yet", () => {
    setup({ gradeLevels: [] });

    expect(screen.getByText("Aún no hay grados en este nivel.")).toBeInTheDocument();
  });

  it("opens no form until asked, then proposes the next order (max + 1) and Enter creates the grade", async () => {
    const user = userEvent.setup();
    const props = setup();
    expect(screen.queryByLabelText("Nombre del grado")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Nuevo grado" }));
    expect(screen.getByLabelText("Orden")).toHaveValue(4);
    await user.type(screen.getByLabelText("Nombre del grado"), "4° primaria{Enter}");

    await waitFor(() => expect(props.onCreate).toHaveBeenCalledWith({ levelId: "l1", name: "4° PRIMARIA", ordinal: 4 }));
    // No dialog: it is an inline row.
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("proposes order 1 for an empty level", async () => {
    const user = userEvent.setup();
    setup({ gradeLevels: [] });

    await user.click(screen.getByRole("button", { name: "Nuevo grado" }));

    expect(screen.getByLabelText("Orden")).toHaveValue(1);
  });

  it("refuses an order below 1 without calling the server", async () => {
    const user = userEvent.setup();
    const props = setup();

    await user.click(screen.getByRole("button", { name: "Nuevo grado" }));
    await user.clear(screen.getByLabelText("Orden"));
    await user.type(screen.getByLabelText("Orden"), "0");
    await user.type(screen.getByLabelText("Nombre del grado"), "Cero{Enter}");

    expect(await screen.findAllByText("El orden del grado tiene que ser un número mayor que cero.")).not.toHaveLength(0);
    expect(props.onCreate).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Orden")).toHaveAccessibleDescription(/mayor que cero/);
  });

  it("a refused create names both reasons and keeps what was typed", async () => {
    const user = userEvent.setup();
    setup({ onCreate: vi.fn().mockRejectedValue(new Error("conflict")) });

    await user.click(screen.getByRole("button", { name: "Nuevo grado" }));
    await user.type(screen.getByLabelText("Nombre del grado"), "1° primaria{Enter}");

    expect(
      await screen.findByText("No se pudo crear el grado. Revisa que no exista ya en este nivel con ese nombre o ese orden."),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Nombre del grado")).toHaveValue("1° PRIMARIA");
  });
});
