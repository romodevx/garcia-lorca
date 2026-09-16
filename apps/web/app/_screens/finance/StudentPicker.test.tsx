import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";

import { StudentPicker } from "./StudentPicker";

describe("StudentPicker", () => {
  it("se monta sin reventar", async () => {
    // El portal se activa en un efecto, así que hay DOS renders: uno antes de
    // montar y otro después. Cualquier hook declarado debajo del early return
    // sólo existe en el segundo, y React tira "Rendered more hooks than during
    // the previous render". Se ve al abrir Finanzas → Cobrar.
    render(
      <StudentPicker
        students={[{ id: "s1", firstName: "ANA", lastName: "RUIZ", gradeGroup: "1° A" }]}
        onPick={vi.fn()}
        onClose={vi.fn()}
      />
    );

    expect(await screen.findByRole("dialog", { name: /Buscar alumno/ })).toBeInTheDocument();
    expect(screen.getByText(/ANA/)).toBeInTheDocument();
  });
});

/**
 * Lo que se ganó al dejar de dibujar su propio diálogo.
 *
 * El buscador tuvo el suyo hasta sep-2026 y le faltaban estas dos: tabular se
 * salía a la página de atrás con el buscador abierto, y al cerrarlo el foco caía
 * en <body>, donde la siguiente tecla no va a ningún lado. Las dos fallan en
 * silencio — con el ratón no se nota — y las dos las pone la cáscara.
 */
describe("el buscador hereda el contrato de diálogo", () => {
  function Host(): ReactElement {
    const [open, setOpen] = useState(false);
    return (
      <>
        <button onClick={() => setOpen(true)}>Cobrar</button>
        <button>Otro botón de la página</button>
        {open && (
          <StudentPicker
            students={[{ id: "s1", firstName: "ANA", lastName: "RUIZ", gradeGroup: "1° A" }]}
            onPick={vi.fn()}
            onClose={() => setOpen(false)}
          />
        )}
      </>
    );
  }

  it("abre con el foco en el campo de búsqueda", async () => {
    const user = userEvent.setup();
    render(<Host />);

    await user.click(screen.getByRole("button", { name: "Cobrar" }));

    expect(screen.getByLabelText("Buscar alumno")).toHaveFocus();
  });

  it("al cerrar devuelve el foco al botón que lo abrió", async () => {
    const user = userEvent.setup();
    render(<Host />);
    const trigger = screen.getByRole("button", { name: "Cobrar" });
    await user.click(trigger);
    await user.click(screen.getByLabelText("Buscar alumno"));

    await user.keyboard("{Escape}");

    expect(trigger).toHaveFocus();
  });

  it("filtra al teclear", async () => {
    const user = userEvent.setup();
    render(<Host />);
    await user.click(screen.getByRole("button", { name: "Cobrar" }));

    await user.type(screen.getByLabelText("Buscar alumno"), "zzz");

    expect(screen.getByText("Ningún alumno con ese nombre.")).toBeInTheDocument();
  });
});
