import { render, screen } from "@testing-library/react";
import type { ReactElement } from "react";
import { describe, expect, it } from "vitest";

import { Field, inputStyle } from "./Field";

const child = (id: string, describedBy: string | undefined): ReactElement => (
  <input id={id} aria-describedby={describedBy} />
);

describe("Field", () => {
  it("pinta el asterisco cuando es obligatorio", () => {
    render(<Field label="Nombre" required>{child}</Field>);
    expect(screen.getByText("*")).toBeInTheDocument();
  });

  it("no lo pinta cuando es opcional", () => {
    render(<Field label="Nota">{child}</Field>);
    expect(screen.queryByText("*")).toBeNull();
  });

  it("el asterisco no se lee dos veces", () => {
    // aria-required ya dice que es obligatorio; el * es sólo para el ojo.
    render(<Field label="Nombre" required>{child}</Field>);
    expect(screen.getByText("*")).toHaveAttribute("aria-hidden", "true");
  });

  it("la etiqueta apunta al campo, así que el clic enfoca", () => {
    render(<Field label="Nombre">{child}</Field>);
    expect(screen.getByLabelText("Nombre")).toBeInTheDocument();
  });

  it("con error, liga el mensaje al campo por aria-describedby", () => {
    render(<Field label="Fecha" required error="Falta este dato">{child}</Field>);
    const input = screen.getByLabelText("Fecha");
    const msg = screen.getByText("Falta este dato");
    expect(input.getAttribute("aria-describedby")).toBe(msg.id);
  });

  it("el error se anuncia solo, sin que nadie tenga que buscarlo", () => {
    render(<Field label="Fecha" error="Falta este dato">{child}</Field>);
    expect(screen.getByRole("alert")).toHaveTextContent("Falta este dato");
  });

  it("sin error no hay mensaje colgando", () => {
    render(<Field label="Fecha">{child}</Field>);
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("la ayuda se liga al campo sin que nadie tenga que señalarla", () => {
    render(<Field label="Día" help="Del 1 al 31">{child}</Field>);
    const input = screen.getByLabelText("Día");
    const dicho = document.getElementById(input.getAttribute("aria-describedby") ?? "");
    expect(dicho).toHaveTextContent("Del 1 al 31");
  });

  it("la ayuda va en un (?) y NO en un párrafo colgando del campo", () => {
    // Un párrafo debajo de cada campo estira la forma y empuja el pie fuera de
    // la vista. La explicación se guarda en el (?) y sale cuando se pide.
    render(<Field label="Día" help="Del 1 al 31">{child}</Field>);
    expect(screen.getByRole("button", { name: "Ayuda sobre Día" })).toBeInTheDocument();
    expect(screen.queryByRole("paragraph")).toBeNull();
  });

  it("sin ayuda no hay (?) que estorbe", () => {
    render(<Field label="Día">{child}</Field>);
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("cuando hay error, el error gana sobre la ayuda", () => {
    // Cuando algo está mal, lo que hay que leer es QUÉ está mal, no cómo se
    // llenaba el campo.
    render(<Field label="Día" help="Del 1 al 31" error="Falta este dato">{child}</Field>);
    const input = screen.getByLabelText("Día");
    expect(input.getAttribute("aria-describedby")).toBe(screen.getByRole("alert").id);
  });

  it("aun con error, el (?) sigue ahí para consultarlo", () => {
    render(<Field label="Día" help="Del 1 al 31" error="Falta este dato">{child}</Field>);
    expect(screen.getByRole("button", { name: "Ayuda sobre Día" })).toBeInTheDocument();
  });

  it("dos campos con la misma etiqueta no se roban el id", () => {
    render(
      <>
        <Field label="Fecha">{child}</Field>
        <Field label="Fecha">{child}</Field>
      </>
    );
    const campos = screen.getAllByLabelText("Fecha");
    expect(campos).toHaveLength(2);
    expect(campos[0]?.id).not.toBe(campos[1]?.id);
  });
});

describe("inputStyle", () => {
  it("el campo inválido no sólo lleva borde: lleva halo", () => {
    // Un borde de 1px en una forma de veinte campos no se encuentra de un
    // vistazo, y encontrarlo rápido es justo lo que se pidió.
    const malo = inputStyle(true);
    expect(malo.border).toContain("var(--red)");
    expect(malo.boxShadow).not.toBe("none");
  });

  it("el campo sano no lleva ninguno de los dos", () => {
    const bueno = inputStyle(false);
    expect(bueno.border).toContain("var(--border)");
    expect(bueno.boxShadow).toBe("none");
  });
});
