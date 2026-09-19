import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { CreatePage } from "./CreatePage";

function open(): void {
  render(
    <CreatePage
      breadcrumb={["Configuración", "Colegiaturas"]}
      title="Precios de KÍNDER · 2026-2027"
      subtitle="Inscripción, colegiatura, cobranza y cuotas"
      aside={<p>Año completo</p>}
      footer={<button>Guardar tabulador</button>}
    >
      <input aria-label="Colegiatura mensual" />
    </CreatePage>,
  );
}

describe("CreatePage", () => {
  it("dice dónde se está y qué se está haciendo", () => {
    open();

    expect(
      screen.getByRole("heading", { name: "Precios de KÍNDER · 2026-2027" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Configuración › Colegiaturas ›")).toBeInTheDocument();
  });

  /**
   * Es una PÁGINA. Que no sea un diálogo no es un detalle de estilo: es lo que
   * hace que catorce campos quepan sin recurrir al flujo en dos columnas que
   * causó todo esto.
   */
  it("no es un diálogo", () => {
    open();

    expect(screen.queryByRole("dialog")).toBeNull();
  });

  // Las dos cosas que no pueden irse de la vista mientras se captura: el
  // resumen de lo que se está armando y el botón de guardar.
  it("el resumen y el guardar viven fuera del cuerpo", () => {
    open();

    expect(screen.getByTestId("create-page-aside")).toHaveTextContent("Año completo");
    expect(screen.getByTestId("create-page-footer")).toHaveTextContent("Guardar tabulador");
  });

  it("funciona sin resumen, para el alta que no tiene nada que resumir", () => {
    render(
      <CreatePage breadcrumb={["Configuración"]} title="Alta" footer={<button>Guardar</button>}>
        <input aria-label="Campo" />
      </CreatePage>,
    );

    expect(screen.queryByTestId("create-page-aside")).toBeNull();
    expect(screen.getByTestId("create-page-footer")).toBeInTheDocument();
  });
});
