import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppShell } from "./AppShell";
import { Providers } from "./test-utils";
import { changePassword as apiChangePassword, login as apiLogin, logout as apiLogout, me as apiMe, refresh as apiRefresh } from "./auth/api";

vi.mock("./auth/api", () => ({
  login: vi.fn(),
  changePassword: vi.fn(),
  logout: vi.fn(),
  // Default: no live session on boot, so every existing flow starts at login.
  refresh: vi.fn(() => Promise.reject(new Error("no session"))),
  me: vi.fn(() => Promise.reject(new Error("no session"))),
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
const mockMe = vi.mocked(apiMe);

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
    await user.click(screen.getByRole("button", { name: /Por cobrar/ }));
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
    // The gateway re-issues fresh cookies IN the change-password response, so the
    // client must NOT fire an extra refresh round-trip after the change. Capture
    // the count now (boot restore may have called refresh once) and assert the
    // change itself adds none.
    const refreshCallsBeforeChange = mockRefresh.mock.calls.length;
    await user.click(screen.getByRole("button", { name: "Guardar y entrar" }));

    expect(mockChangePassword).toHaveBeenCalledWith(expect.any(String), "test-owner-pw", "test-new-pw-123");
    expect(await screen.findByRole("heading", { name: /Módulos/ })).toBeInTheDocument();
    expect(mockRefresh.mock.calls.length).toBe(refreshCallsBeforeChange);
  });

  it("restores the session on reload when the cookie is still live (me → dashboard, no re-login)", async () => {
    // A valid HttpOnly cookie survives a page reload; boot must rehydrate the
    // session instead of dropping the user back to the login screen.
    mockMe.mockResolvedValueOnce({ userId: "u1", email: "owner@example.com", permissions: [] });
    render(
      <Providers flags={{}}>
        <AppShell />
      </Providers>
    );

    // No login submit — the boot restore lands straight on the dashboard.
    expect(await screen.findByRole("heading", { name: /Módulos/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Entrar al sistema" })).not.toBeInTheDocument();
  });

  it("shows the real authenticated identity (email + derived name), not the static owner chrome", async () => {
    // The boot restore hits /me and lands on the dashboard. The top bar must
    // reflect the REAL principal returned by the gateway — showing the actual
    // email and a name derived from it — instead of the hardcoded "Dirección"
    // owner identity that every user used to see regardless of who logged in.
    mockMe.mockResolvedValueOnce({
      userId: "u9",
      email: "patricia.lugo@garcialorca.edu.mx",
      permissions: ["grades:read"],
    });
    render(
      <Providers flags={{}}>
        <AppShell />
      </Providers>
    );

    expect(await screen.findByRole("heading", { name: /Módulos/ })).toBeInTheDocument();
    // Real email is visible in the chrome (identity subtitle).
    expect(screen.getByText("patricia.lugo@garcialorca.edu.mx")).toBeInTheDocument();
    // Name derived from the email local-part (dots → spaces, title-cased).
    expect(screen.getByText("Patricia Lugo")).toBeInTheDocument();
    // The old static owner identity must be gone.
    expect(screen.queryByText("Dirección")).not.toBeInTheDocument();
  });

  it("restores via the refresh cookie when the access cookie has expired (me → refresh → me → dashboard)", async () => {
    // /me fails (access cookie expired) but the 24h refresh cookie mints a fresh
    // one; boot must recover instead of dropping to login.
    mockMe
      .mockRejectedValueOnce(new Error("HTTP 401"))
      .mockResolvedValueOnce({ userId: "u1", email: "owner@example.com", permissions: [] });
    mockRefresh.mockResolvedValueOnce({ expiresIn: 900 });
    render(
      <Providers flags={{}}>
        <AppShell />
      </Providers>
    );

    expect(await screen.findByRole("heading", { name: /Módulos/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Entrar al sistema" })).not.toBeInTheDocument();
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

  it("opens the finance surface when the palette's 'Registrar pago' command is picked", async () => {
    mockLogin.mockResolvedValue({ expiresIn: 900, mustChangePassword: false });
    const user = userEvent.setup();
    render(
      <Providers flags={{ module_finance: true }}>
        <AppShell />
      </Providers>
    );

    await submitLogin(user);
    expect(await screen.findByRole("heading", { name: /Módulos/ })).toBeInTheDocument();

    // Cmd+K → the "Registrar pago" shortcut routes to module:finance and opens
    // the Finanzas surface (payments are now registered per charge from its table).
    fireEvent.keyDown(window, { key: "k", metaKey: true });
    await user.click(await screen.findByText("Registrar pago"));

    expect(await screen.findByRole("heading", { name: "Finanzas y Adeudos" })).toBeInTheDocument();
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
    await user.click(screen.getByRole("button", { name: /Por cobrar/ }));

    // Flag OFF → friendly stub, not the real finance table. The placeholder
    // shows the "Módulo «...»" heading; the real surface shows the status tablist.
    expect(await screen.findByRole("heading", { name: /Módulo «Finanzas y Adeudos»/ })).toBeInTheDocument();
    expect(screen.queryByRole("tablist", { name: "Filtrar por estado" })).not.toBeInTheDocument();
  });
});
