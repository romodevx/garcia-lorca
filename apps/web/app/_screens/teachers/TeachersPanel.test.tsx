import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { TeachersPanel } from "./TeachersPanel";

describe("TeachersPanel", () => {
  // Regression: an EmailField is type="text", so the browser no longer blocks
  // "ana@colegio" — and Enter saves without leaving the field, so the on-blur
  // check never runs. The panel checks the shape itself, on the field.
  // Found by /code-review on 2026-09-24
  it("marca un correo mal escrito al guardar con Enter y no lo manda", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    render(<TeachersPanel teachers={[]} onCreate={onCreate} onOpenLoad={vi.fn()} />);

    await user.type(screen.getByLabelText(/^Nombre/), "Ana");
    await user.type(screen.getByLabelText(/^Apellidos/), "Ruiz");
    await user.type(screen.getByLabelText(/^Correo/), "ana@colegio{Enter}");

    expect(screen.getByLabelText(/^Correo/)).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByText(/nombre@dominio/)).toBeInTheDocument();
    expect(onCreate).not.toHaveBeenCalled();

    // The red stays while the address is still wrong, and goes the moment it
    // is right — not on the first key.
    await user.type(screen.getByLabelText(/^Correo/), "x");
    expect(screen.getByLabelText(/^Correo/)).toHaveAttribute("aria-invalid", "true");
    await user.type(screen.getByLabelText(/^Correo/), ".mx");
    expect(screen.getByLabelText(/^Correo/)).not.toHaveAttribute("aria-invalid");
  });

  it("con un correo bien escrito sí lo crea", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    render(<TeachersPanel teachers={[]} onCreate={onCreate} onOpenLoad={vi.fn()} />);

    await user.type(screen.getByLabelText(/^Nombre/), "Ana");
    await user.type(screen.getByLabelText(/^Apellidos/), "Ruiz");
    await user.type(screen.getByLabelText(/^Correo/), "Ana@Colegio.mx{Enter}");

    expect(onCreate).toHaveBeenCalledWith({ firstName: "ANA", lastName: "RUIZ", email: "ana@colegio.mx" });
  });

  // QA 2026-09-25, ISSUE-003: the server requires the email, the help said it
  // could stay empty, and "Crear profesor" sat disabled with no reason given.
  // The button is never disabled: saving says what is missing, on the field.
  it("nunca deshabilita Crear profesor: al guardar marca lo que falta, correo incluido", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    render(<TeachersPanel teachers={[]} onCreate={onCreate} onOpenLoad={vi.fn()} />);

    await user.type(screen.getByLabelText(/^Nombre/), "Ana");
    await user.type(screen.getByLabelText(/^Apellidos/), "Torres");
    const save = screen.getByRole("button", { name: /Crear profesor/ });
    expect(save).toBeEnabled();
    await user.click(save);

    expect(onCreate).not.toHaveBeenCalled();
    expect(screen.getByLabelText(/^Correo/)).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByLabelText(/^Correo/)).toHaveFocus();
    expect(screen.getByText("Correo").parentElement).toHaveTextContent("*");
  });

  it("cada docente de la plantilla abre su carga académica", async () => {
    const user = userEvent.setup();
    const onOpenLoad = vi.fn();
    const ada = { id: "t1", firstName: "Ada", lastName: "Lovelace", email: "ada@escuela.mx", userId: "" };
    render(<TeachersPanel teachers={[ada]} onCreate={vi.fn()} onOpenLoad={onOpenLoad} />);

    await user.click(screen.getByRole("button", { name: "Carga académica de Ada Lovelace" }));

    expect(onOpenLoad).toHaveBeenCalledWith("t1");
  });
});
