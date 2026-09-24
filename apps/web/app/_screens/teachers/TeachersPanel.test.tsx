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
    render(<TeachersPanel teachers={[]} onCreate={onCreate} />);

    await user.type(screen.getByLabelText(/^Nombre/), "Ana");
    await user.type(screen.getByLabelText(/^Apellidos/), "Ruiz");
    await user.type(screen.getByLabelText(/^Correo/), "ana@colegio{Enter}");

    expect(screen.getByLabelText(/^Correo/)).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByText(/nombre@dominio/)).toBeInTheDocument();
    expect(onCreate).not.toHaveBeenCalled();
  });

  it("con un correo bien escrito sí lo crea", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    render(<TeachersPanel teachers={[]} onCreate={onCreate} />);

    await user.type(screen.getByLabelText(/^Nombre/), "Ana");
    await user.type(screen.getByLabelText(/^Apellidos/), "Ruiz");
    await user.type(screen.getByLabelText(/^Correo/), "Ana@Colegio.mx{Enter}");

    expect(onCreate).toHaveBeenCalledWith({ firstName: "ANA", lastName: "RUIZ", email: "ana@colegio.mx" });
  });
});
