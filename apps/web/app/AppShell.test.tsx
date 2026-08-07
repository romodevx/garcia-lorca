import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppShell } from "./AppShell";
import { Providers } from "./test-utils";
import { changePassword as apiChangePassword, login as apiLogin, logout as apiLogout, refresh as apiRefresh } from "./auth/api";

vi.mock("./auth/api", () => ({
  login: vi.fn(),
  changePassword: vi.fn(),
  logout: vi.fn(),
  refresh: vi.fn(),
}));

// The finance surface is the only screen these flows mount that fetches on boot;
// stub its clients so navigation is tested without touching the network (the
// surface itself is covered by app/_screens/finance/Finance.test.tsx).
vi.mock("./finance/api", () => ({
  listConcepts: vi.fn().mockResolvedValue([]),
  createConcept: vi.fn(),
  updateConcept: vi.fn(),
  listCharges: vi.fn().mockResolvedValue([]),
  generateCharges: vi.fn(),
  getOverdue: vi.fn().mockResolvedValue([]),
  registerPayment: vi.fn(),
  getAccount: vi.fn(),
}));

vi.mock("./config/api", () => ({
  listCycles: vi.fn().mockResolvedValue([]),
  listGroups: vi.fn().mockResolvedValue([]),
}));

vi.mock("./students/api", () => ({
  listStudents: vi.fn().mockResolvedValue([]),
}));

const mockLogin = vi.mocked(apiLogin);
const mockChangePassword = vi.mocked(apiChangePassword);
const mockLogout = vi.mocked(apiLogout);
const mockRefresh = vi.mocked(apiRefresh);

afterEach(() => vi.clearAllMocks());

async function submitLogin(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await user.type(screen.getByLabelText("Correo"), "owner@example.com");
  await user.type(screen.getByLabelText("Contraseña"), "test-owner-pw");
  await user.click(screen.getByRole("button", { name: "Entrar al sistema" }));
}

describe("AppShell navigation flow", () => {
  it("goes login → dashboard → finance → back, then logs out", async () => {
    mockLogin.mockResolvedValue({ expiresIn: 900, mustChangePassword: false });
    mockLogout.mockResolvedValue(undefined);
    const user = userEvent.setup();
    render(
      <Providers flags={{ module_finance: true }}>
        <AppShell />
      </Providers>
    );

    await submitLogin(user);
    expect(await screen.findByRole("heading", { name: /Módulos/ })).toBeInTheDocument();

    // Open Finanzas (KPI shortcut) — finance flag is ON, so the real surface shows.
    await user.click(screen.getByRole("button", { name: /Adeudos del mes/ }));
    expect(await screen.findByRole("heading", { name: "Finanzas y Adeudos" })).toBeInTheDocument();

    // Back home.
    await user.click(screen.getByRole("button", { name: /Volver al inicio/ }));
    expect(await screen.findByRole("heading", { name: /Módulos/ })).toBeInTheDocument();

    // Logout → server call to expire the cookie, then back to login.
    await user.click(screen.getByRole("button", { name: "Cerrar sesión" }));
    expect(await screen.findByRole("button", { name: "Entrar al sistema" })).toBeInTheDocument();
    expect(mockLogout).toHaveBeenCalledWith(expect.any(String));
  });

  it("routes to the forced change-password screen and then to the dashboard", async () => {
    mockLogin.mockResolvedValue({ expiresIn: 900, mustChangePassword: true });
    mockChangePassword.mockResolvedValue(undefined);
    mockRefresh.mockResolvedValue({ expiresIn: 900 });
    const user = userEvent.setup();
    render(
      <Providers flags={{ module_finance: true }}>
        <AppShell />
      </Providers>
    );

    await submitLogin(user);

    // mustChangePassword=true → forced rotation screen.
    expect(await screen.findByRole("heading", { name: "Crea tu contraseña" })).toBeInTheDocument();

    await user.type(screen.getByLabelText("Contraseña actual"), "test-owner-pw");
    await user.type(screen.getByLabelText("Nueva contraseña"), "test-new-pw-123");
    await user.type(screen.getByLabelText("Confirmar contraseña"), "test-new-pw-123");
    await user.click(screen.getByRole("button", { name: "Guardar y entrar" }));

    expect(mockChangePassword).toHaveBeenCalledWith(expect.any(String), "test-owner-pw", "test-new-pw-123");
    // The cookie must be rotated after the change so it carries mcp=false.
    expect(mockRefresh).toHaveBeenCalledWith(expect.any(String));
    expect(await screen.findByRole("heading", { name: /Módulos/ })).toBeInTheDocument();
  });

  it("shows an inline error when login fails", async () => {
    mockLogin.mockRejectedValue(new Error("HTTP 401"));
    const user = userEvent.setup();
    render(
      <Providers flags={{}}>
        <AppShell />
      </Providers>
    );

    await submitLogin(user);

    expect(await screen.findByRole("alert")).toHaveTextContent("Correo o contraseña incorrectos.");
    expect(screen.getByRole("button", { name: "Entrar al sistema" })).toBeInTheDocument();
  });

  it("lands on the ventanilla when the palette's 'Registrar pago' command is picked", async () => {
    mockLogin.mockResolvedValue({ expiresIn: 900, mustChangePassword: false });
    const user = userEvent.setup();
    render(
      <Providers flags={{ module_finance: true }}>
        <AppShell />
      </Providers>
    );

    await submitLogin(user);
    expect(await screen.findByRole("heading", { name: /Módulos/ })).toBeInTheDocument();

    // Cmd+K → the create shortcut routes to module:finance:payment, so the
    // surface opens ON the payment form instead of the debts table.
    fireEvent.keyDown(window, { key: "k", metaKey: true });
    await user.click(await screen.findByText("Registrar pago"));

    expect(await screen.findByLabelText("Cargo")).toBeInTheDocument();
  });

  it("falls back to the placeholder when module_finance is OFF (UX gating)", async () => {
    mockLogin.mockResolvedValue({ expiresIn: 900, mustChangePassword: false });
    const user = userEvent.setup();
    render(
      <Providers flags={{}}>
        <AppShell />
      </Providers>
    );

    await submitLogin(user);
    await user.click(screen.getByRole("button", { name: /Adeudos del mes/ }));

    // Flag OFF → friendly stub, not the real finance table. The placeholder
    // shows the "Módulo «...»" heading; the real surface shows the status tablist.
    expect(await screen.findByRole("heading", { name: /Módulo «Finanzas y Adeudos»/ })).toBeInTheDocument();
    expect(screen.queryByRole("tablist", { name: "Filtrar por estado" })).not.toBeInTheDocument();
  });
});
