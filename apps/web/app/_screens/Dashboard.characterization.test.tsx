import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { USERS } from "../_design/data";
import { DESKTOP, clearHovered, markHovered, styleTree } from "../_design/testing/declaredStyle";
import { Dashboard } from "./Dashboard";

vi.mock("../students/api", () => ({ listStudents: vi.fn().mockResolvedValue([]) }));
vi.mock("../config/api", () => ({ listCycles: vi.fn().mockResolvedValue([]) }));
vi.mock("../finance/api", () => ({ listCharges: vi.fn().mockResolvedValue([]) }));

/*
 * F2a characterization (PLAN-redesign-home §3.3, review D1): the module card
 * and the KPI card move from inline styles (hover via useState) to classes
 * (hover via :hover) with ZERO visual change. Recorded against the inline
 * version. Hover is driven both ways at once: a mouseenter (what the inline
 * version listens to) and a :hover mark (what the CSS version matches), so the
 * same assertion holds before and after.
 */

const PERMS = ["students:read", "finance:read", "attendance:read", "grades:read", "comms:read", "config:read", "users:read"];
const NOW = new Date("2026-06-12T09:00:00");

function renderHome(): void {
  render(<Dashboard user={USERS.admin} onOpen={() => {}} permissions={PERMS} now={NOW} />);
}

function cardOf(label: string): HTMLElement {
  const card = screen.getByText(label).closest("button");
  if (card === null) throw new Error(`no card for ${label}`);
  return card;
}

function hover(el: HTMLElement): void {
  fireEvent.mouseEnter(el);
  markHovered(el);
}

afterEach(() => {
  clearHovered();
  document.documentElement.removeAttribute("data-theme");
});

describe("home cards styles (characterization)", () => {
  for (const theme of ["light", "dark"] as const) {
    it(`module grid and KPI row resolve to today's values at rest (${theme})`, () => {
      renderHome();
      document.documentElement.setAttribute("data-theme", theme);
      const kpis = cardOf("Alumnos").parentElement;
      const grid = cardOf("Estudiantes").parentElement;
      if (kpis === null || grid === null) throw new Error("missing containers");
      expect(styleTree(kpis, DESKTOP)).toMatchSnapshot("kpi row");
      expect(styleTree(grid, DESKTOP)).toMatchSnapshot("module grid");
    });
  }

  it.each(["Estudiantes", "Configuración"])("a hovered module card (%s) resolves to today's hover values", (label) => {
    renderHome();
    const card = cardOf(label);
    hover(card);
    expect(styleTree(card, DESKTOP)).toMatchSnapshot();
  });

  it("a hovered KPI card resolves to today's hover values", () => {
    renderHome();
    const card = cardOf("Por cobrar");
    hover(card);
    expect(styleTree(card, DESKTOP)).toMatchSnapshot();
  });

  // An inline style beats every author rule: the look under html[data-look]
  // (F2b) could not restyle a card that carried one. Only custom properties
  // (the module's color, the stagger) may stay inline — they feed the classes.
  it("the cards and everything in them carry no inline style but custom properties", () => {
    renderHome();
    const cards = [cardOf("Alumnos"), cardOf("Por cobrar"), cardOf("Estudiantes"), cardOf("Configuración")];
    const offenders = cards.flatMap((card) =>
      [card, ...card.querySelectorAll("*:not(svg *)")]
        .filter((el) => el.tagName.toLowerCase() !== "svg")
        .flatMap((el) => [...(el as HTMLElement).style].filter((prop) => !prop.startsWith("--")).map((prop) => `${el.tagName}.${prop}`))
    );
    expect(offenders).toEqual([]);
  });
});
