import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ConfigError, type Cycle, type Level, type Term } from "../../config/api";
import { TermsPanel, type TermsPanelProps } from "./TermsPanel";

const cycle: Cycle = { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-07-31" };
const level = (id: string, name: string): Level => ({ id, name, sepCode: "", enrollmentGranularity: "cycle" });
const PRIMARIA = level("l1", "PRIMARIA");
const levels = [PRIMARIA, level("l2", "SECUNDARIA"), level("l3", "PREESCOLAR")];

const term = (id: string, ordinal: number, months: string[]): Term => ({
  id, cycleId: "c1", levelId: "l1", name: `${ordinal}.er trimestre`, ordinal, startsOn: "2026-08-01", endsOn: "2026-11-30", months,
});

function setup(overrides: Partial<TermsPanelProps> = {}) {
  const props: TermsPanelProps = {
    cycle,
    level: PRIMARIA,
    levels,
    terms: [term("t1", 1, ["2026-09", "2026-10"]), term("t2", 2, [])],
    onCreate: vi.fn().mockResolvedValue(undefined),
    onSetMonths: vi.fn(async (termId: string, months: readonly string[]) => ({ ...term(termId, 1, []), months: [...months] })),
    onCopyMonths: vi.fn().mockResolvedValue([]),
    ...overrides,
  };
  render(<TermsPanel {...props} />);
  return props;
}

const cell = (month: string, termName: string): HTMLElement => screen.getByRole("button", { name: `${month} en ${termName}` });

afterEach(() => vi.clearAllMocks());

describe("TermsPanel — months × trimesters grid", () => {
  it("shows the cycle's months as columns and marks what each term already has", () => {
    setup();
    const grid = screen.getByRole("table", { name: "Meses de cada periodo" });
    expect(within(grid).getAllByRole("columnheader").map((h) => h.textContent)).toEqual(
      ["Periodo", "Ago", "Sep", "Oct", "Nov", "Dic", "Ene", "Feb", "Mar", "Abr", "May", "Jun", "Jul"],
    );
    expect(cell("septiembre de 2026", "1.er trimestre")).toHaveAttribute("aria-pressed", "true");
    expect(cell("noviembre de 2026", "1.er trimestre")).toHaveAttribute("aria-pressed", "false");
  });

  it("assigns a free month with one tap and saves it", async () => {
    const user = userEvent.setup();
    const props = setup();

    await user.click(cell("noviembre de 2026", "2.er trimestre"));

    expect(cell("noviembre de 2026", "2.er trimestre")).toHaveAttribute("aria-pressed", "true");
    await waitFor(() => expect(props.onSetMonths).toHaveBeenCalledWith("t2", ["2026-11"]));
  });

  it("moves a month to another term in one tap: first frees it, then assigns it", async () => {
    const user = userEvent.setup();
    const props = setup();

    await user.click(cell("octubre de 2026", "2.er trimestre"));

    expect(cell("octubre de 2026", "1.er trimestre")).toHaveAttribute("aria-pressed", "false");
    expect(cell("octubre de 2026", "2.er trimestre")).toHaveAttribute("aria-pressed", "true");
    await waitFor(() => expect(props.onSetMonths).toHaveBeenCalledTimes(2));
    expect(vi.mocked(props.onSetMonths).mock.calls).toEqual([["t1", ["2026-09"]], ["t2", ["2026-10"]]]);
  });

  it("reverts the tap and says why when the server refuses it", async () => {
    const user = userEvent.setup();
    const onSetMonths = vi.fn().mockRejectedValue(new ConfigError(400, "El mes está fuera del ciclo", "MONTH_OUTSIDE_CYCLE"));
    setup({ onSetMonths });

    await user.click(cell("noviembre de 2026", "2.er trimestre"));

    expect(await screen.findByRole("alert")).toHaveTextContent("El mes está fuera del ciclo");
    expect(cell("noviembre de 2026", "2.er trimestre")).toHaveAttribute("aria-pressed", "false");
  });

  it("lists the months no term has yet", () => {
    setup();
    expect(screen.getByText(/Sin periodo: /)).toHaveTextContent("Sin periodo: Ago, Nov, Dic, Ene, Feb, Mar, Abr, May, Jun, Jul");
  });

  it("offers each term's months as toggle chips (the list view)", async () => {
    const user = userEvent.setup();
    const props = setup();

    await user.click(screen.getByRole("button", { name: "Lista" }));
    const second = screen.getByRole("group", { name: "Meses de 2.er trimestre" });
    await user.click(within(second).getByRole("button", { name: "diciembre de 2026" }));

    expect(within(second).getByRole("button", { name: "diciembre de 2026" })).toHaveAttribute("aria-pressed", "true");
    await waitFor(() => expect(props.onSetMonths).toHaveBeenCalledWith("t2", ["2026-12"]));
  });
});

describe("TermsPanel — quick actions", () => {
  it("creates the three trimesters in one tap when the level has none", async () => {
    const user = userEvent.setup();
    const props = setup({ terms: [] });

    await user.click(screen.getByRole("button", { name: "Crear trimestres" }));

    await waitFor(() => expect(props.onCreate).toHaveBeenCalledTimes(3));
    expect(vi.mocked(props.onCreate).mock.calls.map(([input]) => [input.name, input.ordinal, input.startsOn, input.endsOn])).toEqual([
      ["1.er trimestre", 1, "2026-08-01", "2026-11-30"],
      ["2.º trimestre", 2, "2026-12-01", "2027-03-31"],
      ["3.er trimestre", 3, "2027-04-01", "2027-07-31"],
    ]);
  });

  it("does not offer Crear trimestres when the level already has terms", () => {
    setup();
    expect(screen.queryByRole("button", { name: "Crear trimestres" })).toBeNull();
  });

  it("copies the layout to the chosen levels and confirms with a toast", async () => {
    const user = userEvent.setup();
    const props = setup();

    await user.click(screen.getByRole("button", { name: "Copiar a otros niveles" }));
    const targets = screen.getByRole("group", { name: "Niveles destino" });
    expect(within(targets).queryByRole("button", { name: "PRIMARIA" })).toBeNull();
    await user.click(within(targets).getByRole("button", { name: "SECUNDARIA" }));
    await user.click(within(targets).getByRole("button", { name: "PREESCOLAR" }));
    await user.click(screen.getByRole("button", { name: "Copiar meses" }));

    expect(props.onCopyMonths).toHaveBeenCalledWith({ cycleId: "c1", fromLevelId: "l1", toLevelIds: ["l2", "l3"] });
    expect(await screen.findByRole("status")).toHaveTextContent("Se copiaron los meses a 2 niveles");
  });

  it("asks for a level instead of copying to none (the button is never disabled)", async () => {
    const user = userEvent.setup();
    const props = setup();

    await user.click(screen.getByRole("button", { name: "Copiar a otros niveles" }));
    const copy = screen.getByRole("button", { name: "Copiar meses" });
    expect(copy).toBeEnabled();
    await user.click(copy);

    expect(await screen.findByRole("alert")).toHaveTextContent("Elige al menos un nivel");
    expect(props.onCopyMonths).not.toHaveBeenCalled();
  });
});
