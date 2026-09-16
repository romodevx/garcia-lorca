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

/**
 * Los valores que el sistema ya sabe.
 *
 * El orden de un grado y el cupo de un grupo no se le preguntan a nadie: se
 * proponen. Pero SIGUEN ESTANDO, porque quitarlos quitaría la capacidad de
 * intercalar un grado o de abrir un salón más chico — y no hay otra pantalla
 * donde corregirlo después.
 */
describe("InlineCreateRow con valores propuestos", () => {
  const COLS: readonly InlineColumn[] = [
    { key: "name", label: "Nombre del grado", kind: "text", uppercase: true, required: true },
    { key: "ordinal", label: "Orden", kind: "number", required: true, width: 110 },
  ];

  it("llega con el valor propuesto ya escrito", async () => {
    const user = userEvent.setup();
    render(
      <InlineCreateRow
        columns={COLS}
        addLabel="Nuevo grado"
        defaults={{ ordinal: "3" }}
        onCreate={vi.fn().mockResolvedValue(undefined)}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Nuevo grado" }));

    expect(screen.getByLabelText("Orden")).toHaveValue(3);
  });

  it("guarda con sólo escribir el nombre y dar Enter", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    render(
      <InlineCreateRow
        columns={COLS}
        addLabel="Nuevo grado"
        defaults={{ ordinal: "3" }}
        onCreate={onCreate}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Nuevo grado" }));

    await user.type(screen.getByLabelText("Nombre del grado"), "3° primaria{Enter}");

    expect(onCreate).toHaveBeenCalledWith({ name: "3° PRIMARIA", ordinal: "3" });
  });

  it("deja intercalar: el valor propuesto se puede sobrescribir", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    render(
      <InlineCreateRow
        columns={COLS}
        addLabel="Nuevo grado"
        defaults={{ ordinal: "3" }}
        onCreate={onCreate}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Nuevo grado" }));

    await user.type(screen.getByLabelText("Nombre del grado"), "2° BIS");
    await user.clear(screen.getByLabelText("Orden"));
    await user.type(screen.getByLabelText("Orden"), "2{Enter}");

    expect(onCreate).toHaveBeenCalledWith({ name: "2° BIS", ordinal: "2" });
  });

  // La lista crece al guardar, así que la propuesta cambia. La celda que la
  // persona NO tocó se pone al día sola; la que sí tocó se respeta.
  it("sigue la propuesta cuando la lista cambia, salvo donde ya escribieron", async () => {
    const user = userEvent.setup();
    const { rerender } = render(
      <InlineCreateRow
        columns={COLS}
        addLabel="Nuevo grado"
        defaults={{ ordinal: "3" }}
        onCreate={vi.fn().mockResolvedValue(undefined)}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Nuevo grado" }));

    rerender(
      <InlineCreateRow
        columns={COLS}
        addLabel="Nuevo grado"
        defaults={{ ordinal: "4" }}
        onCreate={vi.fn().mockResolvedValue(undefined)}
      />,
    );

    expect(screen.getByLabelText("Orden")).toHaveValue(4);
  });
});
