import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Level } from "../../config/api";
import { LevelsPanel } from "./LevelsPanel";

const primaria: Level = { id: "l1", name: "PRIMARIA", sepCode: "", enrollmentGranularity: "cycle", attendanceMode: "daily" };
const secundaria: Level = { id: "l2", name: "SECUNDARIA", sepCode: "", enrollmentGranularity: "cycle", attendanceMode: "per_class" };

afterEach(() => vi.clearAllMocks());

function renderPanel(selectedLevelId: string | undefined, onSetAttendanceMode = vi.fn().mockResolvedValue(undefined)) {
  render(
    <LevelsPanel
      levels={[primaria, secundaria, { ...primaria, id: "l3", name: "KINDER", attendanceMode: undefined }]}
      selectedLevelId={selectedLevelId}
      onSelect={() => {}}
      onCreate={vi.fn()}
      onSetAttendanceMode={onSetAttendanceMode}
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
