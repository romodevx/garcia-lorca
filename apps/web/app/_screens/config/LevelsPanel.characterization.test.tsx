import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Level } from "../../config/api";
import { LevelsPanel, type LevelsPanelProps } from "./LevelsPanel";

/*
 * Characterization tests (2026-10-09) for "Niveles educativos", frozen before
 * the Configuración académica redesign (behavior.md §7: S7.1, chip label,
 * A39 validation, S7.2, "Por periodo" → term; §7a: same mode makes no call).
 */

const PRIMARIA: Level = { id: "l1", name: "PRIMARIA", sepCode: "", enrollmentGranularity: "cycle", attendanceMode: "daily" };
const PREPA: Level = { id: "l2", name: "PREPARATORIA", sepCode: "", enrollmentGranularity: "term" };

function setup(overrides: Partial<LevelsPanelProps> = {}): LevelsPanelProps {
  const props: LevelsPanelProps = {
    levels: [PRIMARIA, PREPA],
    selectedLevelId: undefined,
    onSelect: vi.fn(),
    onCreate: vi.fn().mockResolvedValue(undefined),
    onSetAttendanceMode: vi.fn().mockResolvedValue(undefined),
    onSetGrading: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
  render(<LevelsPanel {...props} />);
  return props;
}

afterEach(() => vi.clearAllMocks());

describe("LevelsPanel — characterization", () => {
  it("says there are no levels yet", () => {
    setup({ levels: [] });

    expect(screen.getByText("Aún no hay niveles. Agrega el primero.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Nuevo nivel" })).toBeInTheDocument();
  });

  it("labels each chip '{name} · por ciclo|por periodo' and hands the id up on click", async () => {
    const user = userEvent.setup();
    const props = setup();

    expect(screen.getByRole("button", { name: "PRIMARIA · por ciclo" })).toHaveAttribute("aria-pressed", "false");
    await user.click(screen.getByRole("button", { name: "PREPARATORIA · por periodo" }));

    expect(props.onSelect).toHaveBeenCalledWith("l2");
  });

  it("a level with no name is refused in the drawer, without calling the server", async () => {
    const user = userEvent.setup();
    const props = setup();

    await user.click(screen.getByRole("button", { name: "Nuevo nivel" }));
    await user.click(await screen.findByRole("button", { name: "Crear nivel" }));

    expect(await screen.findByText("Un nivel necesita nombre.")).toBeInTheDocument();
    expect(props.onCreate).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog", { name: "Nuevo nivel educativo" })).toBeInTheDocument();
  });

  it("'Por periodo' is sent as enrollmentGranularity 'term'", async () => {
    const user = userEvent.setup();
    const props = setup();

    await user.click(screen.getByRole("button", { name: "Nuevo nivel" }));
    await user.type(await screen.findByLabelText(/^Nombre del nivel/), "Universidad");
    await user.click(screen.getByRole("radio", { name: "Por periodo" }));
    await user.click(screen.getByRole("button", { name: "Crear nivel" }));

    await waitFor(() =>
      expect(props.onCreate).toHaveBeenCalledWith({ name: "UNIVERSIDAD", sepCode: "", enrollmentGranularity: "term" }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("a refused create says the name may already exist and keeps the drawer open", async () => {
    const user = userEvent.setup();
    setup({ onCreate: vi.fn().mockRejectedValue(new Error("conflict")) });

    await user.click(screen.getByRole("button", { name: "Nuevo nivel" }));
    await user.type(await screen.findByLabelText(/^Nombre del nivel/), "Primaria");
    await user.click(screen.getByRole("button", { name: "Crear nivel" }));

    expect(await screen.findByText("No se pudo crear el nivel. Revisa que no exista ya con ese nombre.")).toBeInTheDocument();
    expect(screen.getByRole("dialog", { name: "Nuevo nivel educativo" })).toBeInTheDocument();
  });

  it("choosing the attendance mode already in force makes no call", async () => {
    const user = userEvent.setup();
    const props = setup({ selectedLevelId: "l1" });

    await user.click(screen.getByRole("radio", { name: "Por día" }));

    expect(props.onSetAttendanceMode).not.toHaveBeenCalled();
  });

  it("shows no delete action unless onDelete is given", () => {
    setup({ selectedLevelId: "l1" });

    expect(screen.queryByRole("button", { name: /Eliminar nivel/ })).toBeNull();
  });
});
