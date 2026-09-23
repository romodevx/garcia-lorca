import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { InlineCreateChip } from "./InlineCreateChip";

function setup(onCreate = vi.fn().mockResolvedValue(undefined)): typeof onCreate {
  render(
    <InlineCreateChip
      label="Nueva materia"
      fieldLabel="Nombre de la materia"
      help="La materia como aparecerá en la boleta."
      onCreate={onCreate}
    />,
  );
  return onCreate;
}

describe("InlineCreateChip", () => {
  it("empieza como un chip que invita", () => {
    setup();

    expect(screen.getByRole("button", { name: "Nueva materia" })).toBeInTheDocument();
    expect(screen.queryByLabelText("Nombre de la materia")).toBeNull();
  });

  it("se convierte en campo con el foco ya puesto", async () => {
    const user = userEvent.setup();
    setup();

    await user.click(screen.getByRole("button", { name: "Nueva materia" }));

    expect(screen.getByLabelText("Nombre de la materia")).toHaveFocus();
  });

  it("Enter guarda en mayúsculas y deja listo el siguiente", async () => {
    const user = userEvent.setup();
    const onCreate = setup();
    await user.click(screen.getByRole("button", { name: "Nueva materia" }));

    await user.type(screen.getByLabelText("Nombre de la materia"), "matemáticas{Enter}");

    expect(onCreate).toHaveBeenCalledWith("MATEMÁTICAS");
    expect(screen.getByLabelText("Nombre de la materia")).toHaveValue("");
    expect(screen.getByLabelText("Nombre de la materia")).toHaveFocus();
  });

  // Enter en vacío no es un error que haya que gritar: simplemente todavía no
  // hay nada que guardar.
  it("Enter en vacío no guarda ni reclama", async () => {
    const user = userEvent.setup();
    const onCreate = setup();
    await user.click(screen.getByRole("button", { name: "Nueva materia" }));

    await user.keyboard("{Enter}");

    expect(onCreate).not.toHaveBeenCalled();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("Esc lo vuelve a cerrar", async () => {
    const user = userEvent.setup();
    setup();
    await user.click(screen.getByRole("button", { name: "Nueva materia" }));

    await user.keyboard("{Escape}");

    expect(screen.getByRole("button", { name: "Nueva materia" })).toBeInTheDocument();
  });

  // Irse a otro lado con algo escrito no puede tirarlo: el chip se queda abierto
  // con lo capturado hasta que la persona decida.
  it("no pierde lo escrito al salirse del campo", async () => {
    const user = userEvent.setup();
    setup();
    await user.click(screen.getByRole("button", { name: "Nueva materia" }));
    await user.type(screen.getByLabelText("Nombre de la materia"), "ESPAÑOL");

    await user.tab();

    expect(screen.getByLabelText("Nombre de la materia")).toHaveValue("ESPAÑOL");
  });
});

/**
 * El chip está hecho para encadenar: "cada Enter agrega la siguiente". Si el
 * foco se pierde después del primero, esa promesa se rompe en la materia dos.
 */
describe("InlineCreateChip encadenando", () => {
  it("sigue enfocado después de guardar, y el segundo Enter también guarda", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    render(
      <InlineCreateChip label="Nueva materia" fieldLabel="Nombre de la materia" onCreate={onCreate} />,
    );
    await user.click(screen.getByRole("button", { name: "Nueva materia" }));

    await user.type(screen.getByLabelText("Nombre de la materia"), "matemáticas{Enter}");
    await user.type(screen.getByLabelText("Nombre de la materia"), "español{Enter}");

    expect(onCreate).toHaveBeenNthCalledWith(1, "MATEMÁTICAS");
    expect(onCreate).toHaveBeenNthCalledWith(2, "ESPAÑOL");
    expect(screen.getByLabelText("Nombre de la materia")).toHaveFocus();
  });

  it("no guarda dos veces si se aprieta Enter dos veces", async () => {
    const user = userEvent.setup();
    let resolver: (() => void) | undefined;
    const onCreate = vi.fn().mockImplementation(
      () => new Promise<void>((resolve) => { resolver = resolve; }),
    );
    render(
      <InlineCreateChip label="Nueva materia" fieldLabel="Nombre de la materia" onCreate={onCreate} />,
    );
    await user.click(screen.getByRole("button", { name: "Nueva materia" }));
    await user.type(screen.getByLabelText("Nombre de la materia"), "MATEMÁTICAS");

    await user.keyboard("{Enter}");
    await user.keyboard("{Enter}");

    expect(onCreate).toHaveBeenCalledTimes(1);
    resolver?.();
  });

  it("conserva lo escrito cuando el alta falla", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockRejectedValue(new Error("ya existe"));
    render(
      <InlineCreateChip label="Nueva materia" fieldLabel="Nombre de la materia" onCreate={onCreate} />,
    );
    await user.click(screen.getByRole("button", { name: "Nueva materia" }));

    await user.type(screen.getByLabelText("Nombre de la materia"), "MATEMÁTICAS{Enter}");

    expect(screen.getByLabelText("Nombre de la materia")).toHaveValue("MATEMÁTICAS");
  });
});
