import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";

import { StudentFilters, StudentSelectionBand } from "./StudentFilters";
import type { FilterCatalog, FilterableStudent } from "./studentFilter";
import { useStudentFilter } from "./useStudentFilter";

const catalog: FilterCatalog = {
  levels: [{ id: "pre", name: "Preescolar" }, { id: "pri", name: "Primaria" }],
  grades: [
    { id: "pre-1", levelId: "pre", name: "1°", ordinal: 1 },
    { id: "pri-1", levelId: "pri", name: "1°", ordinal: 1 },
    { id: "pri-2", levelId: "pri", name: "2°", ordinal: 2 },
  ],
  groups: [{ id: "pre-1-a", gradeLevelId: "pre-1", name: "A" }, { id: "pre-1-b", gradeLevelId: "pre-1", name: "B" }],
};

const roll: FilterableStudent[] = [
  { id: "1", lastName: "Arce", firstName: "Ana", levelId: "pre", gradeLevelId: "pre-1", groupId: "pre-1-a" },
  { id: "2", lastName: "Bello", firstName: "Beto", levelId: "pre", gradeLevelId: "pre-1", groupId: "pre-1-b" },
  { id: "3", lastName: "Cano", firstName: "Ceci", levelId: "pre", gradeLevelId: "pre-1", groupId: "pre-1-b" },
  { id: "4", lastName: "Duarte", firstName: "Dani", levelId: "pri", gradeLevelId: "pri-1", groupId: "" },
];

function Harness({ onClear, extraActive = false }: { onClear?: () => void; extraActive?: boolean }): ReactElement {
  const filter = useStudentFilter(roll, catalog);
  return (
    <>
      <StudentFilters filter={filter} selectable onClear={onClear} extraActive={extraActive} />
      <StudentSelectionBand filter={filter} />
      <ul aria-label="Resultado">{filter.filtered.map((s) => <li key={s.id}>{s.lastName}</li>)}</ul>
    </>
  );
}

const shown = (): string[] => within(screen.getByRole("list", { name: "Resultado" })).queryAllByRole("listitem").map((li) => li.textContent ?? "");

describe("StudentFilters", () => {
  it("reaches 1°B preescolar in three clicks, each row showing only the chosen parent's children", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    expect(screen.queryByRole("group", { name: "Grado" })).toBeNull();
    await user.click(within(screen.getByRole("group", { name: "Nivel" })).getByRole("button", { name: /Preescolar/ }));
    const grades = screen.getByRole("group", { name: "Grado" });
    expect(within(grades).getAllByRole("button").map((b) => b.textContent)).toEqual(["1°3"]);

    await user.click(within(grades).getByRole("button", { name: /1°/ }));
    await user.click(within(screen.getByRole("group", { name: "Grupo" })).getByRole("button", { name: /^B/ }));

    expect(shown()).toEqual(["Bello", "Cano"]);
    expect(within(screen.getByRole("group", { name: "Grupo" })).getByRole("button", { name: /^B/ })).toHaveAttribute("aria-pressed", "true");
  });

  it("shows a count on each chip", () => {
    render(<Harness />);
    const levels = screen.getByRole("group", { name: "Nivel" });
    expect(within(levels).getByRole("button", { name: /Preescolar/ })).toHaveTextContent("3");
    expect(within(levels).getByRole("button", { name: /Primaria/ })).toHaveTextContent("1");
  });

  it("searches while typing, without accents or case", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.type(screen.getByRole("searchbox", { name: "Buscar" }), "CÉCI");
    await waitFor(() => expect(shown()).toEqual(["Cano"]));
  });

  it("'Seleccionar todos (N)' picks the filtered pupils and the lime band counts them", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.click(within(screen.getByRole("group", { name: "Nivel" })).getByRole("button", { name: /Preescolar/ }));

    await user.click(screen.getByRole("button", { name: "Seleccionar todos (3)" }));

    expect(screen.getByRole("status")).toHaveTextContent("3 seleccionados");
    await user.click(screen.getByRole("button", { name: "Quitar selección" }));
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("offers 'Limpiar' only while something narrows the list, and clears the screen's own filters too", async () => {
    const user = userEvent.setup();
    const onClear = vi.fn();
    const { rerender } = render(<Harness onClear={onClear} />);
    expect(screen.queryByRole("button", { name: "Limpiar" })).toBeNull();

    rerender(<Harness onClear={onClear} extraActive />);
    await user.click(screen.getByRole("button", { name: "Limpiar" }));
    expect(onClear).toHaveBeenCalledOnce();

    await user.click(within(screen.getByRole("group", { name: "Nivel" })).getByRole("button", { name: /Primaria/ }));
    await user.click(screen.getByRole("button", { name: "Limpiar" }));
    expect(shown()).toHaveLength(4);
  });
});
