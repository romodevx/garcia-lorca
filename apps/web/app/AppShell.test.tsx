import { act, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
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
  getIncome: vi.fn().mockResolvedValue({ lines: [], totalCents: 0 }),
  getCashUp: vi.fn().mockResolvedValue({ deposits: [], cancelled: [], cashCents: 0, cardCents: 0, transferCents: 0, totalCents: 0 }),
  getExpected: vi.fn().mockResolvedValue({ amountCents: 0, surchargeCents: 0, discountCents: 0, annualBalanceCents: 0 }),
  previewDeposit: vi.fn(),
  registerDeposit: vi.fn(),
}));

vi.mock("./config/api", () => ({
  listCycles: vi.fn().mockResolvedValue([]),
  listGroups: vi.fn().mockResolvedValue([]),
}));

vi.mock("./students/api", () => ({
  listStudents: vi.fn().mockResolvedValue([]),
}));

// The signed-in principal for the navigation flows: READ on the modules these
// tests reach (finance + students, for the dashboard KPI shortcuts). Module
// tiles/KPIs are now permission-gated, so a flow that clicks one must resolve
// `me()` with that permission.
const OWNER_ME = {
  userId: "u1",
  email: "owner@example.com",
  permissions: ["students:read", "finance:read", "grades:read", "comms:read", "config:read", "users:read", "attendance:read"],
};

const mockLogin = vi.mocked(apiLogin);
const mockChangePassword = vi.mocked(apiChangePassword);
const mockLogout = vi.mocked(apiLogout);
const mockRefresh = vi.mocked(apiRefresh);
const mockMe = vi.mocked(apiMe);

// A persistent `mockResolvedValue` (used by the nav flows below) survives
// clearAllMocks, so reset the session probes to their default "no session"
// baseline before every test — each flow then opts into a live session
// explicitly. Without this, a resolved `me()` would leak into the login-first
// and boot-restore tests.
beforeEach(() => {
  mockMe.mockReset();
  mockRefresh.mockReset();
  mockMe.mockRejectedValue(new Error("no session"));
  mockRefresh.mockRejectedValue(new Error("no session"));
});

afterEach(() => vi.clearAllMocks());

async function submitLogin(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await user.type(screen.getByLabelText("Correo"), "owner@example.com");
  await user.type(screen.getByLabelText("Contraseña"), "test-owner-pw");
  await user.click(screen.getByRole("button", { name: "Entrar al sistema" }));
}

describe("AppShell navigation flow", () => {
  it("goes login → dashboard → finance → back, then logs out", async () => {
    mockLogin.mockResolvedValue({ expiresIn: 900, mustChangePassword: false });
    // Boot restore fails (stay at login); the post-login /me returns the principal.
    mockMe.mockRejectedValueOnce(new Error("no session")).mockResolvedValue(OWNER_ME);
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
    expect(await screen.findByRole("heading", { name: "Finanzas" })).toBeInTheDocument();

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

  it("shows the gateway's real display name (not the email-derived one), with the email as the subtitle", async () => {
    // The boot restore hits /me and lands on the dashboard. When the gateway
    // returns a real displayName, the top bar must show THAT as the bold name —
    // not a name derived from the email local-part — and never the old static
    // "Dirección" owner identity. The email stays the identity subtitle.
    mockMe.mockResolvedValueOnce({
      userId: "u9",
      email: "director@garcialorca.edu.mx",
      permissions: ["grades:read"],
      displayName: "Patricia Lugo",
      role: "owner",
    });
    render(
      <Providers flags={{}}>
        <AppShell />
      </Providers>
    );

    expect(await screen.findByRole("heading", { name: /Módulos/ })).toBeInTheDocument();
    // The real email is the identity subtitle.
    expect(screen.getByText("director@garcialorca.edu.mx")).toBeInTheDocument();
    // The gateway's real display name wins the bold name line.
    expect(screen.getByText("Patricia Lugo")).toBeInTheDocument();
    // NOT the email-derived name ("Director"), and NOT the old static owner row.
    expect(screen.queryByText("Director")).not.toBeInTheDocument();
    expect(screen.queryByText("Dirección")).not.toBeInTheDocument();
    // The dashboard greeting uses only the FIRST name token, not the full display
    // name — "Patricia Lugo" greets as "Patricia", never "Patricia Lugo 🌱".
    expect(screen.getByRole("heading", { name: /,\s*Patricia\s*🌱/ })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /Patricia Lugo\s*🌱/ })).not.toBeInTheDocument();
  });

  it("falls back to the email-derived name when the gateway returns no display name", async () => {
    // A directory outage degrades /me to the token principal (no displayName), so
    // the chrome falls back to a name derived from the email local-part.
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
    // Real email is the subtitle.
    expect(screen.getByText("patricia.lugo@garcialorca.edu.mx")).toBeInTheDocument();
    // Name derived from the email local-part (dots → spaces, title-cased).
    expect(screen.getByText("Patricia Lugo")).toBeInTheDocument();
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

    expect(await screen.findByRole("heading", { name: "Finanzas" })).toBeInTheDocument();
  });

  it("falls back to the placeholder when module_finance is OFF (UX gating)", async () => {
    mockLogin.mockResolvedValue({ expiresIn: 900, mustChangePassword: false });
    // The caller CAN read finance (KPI shortcut visible); the module flag being
    // OFF is what routes to the placeholder — permission governs visibility,
    // flag governs real-surface-vs-stub. Boot restore fails, post-login /me resolves.
    mockMe.mockRejectedValueOnce(new Error("no session")).mockResolvedValue(OWNER_ME);
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
    expect(await screen.findByRole("heading", { name: /Módulo «Finanzas»/ })).toBeInTheDocument();
    expect(screen.queryByRole("tablist", { name: "Filtrar por estado" })).not.toBeInTheDocument();
  });
});

describe("AppShell — sesión por inactividad", () => {
  // The school's rule, end to end: the panel closes itself and says why. On a
  // shared office computer, walking away has to end the session.
  it("closes the session after fifteen idle minutes and explains it", async () => {
    // Fake timers BEFORE the render: the watchdog's interval is created on
    // mount, and one created under real timers would never see the clock move.
    // shouldAdvanceTime keeps the session probe's promises resolving.
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      mockMe.mockResolvedValueOnce({ userId: "u1", email: "owner@example.com", permissions: [] });
      render(
        <Providers flags={{}}>
          <AppShell />
        </Providers>,
      );
      // Wait for the live session to land, or the hook has nothing to close.
      await screen.findByRole("button", { name: /Salir|Cerrar sesión/i });

      await act(async () => { vi.advanceTimersByTime(16 * 60 * 1000); });
    } finally {
      vi.useRealTimers();
    }

    expect(mockLogout).toHaveBeenCalled();
    expect(await screen.findByText(/inactividad/i)).toBeInTheDocument();
  });
});
