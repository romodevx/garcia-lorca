import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FlagSet } from "@garcia-lorca/ui/flags";
import { describe, expect, it, vi } from "vitest";

import { MODULES, USERS } from "../_design/data";
import { HomeView } from "./HomeView";
import { renderHome } from "./testing/renderHome";

vi.mock("../students/api", () => ({ listStudents: vi.fn().mockResolvedValue([]) }));
vi.mock("../config/api", () => ({ listCycles: vi.fn().mockResolvedValue([]) }));
vi.mock("../finance/api", () => ({ listCharges: vi.fn().mockResolvedValue([]) }));

/*
 * The card's shortcuts (PLAN-redesign-home §3.2, reviews D5/D6/D10/E4): a ⋯
 * disclosure, two Tab stops per card, closed means `hidden`, Esc returns to
 * the ⋯, one open at a time.
 */

const NOW = new Date("2026-06-12T09:00:00");
const ALL_ON: FlagSet = Object.fromEntries(MODULES.map((m) => [m.flag, true]));
const READ = ["students:read", "finance:read", "finance:write", "attendance:read", "grades:read", "comms:read", "config:read", "users:read", "expenses:read", "schedule:read"];
const WRITE = [...READ, "students:create", "expenses:write"];

function show(permissions: readonly string[] = WRITE, flags: FlagSet = ALL_ON): ReturnType<typeof vi.fn> {
  const onOpen = vi.fn();
  renderHome(<HomeView user={USERS.admin} onOpen={onOpen} permissions={permissions} now={NOW} />, true, flags);
  return onOpen;
}

const more = (label: string): HTMLElement => screen.getByRole("button", { name: `Accesos de ${label}` });
const peekOf = (label: string): HTMLElement => {
  const id = more(label).getAttribute("aria-controls");
  const peek = id === null ? null : document.getElementById(id);
  if (peek === null) throw new Error(`no peek for ${label}`);
  return peek;
};

describe("HomeView — card shortcuts (peek)", () => {
  it("a ⋯ only on cards whose shortcuts go somewhere the card does not (D10)", () => {
    show();
    expect(more("Estudiantes")).toBeInTheDocument();
    expect(more("Egresos")).toBeInTheDocument();
    for (const label of ["Registro de pagos", "Asistencia", "Calificaciones", "Avisos", "Configuración", "Administración"]) {
      expect(screen.queryByRole("button", { name: `Accesos de ${label}` })).not.toBeInTheDocument();
    }
  });

  it("closed, the peek is hidden: out of Tab and of the accessibility tree", () => {
    show();
    expect(more("Estudiantes")).toHaveAttribute("aria-expanded", "false");
    expect(peekOf("Estudiantes")).toHaveAttribute("hidden");
    expect(screen.queryByRole("button", { name: "Crear alumno" })).not.toBeInTheDocument();
  });

  it("the ⋯ opens it; a shortcut opens its module on its intent", async () => {
    const onOpen = show();
    await userEvent.click(more("Estudiantes"));
    expect(more("Estudiantes")).toHaveAttribute("aria-expanded", "true");
    expect(peekOf("Estudiantes")).not.toHaveAttribute("hidden");
    const shortcuts = within(peekOf("Estudiantes")).getAllByRole("button").map((b) => b.textContent?.trim());
    expect(shortcuts).toEqual(["Crear alumno →", "Buscar alumno →"]);
    await userEvent.click(screen.getByRole("button", { name: "Buscar alumno" }));
    expect(onOpen).toHaveBeenCalledWith("students", "search");
  });

  it("keyboard: card → ⋯ → Enter → Tab and arrows through the shortcuts → Esc back on the ⋯", async () => {
    const user = userEvent.setup();
    show();
    screen.getByRole("button", { name: "Estudiantes" }).focus();
    await user.tab();
    expect(more("Estudiantes")).toHaveFocus();
    await user.keyboard("{Enter}");
    await user.tab();
    expect(screen.getByRole("button", { name: "Crear alumno" })).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("button", { name: "Buscar alumno" })).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("button", { name: "Crear alumno" })).toHaveFocus();
    await user.keyboard("{ArrowUp}");
    expect(screen.getByRole("button", { name: "Buscar alumno" })).toHaveFocus();
    await user.keyboard("{Escape}");
    expect(peekOf("Estudiantes")).toHaveAttribute("hidden");
    expect(more("Estudiantes")).toHaveFocus();
  });

  it("two stops per card while closed: the card, then its ⋯, then the next card", async () => {
    const user = userEvent.setup();
    show();
    screen.getByRole("button", { name: "Estudiantes" }).focus();
    await user.tab();
    await user.tab();
    expect(screen.getByRole("button", { name: "Avisos" })).toHaveFocus();
  });

  it("only one peek open at a time", async () => {
    show();
    await userEvent.click(more("Estudiantes"));
    await userEvent.click(more("Egresos"));
    expect(peekOf("Estudiantes")).toHaveAttribute("hidden");
    expect(peekOf("Egresos")).not.toHaveAttribute("hidden");
  });

  it("a click outside closes it", async () => {
    show();
    await userEvent.click(more("Estudiantes"));
    await userEvent.click(screen.getByRole("heading", { level: 1 }));
    expect(peekOf("Estudiantes")).toHaveAttribute("hidden");
  });

  // E4 regression: expenses:read without expenses:write sees no "Registrar egreso".
  it("drops shortcuts whose write permission the caller lacks", async () => {
    show(READ);
    expect(screen.queryByRole("button", { name: "Accesos de Egresos" })).not.toBeInTheDocument();
    await userEvent.click(more("Estudiantes"));
    expect(within(peekOf("Estudiantes")).getAllByRole("button").map((b) => b.textContent?.trim())).toEqual(["Buscar alumno →"]);
  });

  it("a module whose flag is off offers no shortcut", () => {
    show(WRITE, { ...ALL_ON, module_expenses: false });
    expect(screen.queryByRole("button", { name: "Accesos de Egresos" })).not.toBeInTheDocument();
  });
});
