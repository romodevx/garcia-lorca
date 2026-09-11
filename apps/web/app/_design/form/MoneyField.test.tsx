import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { MoneyField } from "./MoneyField";

async function salirDelCampo(etiqueta: string): Promise<void> {
  await userEvent.click(screen.getByLabelText(etiqueta));
  await userEvent.tab();
}

describe("MoneyField", () => {
  it("muestra el signo de pesos sin meterlo en lo que se teclea", () => {
    render(<MoneyField label="Monto" value="" onChange={vi.fn()} />);
    expect(screen.getByText("$")).toBeInTheDocument();
    expect(screen.getByLabelText("Monto")).toHaveValue("");
  });

  it("al salir del campo, formatea con separador de miles y dos decimales", async () => {
    const onChange = vi.fn();
    render(<MoneyField label="Monto" value="1850" onChange={onChange} />);
    await salirDelCampo("Monto");
    expect(onChange).toHaveBeenLastCalledWith("1,850.00");
  });

  it("completa los decimales de un monto a medias", async () => {
    const onChange = vi.fn();
    render(<MoneyField label="Monto" value="1850.5" onChange={onChange} />);
    await salirDelCampo("Monto");
    expect(onChange).toHaveBeenLastCalledWith("1,850.50");
  });

  it("no formatea lo que no es un monto — lo deja para que se corrija", async () => {
    const onChange = vi.fn();
    render(<MoneyField label="Monto" value="abc" onChange={onChange} />);
    await salirDelCampo("Monto");
    expect(onChange).not.toHaveBeenCalled();
  });

  it("la coma decimal NO se lee como separador de miles", async () => {
    // pesosToCents es estricto justo por esto: "1850,50" leído a la ligera se
    // volvía 185050 pesos — un cobro cien veces mayor, multiplicado además por
    // la generación masiva de cargos.
    const onChange = vi.fn();
    render(<MoneyField label="Monto" value="1850,50" onChange={onChange} />);
    await salirDelCampo("Monto");
    expect(onChange).not.toHaveBeenCalled();
  });

  it("un campo vacío se queda vacío, no se vuelve $0.00", async () => {
    const onChange = vi.fn();
    render(<MoneyField label="Monto" value="" onChange={onChange} />);
    await salirDelCampo("Monto");
    expect(onChange).not.toHaveBeenCalled();
  });

  it("no reformatea lo que ya estaba bien escrito", async () => {
    const onChange = vi.fn();
    render(<MoneyField label="Monto" value="1,850.00" onChange={onChange} />);
    await salirDelCampo("Monto");
    expect(onChange).not.toHaveBeenCalled();
  });

  it("mientras se teclea no estorba", async () => {
    const onChange = vi.fn();
    render(<MoneyField label="Monto" value="" onChange={onChange} />);
    await userEvent.type(screen.getByLabelText("Monto"), "185");
    expect(onChange).toHaveBeenLastCalledWith("5");
  });

  it("con error se marca inválido y limpia su rojo al teclear", async () => {
    const onClearError = vi.fn();
    render(
      <MoneyField label="Monto" value="" onChange={vi.fn()} error="Falta este dato" onClearError={onClearError} />
    );
    expect(screen.getByLabelText("Monto")).toHaveAttribute("aria-invalid", "true");
    await userEvent.type(screen.getByLabelText("Monto"), "1");
    expect(onClearError).toHaveBeenCalled();
  });
});
