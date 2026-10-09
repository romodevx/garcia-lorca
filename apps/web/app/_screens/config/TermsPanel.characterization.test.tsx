import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Cycle, Level, Term } from "../../config/api";
import { TermsPanel, type TermsPanelProps } from "./TermsPanel";

/*
 * Characterization tests (2026-10-09) for "Periodos de {nivel}", frozen before
 * the Configuración académica redesign (behavior.md §9: S9.3, A45 error, A46
 * "Copiando…"/error/Cancelar/singular toast, A47 errors and date rule).
 */

const cycle: Cycle = { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-07-31" };
const level = (id: string, name: string): Level => ({ id, name, sepCode: "", enrollmentGranularity: "cycle" });
const PRIMARIA = level("l1", "PRIMARIA");
const levels = [PRIMARIA, level("l2", "SECUNDARIA"), level("l3", "PREESCOLAR")];
const term = (id: string, ordinal: number, months: string[]): Term => ({
  id, cycleId: "c1", levelId: "l1", name: `${ordinal}.er trimestre`, ordinal, startsOn: "2026-08-01", endsOn: "2026-11-30", months,
});

function setup(overrides: Partial<TermsPanelProps> = {}): TermsPanelProps {
  const props: TermsPanelProps = {
    cycle,
    level: PRIMARIA,
    levels,
    terms: [term("t1", 1, ["2026-09"])],
    onCreate: vi.fn().mockResolvedValue(undefined),
    onSetMonths: vi.fn(async (termId: string, months: readonly string[]) => ({ ...term(termId, 1, []), months: [...months] })),
    onCopyMonths: vi.fn().mockResolvedValue([]),
    ...overrides,
  };
  render(<TermsPanel {...props} />);
  return props;
}

afterEach(() => vi.clearAllMocks());

describe("TermsPanel — characterization", () => {
  it("is titled after the level and the cycle", () => {
    setup();

    expect(screen.getByRole("heading", { name: "Periodos de PRIMARIA" })).toBeInTheDocument();
    expect(screen.getByText("En el ciclo 2026-2027. Toca un mes para sumarlo al periodo.")).toBeInTheDocument();
  });

  it("with no terms says so, offers 'Crear trimestres', and no grid or copy action", () => {
    setup({ terms: [] });

    expect(screen.getByText("Aún no hay periodos para este nivel en el ciclo.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Crear trimestres" })).toBeInTheDocument();
    expect(screen.queryByRole("table", { name: "Meses de cada periodo" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Copiar a otros niveles" })).toBeNull();
  });

  it("says when the trimesters could not be created", async () => {
    const user = userEvent.setup();
    setup({ terms: [], onCreate: vi.fn().mockRejectedValue(new Error("down")) });

    await user.click(screen.getByRole("button", { name: "Crear trimestres" }));

    expect(await screen.findByText("No se pudieron crear los trimestres. Revisa las fechas del ciclo.")).toBeInTheDocument();
  });

  it("copying says 'Copiando…' while it runs and toasts the singular for one level", async () => {
    let finish: () => void = () => {};
    const onCopyMonths = vi.fn(() => new Promise<Term[]>((resolve) => { finish = () => resolve([]); }));
    const user = userEvent.setup();
    setup({ onCopyMonths });

    await user.click(screen.getByRole("button", { name: "Copiar a otros niveles" }));
    const targets = screen.getByRole("group", { name: "Niveles destino" });
    expect(within(targets).queryByRole("button", { name: "PRIMARIA" })).toBeNull();
    await user.click(within(targets).getByRole("button", { name: "SECUNDARIA" }));
    await user.click(screen.getByRole("button", { name: "Copiar meses" }));

    expect(await screen.findByRole("button", { name: "Copiando…" })).toBeInTheDocument();
    expect(onCopyMonths).toHaveBeenCalledWith({ cycleId: "c1", fromLevelId: "l1", toLevelIds: ["l2"] });
    finish();

    expect(await screen.findByText("Se copiaron los meses a 1 nivel")).toBeInTheDocument();
    expect(screen.queryByRole("group", { name: "Niveles destino" })).toBeNull();
  });

  it("says when the months could not be copied, and keeps the picker open", async () => {
    const user = userEvent.setup();
    setup({ onCopyMonths: vi.fn().mockRejectedValue(new Error("down")) });

    await user.click(screen.getByRole("button", { name: "Copiar a otros niveles" }));
    await user.click(screen.getByRole("button", { name: "SECUNDARIA" }));
    await user.click(screen.getByRole("button", { name: "Copiar meses" }));

    expect(await screen.findByText("No se pudieron copiar los meses. Intenta de nuevo.")).toBeInTheDocument();
    expect(screen.getByRole("group", { name: "Niveles destino" })).toBeInTheDocument();
  });

  it("Cancelar closes the copy picker without copying", async () => {
    const user = userEvent.setup();
    const props = setup();

    await user.click(screen.getByRole("button", { name: "Copiar a otros niveles" }));
    await user.click(screen.getByRole("button", { name: "Cancelar" }));

    expect(screen.queryByRole("group", { name: "Niveles destino" })).toBeNull();
    expect(screen.getByRole("button", { name: "Copiar a otros niveles" })).toBeInTheDocument();
    expect(props.onCopyMonths).not.toHaveBeenCalled();
  });

  it("a new term proposes the next order and is created with the cycle and level", async () => {
    const user = userEvent.setup();
    const props = setup();

    await user.click(screen.getByRole("button", { name: "Nuevo periodo" }));
    expect(screen.getByLabelText("Orden")).toHaveValue(2);
    await user.type(screen.getByLabelText("Nombre del periodo"), "Segundo");
    await user.type(screen.getByLabelText("Inicio del periodo"), "2026-12-01");
    await user.type(screen.getByLabelText("Fin del periodo"), "2027-03-31");
    await user.keyboard("{Enter}");

    await waitFor(() =>
      expect(props.onCreate).toHaveBeenCalledWith({
        cycleId: "c1", levelId: "l1", name: "SEGUNDO", ordinal: 2, startsOn: "2026-12-01", endsOn: "2027-03-31",
      }),
    );
  });

  it("refuses a term that ends before it starts, without calling the server", async () => {
    const user = userEvent.setup();
    const props = setup();

    await user.click(screen.getByRole("button", { name: "Nuevo periodo" }));
    await user.type(screen.getByLabelText("Nombre del periodo"), "Segundo");
    await user.type(screen.getByLabelText("Inicio del periodo"), "2027-03-31");
    await user.type(screen.getByLabelText("Fin del periodo"), "2026-12-01");
    await user.keyboard("{Enter}");

    expect(await screen.findByText("El fin del periodo es antes que su inicio.")).toBeInTheDocument();
    expect(props.onCreate).not.toHaveBeenCalled();
  });

  it("says when a new term could not be created", async () => {
    const user = userEvent.setup();
    setup({ onCreate: vi.fn().mockRejectedValue(new Error("down")) });

    await user.click(screen.getByRole("button", { name: "Nuevo periodo" }));
    await user.type(screen.getByLabelText("Nombre del periodo"), "Segundo");
    await user.type(screen.getByLabelText("Inicio del periodo"), "2026-12-01");
    await user.type(screen.getByLabelText("Fin del periodo"), "2027-03-31");
    await user.keyboard("{Enter}");

    expect(await screen.findByText("No se pudo crear el periodo. Revisa las fechas y el orden.")).toBeInTheDocument();
  });
});
