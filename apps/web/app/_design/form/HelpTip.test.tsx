import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { HelpTip } from "./HelpTip";

const AYUDA = "Los 18 caracteres del acta.";

const boton = (): HTMLElement => screen.getByRole("button", { name: /ayuda sobre curp/i });

/**
 * La burbuja es SÓLO para el ojo: va `aria-hidden`, porque el texto ya le llega
 * al lector de pantalla por el `aria-describedby` del campo. Por eso hay que
 * pedirla con `hidden: true`.
 */
const burbuja = (): HTMLElement | null => screen.queryByRole("tooltip", { hidden: true });

describe("HelpTip", () => {
  it("de entrada no hay burbuja", () => {
    render(<HelpTip id="h1" label="CURP" text={AYUDA} />);
    expect(burbuja()).toBeNull();
  });

  it("el texto está en el DOM aunque la burbuja esté cerrada", () => {
    // Esto es lo que oye un lector de pantalla al llegar al campo con Tab, y
    // por eso no puede depender de que alguien descubra el (?) y lo señale.
    render(<HelpTip id="h1" label="CURP" text={AYUDA} />);
    const texto = document.getElementById("h1");
    expect(texto).not.toBeNull();
    expect(texto).toHaveTextContent(AYUDA);
  });

  it("el cursor encima la abre, y al salir se cierra", async () => {
    const user = userEvent.setup();
    render(<HelpTip id="h1" label="CURP" text={AYUDA} />);
    await user.hover(boton());
    expect(burbuja()).toBeInTheDocument();
    await user.unhover(boton());
    await waitFor(() => expect(burbuja()).toBeNull());
  });

  it("el cursor puede pasar del (?) a la burbuja sin que se desvanezca", async () => {
    // WCAG 2.2 · 1.4.13: señalable. Una burbuja que se esfuma al ir hacia ella
    // no se puede leer con lupa ni copiar.
    const user = userEvent.setup();
    render(<HelpTip id="h1" label="CURP" text={AYUDA} />);
    await user.hover(boton());
    const b = burbuja();
    expect(b).not.toBeNull();
    await user.hover(b as HTMLElement);
    // Pasado el respiro de cierre: si el cursor no la sostuviera, aquí ya no
    // estaría.
    await new Promise((listo) => setTimeout(listo, 250));
    expect(burbuja()).toBeInTheDocument();
  });

  it("se llega con Tab y eso la abre", async () => {
    const user = userEvent.setup();
    render(<HelpTip id="h1" label="CURP" text={AYUDA} />);
    await user.tab();
    expect(boton()).toHaveFocus();
    expect(burbuja()).toBeInTheDocument();
  });

  it("al irse el foco se cierra", async () => {
    const user = userEvent.setup();
    render(
      <>
        <HelpTip id="h1" label="CURP" text={AYUDA} />
        <button type="button">otro</button>
      </>,
    );
    await user.tab();
    expect(burbuja()).toBeInTheDocument();
    await user.tab();
    expect(burbuja()).toBeNull();
  });

  it("un toque la abre, y otro la cierra", async () => {
    // En la tableta del colegio no hay cursor: sin esto el (?) no hace nada.
    const user = userEvent.setup();
    render(<HelpTip id="h1" label="CURP" text={AYUDA} />);
    await user.click(boton());
    expect(burbuja()).toBeInTheDocument();
    await user.click(boton());
    expect(burbuja()).toBeNull();
  });

  it("Escape la cierra", async () => {
    const user = userEvent.setup();
    render(<HelpTip id="h1" label="CURP" text={AYUDA} />);
    await user.click(boton());
    await user.keyboard("{Escape}");
    expect(burbuja()).toBeNull();
  });

  it("ese Escape no llega al cajón que hay detrás", async () => {
    // Si se propagara, cerrar la ayuda cerraría la forma entera y se perdería
    // lo capturado. Primero se cierra lo de encima.
    const user = userEvent.setup();
    const alCerrarCajon = vi.fn();
    render(
      <div onKeyDown={alCerrarCajon}>
        <HelpTip id="h1" label="CURP" text={AYUDA} />
      </div>,
    );
    await user.click(boton());
    await user.keyboard("{Escape}");
    expect(alCerrarCajon).not.toHaveBeenCalled();
  });

  it("con la burbuja cerrada, Escape sí pasa de largo", async () => {
    // El cajón tiene que poder cerrarse con Esc como siempre; la ayuda sólo se
    // interpone cuando está abierta.
    const user = userEvent.setup();
    const alCerrarCajon = vi.fn();
    render(
      <div onKeyDown={alCerrarCajon}>
        <HelpTip id="h1" label="CURP" text={AYUDA} />
        <input aria-label="algo" />
      </div>,
    );
    screen.getByLabelText("algo").focus();
    await user.keyboard("{Escape}");
    expect(alCerrarCajon).toHaveBeenCalled();
  });

  it("el botón dice de qué campo es la ayuda", () => {
    // "Ayuda" a secas, repetido veinte veces en una forma, no distingue nada.
    render(<HelpTip id="h1" label="CURP" text={AYUDA} />);
    expect(boton()).toHaveAccessibleName("Ayuda sobre CURP");
  });

  it("la burbuja no se lee dos veces", async () => {
    // El texto ya viaja por el aria-describedby del campo.
    const user = userEvent.setup();
    render(<HelpTip id="h1" label="CURP" text={AYUDA} />);
    await user.click(boton());
    expect(burbuja()).toHaveAttribute("aria-hidden", "true");
  });

  it("la burbuja NO vive dentro del campo: cuelga del body", async () => {
    // Si colgara del campo, un ancestro con `transform` —como el cajón que
    // anima su entrada— convertiría su `position: fixed` en relativa a ese
    // ancestro, y la burbuja saldría desplazada encima del campo de al lado.
    // Pasó de verdad en el alta de alumno: la ayuda de CURP tapaba Apellidos.
    const user = userEvent.setup();
    render(
      <div style={{ transform: "translateX(0)" }} data-cajon>
        <HelpTip id="h1" label="CURP" text={AYUDA} />
      </div>,
    );
    await user.click(boton());
    const b = burbuja();
    expect(b).not.toBeNull();
    expect(b?.closest("[data-cajon]")).toBeNull();
    expect(b?.parentElement).toBe(document.body);
  });

  it("un clic DENTRO de la burbuja no la cierra", async () => {
    // Cuelga del body, así que la prueba de "clic fuera" tiene que saber que la
    // burbuja también es parte de la ayuda — si no, seleccionar su texto para
    // copiarlo la apagaría.
    const user = userEvent.setup();
    render(<HelpTip id="h1" label="CURP" text={AYUDA} />);
    await user.click(boton());
    await user.click(burbuja() as HTMLElement);
    expect(burbuja()).toBeInTheDocument();
  });

  it("el (?) no envía la forma", () => {
    render(<HelpTip id="h1" label="CURP" text={AYUDA} />);
    expect(boton()).toHaveAttribute("type", "button");
  });
});
