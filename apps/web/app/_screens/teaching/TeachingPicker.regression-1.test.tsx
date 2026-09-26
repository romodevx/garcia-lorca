// Regression: ISSUE-001 — the teacher's picker showed "ESPAÑOL, ESPAÑOL" and
// "A, A, A, B, B": subjects exist per grade and groups are named by letter, so
// nothing told 1° Primaria A from 1° Secundaria A.
// Found by /qa on 2026-09-25
// Report: .gstack/qa-reports/qa-report-localhost-3002-2026-09-25-t2.md
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";

import { TeachingPicker } from "./TeachingPicker";
import { groupLabel, type TeachingOption } from "./selection";

const espPrimA: TeachingOption = { id: "a1", groupId: "gpa", groupName: "A", gradeLevelName: "1° PRIMARIA", subjectId: "s-esp-p", subjectName: "ESPAÑOL" };
const espPrimB: TeachingOption = { id: "a2", groupId: "gpb", groupName: "B", gradeLevelName: "1° PRIMARIA", subjectId: "s-esp-p", subjectName: "ESPAÑOL" };
const espSecA: TeachingOption = { id: "a3", groupId: "gsa", groupName: "A", gradeLevelName: "1° SECUNDARIA", subjectId: "s-esp-s", subjectName: "ESPAÑOL" };
const espSecB: TeachingOption = { id: "a4", groupId: "gsb", groupName: "B", gradeLevelName: "1° SECUNDARIA", subjectId: "s-esp-s", subjectName: "ESPAÑOL" };

function renderPicker(options: readonly TeachingOption[], bySubject = true): void {
  render(
    <TeachingPicker title="Calificaciones" options={options} bySubject={bySubject}>
      {(selected) => <p>Lista de {selected.id}</p>}
    </TeachingPicker>,
  );
}

beforeEach(() => {
  window.history.replaceState(null, "", "/calificaciones");
});

describe("groupLabel", () => {
  it("prefixes the grade when there is one", () => {
    expect(groupLabel(espSecA)).toBe("1° SECUNDARIA · A");
  });

  it("keeps the bare name when the grade is unknown", () => {
    expect(groupLabel({ groupName: "A" })).toBe("A");
    expect(groupLabel({ groupName: "A", gradeLevelName: "  " })).toBe("A");
  });
});

describe("TeachingPicker — same names in different grades", () => {
  it("tells two same-named subjects apart by their grade", () => {
    renderPicker([espPrimA, espPrimB, espSecA, espSecB]);

    const subjects = within(screen.getByRole("list", { name: "Asignaturas" })).getAllByRole("button");
    expect(subjects.map((b) => b.textContent)).toEqual(["ESPAÑOL1° PRIMARIA · 2 grupos", "ESPAÑOL1° SECUNDARIA · 2 grupos"]);
  });

  it("names each group with its grade, at the step, in the crumbs and in the switcher", async () => {
    const user = userEvent.setup();
    renderPicker([espPrimA, espPrimB, espSecA, espSecB]);

    await user.click(screen.getByRole("button", { name: /1° SECUNDARIA/ }));
    const groups = within(screen.getByRole("list", { name: "Grupos" })).getAllByRole("button");
    expect(groups.map((b) => b.textContent)).toEqual(["1° SECUNDARIA · A", "1° SECUNDARIA · B"]);

    await user.click(screen.getByRole("button", { name: "1° SECUNDARIA · A" }));
    expect(within(screen.getByRole("navigation", { name: "Ruta" })).getByText("1° SECUNDARIA · A")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Cambiar" }));
    expect(screen.getByRole("button", { name: "ESPAÑOL · 1° PRIMARIA · B" })).toBeInTheDocument();
  });

  it("orders groups by their label in group-only mode (attendance)", () => {
    renderPicker([espSecB, espPrimA, espSecA], false);

    const groups = within(screen.getByRole("list", { name: "Grupos" })).getAllByRole("button");
    expect(groups.map((b) => b.textContent)).toEqual(["1° PRIMARIA · A", "1° SECUNDARIA · A", "1° SECUNDARIA · B"]);
  });
});
