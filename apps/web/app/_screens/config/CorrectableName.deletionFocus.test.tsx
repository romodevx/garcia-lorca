import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";

import type { Cycle, GradeLevel, Group, Level } from "../../config/api";
import { GradeLevelsPanel } from "./GradeLevelsPanel";
import { GroupsPanel } from "./GroupsPanel";

/*
 * After a delete the row is gone: focus must land somewhere logical (the next
 * row's pencil, else the previous one's, else the section's create control),
 * never on <body>, and the deletion is said in a status region.
 */

const CYCLE: Cycle = { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-24", endsOn: "2027-07-09" };
const PRIMARIA: Level = { id: "l1", name: "PRIMARIA", sepCode: "", enrollmentGranularity: "cycle" };
const GRADE: GradeLevel = { id: "g1", levelId: "l1", name: "1° PRIMARIA", ordinal: 1 };
const group = (id: string, name: string): Group => ({ id, cycleId: "c1", gradeLevelId: "g1", name, capacity: 30 });

function GroupsHost({ initial }: { initial: Group[] }): ReactElement {
  const [groups, setGroups] = useState(initial);
  const corrections = {
    rename: vi.fn(),
    remove: async (id: string): Promise<void> => setGroups((prev) => prev.filter((g) => g.id !== id)),
  };
  return <GroupsPanel cycle={CYCLE} gradeLevel={GRADE} groups={groups} onCreate={vi.fn()} corrections={corrections} />;
}

async function deleteGroup(name: string): Promise<void> {
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: `Borrar grupo ${name}` }));
  await user.click(await screen.findByRole("button", { name: "Borrar grupo" }));
}

describe("deleting a group moves the focus and says so", () => {
  it("to the next group's rename button", async () => {
    render(<GroupsHost initial={[group("a", "A"), group("b", "B"), group("c", "C")]} />);
    await deleteGroup("B");
    await waitFor(() => expect(screen.getByRole("button", { name: "Renombrar grupo C" })).toHaveFocus());
    expect(screen.getByRole("status")).toHaveTextContent("Grupo B borrado");
  });

  it("to the previous one when the last was deleted", async () => {
    render(<GroupsHost initial={[group("a", "A"), group("b", "B")]} />);
    await deleteGroup("B");
    await waitFor(() => expect(screen.getByRole("button", { name: "Renombrar grupo A" })).toHaveFocus());
  });

  it("to the section's create control when none is left", async () => {
    render(<GroupsHost initial={[group("a", "A")]} />);
    await deleteGroup("A");
    await waitFor(() => expect(screen.getByRole("button", { name: /Nuevo grupo/ })).toHaveFocus());
    expect(screen.getByRole("status")).toHaveTextContent("Grupo A borrado");
  });
});

describe("deleting a grade", () => {
  it("names the grade in the status and focuses the next grade's rename button", async () => {
    const grades = [GRADE, { id: "g7", levelId: "l1", name: "7O PRIMARIA", ordinal: 7 }, { id: "g8", levelId: "l1", name: "8O PRIMARIA", ordinal: 8 }];
    function Host(): ReactElement {
      const [list, setList] = useState<GradeLevel[]>(grades);
      const corrections = { rename: vi.fn(), remove: async (id: string): Promise<void> => setList((prev) => prev.filter((g) => g.id !== id)) };
      return <GradeLevelsPanel level={PRIMARIA} gradeLevels={list} selectedGradeLevelId={undefined} onSelect={vi.fn()} onCreate={vi.fn()} corrections={corrections} />;
    }
    render(<Host />);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Borrar grado 7O PRIMARIA" }));
    await user.click(await screen.findByRole("button", { name: "Borrar grado" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Renombrar grado 8O PRIMARIA" })).toHaveFocus());
    expect(screen.getByRole("status")).toHaveTextContent("Grado 7O PRIMARIA borrado");
  });
});
