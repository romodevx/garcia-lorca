import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Cycle, LabelledGroup, Subject } from "../../config/api";
import type { Teacher } from "../../teachers/api";
import { AssignmentsPanel } from "./AssignmentsPanel";

const CYCLE: Cycle = { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-24", endsOn: "2027-07-09" };
const GROUP: LabelledGroup = { id: "g1", cycleId: "c1", gradeLevelId: "gl1", name: "A", capacity: 30, label: "1° A" };
const SUBJECT: Subject = { id: "s1", gradeLevelId: "gl1", name: "Matemáticas" };
const TEACHER: Teacher = { id: "t1", firstName: "Gabriela", lastName: "Rojas", email: "g@x.mx", userId: "" };

/** The field itself, not its (?) help button. */
const CAMPO = { selector: "select" } as const;
const FALTA = "Falta este dato";

function renderPanel(onCreate = vi.fn().mockResolvedValue(undefined), teachers: Teacher[] = [TEACHER]) {
  render(
    <AssignmentsPanel
      cycles={[CYCLE]}
      teachers={teachers}
      loadGroups={vi.fn().mockResolvedValue([GROUP])}
      loadSubjects={vi.fn().mockResolvedValue([SUBJECT])}
      loadAssignments={vi.fn().mockResolvedValue([])}
      onCreate={onCreate}
    />,
  );
  return onCreate;
}

const button = (): HTMLElement => screen.getByRole("button", { name: "Asignar profesor" });

afterEach(() => vi.clearAllMocks());

describe("AssignmentsPanel — Asignar profesor", () => {
  it("is never disabled, even with nothing chosen", async () => {
    renderPanel();
    await screen.findByRole("option", { name: "1° A" });
    expect(button()).not.toBeDisabled();
  });

  it("on submit with nothing chosen, marks every missing field and focuses the first", async () => {
    const user = userEvent.setup();
    const onCreate = renderPanel();
    await screen.findByRole("option", { name: "1° A" });

    await user.click(button());

    expect(onCreate).not.toHaveBeenCalled();
    const grupo = screen.getByLabelText("Grupo", CAMPO);
    await waitFor(() => expect(grupo).toHaveFocus());
    expect(grupo).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByLabelText("Materia", CAMPO)).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByLabelText("Profesor", CAMPO)).toHaveAttribute("aria-invalid", "true");
    expect(screen.getAllByText(FALTA)).toHaveLength(3);
  });

  it("focuses the subject when only it is missing, and clears a field's error once chosen", async () => {
    const user = userEvent.setup();
    const onCreate = renderPanel();
    await screen.findByRole("option", { name: "1° A" });

    await user.click(button());
    await user.selectOptions(screen.getByLabelText("Grupo", CAMPO), "g1");
    expect(screen.getByLabelText("Grupo", CAMPO)).not.toHaveAttribute("aria-invalid");
    await screen.findByRole("option", { name: "Matemáticas" });
    await user.selectOptions(screen.getByLabelText("Profesor", CAMPO), "t1");
    await user.click(button());

    expect(onCreate).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.getByLabelText("Materia", CAMPO)).toHaveFocus());
    expect(screen.getAllByText(FALTA)).toHaveLength(1);
  });

  it("a teacher field with no teachers yet can still take the focus and the reason", async () => {
    const user = userEvent.setup();
    renderPanel(vi.fn(), []);
    await screen.findByRole("option", { name: "1° A" });
    await user.selectOptions(screen.getByLabelText("Grupo", CAMPO), "g1");
    await screen.findByRole("option", { name: "Matemáticas" });
    await user.selectOptions(screen.getByLabelText("Materia", CAMPO), "s1");

    await user.click(button());

    await waitFor(() => expect(screen.getByLabelText("Profesor", CAMPO)).toHaveFocus());
  });

  it("with everything chosen, assigns once even when pressed twice while saving", async () => {
    const user = userEvent.setup();
    let release!: () => void;
    const onCreate = renderPanel(vi.fn(() => new Promise<void>((resolve) => { release = resolve; })));
    await screen.findByRole("option", { name: "1° A" });
    await user.selectOptions(screen.getByLabelText("Grupo", CAMPO), "g1");
    await screen.findByRole("option", { name: "Matemáticas" });
    await user.selectOptions(screen.getByLabelText("Materia", CAMPO), "s1");
    await user.selectOptions(screen.getByLabelText("Profesor", CAMPO), "t1");

    await user.dblClick(button());

    expect(onCreate).toHaveBeenCalledTimes(1);
    expect(onCreate).toHaveBeenCalledWith({ groupId: "g1", subjectId: "s1", teacherId: "t1" });
    expect(screen.queryByText(FALTA)).toBeNull();
    await act(async () => release());
    await waitFor(() => expect(screen.getByLabelText("Materia", CAMPO)).toHaveValue(""));
  });
});
