import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../../config/api";
import { ConfigError, type LevelTrashItem } from "../../../config/api";
import { LevelTrashPanel } from "./LevelTrashPanel";

vi.mock("../../../config/api", async (importOriginal) => ({
  ConfigError: (await importOriginal<typeof import("../../../config/api")>()).ConfigError,
  listLevelTrash: vi.fn(),
  restoreLevel: vi.fn(),
}));
const api = vi.mocked(configApi);

const NOW = new Date("2026-09-28T12:00:00Z");
const COUNTS = {
  gradeLevels: 6, groups: 12, subjects: 0, terms: 0, teachingAssignments: 0, students: 120, enrollments: 0,
  attendanceMarks: 0, monthlyGrades: 0, reportCards: 0, tuitionPlans: 0, uniformItems: 0, schoolEvents: 0,
};
const ITEM: LevelTrashItem = {
  batchId: "b1", levelId: "l1", levelName: "PRIMARIA", deletedAt: "2026-09-27T17:00:00Z", restorableUntil: "2026-10-04T17:00:00Z",
  deletedByName: "Luis Beltrán", counts: COUNTS, status: "active",
};

afterEach(() => vi.clearAllMocks());

function renderPanel(items: LevelTrashItem[] = [ITEM]) {
  api.listLevelTrash.mockResolvedValue(items);
  const onRestored = vi.fn();
  render(<LevelTrashPanel now={() => NOW} onRestored={onRestored} />);
  return { onRestored, user: userEvent.setup() };
}

async function openRestore(user: ReturnType<typeof userEvent.setup>): Promise<HTMLElement> {
  await user.click(await screen.findByRole("button", { name: "Restaurar PRIMARIA" }));
  return screen.getByRole("dialog");
}

describe("LevelTrashPanel", () => {
  it("lists each deleted level with when, who, days left and what it held", async () => {
    renderPanel();

    const row = (await screen.findByText("PRIMARIA")).closest("li") as HTMLElement;
    expect(row).toHaveTextContent("Eliminado el 27 de septiembre de 2026 por Luis Beltrán");
    expect(row).toHaveTextContent("Quedan 7 días");
    expect(row).toHaveTextContent("6 grados · 12 grupos · 120 alumnos");
  });

  it("says when the trash is empty", async () => {
    renderPanel([]);
    expect(await screen.findByText("La papelera está vacía")).toBeInTheDocument();
  });

  it("says when the trash could not be read", async () => {
    api.listLevelTrash.mockRejectedValue(new TypeError("offline"));
    render(<LevelTrashPanel now={() => NOW} onRestored={vi.fn()} />);
    expect(await screen.findByRole("alert")).toHaveTextContent("No se pudo cargar la papelera.");
  });

  it("asks once, restores, and lists what did not come back", async () => {
    api.restoreLevel.mockResolvedValue({
      restored: { gradeLevels: 6, students: 117 },
      skipped: [{ kind: "enrollments", count: 3, reason: "los alumnos ya están en otro grupo" }],
    });
    const { user, onRestored } = renderPanel();
    const dialog = await openRestore(user);
    expect(dialog).toHaveAttribute("aria-modal", "true");

    api.listLevelTrash.mockResolvedValue([]);
    await user.click(within(dialog).getByRole("button", { name: "Restaurar nivel" }));

    expect(api.restoreLevel).toHaveBeenCalledWith(expect.any(String), "b1");
    expect(await screen.findByText("PRIMARIA restaurado: 6 grados, 117 alumnos.")).toBeInTheDocument();
    expect(screen.getByText("3 inscripciones a grupos no se restauraron: los alumnos ya están en otro grupo")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(onRestored).toHaveBeenCalled();
    expect(await screen.findByText("La papelera está vacía")).toBeInTheDocument();
  });

  it("Cancelar closes without restoring", async () => {
    const { user } = renderPanel();
    const dialog = await openRestore(user);
    await user.click(within(dialog).getByRole("button", { name: "Cancelar" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(api.restoreLevel).not.toHaveBeenCalled();
  });

  it("explains a name already taken", async () => {
    api.restoreLevel.mockRejectedValue(new ConfigError(409, "", "LEVEL_NAME_TAKEN"));
    const { user } = renderPanel();
    const dialog = await openRestore(user);
    await user.click(within(dialog).getByRole("button", { name: "Restaurar nivel" }));

    expect(await within(dialog).findByRole("alert")).toHaveTextContent("Ya existe otro nivel llamado PRIMARIA.");
  });

  it("an expired batch says so and refreshes the list", async () => {
    api.restoreLevel.mockRejectedValue(new ConfigError(410, "", "TRASH_EXPIRED"));
    const { user } = renderPanel();
    const dialog = await openRestore(user);
    await user.click(within(dialog).getByRole("button", { name: "Restaurar nivel" }));

    expect(await within(dialog).findByRole("alert")).toHaveTextContent("Pasaron los 7 días: PRIMARIA ya no se puede restaurar.");
    await waitFor(() => expect(api.listLevelTrash).toHaveBeenCalledTimes(2));
  });

  it("a double click restores once", async () => {
    api.restoreLevel.mockReturnValue(new Promise(() => undefined));
    const { user } = renderPanel();
    const dialog = await openRestore(user);
    await user.dblClick(within(dialog).getByRole("button", { name: "Restaurar nivel" }));
    expect(api.restoreLevel).toHaveBeenCalledTimes(1);
  });

  it("a batch past its window offers no restore", async () => {
    renderPanel([{ ...ITEM, restorableUntil: "2026-09-28T11:00:00Z" }]);
    const row = (await screen.findByText("PRIMARIA")).closest("li") as HTMLElement;
    expect(row).toHaveTextContent("Ya no se puede restaurar");
    expect(within(row).queryByRole("button")).toBeNull();
  });

  it("wraps each row at phone width", async () => {
    renderPanel();
    const row = (await screen.findByText("PRIMARIA")).closest("li") as HTMLElement;
    expect(row).toHaveStyle({ flexWrap: "wrap" });
  });
});
