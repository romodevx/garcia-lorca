import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Level } from "../../config/api";
import { LevelsPanel } from "./LevelsPanel";

const primaria: Level = { id: "l1", name: "PRIMARIA", sepCode: "", enrollmentGranularity: "cycle", attendanceMode: "daily" };
const secundaria: Level = { id: "l2", name: "SECUNDARIA", sepCode: "", enrollmentGranularity: "cycle", attendanceMode: "per_class" };

afterEach(() => vi.clearAllMocks());

function renderPanel(
  selectedLevelId: string | undefined,
  onSetAttendanceMode = vi.fn().mockResolvedValue(undefined),
  onSetGrading = vi.fn().mockResolvedValue(undefined),
) {
  render(
    <LevelsPanel
      levels={[primaria, secundaria, { ...primaria, id: "l3", name: "KINDER", attendanceMode: undefined }]}
      selectedLevelId={selectedLevelId}
      onSelect={() => {}}
      onCreate={vi.fn()}
      onSetAttendanceMode={onSetAttendanceMode}
      onSetGrading={onSetGrading}
    />,
  );
  return onSetAttendanceMode;
}

describe("LevelsPanel — attendance mode", () => {
  it("shows the selected level's current mode", () => {
    renderPanel("l2");
    expect(screen.getByRole("group", { name: "Asistencia de SECUNDARIA" })).toBeInTheDocument();
    expect(screen.getByRole("radio", { name: "Por clase" })).toBeChecked();
    expect(screen.getByRole("radio", { name: "Por día" })).not.toBeChecked();
  });

  it("a level that does not say takes attendance by day", () => {
    renderPanel("l3");
    expect(screen.getByRole("radio", { name: "Por día" })).toBeChecked();
  });

  it("saves the new mode and confirms with a toast", async () => {
    const user = userEvent.setup();
    const save = renderPanel("l1");

    await user.click(screen.getByRole("radio", { name: "Por clase" }));

    expect(save).toHaveBeenCalledWith("l1", "per_class");
    expect(await screen.findByRole("status")).toHaveTextContent("PRIMARIA: asistencia por clase");
  });

  it("says so when the save fails", async () => {
    const user = userEvent.setup();
    renderPanel("l1", vi.fn().mockRejectedValue(new Error("nivel no encontrado")));

    await user.click(screen.getByRole("radio", { name: "Por clase" }));

    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("No se pudo cambiar la asistencia"));
  });

  it("offers nothing until a level is chosen", () => {
    renderPanel(undefined);
    expect(screen.queryByRole("radio", { name: "Por clase" })).toBeNull();
  });
});

describe("LevelsPanel — grading", () => {
  it("offers the selected level's numeric grading under its attendance", async () => {
    const user = userEvent.setup();
    const setGrading = vi.fn().mockResolvedValue(undefined);
    renderPanel("l1", undefined, setGrading);

    await user.click(screen.getByRole("checkbox", { name: "Lleva calificación numérica" }));

    expect(setGrading).toHaveBeenCalledWith("l1", { enabled: false, letters: false, min: 5, max: 10, decimals: 0, passing: 6 });
  });

  it("offers nothing until a level is chosen", () => {
    renderPanel(undefined);
    expect(screen.queryByRole("checkbox", { name: "Lleva calificación numérica" })).toBeNull();
  });
});

describe("LevelsPanel — deleting a level (owner only)", () => {
  function renderWith(onDelete: ((id: string) => void) | undefined, selected: string | undefined) {
    render(
      <LevelsPanel
        levels={[primaria, secundaria]}
        selectedLevelId={selected}
        onSelect={() => {}}
        onCreate={vi.fn()}
        onSetAttendanceMode={vi.fn()}
        onSetGrading={vi.fn()}
        onDelete={onDelete}
      />,
    );
  }

  it("offers 'Eliminar nivel' on the selected level to whoever may delete", async () => {
    const onDelete = vi.fn();
    renderWith(onDelete, "l1");

    await userEvent.setup().click(screen.getByRole("button", { name: "Eliminar nivel PRIMARIA" }));

    expect(onDelete).toHaveBeenCalledWith("l1");
  });

  it("offers nothing without the permission", () => {
    renderWith(undefined, "l1");
    expect(screen.queryByRole("button", { name: /Eliminar nivel/ })).toBeNull();
  });

  it("offers nothing until a level is chosen", () => {
    renderWith(vi.fn(), undefined);
    expect(screen.queryByRole("button", { name: /Eliminar nivel/ })).toBeNull();
  });
});
