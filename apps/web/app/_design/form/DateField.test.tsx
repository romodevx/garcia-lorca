import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { DateField } from "./DateField";

describe("DateField", () => {
  it("es un campo de fecha de verdad, con su calendario", () => {
    // Antes se tecleaba como texto, con placeholder "2026-09-20", y había un
    // mensaje de error que explicaba el formato aaaa-mm-dd.
    render(<DateField label="Fecha de nacimiento" value="" onChange={vi.fn()} />);
    expect(screen.getByLabelText("Fecha de nacimiento")).toHaveAttribute("type", "date");
  });

  it("entrega la fecha como aaaa-mm-dd, que es lo que el API espera", async () => {
    const onChange = vi.fn();
    render(<DateField label="Fecha" value="" onChange={onChange} />);
    await userEvent.type(screen.getByLabelText("Fecha"), "2026-09-20");
    expect(onChange).toHaveBeenLastCalledWith("2026-09-20");
  });

  it("acepta un rango cuando se le da", () => {
    render(<DateField label="Fecha" value="" onChange={vi.fn()} min="2026-01-01" max="2026-12-31" />);
    const input = screen.getByLabelText("Fecha");
    expect(input).toHaveAttribute("min", "2026-01-01");
    expect(input).toHaveAttribute("max", "2026-12-31");
  });

  it("al elegir una fecha limpia su rojo", async () => {
    const onClearError = vi.fn();
    render(
      <DateField label="Fecha" value="" onChange={vi.fn()} error="Falta este dato" onClearError={onClearError} />
    );
    await userEvent.type(screen.getByLabelText("Fecha"), "2026-09-20");
    expect(onClearError).toHaveBeenCalled();
  });

  it("obligatorio se anuncia y se marca", () => {
    render(<DateField label="Fecha" value="" onChange={vi.fn()} required error="Falta este dato" />);
    const input = screen.getByLabelText("Fecha");
    expect(input).toHaveAttribute("aria-required", "true");
    expect(input).toHaveAttribute("aria-invalid", "true");
  });
});
