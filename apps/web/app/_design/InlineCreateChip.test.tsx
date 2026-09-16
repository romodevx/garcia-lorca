import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { InlineCreateChip } from "./InlineCreateChip";

function setup(onCreate = vi.fn().mockResolvedValue(undefined)): typeof onCreate {
  render(
    <InlineCreateChip
      label="Nueva materia"
      fieldLabel="Nombre de la materia"
      placeholder="MATEMÁTICAS"
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
