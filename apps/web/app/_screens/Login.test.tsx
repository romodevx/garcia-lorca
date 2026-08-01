import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Login } from "./Login";

describe("Login", () => {
  it("calls onSubmit with the typed email and password", async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    render(<Login theme="light" onToggle={() => {}} onSubmit={onSubmit} />);

    await user.type(screen.getByLabelText("Correo"), "director@garcialorca.edu.mx");
    await user.type(screen.getByLabelText("Contraseña"), "GarciaLorca2026!");
    await user.click(screen.getByRole("button", { name: "Entrar al sistema" }));

    expect(onSubmit).toHaveBeenCalledExactlyOnceWith("director@garcialorca.edu.mx", "GarciaLorca2026!");
  });

  it("reveals and hides the password via the show-password toggle", async () => {
    const user = userEvent.setup();
    render(<Login theme="light" onToggle={() => {}} onSubmit={vi.fn()} />);

    const pwd = screen.getByLabelText<HTMLInputElement>("Contraseña");
    expect(pwd.type).toBe("password");

    await user.click(screen.getByRole("button", { name: "Mostrar contraseña" }));
    expect(pwd.type).toBe("text");

    await user.click(screen.getByRole("button", { name: "Ocultar contraseña" }));
    expect(pwd.type).toBe("password");
  });

  it("renders the error as an alert and shows a pending label", () => {
    render(<Login theme="light" onToggle={() => {}} onSubmit={vi.fn()} error="Correo o contraseña incorrectos." pending />);

    expect(screen.getByRole("alert")).toHaveTextContent("Correo o contraseña incorrectos.");
    expect(screen.getByRole("button", { name: "Entrando…" })).toBeDisabled();
  });
});
