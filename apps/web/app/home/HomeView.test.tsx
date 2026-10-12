import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { USERS } from "../_design/data";
import { HomeView } from "./HomeView";
import { renderHome } from "./testing/renderHome";

vi.mock("../students/api", () => ({ listStudents: vi.fn().mockResolvedValue([]) }));
vi.mock("../config/api", () => ({ listCycles: vi.fn().mockResolvedValue([]) }));
vi.mock("../finance/api", () => ({ listCharges: vi.fn().mockResolvedValue([]) }));

const NOW = new Date("2026-06-12T09:00:00");
const OWNER = ["students:read", "finance:read", "finance:write", "attendance:read", "grades:read", "comms:read", "config:read", "users:read", "teachers:read", "expenses:read", "schedule:read", "activities:read", "scholarships:read", "uniforms:stock"];
const MAESTRO = ["teaching:read", "teaching:write", "attendance_own:read", "attendance_own:write"];

function show(permissions: readonly string[] | undefined, extra: Partial<Parameters<typeof HomeView>[0]> = {}): ReturnType<typeof vi.fn> {
  const onOpen = vi.fn();
  renderHome(<HomeView user={USERS.admin} onOpen={onOpen} permissions={permissions} now={NOW} {...extra} />, true);
  return onOpen;
}

/** The module cards of a region, in order: each list item's main button. */
const cardNames = (region: HTMLElement): string[] =>
  [...within(region).getByRole("list").children].map((li) => li.querySelector(".home-card__main")?.textContent ?? "");
const areaTitles = (): string[] => screen.getAllByRole("region").map((r) => within(r).getByRole("heading", { level: 2 }).textContent ?? "");

describe("HomeView — areas (PLAN-redesign-home §3.1)", () => {
  it("greets in the h1 with the sprout icon, not an emoji", () => {
    show(OWNER);
    const h1 = screen.getByRole("heading", { level: 1, name: "Buenos días, Director" });
    expect(h1).toHaveClass("look-greeting");
    expect(h1.textContent).not.toContain("🌱");
    expect(h1.querySelector("svg")).toHaveAttribute("aria-hidden", "true");
  });

  it("draws one labelled region per area, in order, each an h2 over a list", () => {
    show(OWNER);
    const regions = screen.getAllByRole("region");
    expect(areaTitles()).toEqual(["Alumnos y familias", "Académico", "Dinero", "Sistema"]);
    expect(cardNames(regions[0]!)).toEqual(["Estudiantes", "Avisos", "Actividades"]);
    expect(cardNames(regions[1]!)).toEqual(["Asistencia", "Calificaciones", "Horarios", "Docentes"]);
    expect(cardNames(regions[2]!)).toEqual(["Registro de pagos", "Finanzas", "Becas", "Inventario", "Egresos"]);
    expect(cardNames(regions[3]!)).toEqual(["Configuración", "Administración"]);
  });

  it("leaves out an area with nothing in it", () => {
    show(["attendance:read", "grades:read", "config:read", "users:read", "schedule:read", "teachers:read"]);
    expect(areaTitles()).toEqual(["Académico", "Sistema"]);
  });

  // B12: a maestro sees three modules — areas would be three one-card rows.
  it("with four modules or fewer: one \"Tus módulos\" list, no area titles", () => {
    show(MAESTRO, { user: USERS.teacher });
    const region = screen.getByRole("region", { name: "Tus módulos" });
    expect(cardNames(region)).toEqual(["Asistencia", "Calificaciones", "Horarios"]);
    expect(screen.getAllByRole("region")).toHaveLength(1);
    expect(screen.queryByRole("heading", { name: "Académico" })).not.toBeInTheDocument();
  });

  it("each card opens its module (B11) and is described by its verb-led line", async () => {
    const onOpen = show(OWNER);
    const card = screen.getByRole("button", { name: "Finanzas" });
    expect(card).toHaveAccessibleDescription("Revisa adeudos, colegiaturas y recargos");
    await userEvent.click(card);
    expect(onOpen).toHaveBeenCalledWith("finance");
  });

  it("tags each card with its area, which picks its color", () => {
    show(OWNER);
    expect(screen.getByRole("button", { name: "Egresos" }).closest("li")).toHaveAttribute("data-group", "money");
    expect(screen.getByRole("button", { name: "Configuración" }).closest("li")).toHaveAttribute("data-group", "system");
  });

  it("keeps \"Nuevo aviso\" for comms:read (B3)", async () => {
    const onOpen = show(OWNER);
    await userEvent.click(screen.getByRole("button", { name: "Nuevo aviso" }));
    expect(onOpen).toHaveBeenCalledWith("announcements");
  });

  it("shapes the loading skeleton like areas (B8)", () => {
    show(undefined);
    const skeleton = screen.getByRole("status", { name: "Cargando módulos" });
    expect(skeleton.querySelectorAll(".home-skeleton__area").length).toBeGreaterThan(1);
    expect(screen.queryByRole("region")).not.toBeInTheDocument();
  });

  // An inline style beats every author rule, and media queries cannot reach it.
  it("styles nothing inline but custom properties", () => {
    const { container } = renderHome(<HomeView user={USERS.admin} onOpen={() => {}} permissions={OWNER} now={NOW} />, true);
    const offenders = [...container.querySelectorAll<HTMLElement>("[class*='home']")].flatMap((el) =>
      [...el.style].filter((p) => !p.startsWith("--")).map((p) => `${el.className}.${p}`),
    );
    expect(offenders).toEqual([]);
  });
});
