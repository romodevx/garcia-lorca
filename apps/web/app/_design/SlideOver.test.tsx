import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { SlideOver } from "./SlideOver";

function open(props: Partial<React.ComponentProps<typeof SlideOver>> = {}) {
  return render(
    <SlideOver
      title="Nuevo nivel"
      subtitle="Alta rápida"
      icon="settings"
      onClose={props.onClose ?? (() => {})}
      onSubmit={props.onSubmit ?? (() => {})}
      submitLabel="Crear nivel"
      {...props}
    >
      <label>
        Nombre
        <input aria-label="Nombre" />
      </label>
    </SlideOver>,
  );
}

describe("SlideOver", () => {
  it("names itself, so a person and a test find the same thing", () => {
    open();
    expect(screen.getByRole("dialog", { name: "Nuevo nivel" })).toBeInTheDocument();
    expect(screen.getByLabelText("Nombre")).toBeInTheDocument();
  });

  // Enter saves. A form that only submits by mouse is a form somebody fills in
  // slower than the paper it replaced.
  it("saves on Enter", async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    open({ onSubmit });

    await user.type(screen.getByLabelText("Nombre"), "Primaria{Enter}");

    expect(onSubmit).toHaveBeenCalled();
  });

  it("closes on Escape and on the backdrop", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    open({ onClose });

    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledTimes(1);

    await user.click(screen.getByTestId("drawer-backdrop"));
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  // Clicking inside must not close it. Losing a half-typed form to a stray
  // click is the fastest way to make somebody stop using a screen.
  it("does not close when the panel itself is clicked", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    open({ onClose });

    await user.click(screen.getByLabelText("Nombre"));

    expect(onClose).not.toHaveBeenCalled();
  });

  // While saving, the button says so and cannot be pressed twice — one click,
  // one row.
  it("shows the save in flight and refuses a second press", async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    open({ onSubmit, pending: true });

    const save = screen.getByRole("button", { name: /Guardando/ });
    await user.click(save);

    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("shows the reason a save was refused", () => {
    open({ error: "Ya existe un nivel con ese nombre." });
    expect(screen.getByRole("alert")).toHaveTextContent("Ya existe un nivel con ese nombre.");
  });
});

describe("dónde se monta el cajón", () => {
  const base = {
    title: "Nuevo ciclo escolar",
    subtitle: "El año que la escuela va a cursar",
    icon: "calendar" as const,
    submitLabel: "Crear ciclo",
  };

  it("sale a document.body, fuera de cualquier ancestro que pueda atraparlo", () => {
    // EL DEFECTO QUE ESTO PREVIENE, visto en Configuración el 11-sep-2026:
    // en Catalog.tsx el cajón nacía dentro de <div className="fade-up">, cuya
    // animación de transform CREA UN BLOQUE CONTENEDOR. `inset: 0` dejaba de
    // significar "el viewport" y pasaba a significar "esa columna de 1320px":
    // el velo no cubría la barra superior, el cajón no llegaba al borde del
    // monitor, y se podía cerrar sesión con el cajón "modal" abierto.
    //
    // Desde document.body ningún ancestro puede volver a capturarlo.
    const { container } = render(
      <div className="fade-up" style={{ transform: "translateY(0)" }}>
        <SlideOver {...base} onClose={vi.fn()} onSubmit={vi.fn()}>
          <p>contenido</p>
        </SlideOver>
      </div>,
    );

    expect(container.querySelector("[role=dialog]")).toBeNull();
    expect(document.body.querySelector("[role=dialog]")).not.toBeNull();
  });

  it("se anuncia como modal", () => {
    render(<SlideOver {...base} onClose={vi.fn()} onSubmit={vi.fn()}><p>c</p></SlideOver>);
    expect(screen.getByRole("dialog")).toHaveAttribute("aria-modal", "true");
  });

  it("una forma grande pide un cajón grande", () => {
    // El tabulador de colegiaturas —inscripción multi-concepto, tramos,
    // colegiatura, cobranza y cuotas— se capturaba por un túnel de 520px
    // tuviera el monitor 1280 o 2560.
    render(<SlideOver {...base} size="lg" onClose={vi.fn()} onSubmit={vi.fn()}><p>c</p></SlideOver>);
    expect(screen.getByRole("dialog").className).toContain("drawer-lg");
  });

  it("el ancho vive en CSS, no en el JSX", () => {
    // Un estilo en línea le gana a cualquier @media: mientras el ancho
    // estuviera en el JSX, el cajón no podía responder al monitor.
    render(<SlideOver {...base} onClose={vi.fn()} onSubmit={vi.fn()}><p>c</p></SlideOver>);
    const dialog = screen.getByRole("dialog");
    expect(dialog.getAttribute("style") ?? "").not.toMatch(/width/);
    expect(dialog.className).toContain("drawer-md");
  });

  it("mientras está abierto, la página de atrás no se desplaza", () => {
    const { unmount } = render(
      <SlideOver {...base} onClose={vi.fn()} onSubmit={vi.fn()}><p>c</p></SlideOver>
    );
    expect(document.body.style.overflow).toBe("hidden");
    unmount();
    expect(document.body.style.overflow).toBe("");
  });
});

describe("qué cierra el cajón", () => {
  const base = {
    title: "Cobrar",
    icon: "card" as const,
    submitLabel: "Cobrar",
  };

  it("con algo a medio capturar, el velo y Esc NO lo cierran", async () => {
    // En el mostrador, perder un cobro a medio teclear por un clic fuera de
    // lugar es la forma más rápida de que alguien deje de usar la pantalla.
    const onClose = vi.fn();
    render(
      <SlideOver {...base} dismissible={false} onClose={onClose} onSubmit={vi.fn()}>
        <p>c</p>
      </SlideOver>
    );

    await userEvent.click(screen.getByTestId("drawer-backdrop"));
    await userEvent.keyboard("{Escape}");
    expect(onClose).not.toHaveBeenCalled();
  });

  it("pero el botón de cerrar SIEMPRE cierra", async () => {
    // La protección es sólo contra el accidente. Un ✕ que no hace nada deja a
    // la persona atrapada, que es peor que el cierre que se quería evitar.
    const onClose = vi.fn();
    render(
      <SlideOver {...base} dismissible={false} onClose={onClose} onSubmit={vi.fn()}>
        <p>c</p>
      </SlideOver>
    );

    await userEvent.click(screen.getByRole("button", { name: "Cerrar" }));
    expect(onClose).toHaveBeenCalled();
  });

  it("y Cancelar también", async () => {
    const onClose = vi.fn();
    render(
      <SlideOver {...base} dismissible={false} onClose={onClose} onSubmit={vi.fn()}>
        <p>c</p>
      </SlideOver>
    );

    await userEvent.click(screen.getByRole("button", { name: "Cancelar" }));
    expect(onClose).toHaveBeenCalled();
  });
});
