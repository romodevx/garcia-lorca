import { fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";

import { CommandPalette } from "./CommandPalette";
import type { Command } from "./types";

const cmd = (over: Partial<Command> & Pick<Command, "id" | "kind" | "title">): Command => ({
  icon: "dot",
  route: "/x",
  ...over,
});

const COMMANDS: Command[] = [
  cmd({ id: "students.nav", kind: "module", title: "Estudiantes", subtitle: "1,248 alumnos", aliases: ["alumnos"] }),
  cmd({ id: "students.create", kind: "create", title: "Crear alumno", aliases: ["nuevo estudiante"] }),
  cmd({ id: "grades.nav", kind: "module", title: "Calificaciones", aliases: ["notas"] }),
];

function labels(text: {
  placeholder: string;
  empty: string;
  groups: Record<string, string>;
}): Parameters<typeof CommandPalette>[0]["labels"] {
  return text;
}

const LABELS = labels({
  placeholder: "Buscar o ejecutar…",
  empty: "Sin resultados",
  groups: { module: "Módulos", create: "Crear", recent: "Recientes", record: "Registros", quick: "Acciones" },
});

describe("CommandPalette", () => {
  it("renders nothing when closed", () => {
    render(<CommandPalette open={false} onOpenChange={() => {}} commands={COMMANDS} labels={LABELS} onSelect={() => {}} />);
    expect(screen.queryByPlaceholderText("Buscar o ejecutar…")).toBeNull();
  });

  it("lists all commands when open with an empty query, grouped", () => {
    render(<CommandPalette open onOpenChange={() => {}} commands={COMMANDS} labels={LABELS} onSelect={() => {}} />);
    expect(screen.getByText("Estudiantes")).toBeInTheDocument();
    expect(screen.getByText("Crear alumno")).toBeInTheDocument();
    expect(screen.getByText("Módulos")).toBeInTheDocument();
    expect(screen.getByText("Crear")).toBeInTheDocument();
  });

  it("filters as the user types (accent-insensitive, alias-aware)", () => {
    render(<CommandPalette open onOpenChange={() => {}} commands={COMMANDS} labels={LABELS} onSelect={() => {}} />);
    const input = screen.getByPlaceholderText("Buscar o ejecutar…");
    fireEvent.change(input, { target: { value: "notas" } });
    expect(screen.getByText("Calificaciones")).toBeInTheDocument();
    expect(screen.queryByText("Crear alumno")).toBeNull();
  });

  it("shows the empty state when nothing matches", () => {
    render(<CommandPalette open onOpenChange={() => {}} commands={COMMANDS} labels={LABELS} onSelect={() => {}} />);
    fireEvent.change(screen.getByPlaceholderText("Buscar o ejecutar…"), { target: { value: "zzzzz" } });
    expect(screen.getByText("Sin resultados")).toBeInTheDocument();
  });

  it("fires onSelect with the chosen command when clicked", () => {
    const onSelect = vi.fn();
    render(<CommandPalette open onOpenChange={() => {}} commands={COMMANDS} labels={LABELS} onSelect={onSelect} />);
    fireEvent.click(screen.getByText("Crear alumno"));
    expect(onSelect).toHaveBeenCalledOnce();
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: "students.create", route: "/x" }));
  });

  it("closes on Escape", () => {
    function Harness(): React.ReactNode {
      const [open, setOpen] = useState(true);
      return <CommandPalette open={open} onOpenChange={setOpen} commands={COMMANDS} labels={LABELS} onSelect={() => {}} />;
    }
    render(<Harness />);
    const input = screen.getByPlaceholderText("Buscar o ejecutar…");
    fireEvent.keyDown(input, { key: "Escape" });
    expect(screen.queryByPlaceholderText("Buscar o ejecutar…")).toBeNull();
  });
});
