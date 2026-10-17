import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { ConfigError, type Cycle } from "../../config/api";
import { CycleBar, cycleFailureMessage, type CycleBarProps } from "./CycleBar";

/*
 * QA ciclo completo 2026-10-17:
 *  CC-101 — an overlapping year is refused; the screen must say which one.
 *  CC-102 — one click on "Activar" paused the live year of the whole school.
 *  CC-103 — a draft year could never be renamed.
 */

const live: Cycle = { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-24", endsOn: "2027-07-09" };
const draft: Cycle = { id: "c2", name: "2027-2028", status: "draft", startsOn: "2027-08-23", endsOn: "2028-07-07" };

function setup(overrides: Partial<CycleBarProps> = {}): CycleBarProps {
  const props: CycleBarProps = {
    cycles: [live, draft],
    selectedCycleId: "c2",
    onSelect: vi.fn(),
    onCreate: vi.fn().mockResolvedValue(undefined),
    onActivate: vi.fn().mockResolvedValue(undefined),
    onRename: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
  render(<CycleBar {...props} />);
  return props;
}

describe("CC-102 · activating a year asks first when it pauses the live one", () => {
  it("opens a modal dialog naming the year that is paused, and activates only on confirm", async () => {
    const user = userEvent.setup();
    const props = setup();

    await user.click(screen.getByRole("button", { name: "Activar ciclo 2027-2028" }));

    const dialog = await screen.findByRole("dialog", { name: "¿Activar el ciclo 2027-2028?" });
    expect(dialog.tagName).toBe("DIALOG");
    expect(dialog).toHaveTextContent("El ciclo 2026-2027 queda en pausa para toda la escuela");
    expect(props.onActivate).not.toHaveBeenCalled();

    await user.click(within(dialog).getByRole("button", { name: "Activar 2027-2028" }));

    expect(props.onActivate).toHaveBeenCalledWith("c2");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("Cancelar closes it and nothing changes", async () => {
    const user = userEvent.setup();
    const props = setup();

    await user.click(screen.getByRole("button", { name: "Activar ciclo 2027-2028" }));
    const dialog = await screen.findByRole("dialog", { name: "¿Activar el ciclo 2027-2028?" });
    await user.click(within(dialog).getByRole("button", { name: "Cancelar" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(props.onActivate).not.toHaveBeenCalled();
  });

  it("with no live year there is nothing to pause, so it activates at once", async () => {
    const user = userEvent.setup();
    const props = setup({ cycles: [{ ...live, status: "closed" }, draft] });

    await user.click(screen.getByRole("button", { name: "Activar ciclo 2027-2028" }));

    expect(props.onActivate).toHaveBeenCalledWith("c2");
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

describe("CC-101 · an overlapping year", () => {
  it("shows the server's sentence, which names the year it collides with", () => {
    const sentence = 'Las fechas se traslapan con el ciclo "2026-2027" (del 24 de agosto de 2026 al 9 de julio de 2027)';
    expect(cycleFailureMessage(new ConfigError(409, sentence, "CYCLE_OVERLAP"))).toBe(sentence);
  });

  it("a name already used still says so", () => {
    expect(cycleFailureMessage(new ConfigError(409, "Ya existe un ciclo con ese nombre"))).toBe("Ya existe un ciclo con ese nombre. Usa otro.");
  });
});

describe("CC-103 · renaming the selected year", () => {
  it("renames in place: the pencil opens the name, Guardar sends it trimmed", async () => {
    const user = userEvent.setup();
    const props = setup();

    await user.click(screen.getByRole("button", { name: "Renombrar ciclo 2027-2028" }));
    const input = screen.getByLabelText("Nuevo nombre del ciclo 2027-2028");
    expect(input).toHaveFocus();
    await user.clear(input);
    await user.type(input, " ciclo 2027-2028 ");
    await user.click(screen.getByRole("button", { name: "Guardar" }));

    expect(props.onRename).toHaveBeenCalledWith("c2", "CICLO 2027-2028");
    await waitFor(() => expect(screen.queryByLabelText("Nuevo nombre del ciclo 2027-2028")).toBeNull());
  });

  it("a refusal stays on the field, in the server's words", async () => {
    const user = userEvent.setup();
    setup({ onRename: vi.fn().mockRejectedValue(new ConfigError(409, "Ya existe un ciclo con ese nombre")) });

    await user.click(screen.getByRole("button", { name: "Renombrar ciclo 2027-2028" }));
    const input = screen.getByLabelText("Nuevo nombre del ciclo 2027-2028");
    await user.clear(input);
    await user.type(input, "2026-2027{Enter}");

    expect(await screen.findByRole("alert")).toHaveTextContent("Ya existe un ciclo con ese nombre");
    expect(input).toHaveAttribute("aria-invalid", "true");
  });

  it("a blank name is refused before calling the server", async () => {
    const user = userEvent.setup();
    const props = setup();

    await user.click(screen.getByRole("button", { name: "Renombrar ciclo 2027-2028" }));
    await user.clear(screen.getByLabelText("Nuevo nombre del ciclo 2027-2028"));
    await user.click(screen.getByRole("button", { name: "Guardar" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Escribe el nombre del ciclo.");
    expect(props.onRename).not.toHaveBeenCalled();
  });

  it("Esc cancels without saving", async () => {
    const user = userEvent.setup();
    const props = setup();

    await user.click(screen.getByRole("button", { name: "Renombrar ciclo 2027-2028" }));
    await user.keyboard("{Escape}");

    expect(screen.queryByLabelText("Nuevo nombre del ciclo 2027-2028")).toBeNull();
    expect(props.onRename).not.toHaveBeenCalled();
  });

  it("is not offered when nobody can rename (no handler)", () => {
    setup({ onRename: undefined });
    expect(screen.queryByRole("button", { name: /Renombrar ciclo/ })).toBeNull();
  });
});
