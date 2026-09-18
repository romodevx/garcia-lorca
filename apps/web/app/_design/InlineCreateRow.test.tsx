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

/**
 * Lo que el renglón hace cuando algo sale mal, y cuando alguien va rápido.
 *
 * Los cinco casos de abajo los destapó una revisión, no una prueba: todos pasan
 * por el camino feliz y ninguno se ve con el ratón.
 */
describe("InlineCreateRow bajo presión", () => {
  it("conserva lo escrito cuando el alta falla", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockRejectedValue(new Error("ya existe"));
    setup({ onCreate });
    await user.click(screen.getByRole("button", { name: "Agregar día cerrado" }));

    await user.type(screen.getByLabelText("Nombre"), "INDEPENDENCIA");
    await user.type(screen.getByLabelText("Del día"), "2026-09-16");
    await user.keyboard("{Enter}");

    // Vaciar el renglón tras un fallo obliga a reescribir todo, y el mensaje del
    // panel queda hablando de un renglón que ya está en blanco.
    expect(screen.getByLabelText("Nombre")).toHaveValue("INDEPENDENCIA");
    expect(screen.getByLabelText("Del día")).toHaveValue("2026-09-16");
  });

  // Enter no hace `blur`, así que la celda que se está escribiendo nunca se
  // revisaba: la regla de formato existía y el dato imposible salía igual.
  it("revisa el formato también cuando se guarda con Enter", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    setup({
      onCreate,
      rules: {
        endsOn: (value, all) => {
          const from = all.startsOn ?? "";
          return from !== "" && value < from ? "El fin es antes que el inicio." : undefined;
        },
      },
    });
    await user.click(screen.getByRole("button", { name: "Agregar día cerrado" }));

    await user.type(screen.getByLabelText("Nombre"), "PUENTE");
    await user.type(screen.getByLabelText("Del día"), "2026-11-20");
    // Se teclea el fin imposible y se envía SIN salir de la celda.
    await user.type(screen.getByLabelText("Al día"), "2026-11-01{Enter}");

    expect(onCreate).not.toHaveBeenCalled();
    expect(screen.getByText("El fin es antes que el inicio.")).toBeInTheDocument();
  });

  // El renglón está hecho para ir rápido ("doce feriados, doce Enter"), así que
  // dos Enter seguidos en una conexión lenta son el caso normal, no el raro.
  it("no da de alta dos veces si se aprieta Enter dos veces", async () => {
    const user = userEvent.setup();
    let resolver: (() => void) | undefined;
    const onCreate = vi.fn().mockImplementation(
      () => new Promise<void>((resolve) => { resolver = resolve; }),
    );
    setup({ onCreate });
    await user.click(screen.getByRole("button", { name: "Agregar día cerrado" }));
    await user.type(screen.getByLabelText("Nombre"), "INDEPENDENCIA");
    await user.type(screen.getByLabelText("Del día"), "2026-09-16");

    await user.keyboard("{Enter}");
    await user.keyboard("{Enter}");

    expect(onCreate).toHaveBeenCalledTimes(1);
    resolver?.();
  });

  /**
   * Dos renglones abiertos en la misma pantalla comparten nombres de celda
   * (`startsOn`, `endsOn` en días cerrados y en actividades). Buscar la celda en
   * TODO el documento mandaba el foco al renglón de al lado.
   */
  it("manda el foco a SU propia celda, no a la del renglón vecino", async () => {
    const user = userEvent.setup();
    render(
      <>
        <InlineCreateRow columns={COLUMNS} addLabel="Días" onCreate={vi.fn()} />
        <InlineCreateRow columns={COLUMNS} addLabel="Actividades" onCreate={vi.fn()} />
      </>,
    );

    await user.click(screen.getByRole("button", { name: "Días" }));
    await user.click(screen.getByRole("button", { name: "Actividades" }));

    const actividades = screen.getAllByLabelText("Nombre")[1];
    const fechasActividades = screen.getAllByLabelText("Del día")[1];
    await user.type(actividades as HTMLElement, "FESTIVAL");
    await user.keyboard("{Enter}");

    expect(fechasActividades).toHaveFocus();
  });
});
