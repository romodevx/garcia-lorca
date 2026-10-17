import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { ConfigError, type Cycle, type LabelledGroup, type Subject } from "../../config/api";
import type { CreateTeachingAssignmentInput, Teacher } from "../../teachers/api";
import { AssignmentsPanel } from "./AssignmentsPanel";

const CYCLE: Cycle = { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-24", endsOn: "2027-07-09" };
const GROUP: LabelledGroup = { id: "g1", cycleId: "c1", gradeLevelId: "gl1", name: "A", capacity: 30, label: "1° A" };
const SUBJECT: Subject = { id: "s1", gradeLevelId: "gl1", name: "Matemáticas" };
const TEACHER: Teacher = { id: "t1", firstName: "Gabriela", lastName: "Rojas", email: "g@x.mx", userId: "" };
const CAMPO = { selector: "select" } as const;
const TAKEN = "Esa asignatura ya tiene docente en ese grupo.";

function renderPanel(onCreate: (input: CreateTeachingAssignmentInput) => Promise<void>, loadAssignments = vi.fn().mockResolvedValue([])): void {
  render(
    <AssignmentsPanel
      cycles={[CYCLE]}
      teachers={[TEACHER]}
      loadGroups={vi.fn().mockResolvedValue([GROUP])}
      loadSubjects={vi.fn().mockResolvedValue([SUBJECT])}
      loadAssignments={loadAssignments}
      onCreate={onCreate}
    />,
  );
}

async function chooseAndAssign(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await screen.findByRole("option", { name: "1° A" });
  await user.selectOptions(screen.getByLabelText("Grupo", CAMPO), "g1");
  await screen.findByRole("option", { name: "Matemáticas" });
  await user.selectOptions(screen.getByLabelText("Materia", CAMPO), "s1");
  await user.selectOptions(screen.getByLabelText("Profesor", CAMPO), "t1");
  await user.click(screen.getByRole("button", { name: "Asignar profesor" }));
}

// QA 2026-10-15, ISSUE-012: an occupied slot answered 409 and the panel said
// "Intenta de nuevo"; retrying the same slot never works.
describe("AssignmentsPanel — the server's reason (ISSUE-012)", () => {
  it("says the slot already has a teacher when the server answers 409", async () => {
    const user = userEvent.setup();
    renderPanel(vi.fn().mockRejectedValue(new ConfigError(409, "a teacher is already assigned to this group and subject in the cycle")));

    await chooseAndAssign(user);

    expect(await screen.findByText(TAKEN)).toBeInTheDocument();
    expect(screen.queryByText(/Intenta de nuevo/)).toBeNull();
  });

  it("clears the previous error once the next assignment is saved", async () => {
    const user = userEvent.setup();
    const onCreate = vi
      .fn()
      .mockRejectedValueOnce(new ConfigError(409, "a teacher is already assigned to this group and subject in the cycle"))
      .mockResolvedValueOnce(undefined);
    renderPanel(onCreate);

    await chooseAndAssign(user);
    expect(await screen.findByText(TAKEN)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Asignar profesor" }));

    await waitFor(() => expect(onCreate).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(screen.queryByText(TAKEN)).toBeNull());
  });

  // The assignment WAS saved; only re-reading the list failed. Saying "No se
  // pudo asignar" invites a second, conflicting attempt.
  it("does not call a saved assignment a failure when only the refresh fails", async () => {
    const user = userEvent.setup();
    const loadAssignments = vi.fn().mockResolvedValueOnce([]).mockRejectedValue(new TypeError("Failed to fetch"));
    renderPanel(vi.fn().mockResolvedValue(undefined), loadAssignments);

    await chooseAndAssign(user);

    expect(await screen.findByText(/Se asignó, pero no pudimos releer/)).toBeInTheDocument();
    expect(screen.queryByText(/No se pudo asignar/)).toBeNull();
  });
});

describe("AssignmentsPanel — Asignar profesor while saving", () => {
  it("is busy, not disabled", async () => {
    const user = userEvent.setup();
    renderPanel(vi.fn(() => new Promise<void>(() => {})));

    await chooseAndAssign(user);

    const busy = screen.getByRole("button", { name: /Asignando/ });
    expect(busy).toBeEnabled();
    expect(busy).toHaveAttribute("aria-busy", "true");
  });
});
