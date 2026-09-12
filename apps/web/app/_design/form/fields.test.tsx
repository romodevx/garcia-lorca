import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { EmailField, NumberField, SelectField, TextArea, TextField } from "./fields";

describe("TextField", () => {
  it("con uppercase, lo capturado sube a mayúsculas", async () => {
    const onChange = vi.fn();
    render(<TextField label="Alumno" value="" onChange={onChange} uppercase />);
    await userEvent.type(screen.getByLabelText("Alumno"), "a");
    expect(onChange).toHaveBeenCalledWith("A");
  });

  it("respeta los acentos y la eñe al subir", async () => {
    const onChange = vi.fn();
    render(<TextField label="Alumno" value="" onChange={onChange} uppercase />);
    await userEvent.type(screen.getByLabelText("Alumno"), "á");
    expect(onChange).toHaveBeenCalledWith("Á");
    onChange.mockClear();
    await userEvent.type(screen.getByLabelText("Alumno"), "ñ");
    expect(onChange).toHaveBeenCalledWith("Ñ");
  });

  it("sin uppercase no toca lo que se escribe", async () => {
    const onChange = vi.fn();
    render(<TextField label="Nota" value="" onChange={onChange} />);
    await userEvent.type(screen.getByLabelText("Nota"), "a");
    expect(onChange).toHaveBeenCalledWith("a");
  });

  it("transforma al CAPTURAR, no al pintar", async () => {
    // Guardar tal cual y sólo pintar en mayúsculas dejaría "Ana Ruiz" y
    // "ana ruiz" idénticos en pantalla y distintos en la base.
    const onChange = vi.fn();
    render(<TextField label="Alumno" value="" onChange={onChange} uppercase />);
    const input = screen.getByLabelText("Alumno");
    await userEvent.type(input, "ana");
    expect(onChange).toHaveBeenLastCalledWith("A");
    expect(input).not.toHaveStyle({ textTransform: "uppercase" });
  });

  it("al escribir avisa que se limpie su rojo", async () => {
    const onClearError = vi.fn();
    render(
      <TextField label="Alumno" value="" onChange={vi.fn()} error="Falta este dato" onClearError={onClearError} />
    );
    await userEvent.type(screen.getByLabelText("Alumno"), "x");
    expect(onClearError).toHaveBeenCalled();
  });

  it("con error se marca inválido para el lector de pantalla", () => {
    render(<TextField label="Alumno" value="" onChange={vi.fn()} error="Falta este dato" />);
    expect(screen.getByLabelText("Alumno")).toHaveAttribute("aria-invalid", "true");
  });

  it("sin error NO se marca inválido", () => {
    render(<TextField label="Alumno" value="" onChange={vi.fn()} />);
    expect(screen.getByLabelText("Alumno")).not.toHaveAttribute("aria-invalid", "true");
  });

  it("obligatorio se anuncia, no sólo se pinta", () => {
    render(<TextField label="Alumno" value="" onChange={vi.fn()} required />);
    expect(screen.getByLabelText("Alumno")).toHaveAttribute("aria-required", "true");
  });
});

describe("EmailField", () => {
  it("SIEMPRE baja a minúsculas", async () => {
    const onChange = vi.fn();
    render(<EmailField label="Correo" value="" onChange={onChange} />);
    await userEvent.type(screen.getByLabelText("Correo"), "A");
    expect(onChange).toHaveBeenCalledWith("a");
  });

  it("no deja que el teclado del sistema ponga mayúscula inicial", () => {
    render(<EmailField label="Correo" value="" onChange={vi.fn()} />);
    const input = screen.getByLabelText("Correo");
    expect(input).toHaveAttribute("autocapitalize", "none");
    expect(input).toHaveAttribute("spellcheck", "false");
    expect(input).toHaveAttribute("type", "email");
  });
});

describe("TextArea", () => {
  it("NO sube a mayúsculas — un aviso a los papás no se grita", async () => {
    const onChange = vi.fn();
    render(<TextArea label="Mensaje" value="" onChange={onChange} />);
    await userEvent.type(screen.getByLabelText("Mensaje"), "M");
    expect(onChange).toHaveBeenCalledWith("M");
  });

  it("es un área de varias líneas, no un renglón", () => {
    render(<TextArea label="Mensaje" value="" onChange={vi.fn()} />);
    expect(screen.getByLabelText("Mensaje").tagName).toBe("TEXTAREA");
  });
});

describe("NumberField", () => {
  it("entrega el texto tal cual, sin convertir a número", async () => {
    // Convertir aquí volvía "" en 0, y un 0 accidental en una calificación o en
    // una cantidad es un dato falso que nadie escribió.
    const onChange = vi.fn();
    render(<NumberField label="Calificación" value="" onChange={onChange} />);
    await userEvent.type(screen.getByLabelText("Calificación"), "8");
    expect(onChange).toHaveBeenCalledWith("8");
  });

  it("acepta el rango que se le dé", () => {
    render(<NumberField label="Calificación" value="" onChange={vi.fn()} min={0} max={10} step={0.1} />);
    const input = screen.getByLabelText("Calificación");
    expect(input).toHaveAttribute("min", "0");
    expect(input).toHaveAttribute("max", "10");
    expect(input).toHaveAttribute("step", "0.1");
  });
});

describe("SelectField", () => {
  it("muestra el texto de reposo cuando no hay nada elegido", () => {
    render(
      <SelectField
        label="Nivel"
        value=""
        onChange={vi.fn()}
        placeholder="Elige un nivel"
        options={[{ value: "k", label: "Kínder" }]}
      />
    );
    expect(screen.getByRole("option", { name: "Elige un nivel" })).toBeInTheDocument();
  });

  it("al elegir limpia su rojo", async () => {
    const onClearError = vi.fn();
    render(
      <SelectField
        label="Nivel"
        value=""
        onChange={vi.fn()}
        error="Falta este dato"
        onClearError={onClearError}
        options={[{ value: "k", label: "Kínder" }]}
      />
    );
    await userEvent.selectOptions(screen.getByLabelText("Nivel"), "k");
    expect(onClearError).toHaveBeenCalled();
  });
});
