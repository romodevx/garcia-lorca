import { type ReactElement, useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { DateField } from "./DateField";
import { EmailField, NumberField, SelectField, TextArea, TextField } from "./fields";

describe("la capa entera", () => {
  it("NINGÚN campo pinta un placeholder", () => {
    // Un valor de ejemplo en gris dentro del recuadro es indistinguible de un
    // dato capturado: quien captura duda, y en el peor caso guarda sin
    // escribir. La prop ya no existe; esto defiende que nadie la reviva.
    render(
      <>
        <TextField label="Nombre" value="" onChange={vi.fn()} />
        <EmailField label="Correo" value="" onChange={vi.fn()} />
        <NumberField label="Cupo" value="" onChange={vi.fn()} />
        <TextArea label="Mensaje" value="" onChange={vi.fn()} />
      </>
    );
    for (const campo of screen.getAllByRole("textbox")) {
      expect(campo).not.toHaveAttribute("placeholder");
    }
    expect(screen.getByLabelText("Cupo")).not.toHaveAttribute("placeholder");
  });
});

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
    // type="text" + inputMode="email": el teclado de correo en el teléfono, y
    // un cursor que el navegador deja leer. type="email" no expone la
    // selección, así que al bajar a minúsculas el cursor saltaba al final.
    expect(input).toHaveAttribute("type", "text");
    expect(input).toHaveAttribute("inputmode", "email");
    // Every EmailField captures SOMEBODY ELSE'S address (a teacher, a contact,
    // a new user): the browser must not offer the director's own.
    expect(input).toHaveAttribute("autocomplete", "off");
  });

  it("baja a minúsculas al terminar una composición del teclado", async () => {
    const onChange = vi.fn();
    render(<EmailField label="Correo" value="" onChange={onChange} />);
    const input = screen.getByLabelText<HTMLInputElement>("Correo");
    input.value = "Ana";
    fireEvent.compositionEnd(input);
    expect(onChange).toHaveBeenLastCalledWith("ana");
  });

  it("al salir del campo avisa, en español, si el correo no tiene forma de correo", async () => {
    function Controlado(): ReactElement {
      const [v, setV] = useState("");
      return <EmailField label="Correo" value={v} onChange={setV} />;
    }
    render(<Controlado />);
    const input = screen.getByLabelText("Correo");
    await userEvent.type(input, "ana@gmail");
    await userEvent.tab();
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByText(/nombre@dominio/)).toBeInTheDocument();

    await userEvent.type(input, ".com");
    expect(input).not.toHaveAttribute("aria-invalid");
  });

  it("vacío no es un error de forma: si es obligatorio lo dice quien lo usa", async () => {
    render(<EmailField label="Correo" value="" onChange={vi.fn()} />);
    const input = screen.getByLabelText("Correo");
    await userEvent.click(input);
    await userEvent.tab();
    expect(input).not.toHaveAttribute("aria-invalid");
  });

  it("al terminar una composición también se limpia el error del campo", () => {
    const onClearError = vi.fn();
    render(<EmailField label="Correo" value="" onChange={vi.fn()} error="Falta el correo" onClearError={onClearError} />);
    const input = screen.getByLabelText<HTMLInputElement>("Correo");
    input.value = "ana@colegio.mx";
    fireEvent.compositionEnd(input);
    expect(onClearError).toHaveBeenCalled();
  });

  it("corregir a media dirección no revuelve el correo: el cursor se queda donde estaba", async () => {
    function Controlado(): ReactElement {
      const [v, setV] = useState("ana.ruiz@colegio.mx");
      return <EmailField label="Correo" value={v} onChange={setV} />;
    }
    render(<Controlado />);
    const input = screen.getByLabelText("Correo");
    await userEvent.type(input, "Maria", { initialSelectionStart: 4, initialSelectionEnd: 4 });
    expect(input).toHaveValue("ana.mariaruiz@colegio.mx");
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

  it("avisa al salir del campo, donde la pantalla acusa un dato mal escrito", async () => {
    const onBlur = vi.fn();
    render(<NumberField label="Calificación" value="" onChange={vi.fn()} onBlur={onBlur} />);
    await userEvent.click(screen.getByLabelText("Calificación"));
    await userEvent.tab();
    expect(onBlur).toHaveBeenCalledTimes(1);
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
  it("arranca en una opción vacía, y esa opción NO tiene letras", () => {
    // La opción vacía es lo único que impide que el navegador deje
    // preseleccionada la primera opción real y guarde un nivel que nadie
    // eligió. Pero va SIN texto: "Selecciona un nivel…" dentro del recuadro se
    // lee como un dato ya capturado, que es justo lo que este trabajo quita.
    render(
      <SelectField
        label="Nivel"
        value=""
        onChange={vi.fn()}
        options={[{ value: "k", label: "Kínder" }]}
      />
    );
    const opciones = screen.getAllByRole("option");
    expect(opciones[0]).toHaveValue("");
    expect(opciones[0]).toHaveTextContent("");
  });

  it("con el select vacío, no queda elegida la primera opción real", () => {
    render(
      <SelectField
        label="Nivel"
        value=""
        onChange={vi.fn()}
        options={[{ value: "k", label: "Kínder" }, { value: "p", label: "Primaria" }]}
      />
    );
    expect(screen.getByLabelText("Nivel")).toHaveValue("");
  });

  it("la ayuda del select entra por el (?), como en todos los demás", () => {
    render(
      <SelectField
        label="Nivel"
        value=""
        onChange={vi.fn()}
        help="El nivel educativo donde va a cursar."
        options={[{ value: "k", label: "Kínder" }]}
      />
    );
    expect(screen.getByRole("button", { name: "Ayuda sobre Nivel" })).toBeInTheDocument();
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

describe("onBlur", () => {
  // Validar al salir del campo es la regla de formas; la pantalla necesita
  // enterarse de cuándo se sale para acusar un dato mal escrito.
  it("avisa al salir del campo de texto, de correo y de fecha", () => {
    const text = vi.fn();
    const email = vi.fn();
    const date = vi.fn();
    render(
      <>
        <TextField label="CP" value="" onChange={vi.fn()} onBlur={text} />
        <EmailField label="Correo" value="" onChange={vi.fn()} onBlur={email} />
        <DateField label="Nacimiento" value="" onChange={vi.fn()} onBlur={date} />
      </>
    );
    fireEvent.blur(screen.getByLabelText("CP"));
    fireEvent.blur(screen.getByLabelText("Correo"));
    fireEvent.blur(screen.getByLabelText("Nacimiento"));
    expect(text).toHaveBeenCalledTimes(1);
    expect(email).toHaveBeenCalledTimes(1);
    expect(date).toHaveBeenCalledTimes(1);
  });
});
