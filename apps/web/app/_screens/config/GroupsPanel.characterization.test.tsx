import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Cycle, GradeLevel, Group } from "../../config/api";
import { GroupsPanel, type GroupsPanelProps } from "./GroupsPanel";

/*
 * Characterization tests (2026-10-09) for the groups panel, frozen before the
 * Configuración académica redesign (behavior.md §12, A53, S12.1/S12.2). Until
 * now only the e2e touched it.
 */

const CYCLE: Cycle = { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" };
const GRADE: GradeLevel = { id: "g1", levelId: "l1", name: "1° PRIMARIA", ordinal: 1 };
const group = (id: string, name: string, capacity: number): Group => ({ id, cycleId: "c1", gradeLevelId: "g1", name, capacity });

function setup(overrides: Partial<GroupsPanelProps> = {}): GroupsPanelProps {
  const props: GroupsPanelProps = {
    cycle: CYCLE,
    gradeLevel: GRADE,
    groups: [],
    onCreate: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
  render(<GroupsPanel {...props} />);
  return props;
}

afterEach(() => vi.clearAllMocks());

describe("GroupsPanel", () => {
  it("names the grade and the cycle, and says when there are no groups yet", () => {
    setup();

    expect(screen.getByRole("heading", { name: "Grupos" })).toBeInTheDocument();
    expect(screen.getByText("De 1° PRIMARIA en 2026-2027.")).toBeInTheDocument();
    expect(screen.getByText("Aún no hay grupos para este grado en el ciclo.")).toBeInTheDocument();
  });

  it("lists each group with its capacity, not as a selectable chip", () => {
    setup({ groups: [group("gr1", "A", 30), group("gr2", "B", 25)] });

    expect(screen.getByText("A · 30 lugares")).toBeInTheDocument();
    expect(screen.getByText("B · 25 lugares")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /lugares/ })).toBeNull();
  });

  it("proposes letter A and a capacity of 30, so opening the row and pressing Enter creates the group", async () => {
    const user = userEvent.setup();
    const props = setup();

    await user.click(screen.getByRole("button", { name: "Nuevo grupo" }));
    expect(screen.getByLabelText("Nombre del grupo")).toHaveValue("A");
    expect(screen.getByLabelText("Cupo")).toHaveValue(30);
    await user.keyboard("{Enter}");

    await waitFor(() =>
      expect(props.onCreate).toHaveBeenCalledWith({ cycleId: "c1", gradeLevelId: "g1", name: "A", capacity: 30 }),
    );
  });

  it("proposes the next letter after the groups that exist", async () => {
    const user = userEvent.setup();
    setup({ groups: [group("gr1", "A", 30), group("gr2", "B", 30)] });

    await user.click(screen.getByRole("button", { name: "Nuevo grupo" }));

    expect(screen.getByLabelText("Nombre del grupo")).toHaveValue("C");
  });

  it("refuses a capacity below 1 without calling the server", async () => {
    const user = userEvent.setup();
    const props = setup();

    await user.click(screen.getByRole("button", { name: "Nuevo grupo" }));
    await user.clear(screen.getByLabelText("Cupo"));
    await user.type(screen.getByLabelText("Cupo"), "0{Enter}");

    expect(await screen.findByText("El cupo del grupo tiene que ser un número mayor que cero.")).toBeInTheDocument();
    expect(props.onCreate).not.toHaveBeenCalled();
  });

  it("a refused create says the name may already exist", async () => {
    const user = userEvent.setup();
    setup({ onCreate: vi.fn().mockRejectedValue(new Error("conflict")) });

    await user.click(screen.getByRole("button", { name: "Nuevo grupo" }));
    await user.keyboard("{Enter}");

    expect(await screen.findByText("No se pudo crear el grupo. Revisa que no exista ya con ese nombre.")).toBeInTheDocument();
  });

  it("explains what the capacity decides", () => {
    setup();

    expect(screen.getByText(/decide a qué grupo entra un alumno nuevo/)).toBeInTheDocument();
  });
});
