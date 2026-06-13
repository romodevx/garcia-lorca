import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Login } from "./Login";

describe("Login", () => {
  it("toggles the selected profile and prefills the user field", async () => {
    const user = userEvent.setup();
    render(<Login theme="light" onToggle={() => {}} onLogin={() => {}} />);

    const adminBtn = screen.getByRole("button", { name: "Administración" });
    const teacherBtn = screen.getByRole("button", { name: "Maestro" });
    const userField = screen.getByLabelText<HTMLInputElement>("Usuario");

    expect(adminBtn).toHaveAttribute("aria-pressed", "true");
    expect(userField.value).toBe("director");

    await user.click(teacherBtn);

    expect(teacherBtn).toHaveAttribute("aria-pressed", "true");
    expect(adminBtn).toHaveAttribute("aria-pressed", "false");
    expect(userField.value).toBe("mvazquez");
  });

  it("calls onLogin with the selected role on submit", async () => {
    const user = userEvent.setup();
    const onLogin = vi.fn();
    render(<Login theme="light" onToggle={() => {}} onLogin={onLogin} />);

    await user.click(screen.getByRole("button", { name: "Maestro" }));
    await user.click(screen.getByRole("button", { name: "Entrar al sistema" }));

    expect(onLogin).toHaveBeenCalledExactlyOnceWith("teacher");
  });

  it("reveals and hides the password via the show-password toggle", async () => {
    const user = userEvent.setup();
    render(<Login theme="light" onToggle={() => {}} onLogin={() => {}} />);

    const pwd = screen.getByLabelText<HTMLInputElement>("Contraseña");
    expect(pwd.type).toBe("password");

    await user.click(screen.getByRole("button", { name: "Mostrar contraseña" }));
    expect(pwd.type).toBe("text");

    await user.click(screen.getByRole("button", { name: "Ocultar contraseña" }));
    expect(pwd.type).toBe("password");
  });
});
