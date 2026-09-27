import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Level } from "../../config/api";
import { GradingControl } from "./GradingControl";

const primaria: Level = {
  id: "l1",
  name: "PRIMARIA",
  sepCode: "",
  enrollmentGranularity: "cycle",
  grading: { enabled: true, min: 5, max: 10, decimals: 1, passing: 6 },
};
const kinder: Level = { id: "l3", name: "KINDER", sepCode: "", enrollmentGranularity: "cycle" };

/** The field itself, not its (?) help button. */
const CAMPO = { selector: "input" } as const;

function renderControl(level: Level, save = vi.fn().mockResolvedValue(undefined)) {
  render(<GradingControl level={level} onChange={save} />);
  return save;
}

afterEach(() => vi.clearAllMocks());

describe("GradingControl", () => {
  it("shows the level's scale", () => {
    renderControl(primaria);

    expect(screen.getByRole("checkbox", { name: "Lleva calificación numérica" })).toBeChecked();
    expect(screen.getByLabelText("Calificación mínima", CAMPO)).toHaveValue(5);
    expect(screen.getByLabelText("Calificación máxima", CAMPO)).toHaveValue(10);
    expect(screen.getByLabelText("Mínimo aprobatorio", CAMPO)).toHaveValue(6);
    expect(screen.getByRole("radio", { name: "1 decimal" })).toBeChecked();
  });

  it("a level that does not say grades 5 to 10 with one decimal", () => {
    renderControl(kinder);
    expect(screen.getByRole("checkbox", { name: "Lleva calificación numérica" })).toBeChecked();
    expect(screen.getByLabelText("Calificación máxima", CAMPO)).toHaveValue(10);
  });

  it("turning numeric grading off saves at once and hides the scale", async () => {
    const user = userEvent.setup();
    const save = renderControl(primaria);

    await user.click(screen.getByRole("checkbox", { name: "Lleva calificación numérica" }));

    expect(save).toHaveBeenCalledWith("l1", { enabled: false, min: 5, max: 10, decimals: 1, passing: 6 });
    expect(screen.queryByLabelText("Calificación máxima", CAMPO)).toBeNull();
  });

  it("saves the edited scale and confirms", async () => {
    const user = userEvent.setup();
    const save = renderControl(primaria);

    await user.clear(screen.getByLabelText("Mínimo aprobatorio", CAMPO));
    await user.type(screen.getByLabelText("Mínimo aprobatorio", CAMPO), "7");
    await user.click(screen.getByRole("radio", { name: "Sin decimales" }));
    await user.click(screen.getByRole("button", { name: "Guardar calificación" }));

    expect(save).toHaveBeenCalledWith("l1", { enabled: true, min: 5, max: 10, decimals: 0, passing: 7 });
    expect(await screen.findByRole("status")).toHaveTextContent("PRIMARIA: calificación guardada");
  });

  it("checks min < passing ≤ max on leaving the field, and blocks the save", async () => {
    const user = userEvent.setup();
    const save = renderControl(primaria);
    const passing = screen.getByLabelText("Mínimo aprobatorio", CAMPO);

    await user.clear(passing);
    await user.type(passing, "11");
    expect(screen.queryByText(/no pasar de la máxima/)).toBeNull();
    await user.tab();
    expect(await screen.findByText("El mínimo aprobatorio debe ser mayor que la mínima y no pasar de la máxima.")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Guardar calificación" }));
    expect(save).not.toHaveBeenCalled();
    expect(passing).toHaveFocus();
  });

  it("a maximum not above the minimum is refused", async () => {
    const user = userEvent.setup();
    const save = renderControl(primaria);

    await user.clear(screen.getByLabelText("Calificación máxima", CAMPO));
    await user.type(screen.getByLabelText("Calificación máxima", CAMPO), "5");
    await user.click(screen.getByRole("button", { name: "Guardar calificación" }));

    expect(await screen.findByText("La máxima debe ser mayor que la mínima.")).toBeInTheDocument();
    expect(save).not.toHaveBeenCalled();
  });

  it("says so when the save fails, in the server's words", async () => {
    const user = userEvent.setup();
    renderControl(primaria, vi.fn().mockRejectedValue(new Error("el nivel no existe")));

    await user.click(screen.getByRole("button", { name: "Guardar calificación" }));

    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("No se pudo guardar la calificación del nivel: el nivel no existe"));
  });
});
