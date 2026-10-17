import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { ConfigError, type Cycle } from "../../config/api";
import { CycleBar, cycleFailureMessage, type CycleBarProps } from "./CycleBar";

/*
 * Characterization tests (2026-10-09) for the cycle bar, frozen before the
 * Configuración académica redesign (behavior.md §3: S3.1, A11, chip labels, 404).
 * Uses the REAL ConfigError, like CycleBar.test.tsx.
 */

const cycles: Cycle[] = [
  { id: "c0", name: "2025-2026", status: "closed", startsOn: "2025-08-01", endsOn: "2026-06-30" },
  { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" },
  { id: "c2", name: "2027-2028", status: "draft", startsOn: "2027-08-01", endsOn: "2028-06-30" },
];

function setup(overrides: Partial<CycleBarProps> = {}): CycleBarProps {
  const props: CycleBarProps = {
    cycles,
    selectedCycleId: "c2",
    onSelect: vi.fn(),
    onCreate: vi.fn().mockResolvedValue(undefined),
    onActivate: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
  render(<CycleBar {...props} />);
  return props;
}

describe("CycleBar — characterization", () => {
  it("says there are no cycles yet, and still offers to create one", () => {
    setup({ cycles: [], selectedCycleId: undefined });

    expect(screen.getByText("Todavía no hay ciclos. Crea el primero para empezar.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Nuevo ciclo" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Activar ciclo/ })).toBeNull();
  });

  it("labels each chip '{name} · {Borrador|Activo|Cerrado}' and marks the selected one", () => {
    setup();

    expect(screen.getByRole("button", { name: "2025-2026 · Cerrado" })).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByRole("button", { name: "2026-2027 · Activo" })).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByRole("button", { name: "2027-2028 · Borrador" })).toHaveAttribute("aria-pressed", "true");
  });

  it("clicking a chip hands its id up", async () => {
    const user = userEvent.setup();
    const props = setup();

    await user.click(screen.getByRole("button", { name: "2025-2026 · Cerrado" }));

    expect(props.onSelect).toHaveBeenCalledWith("c0");
  });

  it("offers to activate a closed cycle too (anything not active)", () => {
    setup({ selectedCycleId: "c0" });

    expect(screen.getByRole("button", { name: "Activar ciclo 2025-2026" })).toBeInTheDocument();
  });

  it("says 'Activando…' and is disabled while the activation runs", async () => {
    let finish: () => void = () => {};
    const onActivate = vi.fn(() => new Promise<void>((resolve) => { finish = resolve; }));
    const user = userEvent.setup();
    setup({ onActivate });

    await user.click(screen.getByRole("button", { name: "Activar ciclo 2027-2028" }));
    // A live year (2026-2027) is paused, so it asks first (QA CC-102).
    await user.click(await screen.findByRole("button", { name: "Activar 2027-2028" }));

    const busy = await screen.findByRole("button", { name: "Activando…" });
    expect(busy).toBeDisabled();
    expect(onActivate).toHaveBeenCalledWith("c2");

    finish();
    await waitFor(() => expect(screen.getByRole("button", { name: "Activar ciclo 2027-2028" })).toBeEnabled());
  });

  it("a failed activation shows the cycle failure message outside any drawer", async () => {
    const user = userEvent.setup();
    setup({ onActivate: vi.fn().mockRejectedValue(new ConfigError(404, "")) });

    await user.click(screen.getByRole("button", { name: "Activar ciclo 2027-2028" }));
    // A live year (2026-2027) is paused, so it asks first (QA CC-102).
    await user.click(await screen.findByRole("button", { name: "Activar 2027-2028" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("El módulo de configuración está apagado.");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("a 404 means the module is off", () => {
    expect(cycleFailureMessage(new ConfigError(404, ""))).toBe("El módulo de configuración está apagado.");
  });

  it("a create failure is shown inside the drawer, which stays open", async () => {
    const user = userEvent.setup();
    setup({ onCreate: vi.fn().mockRejectedValue(new ConfigError(409, "")) });

    await user.click(screen.getByRole("button", { name: "Nuevo ciclo" }));
    const drawer = await screen.findByRole("dialog", { name: "Nuevo ciclo escolar" });
    await user.clear(screen.getByLabelText(/^Nombre del ciclo/));
    await user.type(screen.getByLabelText(/^Nombre del ciclo/), "2028-2029");
    await user.clear(screen.getByLabelText(/^Inicia/));
    await user.type(screen.getByLabelText(/^Inicia/), "2028-08-01");
    await user.clear(screen.getByLabelText(/^Termina/));
    await user.type(screen.getByLabelText(/^Termina/), "2029-06-30");
    await user.click(screen.getByRole("button", { name: "Crear ciclo" }));

    expect(await screen.findByText("Ya existe un ciclo con ese nombre. Usa otro.")).toBeInTheDocument();
    expect(drawer).toBeInTheDocument();
  });
});
