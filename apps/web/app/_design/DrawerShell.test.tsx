import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";

import { DrawerShell } from "./DrawerShell";

/**
 * El contrato de diálogo de WAI-ARIA APG, probado UNA vez sobre la cáscara y
 * heredado por las trece pantallas que la usan.
 *
 * Las tres partes fallan EN SILENCIO: el ratón sigue funcionando, la captura de
 * pantalla se ve bien, y la única persona que se entera es quien captura con el
 * teclado — que en un colegio suele ser la persona más rápida del mostrador.
 */
function Host(): ReactElement {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button onClick={() => setOpen(true)}>Nuevo nivel</button>
      <button>Otro botón de la página</button>
      {open && (
        <DrawerShell title="Nuevo nivel" icon="settings" onClose={() => setOpen(false)}>
          <input aria-label="Nombre" />
          <input aria-label="Clave" />
        </DrawerShell>
      )}
    </>
  );
}

describe("el contrato de foco del cajón", () => {
  it("pone el foco en el primer campo al abrir", async () => {
    const user = userEvent.setup();
    render(<Host />);

    await user.click(screen.getByRole("button", { name: "Nuevo nivel" }));

    expect(screen.getByLabelText("Nombre")).toHaveFocus();
  });

  it("no deja que Tab se escape a la página de atrás", async () => {
    const user = userEvent.setup();
    render(<Host />);
    await user.click(screen.getByRole("button", { name: "Nuevo nivel" }));

    // El foco se mete al ÚLTIMO campo a propósito: el borde es donde se escapa.
    // Desde ahí, Clave → Cerrar → debe dar la vuelta a Nombre, nunca salir a
    // "Otro botón de la página".
    await user.click(screen.getByLabelText("Clave"));
    await user.tab();
    await user.tab();

    expect(screen.getByLabelText("Nombre")).toHaveFocus();
  });

  it("devuelve el foco al botón que lo abrió", async () => {
    const user = userEvent.setup();
    render(<Host />);
    const trigger = screen.getByRole("button", { name: "Nuevo nivel" });
    await user.click(trigger);
    // Sin esto la prueba pasa sola: si el foco nunca entró al cajón, al cerrar
    // sigue en el disparador por inercia y no se prueba nada.
    await user.click(screen.getByLabelText("Nombre"));

    await user.keyboard("{Escape}");

    expect(trigger).toHaveFocus();
  });
});

describe("dónde se presenta", () => {
  /**
   * Al centro, no contra un borde.
   *
   * Nació como cajón lateral y, mientras midió toda la pantalla, el anclaje a la
   * derecha se leía como un panel. Al pasar a medir lo que mide su forma, ese
   * mismo anclaje lo dejó flotando contra un borde con el resto de la pantalla
   * vacío. Una forma que no toca ningún borde se lee como lo que es: una sola
   * cosa que atender.
   */
  it("se presenta al centro, no contra un borde", () => {
    render(
      <DrawerShell title="Nuevo ciclo escolar" icon="calendar" onClose={() => {}}>
        <input aria-label="Nombre" />
      </DrawerShell>,
    );

    expect(screen.getByTestId("drawer-backdrop")).toHaveStyle({
      justifyContent: "center",
      alignItems: "center",
    });
  });
});

describe("la altura del cajón", () => {
  it("se ajusta a su contenido en vez de estirarse a la pantalla", () => {
    render(
      <DrawerShell title="Nuevo día cerrado" icon="calendar" onClose={() => {}}>
        <input aria-label="Nombre" />
      </DrawerShell>,
    );

    // Un alta de tres campos flotando arriba de un metro de blanco, con Guardar
    // clavado hasta abajo, obliga al ojo a recorrer el monitor entero para nada.
    const panel = screen.getByRole("dialog");
    expect(panel).toHaveStyle({ height: "auto", maxHeight: "85vh" });
  });
});

describe("WCAG 2.2 · 2.4.11 — el pie no cubre el campo enfocado", () => {
  it("pide desplazar lo mínimo cuando un campo recibe el foco", async () => {
    const user = userEvent.setup();
    const scrollIntoView = vi.fn();
    // jsdom no implementa scrollIntoView; se espía que el cajón lo pida.
    Element.prototype.scrollIntoView = scrollIntoView;

    render(
      <DrawerShell title="Alta" icon="settings" onClose={() => {}} footer={<button>Guardar</button>}>
        <input aria-label="Primero" />
        <input aria-label="Último" />
      </DrawerShell>,
    );

    await user.click(screen.getByLabelText("Último"));

    expect(scrollIntoView).toHaveBeenCalledWith({ block: "nearest", behavior: "smooth" });
  });
});
