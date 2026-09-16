import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { InlineCreateRow, type InlineColumn } from "./InlineCreateRow";

const COLUMNS: readonly InlineColumn[] = [
  { key: "label", label: "Nombre", kind: "text", uppercase: true, required: true },
  { key: "startsOn", label: "Del día", kind: "date", required: true },
  { key: "endsOn", label: "Al día", kind: "date" },
];

function setup(
  props: Partial<React.ComponentProps<typeof InlineCreateRow>> = {},
): ReturnType<typeof vi.fn> {
  const onCreate = props.onCreate ?? vi.fn().mockResolvedValue(undefined);
  render(
    <InlineCreateRow
      columns={COLUMNS}
      addLabel="Agregar día cerrado"
      {...props}
      onCreate={onCreate}
    />,
  );
  return onCreate as ReturnType<typeof vi.fn>;
}

describe("InlineCreateRow", () => {
  it("empieza cerrado, como un renglón que invita", () => {
    setup();

    expect(screen.getByRole("button", { name: "Agregar día cerrado" })).toBeInTheDocument();
    expect(screen.queryByLabelText("Nombre")).toBeNull();
  });

  it("al abrir pone el foco en la primera celda", async () => {
    const user = userEvent.setup();
    setup();

    await user.click(screen.getByRole("button", { name: "Agregar día cerrado" }));

    expect(screen.getByLabelText("Nombre")).toHaveFocus();
  });

  /**
   * El corazón del asunto: doce feriados sin soltar el teclado. Es lo que hace
   * que esto valga la pena frente a un cajón — no el ahorro de píxeles, sino que
   * capturar el siguiente no cueste un viaje.
   */
  it("Enter guarda y deja listo el siguiente renglón", async () => {
    const user = userEvent.setup();
    const onCreate = setup();
    await user.click(screen.getByRole("button", { name: "Agregar día cerrado" }));

    await user.type(screen.getByLabelText("Nombre"), "independencia");
    await user.type(screen.getByLabelText("Del día"), "2026-09-16");
    await user.keyboard("{Enter}");

    expect(onCreate).toHaveBeenCalledWith({
      label: "INDEPENDENCIA",
      startsOn: "2026-09-16",
      endsOn: "",
    });
    expect(screen.getByLabelText("Nombre")).toHaveValue("");
    expect(screen.getByLabelText("Nombre")).toHaveFocus();
  });

  it("Esc cancela sin guardar y cierra el renglón", async () => {
    const user = userEvent.setup();
    const onCreate = setup();
    await user.click(screen.getByRole("button", { name: "Agregar día cerrado" }));
    await user.type(screen.getByLabelText("Nombre"), "ALGO");

    await user.keyboard("{Escape}");

    expect(onCreate).not.toHaveBeenCalled();
    expect(screen.queryByLabelText("Nombre")).toBeNull();
  });

  /**
   * El defecto que se está arreglando es, literalmente, que el mensaje aparecía
   * a metros del campo que lo causó. Aquí el rojo va EN la celda y el foco se va
   * con él.
   */
  it("marca la celda que falta y manda el foco ahí", async () => {
    const user = userEvent.setup();
    const onCreate = setup();
    await user.click(screen.getByRole("button", { name: "Agregar día cerrado" }));

    await user.type(screen.getByLabelText("Nombre"), "INDEPENDENCIA");
    await user.keyboard("{Enter}");

    expect(onCreate).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Del día")).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByLabelText("Del día")).toHaveFocus();
  });

  it("revisa el dato imposible al salir de la celda, no al teclear", async () => {
    const user = userEvent.setup();
    setup({
      rules: {
        endsOn: (value, all) => {
          const from = all.startsOn ?? "";
          return from !== "" && value < from ? "El fin es antes que el inicio." : undefined;
        },
      },
    });
    await user.click(screen.getByRole("button", { name: "Agregar día cerrado" }));

    await user.type(screen.getByLabelText("Del día"), "2026-09-16");
    await user.type(screen.getByLabelText("Al día"), "2026-09-01");
    // Todavía dentro de la celda: no se acusa a nadie a medio escribir.
    expect(screen.queryByText("El fin es antes que el inicio.")).toBeNull();

    await user.tab();

    expect(screen.getByText("El fin es antes que el inicio.")).toBeInTheDocument();
  });
});
