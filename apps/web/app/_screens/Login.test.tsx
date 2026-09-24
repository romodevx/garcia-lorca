import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Login } from "./Login";

describe("Login", () => {
  it("calls onSubmit with the typed email and password", async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    render(<Login theme="light" onToggle={() => {}} onSubmit={onSubmit} />);

    await user.type(screen.getByLabelText("Correo"), "owner@example.com");
    await user.type(screen.getByLabelText("Contraseña"), "test-owner-pw");
    await user.click(screen.getByRole("button", { name: "Entrar al sistema" }));

    expect(onSubmit).toHaveBeenCalledExactlyOnceWith("owner@example.com", "test-owner-pw");
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

describe("el correo de la entrada", () => {
  it("se envía sin los espacios que trae un correo pegado", async () => {
    // type="text" (para no perder el cursor) no recorta como type="email".
    const user = userEvent.setup();
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    render(<Login theme="light" onToggle={() => {}} onSubmit={onSubmit} pending={false} />);

    await user.click(screen.getByLabelText("Correo"));
    await user.paste(" ana@colegio.mx ");
    await user.type(screen.getByLabelText("Contraseña"), "pw-123");
    await user.click(screen.getByRole("button", { name: "Entrar al sistema" }));

    expect(onSubmit).toHaveBeenCalledWith("ana@colegio.mx", "pw-123");
  });

  it("baja a minúsculas mientras se teclea", async () => {
    // La regla es del sistema entero, y la puerta es la primera pantalla donde
    // se escribe un correo. Además evita el caso clásico del teclado que pone
    // mayúscula inicial y deja a alguien fuera sin entender por qué.
    const user = userEvent.setup();
    render(<Login theme="light" onToggle={() => {}} onSubmit={vi.fn()} pending={false} />);

    const correo = screen.getByLabelText("Correo");
    await user.type(correo, "Director@GarciaLorca.edu.MX");

    expect(correo).toHaveValue("director@garcialorca.edu.mx");
  });

  it("corregir a media dirección deja el cursor donde estaba", async () => {
    const user = userEvent.setup();
    render(<Login theme="light" onToggle={() => {}} onSubmit={vi.fn()} pending={false} />);

    const correo = screen.getByLabelText("Correo");
    await user.type(correo, "ana.ruiz@colegio.mx");
    await user.type(correo, "Maria", { initialSelectionStart: 4, initialSelectionEnd: 4 });

    expect(correo).toHaveValue("ana.mariaruiz@colegio.mx");
  });

  it("no deja que el teclado del sistema ponga la mayúscula inicial", () => {
    render(<Login theme="light" onToggle={() => {}} onSubmit={vi.fn()} pending={false} />);
    const correo = screen.getByLabelText("Correo");
    expect(correo).toHaveAttribute("autocapitalize", "none");
    expect(correo).toHaveAttribute("spellcheck", "false");
  });
});

describe("el correo de «¿Olvidaste tu contraseña?»", () => {
  it("también baja a minúsculas mientras se teclea", async () => {
    const user = userEvent.setup();
    render(<Login theme="light" onToggle={() => {}} onSubmit={vi.fn()} pending={false} />);

    await user.click(screen.getByRole("button", { name: "¿Olvidaste tu contraseña?" }));
    const correo = screen.getByLabelText("¿Cuál es tu correo?");
    await user.type(correo, "Ana.Ruiz@Colegio.MX");

    expect(correo).toHaveValue("ana.ruiz@colegio.mx");
  });

  it("corregir a media dirección no revuelve el correo al que se manda el enlace", async () => {
    const user = userEvent.setup();
    render(<Login theme="light" onToggle={() => {}} onSubmit={vi.fn()} pending={false} />);

    await user.click(screen.getByRole("button", { name: "¿Olvidaste tu contraseña?" }));
    const correo = screen.getByLabelText("¿Cuál es tu correo?");
    await user.type(correo, "ana.ruiz@colegio.mx");
    await user.type(correo, "Maria", { initialSelectionStart: 4, initialSelectionEnd: 4 });

    expect(correo).toHaveValue("ana.mariaruiz@colegio.mx");
  });
});
