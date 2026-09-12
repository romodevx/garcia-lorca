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

  it("la ayuda se liga igual que el error", () => {
    render(<Field label="Día" hint="Del 1 al 31">{child}</Field>);
    const input = screen.getByLabelText("Día");
    expect(input.getAttribute("aria-describedby")).toBe(screen.getByText("Del 1 al 31").id);
  });

  it("cuando hay error, el error gana sobre la ayuda", () => {
    render(<Field label="Día" hint="Del 1 al 31" error="Falta este dato">{child}</Field>);
    expect(screen.queryByText("Del 1 al 31")).toBeNull();
    expect(screen.getByRole("alert")).toBeInTheDocument();
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
