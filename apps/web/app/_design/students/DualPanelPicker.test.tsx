import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";

import { DualPanelPicker, type DualPanelPickerProps } from "./DualPanelPicker";
import type { FilterCatalog, FilterableStudent } from "./studentFilter";

const catalog: FilterCatalog = {
  levels: [{ id: "pre", name: "Preescolar" }, { id: "pri", name: "Primaria" }],
  grades: [{ id: "pre-1", levelId: "pre", name: "1°", ordinal: 1 }, { id: "pri-1", levelId: "pri", name: "1°", ordinal: 1 }],
  groups: [{ id: "pre-1-a", gradeLevelId: "pre-1", name: "A" }, { id: "pre-1-b", gradeLevelId: "pre-1", name: "B" }],
};

const roll: FilterableStudent[] = [
  { id: "1", lastName: "Zapata", maternalLastName: "Ruiz", firstName: "Ana", levelId: "pre", gradeLevelId: "pre-1", groupId: "pre-1-a" },
  { id: "2", lastName: "Álvarez", firstName: "Beto", levelId: "pre", gradeLevelId: "pre-1", groupId: "pre-1-b" },
  { id: "3", lastName: "Cano", firstName: "Ceci", levelId: "pre", gradeLevelId: "pre-1", groupId: "pre-1-b" },
  { id: "4", lastName: "Duarte", firstName: "Dani", levelId: "pri", gradeLevelId: "pri-1", groupId: "" },
];

type Props = Partial<DualPanelPickerProps<FilterableStudent>>;

/** The picker over a real "assigned" state, as a screen would hold it. */
function Harness({ initial = [], ...props }: Props & { initial?: string[] }): ReactElement {
  const [assigned, setAssigned] = useState<string[]>(initial);
  return (
    <DualPanelPicker
      students={roll}
      catalog={catalog}
      assignedIds={assigned}
      onAssign={(ids) => setAssigned((a) => [...a, ...ids])}
      onUnassign={(ids) => setAssigned((a) => a.filter((id) => !ids.includes(id)))}
      labels={{ available: "Alumnos", assigned: "Inscritos", assign: "Inscribir", unassign: "Dar de baja" }}
      {...props}
    />
  );
}

const panel = (name: string): HTMLElement => screen.getByRole("region", { name });
const namesIn = (name: string): string[] =>
  within(panel(name)).queryAllByRole("checkbox", { name: /^(?!Seleccionar)/ }).map((c) => c.getAttribute("aria-label") ?? "");

describe("DualPanelPicker", () => {
  it("shows the unassigned pupils on the left and the assigned on the right, surnames first and sorted", () => {
    render(<Harness initial={["3"]} />);
    expect(namesIn("Alumnos")).toEqual(["Álvarez Beto", "Duarte Dani", "Zapata Ruiz Ana"]);
    expect(namesIn("Inscritos")).toEqual(["Cano Ceci"]);
  });

  it("filters the left panel by the level chips and by surname, ignoring accents", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.click(within(panel("Alumnos")).getByRole("button", { name: /Primaria/ }));
    expect(namesIn("Alumnos")).toEqual(["Duarte Dani"]);
    await user.click(within(panel("Alumnos")).getByRole("button", { name: /Limpiar/ }));
    await user.type(within(panel("Alumnos")).getByRole("searchbox"), "alvarez");
    await waitFor(() => expect(namesIn("Alumnos")).toEqual(["Álvarez Beto"]));
  });

  it("moves several at once with the arrow button", async () => {
    const user = userEvent.setup();
    const onAssign = vi.fn();
    render(<Harness onAssign={onAssign} />);
    await user.click(screen.getByRole("checkbox", { name: "Cano Ceci" }));
    await user.click(screen.getByRole("checkbox", { name: "Duarte Dani" }));
    await user.click(screen.getByRole("button", { name: /Inscribir/ }));
    expect(onAssign).toHaveBeenCalledWith(["3", "4"]);
  });

  it("selects every filtered pupil with 'Seleccionar todos (N)'", async () => {
    const user = userEvent.setup();
    const onAssign = vi.fn();
    render(<Harness onAssign={onAssign} />);
    await user.click(within(panel("Alumnos")).getByRole("button", { name: /Preescolar/ }));
    await user.click(within(panel("Alumnos")).getByRole("button", { name: "Seleccionar todos (3)" }));
    await user.click(screen.getByRole("button", { name: /Inscribir/ }));
    expect(onAssign).toHaveBeenCalledWith(["2", "3", "1"]);
  });

  it("moves the right panel's selection back with the other arrow", async () => {
    const user = userEvent.setup();
    render(<Harness initial={["1", "2"]} />);
    await user.click(within(panel("Inscritos")).getByRole("checkbox", { name: "Zapata Ruiz Ana" }));
    await user.click(screen.getByRole("button", { name: /Dar de baja/ }));
    expect(namesIn("Inscritos")).toEqual(["Álvarez Beto"]);
    expect(namesIn("Alumnos")).toContain("Zapata Ruiz Ana");
  });

  it("works from the keyboard: Tab to a pupil, Space or Enter toggles, Tab reaches the arrow", async () => {
    const user = userEvent.setup();
    const onAssign = vi.fn();
    render(<Harness onAssign={onAssign} />);
    const beto = screen.getByRole("checkbox", { name: "Álvarez Beto" });
    beto.focus();
    await user.keyboard(" ");
    expect(beto).toBeChecked();
    const dani = screen.getByRole("checkbox", { name: "Duarte Dani" });
    dani.focus();
    await user.keyboard("{Enter}");
    expect(dani).toBeChecked();
    const arrow = screen.getByRole("button", { name: /Inscribir/ });
    expect(arrow.tabIndex).toBe(0);
    arrow.focus();
    await user.keyboard("{Enter}");
    expect(onAssign).toHaveBeenCalledWith(["2", "4"]);
  });

  it("drags the whole selection onto the right panel", async () => {
    const user = userEvent.setup();
    const onAssign = vi.fn();
    render(<Harness onAssign={onAssign} />);
    await user.click(screen.getByRole("checkbox", { name: "Cano Ceci" }));
    await user.click(screen.getByRole("checkbox", { name: "Duarte Dani" }));
    const dataTransfer = { setData: vi.fn(), getData: vi.fn(), effectAllowed: "", dropEffect: "" };
    fireEvent.dragStart(screen.getByTestId("picker-row-3"), { dataTransfer });
    fireEvent.dragOver(panel("Inscritos"), { dataTransfer });
    fireEvent.drop(panel("Inscritos"), { dataTransfer });
    expect(onAssign).toHaveBeenCalledWith(["3", "4"]);
  });

  it("dragging a row that is not selected moves just that one, and back to the left unassigns", () => {
    const onUnassign = vi.fn();
    render(<Harness initial={["1", "2"]} onUnassign={onUnassign} />);
    const dataTransfer = { setData: vi.fn(), getData: vi.fn(), effectAllowed: "", dropEffect: "" };
    fireEvent.dragStart(screen.getByTestId("picker-row-1"), { dataTransfer });
    fireEvent.drop(panel("Alumnos"), { dataTransfer });
    expect(onUnassign).toHaveBeenCalledWith(["1"]);
  });

  it("a drop on the panel it came from does nothing", () => {
    const onAssign = vi.fn();
    render(<Harness onAssign={onAssign} />);
    const dataTransfer = { setData: vi.fn(), getData: vi.fn(), effectAllowed: "", dropEffect: "" };
    fireEvent.dragStart(screen.getByTestId("picker-row-2"), { dataTransfer });
    fireEvent.drop(panel("Alumnos"), { dataTransfer });
    expect(onAssign).not.toHaveBeenCalled();
  });

  it("an arrow with nothing selected says what to do instead of doing nothing", async () => {
    const user = userEvent.setup();
    const onAssign = vi.fn();
    render(<Harness onAssign={onAssign} />);
    await user.click(screen.getByRole("button", { name: /Inscribir/ }));
    expect(onAssign).not.toHaveBeenCalled();
    expect(screen.getByRole("status")).toHaveTextContent(/Marca primero/);
  });

  it("asks before assigning when the screen wants confirmation; a no keeps the selection", async () => {
    const user = userEvent.setup();
    const onAssign = vi.fn();
    const confirmAssign = vi.fn().mockResolvedValueOnce(false).mockResolvedValueOnce(true);
    render(<Harness onAssign={onAssign} confirmAssign={confirmAssign} />);
    await user.click(screen.getByRole("checkbox", { name: "Cano Ceci" }));
    await user.click(screen.getByRole("button", { name: /Inscribir/ }));
    expect(confirmAssign).toHaveBeenCalledWith(["3"]);
    expect(onAssign).not.toHaveBeenCalled();
    expect(screen.getByRole("checkbox", { name: "Cano Ceci" })).toBeChecked();
    await user.click(screen.getByRole("button", { name: /Inscribir/ }));
    await waitFor(() => expect(onAssign).toHaveBeenCalledWith(["3"]));
  });

  it("a drop asks too", async () => {
    const onAssign = vi.fn();
    const confirmAssign = vi.fn().mockResolvedValue(true);
    render(<Harness onAssign={onAssign} confirmAssign={confirmAssign} />);
    const dataTransfer = { setData: vi.fn(), getData: vi.fn(), effectAllowed: "", dropEffect: "" };
    fireEvent.dragStart(screen.getByTestId("picker-row-2"), { dataTransfer });
    fireEvent.drop(panel("Inscritos"), { dataTransfer });
    expect(confirmAssign).toHaveBeenCalledWith(["2"]);
    await waitFor(() => expect(onAssign).toHaveBeenCalledWith(["2"]));
  });

  it("renders the screen's extra per assigned pupil", () => {
    render(<Harness initial={["4"]} renderAssignedExtra={(s) => <span>{`extra-${s.id}`}</span>} />);
    expect(within(panel("Inscritos")).getByText("extra-4")).toBeInTheDocument();
  });

  it("read-only: no arrows, no checkboxes, no dragging", () => {
    render(<Harness initial={["4"]} readOnly />);
    expect(screen.queryByRole("button", { name: /Inscribir/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("checkbox", { name: "Duarte Dani" })).not.toBeInTheDocument();
    expect(within(panel("Inscritos")).getByText("Duarte Dani")).toBeInTheDocument();
    expect(screen.getByTestId("picker-row-4")).not.toHaveAttribute("draggable", "true");
  });
});
