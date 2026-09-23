import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";

import { TextField } from "./fields";
import { useFocusFirstInvalid } from "./useFocusFirstInvalid";

/**
 * Una forma como las de verdad: se valida al PRESIONAR, se marcan todos los
 * faltantes a la vez, y entonces la vista tiene que ir al primero.
 */
function Forma({ enDialogo = false }: { enDialogo?: boolean } = {}): ReactElement {
  const [errores, setErrores] = useState<Record<string, string>>({});
  const señalarPrimerError = useFocusFirstInvalid();

  const cuerpo = (
    <>
      <TextField label="CURP" value="" onChange={vi.fn()} error={errores.curp} />
      <TextField label="Nombre" value="Ana" onChange={vi.fn()} error={errores.nombre} />
      <TextField label="Tutor" value="" onChange={vi.fn()} error={errores.tutor} />
      <button
        type="button"
        onClick={() => {
          setErrores({ curp: "Falta este dato", tutor: "Falta este dato" });
          señalarPrimerError();
        }}
      >
        Guardar
      </button>
    </>
  );

  return enDialogo ? (
    <div role="dialog" aria-modal="true">
      {cuerpo}
    </div>
  ) : (
    <div>{cuerpo}</div>
  );
}

describe("useFocusFirstInvalid", () => {
  it("al guardar con faltantes, el foco va al PRIMER campo que falla", async () => {
    // El defecto que esto cierra: el foco se quedaba en el botón. Quien captura
    // con teclado apretaba Guardar, no se movía nada, y tenía que salir a buscar
    // cuál de los campos estaba en rojo.
    const user = userEvent.setup();
    render(<Forma />);

    await user.click(screen.getByRole("button", { name: "Guardar" }));

    expect(screen.getByLabelText("CURP")).toHaveFocus();
  });

  it("el primero es el primero DE LA PANTALLA, no el primero que falló", async () => {
    // "Tutor" se marcó también, pero va más abajo. Bajar por la forma es el
    // orden que el ojo sigue.
    const user = userEvent.setup();
    render(<Forma />);

    await user.click(screen.getByRole("button", { name: "Guardar" }));

    expect(screen.getByLabelText("Tutor")).not.toHaveFocus();
  });

  it("lo desplaza hasta el centro, para que el pie pegajoso no lo tape", async () => {
    // La otra mitad del defecto: el error de Tutor existía 53 px POR DEBAJO del
    // pie del cajón, y la forma no se desplazaba (scrollTop seguía en 0). Se
    // corregían los cuatro errores visibles y seguía fallando por uno invisible.
    const user = userEvent.setup();
    const scrollIntoView = vi.fn();
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
      value: scrollIntoView,
      writable: true,
      configurable: true,
    });
    render(<Forma />);

    await user.click(screen.getByRole("button", { name: "Guardar" }));

    expect(scrollIntoView).toHaveBeenCalledWith(expect.objectContaining({ block: "center" }));
  });

  it("dentro de un cajón busca SÓLO en el cajón", async () => {
    // Detrás del cajón puede haber otra forma con lo suyo en rojo. Saltar allá
    // sacaría al capturista del cajón que tiene abierto.
    const user = userEvent.setup();
    render(
      <>
        <input aria-label="De la pantalla de atrás" aria-invalid="true" />
        <Forma enDialogo />
      </>,
    );

    await user.click(screen.getByRole("button", { name: "Guardar" }));

    expect(screen.getByLabelText("CURP")).toHaveFocus();
    expect(screen.getByLabelText("De la pantalla de atrás")).not.toHaveFocus();
  });

  it("si no falla nada, no le quita el foco a nadie", async () => {
    const user = userEvent.setup();
    function SinErrores(): ReactElement {
      const señalar = useFocusFirstInvalid();
      return (
        <>
          <TextField label="Nombre" value="Ana" onChange={vi.fn()} />
          <button type="button" onClick={señalar}>
            Guardar
          </button>
        </>
      );
    }
    render(<SinErrores />);

    const boton = screen.getByRole("button", { name: "Guardar" });
    await user.click(boton);

    expect(boton).toHaveFocus();
  });

  it("dos intentos seguidos vuelven a saltar", async () => {
    // El contador tiene que subir en cada intento: si sólo reaccionara al
    // cambio de errores, apretar Guardar dos veces con lo mismo sin llenar no
    // movería nada la segunda vez.
    const user = userEvent.setup();
    render(<Forma />);

    await user.click(screen.getByRole("button", { name: "Guardar" }));
    screen.getByRole("button", { name: "Guardar" }).focus();
    await user.click(screen.getByRole("button", { name: "Guardar" }));

    expect(screen.getByLabelText("CURP")).toHaveFocus();
  });
});
