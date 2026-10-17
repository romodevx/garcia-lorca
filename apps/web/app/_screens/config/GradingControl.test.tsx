import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ConfigError, type Level } from "../../config/api";
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
  });

  it("never offers decimals: grades are whole numbers only (owner decision), even if a level stored some", () => {
    renderControl(primaria);
    expect(screen.queryByRole("group", { name: "Decimales" })).toBeNull();
    expect(screen.queryByRole("radio", { name: "1 decimal" })).toBeNull();
    expect(screen.queryByRole("radio", { name: "2 decimales" })).toBeNull();
  });

  it("a level that does not say grades 5 to 10", () => {
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
    await user.click(screen.getByRole("button", { name: "Guardar calificación" }));

    // The stored "1 decimal" is ignored: saving the scale saves whole numbers.
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

  it("the toggle saves the last saved scale, never a half-typed invalid one", async () => {
    const user = userEvent.setup();
    const save = renderControl(primaria);

    await user.clear(screen.getByLabelText("Calificación máxima", CAMPO));
    await user.type(screen.getByLabelText("Calificación máxima", CAMPO), "101");
    await user.click(screen.getByRole("checkbox", { name: "Lleva calificación numérica" }));

    expect(save).toHaveBeenCalledTimes(1);
    expect(save).toHaveBeenCalledWith("l1", { enabled: false, min: 5, max: 10, decimals: 1, passing: 6 });
  });

  it("a toggle that fails to save goes back and says so", async () => {
    const user = userEvent.setup();
    renderControl(primaria, vi.fn().mockRejectedValue(new Error("sin red")));
    const toggle = screen.getByRole("checkbox", { name: "Lleva calificación numérica" });

    await user.click(toggle);

    await waitFor(() => expect(toggle).toBeChecked());
    expect(screen.getByRole("alert")).toHaveTextContent("No se pudo guardar la calificación del nivel: sin red");
  });

  it("toggles are sent one at a time; only the latest wish follows", async () => {
    const user = userEvent.setup();
    let release!: () => void;
    const save = vi.fn().mockImplementationOnce(() => new Promise<void>((resolve) => { release = resolve; })).mockResolvedValue(undefined);
    renderControl(primaria, save);
    const toggle = screen.getByRole("checkbox", { name: "Lleva calificación numérica" });

    await user.click(toggle);
    await user.click(toggle);
    await user.click(toggle);
    await user.click(toggle);
    expect(save).toHaveBeenCalledTimes(1);
    expect(toggle).toBeChecked();

    await act(async () => release());

    await waitFor(() => expect(save).toHaveBeenCalledTimes(2));
    expect(save).toHaveBeenLastCalledWith("l1", { enabled: true, min: 5, max: 10, decimals: 1, passing: 6 });
  });

  it("toggles that end where the server already is send nothing more", async () => {
    const user = userEvent.setup();
    let release!: () => void;
    const save = vi.fn().mockImplementationOnce(() => new Promise<void>((resolve) => { release = resolve; })).mockResolvedValue(undefined);
    renderControl(primaria, save);
    const toggle = screen.getByRole("checkbox", { name: "Lleva calificación numérica" });

    await user.click(toggle);
    await user.click(toggle);
    await user.click(toggle);
    await act(async () => release());

    expect(save).toHaveBeenCalledTimes(1);
    expect(toggle).not.toBeChecked();
  });

  describe("the scale save and the toggle share one queue", () => {
    const disabled: Level = { ...primaria, grading: { enabled: false, min: 5, max: 10, decimals: 1, passing: 6 } };
    const held = () => {
      let release!: () => void;
      const save = vi.fn().mockImplementationOnce(() => new Promise<void>((resolve) => { release = resolve; })).mockResolvedValue(undefined);
      return { save, release: () => release() };
    };

    it("a scale saved while the toggle is in flight waits for it, then goes out", async () => {
      const user = userEvent.setup();
      const { save, release } = held();
      renderControl(disabled, save);

      await user.click(screen.getByRole("checkbox", { name: "Lleva calificación numérica" }));
      await user.clear(screen.getByLabelText("Mínimo aprobatorio", CAMPO));
      await user.type(screen.getByLabelText("Mínimo aprobatorio", CAMPO), "7");
      await user.click(screen.getByRole("button", { name: "Guardar calificación" }));
      expect(save).toHaveBeenCalledTimes(1);

      await act(async () => release());

      await waitFor(() => expect(save).toHaveBeenCalledTimes(2));
      expect(save).toHaveBeenNthCalledWith(1, "l1", { enabled: true, min: 5, max: 10, decimals: 1, passing: 6 });
      expect(save).toHaveBeenNthCalledWith(2, "l1", { enabled: true, min: 5, max: 10, decimals: 0, passing: 7 });
    });

    it("a toggle made while the scale is saving waits, then sends the scale just saved", async () => {
      const user = userEvent.setup();
      const { save, release } = held();
      renderControl(primaria, save);

      await user.clear(screen.getByLabelText("Mínimo aprobatorio", CAMPO));
      await user.type(screen.getByLabelText("Mínimo aprobatorio", CAMPO), "8");
      await user.click(screen.getByRole("button", { name: "Guardar calificación" }));
      await user.click(screen.getByRole("checkbox", { name: "Lleva calificación numérica" }));
      expect(save).toHaveBeenCalledTimes(1);

      await act(async () => release());

      await waitFor(() => expect(save).toHaveBeenCalledTimes(2));
      expect(save).toHaveBeenLastCalledWith("l1", { enabled: false, min: 5, max: 10, decimals: 0, passing: 8 });
    });

    it("a failed toggle drops the scale queued behind it and the switch goes back", async () => {
      const user = userEvent.setup();
      let fail!: (e: Error) => void;
      const save = vi.fn().mockImplementationOnce(() => new Promise<void>((_, reject) => { fail = reject; })).mockResolvedValue(undefined);
      renderControl(disabled, save);
      const toggle = screen.getByRole("checkbox", { name: "Lleva calificación numérica" });

      await user.click(toggle);
      await user.click(screen.getByRole("button", { name: "Guardar calificación" }));
      await act(async () => fail(new Error("sin red")));

      await waitFor(() => expect(toggle).not.toBeChecked());
      expect(save).toHaveBeenCalledTimes(1);
      expect(screen.getByRole("alert")).toHaveTextContent("sin red");
    });

    it("a failed scale save leaves the switch where the server is", async () => {
      const user = userEvent.setup();
      const save = vi.fn().mockRejectedValueOnce(new Error("sin red")).mockResolvedValue(undefined);
      renderControl(primaria, save);

      await user.click(screen.getByRole("button", { name: "Guardar calificación" }));
      await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("sin red"));
      expect(screen.getByRole("checkbox", { name: "Lleva calificación numérica" })).toBeChecked();

      await user.click(screen.getByRole("checkbox", { name: "Lleva calificación numérica" }));
      expect(save).toHaveBeenLastCalledWith("l1", { enabled: false, min: 5, max: 10, decimals: 1, passing: 6 });
    });
  });

  it("says so when the save fails, in the server's words", async () => {
    const user = userEvent.setup();
    renderControl(primaria, vi.fn().mockRejectedValue(new Error("el nivel no existe")));

    await user.click(screen.getByRole("button", { name: "Guardar calificación" }));

    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("No se pudo guardar la calificación del nivel: el nivel no existe"));
  });

  describe("a scale the captured grades would not fit (SCALE_TIGHTENING)", () => {
    const TEXT = "Ya hay calificaciones capturadas en este ciclo que no cumplirían la nueva escala";
    const tightening = (): ConfigError => new ConfigError(409, TEXT, "SCALE_TIGHTENING");

    it("the scale form says so next to it and puts the saved values back", async () => {
      const user = userEvent.setup();
      renderControl(primaria, vi.fn().mockRejectedValue(tightening()));

      await user.clear(screen.getByLabelText("Calificación mínima", CAMPO));
      await user.type(screen.getByLabelText("Calificación mínima", CAMPO), "4");
      await user.click(screen.getByRole("button", { name: "Guardar calificación" }));

      expect(await screen.findByRole("alert")).toHaveTextContent(TEXT);
      expect(screen.getByRole("alert")).not.toHaveTextContent("No se pudo guardar");
      expect(screen.getByLabelText("Calificación mínima", CAMPO)).toHaveValue(5);
    });

    it("turning grading off goes back on and says why", async () => {
      const user = userEvent.setup();
      renderControl(primaria, vi.fn().mockRejectedValue(tightening()));
      const toggle = screen.getByRole("checkbox", { name: "Lleva calificación numérica" });

      await user.click(toggle);

      await waitFor(() => expect(toggle).toBeChecked());
      expect(screen.getByRole("alert")).toHaveTextContent(TEXT);
      expect(screen.getByLabelText("Calificación máxima", CAMPO)).toHaveValue(10);
    });
  });
});
