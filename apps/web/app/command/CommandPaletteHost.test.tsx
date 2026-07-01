import type { FlagSet } from "@garcia-lorca/ui/flags";
import { fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it, vi, beforeEach } from "vitest";

import { Providers } from "../test-utils";
import { CommandPaletteHost } from "./CommandPaletteHost";
import { RECENTS_KEY } from "./recents";

function Harness({ onNavigate, startOpen = true }: { onNavigate: (r: string) => void; startOpen?: boolean }): React.ReactNode {
  const [open, setOpen] = useState(startOpen);
  return <CommandPaletteHost open={open} onOpenChange={setOpen} onNavigate={onNavigate} />;
}

const withFlags = (node: React.ReactNode, flags: FlagSet = { module_students: true, module_finance: true }) =>
  render(<Providers flags={flags}>{node}</Providers>);

describe("CommandPaletteHost", () => {
  beforeEach(() => window.localStorage.clear());

  it("lists enabled modules (localized) when open", () => {
    withFlags(<Harness onNavigate={() => {}} />);
    expect(screen.getByText("Estudiantes")).toBeInTheDocument();
    expect(screen.getByText("Finanzas y adeudos")).toBeInTheDocument();
  });

  it("does NOT surface commands of a disabled module", () => {
    withFlags(<Harness onNavigate={() => {}} />, { module_students: true });
    fireEvent.change(screen.getByPlaceholderText(/Buscar/), { target: { value: "finanzas" } });
    expect(screen.queryByText("Finanzas y adeudos")).toBeNull();
  });

  it("matches accent-insensitively via the create alias ('nuevo estudiante')", () => {
    withFlags(<Harness onNavigate={() => {}} />, { module_students: true });
    fireEvent.change(screen.getByPlaceholderText(/Buscar/), { target: { value: "nuevo estudiante" } });
    expect(screen.getByText("Crear alumno")).toBeInTheDocument();
  });

  it("navigates to the module route on select (never mutates)", () => {
    const onNavigate = vi.fn();
    withFlags(<Harness onNavigate={onNavigate} />, { module_students: true });
    fireEvent.click(screen.getByText("Crear alumno"));
    expect(onNavigate).toHaveBeenCalledWith("module:students");
  });

  it("persists a selected command's ID to recents (id only, no record data)", () => {
    withFlags(<Harness onNavigate={() => {}} />, { module_students: true });
    fireEvent.click(screen.getByText("Estudiantes"));
    expect(JSON.parse(window.localStorage.getItem(RECENTS_KEY) ?? "[]")).toEqual(["students.nav"]);
  });

  it("opens on Cmd+K", () => {
    const onNavigate = vi.fn();
    withFlags(<Harness onNavigate={onNavigate} startOpen={false} />, { module_students: true });
    expect(screen.queryByPlaceholderText(/Buscar/)).toBeNull();
    fireEvent.keyDown(window, { key: "k", metaKey: true });
    expect(screen.getByPlaceholderText(/Buscar/)).toBeInTheDocument();
  });
});
