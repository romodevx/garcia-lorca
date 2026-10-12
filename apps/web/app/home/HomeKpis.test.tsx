import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FlagSet } from "@garcia-lorca/ui/flags";
import { describe, expect, it, vi } from "vitest";

import { HomeKpis } from "./HomeKpis";
import { renderHome } from "./testing/renderHome";
import type { HomeKpis as Kpis, Ledger, LedgerReading, Reading } from "./useHomeKpis";

/*
 * PLAN-redesign-home §3.4 (reviews D2, D9): five states per KPI. A zero is
 * only said after a read that worked; loading never shows a number; a failed
 * read says so and offers a retry inside the card.
 */

const OWNER = ["students:read", "students:create", "finance:read", "finance:write", "config:read"];
const ACTIVE = { name: "2026-2027", active: true };
const LEDGER: Ledger = { pendingCents: 1_250_000, collectedCents: 830_000, withBalance: 3, charges: 9 };

function kpis(over: Partial<Kpis> = {}): Kpis {
  return { students: { status: "ready", value: 412 }, cycle: ACTIVE, ledger: { status: "ready", value: LEDGER }, retryStudents: vi.fn(), retryLedger: vi.fn(), ...over };
}

function show(k: Kpis, permissions: readonly string[] = OWNER, flags: FlagSet = { module_students: true, module_config: true }): ReturnType<typeof vi.fn> {
  const onOpen = vi.fn();
  renderHome(<HomeKpis kpis={k} permissions={permissions} onOpen={onOpen} />, true, flags);
  return onOpen;
}

/** The card a KPI's main button lives in. */
const card = (label: string): HTMLElement => {
  const li = screen.getByRole("button", { name: label }).closest("li");
  if (li === null) throw new Error(`no card for ${label}`);
  return li;
};

const loading: Reading<never> = { status: "loading" };
const failed: Reading<never> = { status: "error" };

describe("HomeKpis", () => {
  it("with a value: the number and its line, and the card opens the module", async () => {
    const onOpen = show(kpis());
    expect(card("Alumnos")).toHaveTextContent("412");
    expect(card("Alumnos")).toHaveTextContent("Registrados este ciclo");
    expect(card("Por cobrar")).toHaveTextContent("$12,500.00");
    expect(card("Por cobrar")).toHaveTextContent("3 cargos con saldo");
    expect(card("Cobrado")).toHaveTextContent("Ciclo 2026-2027");
    await userEvent.click(screen.getByRole("button", { name: "Por cobrar" }));
    expect(onOpen).toHaveBeenCalledWith("finance");
  });

  it("loading: the label and a skeleton line, never a 0", () => {
    show(kpis({ students: loading, ledger: loading }));
    for (const label of ["Alumnos", "Por cobrar", "Cobrado"]) {
      expect(card(label).querySelector(".home-kpi__skeleton")).not.toBeNull();
      expect(card(label).textContent).not.toMatch(/\d/);
    }
  });

  it("error: says so and retries from inside the card; the card still opens the module", async () => {
    const k = kpis({ students: failed, ledger: failed });
    const onOpen = show(k);
    expect(card("Alumnos")).toHaveTextContent("No se pudo cargar");
    await userEvent.click(within(card("Alumnos")).getByRole("button", { name: "Reintentar" }));
    expect(k.retryStudents).toHaveBeenCalled();
    await userEvent.click(within(card("Cobrado")).getByRole("button", { name: "Reintentar" }));
    expect(k.retryLedger).toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "Alumnos" }));
    expect(onOpen).toHaveBeenCalledWith("students");
  });

  it.each<[string, Partial<Kpis>]>([
    ["no cycle at all", { cycle: undefined, ledger: { status: "noCycle" } as LedgerReading }],
    ["a cycle that is not active", { cycle: { name: "2025-2026", active: false } }],
  ])("%s: \"Sin ciclo activo\" and a way to Configuración", async (_, over) => {
    const onOpen = show(kpis(over));
    expect(card("Por cobrar")).toHaveTextContent("Sin ciclo activo");
    expect(card("Por cobrar").textContent).not.toMatch(/\$/);
    await userEvent.click(within(card("Por cobrar")).getByRole("button", { name: "Ir a Configuración" }));
    expect(onOpen).toHaveBeenCalledWith("settings");
  });

  // Review M2 / §3.4: "Ir a Configuración →" only for a caller who can open
  // it. With module_config off, settings lands on a placeholder.
  it("no cycle, but Configuración is off: says so, offers no door to it", () => {
    show(kpis({ cycle: undefined, ledger: { status: "noCycle" } as LedgerReading }), OWNER, { module_students: true });
    expect(card("Por cobrar")).toHaveTextContent("Sin ciclo activo");
    expect(screen.queryByRole("button", { name: "Ir a Configuración" })).not.toBeInTheDocument();
  });

  it("zero after a read that worked: the next step, and the card goes there", async () => {
    const empty: Ledger = { pendingCents: 0, collectedCents: 0, withBalance: 0, charges: 0 };
    const onOpen = show(kpis({ students: { status: "ready", value: 0 }, ledger: { status: "ready", value: empty } }));
    expect(screen.getByRole("button", { name: "Alumnos" })).toHaveAccessibleDescription("Aún no hay alumnos · Nuevo alumno");
    expect(screen.getByRole("button", { name: "Por cobrar" })).toHaveAccessibleDescription("Aún no hay cargos este ciclo · Ir a Finanzas");
    expect(screen.getByRole("button", { name: "Cobrado" })).toHaveAccessibleDescription("Sin cobros todavía · Ir a Registro de pagos");

    await userEvent.click(screen.getByRole("button", { name: "Alumnos" }));
    expect(onOpen).toHaveBeenLastCalledWith("students", "new");
    await userEvent.click(screen.getByRole("button", { name: "Por cobrar" }));
    expect(onOpen).toHaveBeenLastCalledWith("finance");
    await userEvent.click(screen.getByRole("button", { name: "Cobrado" }));
    expect(onOpen).toHaveBeenLastCalledWith("payments");
  });

  // A fully paid cycle owes nothing, but it has charges: "no charges yet" would be false.
  it("a paid-up cycle shows $0.00, not \"Aún no hay cargos\"", () => {
    show(kpis({ ledger: { status: "ready", value: { ...LEDGER, pendingCents: 0, withBalance: 0 } } }));
    expect(card("Por cobrar")).toHaveTextContent("$0.00");
    expect(card("Por cobrar")).not.toHaveTextContent("Aún no hay cargos");
  });

  it("zero without the write or the payments door falls back to the plain module", async () => {
    const empty: Ledger = { pendingCents: 0, collectedCents: 0, withBalance: 0, charges: 0 };
    const reader = ["students:read", "finance:read", "config:read"];
    const onOpen = show(kpis({ students: { status: "ready", value: 0 }, ledger: { status: "ready", value: empty } }), reader);
    expect(screen.getByRole("button", { name: "Alumnos" })).toHaveAccessibleDescription("Aún no hay alumnos · Ir a Estudiantes");
    expect(screen.getByRole("button", { name: "Cobrado" })).toHaveAccessibleDescription("Sin cobros todavía · Ir a Finanzas");
    await userEvent.click(screen.getByRole("button", { name: "Alumnos" }));
    expect(onOpen).toHaveBeenLastCalledWith("students");
    await userEvent.click(screen.getByRole("button", { name: "Cobrado" }));
    expect(onOpen).toHaveBeenLastCalledWith("finance");
  });

  // The ledger is read per cycle, and only config:read reads the cycles: a
  // finance reader without it could only ever see these two cards empty.
  it("the money KPIs need config:read too", () => {
    show(kpis(), ["finance:read"]);
    expect(screen.queryByRole("button", { name: "Por cobrar" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Cobrado" })).not.toBeInTheDocument();
  });

  it("each KPI only for a caller who can read it (B4, B5)", () => {
    show(kpis(), ["students:read"]);
    expect(screen.getByRole("button", { name: "Alumnos" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Por cobrar" })).not.toBeInTheDocument();
  });
});
