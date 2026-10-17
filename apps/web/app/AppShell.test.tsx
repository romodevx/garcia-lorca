import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AppShell } from "./AppShell";
import { requestIntent } from "./routing/pendingIntent";
import { authFetch } from "./auth/authFetch";
import { HttpError } from "./auth/httpError";
import { Providers, fakePath, fakeRouter, resetFakeRouter, useFakePathname } from "./test-utils";
import { changePassword as apiChangePassword, login as apiLogin, logout as apiLogout, me as apiMe, refresh as apiRefresh } from "./auth/api";
import { mayHaveSession } from "./auth/sessionHint";
import { getStudent, listStudents } from "./students/api";

// La navegación ES la barra de direcciones. El router falso tiene estado real
// (test-utils.tsx) para que un clic que navega vuelva a pintar, como en el
// navegador; un vi.fn() suelto registraría el empujón y dejaría la prueba
// mirando la pantalla anterior.
vi.mock("next/navigation", () => ({
  useRouter: () => fakeRouter,
  usePathname: () => useFakePathname(),
}));

vi.mock("./auth/api", () => ({
  login: vi.fn(),
  changePassword: vi.fn(),
  logout: vi.fn(),
  // Default: no live session on boot, so every existing flow starts at login.
  refresh: vi.fn(() => Promise.reject(new Error("no session"))),
  me: vi.fn(() => Promise.reject(new Error("no session"))),
}));

// Default: a session cookie MAY be there, so boot asks the gateway as before.
vi.mock("./auth/sessionHint", () => ({ mayHaveSession: vi.fn(() => Promise.resolve(true)) }));

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
  getIncome: vi.fn().mockResolvedValue({ lines: [], totalCents: 0, creditNoteCents: 0 }),
  getCashUp: vi.fn().mockResolvedValue({ deposits: [], cancelled: [], payments: [], cashCents: 0, cardCents: 0, transferCents: 0, totalCents: 0, creditNoteCents: 0 }),
  getExpected: vi.fn().mockResolvedValue({ amountCents: 0, surchargeCents: 0, discountCents: 0, annualBalanceCents: 0, onTimeBalanceCents: 0 }),
  previewDeposit: vi.fn(),
  registerDeposit: vi.fn(),
}));

vi.mock("./config/api", async (importOriginal) => ({
  // The real error class: screens import it at module scope to branch on refusals.
  ConfigError: (await importOriginal<typeof import("./config/api")>()).ConfigError,
  listCycles: vi.fn().mockResolvedValue([]),
  listGroups: vi.fn().mockResolvedValue([]),
  listLabelledGroups: vi.fn().mockResolvedValue([]),
  listLevels: vi.fn().mockResolvedValue([]),
  listGradeLevels: vi.fn().mockResolvedValue([]),
  listEvaluationPeriods: vi.fn().mockResolvedValue([]),
  getSchoolProfile: vi.fn().mockResolvedValue({ profile: { name: "", legalName: "", rfc: "", cct: "", address: "", phone: "", legend: "" }, found: false }),
  listLevelTrash: vi.fn().mockResolvedValue([]),
  getLevelDeletionPreview: vi.fn().mockResolvedValue({
    levelId: "l1", levelName: "PRIMARIA", empty: true, confirmToken: "tok", expiresAt: "2026-09-28T18:00:00Z",
    counts: { gradeLevels: 0, groups: 0, subjects: 0, terms: 0, teachingAssignments: 0, students: 0, enrollments: 0, attendanceMarks: 0, monthlyGrades: 0, reportCards: 0, tuitionPlans: 0, uniformItems: 0, schoolEvents: 0 },
    keeps: { charges: 0, payments: 0 },
  }),
  deleteLevel: vi.fn().mockResolvedValue({ batchId: "b1", deletedAt: "2026-09-28T17:00:00Z", restorableUntil: "2026-10-05T17:00:00Z", counts: {} }),
}));

vi.mock("./teachers/api", () => ({
  listTeachers: vi.fn().mockResolvedValue([
    { id: "t1", firstName: "Ada", lastName: "Lovelace", email: "ada@escuela.mx", userId: "" },
  ]),
  listTeachingAssignments: vi.fn().mockResolvedValue([]),
}));

// Docentes reads Usuarios for the login states when the principal holds users:read.
vi.mock("./admin/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./admin/api")>()),
  listUsers: vi.fn().mockResolvedValue([]),
}));

vi.mock("./students/api", () => ({
  listStudents: vi.fn().mockResolvedValue([]),
  listEnrollments: vi.fn().mockResolvedValue([]),
  listContacts: vi.fn().mockResolvedValue([]),
  listNotes: vi.fn().mockResolvedValue([]),
  getPhotoBlob: vi.fn().mockResolvedValue(null),
  getStudent: vi.fn().mockResolvedValue({
    id: "st1", curp: "CURP1", firstName: "ADA", lastName: "LOVELACE", level: "Primaria", levelId: "lv1",
    gradeGroup: "4° A", guardianName: "", admissionStatus: "enrolled",
  }),
}));

// The signed-in principal for the navigation flows: READ on the modules these
// tests reach (finance + students, for the dashboard KPI shortcuts). Module
// tiles/KPIs are now permission-gated, so a flow that clicks one must resolve
// `me()` with that permission.
const OWNER_ME = {
  userId: "u1",
  email: "owner@example.com",
  permissions: ["students:read", "finance:read", "grades:read", "comms:read", "config:read", "users:read", "attendance:read", "teachers:read"],
};

const mockLogin = vi.mocked(apiLogin);
const mockChangePassword = vi.mocked(apiChangePassword);
const mockLogout = vi.mocked(apiLogout);
const mockRefresh = vi.mocked(apiRefresh);
const mockMe = vi.mocked(apiMe);
const mockMayHaveSession = vi.mocked(mayHaveSession);

// A persistent `mockResolvedValue` (used by the nav flows below) survives
// clearAllMocks, so reset the session probes to their default "no session"
// baseline before every test — each flow then opts into a live session
// explicitly. Without this, a resolved `me()` would leak into the login-first
// and boot-restore tests.
beforeEach(() => {
  // La dirección es estado de módulo: sin esto se filtra de una prueba a otra.
  resetFakeRouter("/");
  mockMe.mockReset();
  mockRefresh.mockReset();
  mockMe.mockRejectedValue(new Error("no session"));
  mockRefresh.mockRejectedValue(new Error("no session"));
  mockMayHaveSession.mockReset();
  mockMayHaveSession.mockResolvedValue(true);
});

afterEach(() => vi.clearAllMocks());

async function submitLogin(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  // El arranque muestra una espera breve mientras se pregunta si hay sesión;
  // antes la pantalla de entrada se pintaba de inmediato porque era el estado
  // inicial de un useState.
  await screen.findByLabelText("Correo");
  await user.type(screen.getByLabelText("Correo"), "owner@example.com");
  await user.type(screen.getByLabelText("Contraseña"), "test-owner-pw");
  await user.click(screen.getByRole("button", { name: "Entrar al sistema" }));
}

/** Logging out now goes through the account menu (ISSUE-T14-103). */
async function clickLogout(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await user.click(screen.getByRole("button", { name: /Mi cuenta/ }));
  await user.click(screen.getByRole("menuitem", { name: "Cerrar sesión" }));
}

// Fake timers WITHOUT shouldAdvanceTime: wall-clock time must never reach the
// fake clock. Testing Library only drives fake timers through a global `jest`;
// this shim hands it vitest's, so findBy/waitFor still resolve.
function useFakeClock(): void {
  vi.useFakeTimers();
  Object.assign(globalThis, { jest: { advanceTimersByTime: (ms: number) => vi.advanceTimersByTime(ms) } });
}
function useRealClock(): void {
  Reflect.deleteProperty(globalThis, "jest");
  vi.useRealTimers();
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
    await clickLogout(user);
    expect(await screen.findByRole("button", { name: "Entrar al sistema" })).toBeInTheDocument();
    expect(mockLogout).toHaveBeenCalledWith(expect.any(String));
  });

  it("a forced change that could not renew the session (204) goes back to sign-in, saying why", async () => {
    mockLogin.mockResolvedValue({ expiresIn: 900, mustChangePassword: true });
    mockChangePassword.mockResolvedValue({ sessionRenewed: false });
    mockLogout.mockResolvedValue(undefined);
    const user = userEvent.setup();
    render(
      <Providers flags={{}}>
        <AppShell />
      </Providers>
    );
    await submitLogin(user);
    await screen.findByRole("heading", { name: "Crea tu contraseña" });
    await user.type(screen.getByLabelText("Contraseña temporal"), "test-owner-pw");
    await user.type(screen.getByLabelText("Nueva contraseña"), "test-new-pw-123");
    await user.type(screen.getByLabelText("Confirmar contraseña"), "test-new-pw-123");
    await user.click(screen.getByRole("button", { name: "Guardar y entrar" }));
    expect(await screen.findByText("Tu contraseña cambió. Entra con la nueva.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Entrar al sistema" })).toBeInTheDocument();
    // The browser's cookies are cleared too, not just the screen.
    expect(mockLogout).toHaveBeenCalledWith(expect.any(String));
  });

  it("routes to the forced change-password screen and then to the dashboard", async () => {
    mockLogin.mockResolvedValue({ expiresIn: 900, mustChangePassword: true });
    mockChangePassword.mockResolvedValue({ sessionRenewed: true });
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

    await user.type(screen.getByLabelText("Contraseña temporal"), "test-owner-pw");
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

  // T13: before anyone signed in there is no cookie to probe with, and the two
  // 401s (/me, /auth/refresh) were logged as console errors on /entrar.
  it("does not probe /me nor /auth/refresh when the browser holds no session cookie", async () => {
    mockMayHaveSession.mockResolvedValue(false);
    render(
      <Providers flags={{}}>
        <AppShell />
      </Providers>
    );

    expect(await screen.findByRole("button", { name: "Entrar al sistema" })).toBeInTheDocument();
    expect(mockMe).not.toHaveBeenCalled();
    expect(mockRefresh).not.toHaveBeenCalled();
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

  // QA 2026-09-25, ISSUE-001: the bell showed a hard-coded "3" on an empty
  // school. ISSUE-T14-104: it then stayed as a button with no onClick. Nothing
  // feeds it, so there is no bell at all.
  it("renders no notifications bell", async () => {
    mockMe.mockResolvedValueOnce({ userId: "u1", email: "owner@example.com", permissions: [] });
    render(
      <Providers flags={{}}>
        <AppShell />
      </Providers>
    );

    await screen.findByRole("heading", { name: /Módulos/ });
    expect(screen.queryByRole("button", { name: /Notificaciones/ })).toBeNull();
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

  it("opens Registro de pagos when the palette's 'Registrar pago' command is picked", async () => {
    mockLogin.mockResolvedValue({ expiresIn: 900, mustChangePassword: false });
    const user = userEvent.setup();
    render(
      <Providers flags={{ module_finance: true }}>
        <AppShell />
      </Providers>
    );

    await submitLogin(user);
    expect(await screen.findByRole("heading", { name: /Módulos/ })).toBeInTheDocument();

    // Cmd+K → "Registrar pago" lands on the ventanilla. It used to open
    // Finanzas, which only lists charges (redesign F1).
    fireEvent.keyDown(window, { key: "k", metaKey: true });
    await user.click(await screen.findByText("Registrar pago"));

    expect(await screen.findByRole("heading", { name: "Registro de pagos" })).toBeInTheDocument();
  });

  // Redesign F1: the shell dropped the intent (`open(id, _intent)`), so every
  // shortcut opened the plain module. The destination mounts after the
  // navigation, so the intent has to outlive the click.
  it("hands the shortcut's intent to the screen it opens", async () => {
    mockLogin.mockResolvedValue({ expiresIn: 900, mustChangePassword: false });
    const user = userEvent.setup();
    render(
      <Providers flags={{ module_finance: true }}>
        <AppShell />
      </Providers>
    );

    await submitLogin(user);
    expect(await screen.findByRole("heading", { name: /Módulos/ })).toBeInTheDocument();

    fireEvent.keyDown(window, { key: "k", metaKey: true });
    await user.click(await screen.findByText("Ver cargos vencidos"));

    expect(await screen.findByRole("heading", { name: "Finanzas" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^Vencidos/ })).toHaveAttribute("aria-pressed", "true");
  });

  // Review of PR 1: on Finanzas already, the same path does not remount the
  // screen, so the shortcut did nothing — and stayed armed for a later visit.
  it("a shortcut works while already on its own screen", async () => {
    mockLogin.mockResolvedValue({ expiresIn: 900, mustChangePassword: false });
    const user = userEvent.setup();
    render(
      <Providers flags={{ module_finance: true }}>
        <AppShell />
      </Providers>
    );

    await submitLogin(user);
    expect(await screen.findByRole("heading", { name: /Módulos/ })).toBeInTheDocument();
    fireEvent.keyDown(window, { key: "k", metaKey: true });
    // findAll: an earlier test may have left these in the palette's "Recientes".
    await user.click((await screen.findAllByText("Finanzas"))[0]!);
    expect(await screen.findByRole("heading", { name: "Finanzas" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^Todos/ })).toHaveAttribute("aria-pressed", "true");

    fireEvent.keyDown(window, { key: "k", metaKey: true });
    await user.click((await screen.findAllByText("Ver cargos vencidos"))[0]!);

    await waitFor(() => expect(screen.getByRole("button", { name: /^Vencidos/ })).toHaveAttribute("aria-pressed", "true"));
  });

  it("an intent nobody took does not fire on a later visit", async () => {
    mockLogin.mockResolvedValue({ expiresIn: 900, mustChangePassword: false });
    mockMe.mockRejectedValueOnce(new Error("no session")).mockResolvedValue(OWNER_ME);
    const user = userEvent.setup();
    // Left armed, e.g. by a shortcut whose screen never mounted.
    requestIntent("finance", "overdue");
    render(
      <Providers flags={{ module_finance: true }}>
        <AppShell />
      </Providers>
    );

    await submitLogin(user);
    // Any deliberate navigation disarms it (here the logo, back to the
    // dashboard); then the browser's forward button lands on Finanzas.
    await user.click(await screen.findByRole("button", { name: "Inicio" }));
    act(() => { fakeRouter.push("/finanzas"); });

    expect(await screen.findByRole("heading", { name: "Finanzas" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^Todos/ })).toHaveAttribute("aria-pressed", "true");
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

describe("AppShell — la sesión no se cierra por inactividad", () => {
  // The panel used to close itself at fifteen idle minutes, for the shared
  // computer at the front desk. The school asked for a day instead, and this is
  // where that used to be decided: a watchdog in the browser called logout(),
  // so the session was gone no matter what the server still honoured.
  //
  // Now nobody in the browser gets a vote. The server refuses a staff session
  // that sat unused for a day and the cookie expires with it; two hours of
  // nobody there must leave the panel exactly where it was.
  it("leaves a panel nobody touched for two hours signed in", async () => {
    // Fake timers BEFORE the render: the keep-alive interval is created on
    // mount, and one created under real timers would never see the clock move.
    // The shim lets Testing Library drive the fake timers; no wall-clock leaks in.
    useFakeClock();
    try {
      mockMe.mockResolvedValueOnce({ userId: "u1", email: "owner@example.com", permissions: [] });
      render(
        <Providers flags={{}}>
          <AppShell />
        </Providers>,
      );
      // Wait for the live session to land, or there is nothing to keep open.
      await screen.findByRole("button", { name: /Mi cuenta/ });

      await act(async () => { vi.advanceTimersByTime(2 * 60 * 60 * 1000); });
    } finally {
      useRealClock();
    }

    expect(mockLogout).not.toHaveBeenCalled();
    expect(screen.queryByText(/inactividad/i)).not.toBeInTheDocument();
    // Still inside, not on the login screen.
    expect(screen.getByRole("button", { name: /Mi cuenta/ })).toBeInTheDocument();
  });
});

describe("restoring a session at boot", () => {
  // THE OTHER HALF OF THE SAME DEFECT. The boot asks /me, and when the access
  // cookie has expired it falls back to rotating the session. Any rejection at
  // all from that rotation used to mean "no session": a 500 from a busy
  // database, or a gateway instance still waking up, put somebody who had a
  // perfectly good session on the login screen to type their password again.
  //
  // A failure is not a refusal. The restore retries it instead of obeying it.
  it("does not send somebody to the login screen because the gateway hiccupped", async () => {
    mockMe.mockRejectedValueOnce(new HttpError(401, "me: HTTP 401")); // access cookie expired
    mockRefresh.mockRejectedValueOnce(new HttpError(500, "refresh: HTTP 500")); // busy database
    mockRefresh.mockResolvedValueOnce({ expiresIn: 900 }); // the retry works
    mockMe.mockResolvedValueOnce(OWNER_ME);

    render(
      <Providers flags={{}}>
        <AppShell />
      </Providers>,
    );

    // Signed in, on the dashboard — not on the login screen.
    expect(await screen.findByRole("button", { name: /Mi cuenta/ })).toBeInTheDocument();
  });

  // FOUND BY /qa (2026-09-12), with the database stopped for real. The retries
  // run out, and the person is shown a bare login form — for a session that is
  // still perfectly valid. They type their password, THAT fails too (the server
  // is still down), and they conclude the system logged them out. It is the
  // exact wrong conclusion, and it is the complaint this whole branch exists to
  // answer.
  //
  // The session is not lost: the cookies are untouched and a reload once the
  // server is back lands on the dashboard with no password. So the screen must
  // say what is true — the server could not be reached — and must not imply the
  // session ended.
  it("says the server is unreachable, not that the session ended", async () => {
    mockMe.mockRejectedValue(new HttpError(401, "me: HTTP 401"));
    mockRefresh.mockRejectedValue(new HttpError(500, "refresh: HTTP 500"));

    render(
      <Providers flags={{}}>
        <AppShell />
      </Providers>,
    );

    expect(await screen.findByText(/no pudimos contactar al servidor/i)).toBeInTheDocument();
    expect(screen.queryByText(/tu sesión terminó/i)).not.toBeInTheDocument();
  });

  // And the opposite must stay silent: somebody arriving with no session at all
  // is not looking at an outage, and telling them the server is unreachable
  // would be its own lie.
  it("says nothing when there was simply no session", async () => {
    mockMe.mockRejectedValue(new HttpError(401, "me: HTTP 401"));
    mockRefresh.mockRejectedValue(new HttpError(401, "refresh: HTTP 401"));

    render(
      <Providers flags={{}}>
        <AppShell />
      </Providers>,
    );

    expect(await screen.findByRole("button", { name: /Entrar/i })).toBeInTheDocument();
    expect(screen.queryByText(/no pudimos contactar al servidor/i)).not.toBeInTheDocument();
  });

  // The other side of the line, which must not regress: a REFUSED rotation is
  // the server saying the session is over, and that goes to the login screen at
  // once rather than being retried.
  it("goes to the login screen when the rotation is refused", async () => {
    mockMe.mockRejectedValueOnce(new HttpError(401, "me: HTTP 401"));
    mockRefresh.mockRejectedValueOnce(new HttpError(401, "refresh: HTTP 401"));

    render(
      <Providers flags={{}}>
        <AppShell />
      </Providers>,
    );

    expect(await screen.findByRole("button", { name: /Entrar/i })).toBeInTheDocument();
    // Refused means refused: no second attempt.
    expect(mockRefresh).toHaveBeenCalledTimes(1);
  });
});

describe("when the server ends the session mid-use", () => {
  // The whole wiring, end to end: authFetch notices a refused refresh, the shell
  // hears it, and the person reads what actually happened.
  //
  // Before this, the 401 went back to whichever panel made the call, and every
  // panel wraps its calls in a blanket catch that reports a domain error. The
  // owner was told "No se pudo crear el grado. Revisa que no exista ya con ese
  // orden" for a session that had simply ended, and asked what "orden" meant —
  // which is the right question to ask a sentence that made no sense
  // (dev, 2026-09-07).
  it("says the session ended, instead of blaming what the person typed", async () => {
    // A live session on boot, so the shell starts where the owner was: inside.
    mockMe.mockResolvedValueOnce({ userId: "u1", email: "owner@example.com", permissions: ["students:read"] });
    render(
      <Providers flags={{}}>
        <AppShell />
      </Providers>
    );
    expect(await screen.findByRole("heading", { name: /Módulos/ })).toBeInTheDocument();

    // The server refuses both the call and the refresh: the session is over.
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 401 })));
    await act(async () => {
      await authFetch("/api/v1/config/grade-levels", { method: "POST" });
    });

    expect(await screen.findByText(/Tu sesión terminó/)).toBeInTheDocument();
    // And back at the door, not stranded on a dashboard that can do nothing.
    expect(screen.getByRole("button", { name: "Entrar al sistema" })).toBeInTheDocument();
  });
});

describe("la dirección manda sobre la pantalla", () => {
  it("abrir un módulo cambia la dirección", async () => {
    mockLogin.mockResolvedValue({ expiresIn: 900, mustChangePassword: false });
    mockMe.mockRejectedValueOnce(new Error("no session")).mockResolvedValue(OWNER_ME);
    const user = userEvent.setup();
    render(
      <Providers flags={{ module_finance: true }}>
        <AppShell />
      </Providers>
    );

    await submitLogin(user);
    await screen.findByRole("heading", { name: /Módulos/ });
    await user.click(screen.getByRole("button", { name: /Por cobrar/ }));

    await screen.findByRole("heading", { name: "Finanzas" });
    expect(fakePath()).toBe("/finanzas");
  });

  it("una ficha tiene su propia dirección de edición", async () => {
    resetFakeRouter("/alumnos/st1/editar");
    mockMe.mockResolvedValue({ ...OWNER_ME, permissions: [...OWNER_ME.permissions, "students:update"] });
    render(
      <Providers flags={{ module_students: true }}>
        <AppShell />
      </Providers>
    );

    expect(await screen.findByRole("heading", { name: "Editar ficha" })).toBeInTheDocument();
    expect(await screen.findByLabelText("Nombre(s)")).toHaveValue("ADA");
  });

  // ISSUE-T14-010: /alumnos/<id> painted Inicio with the address unchanged.
  it("la liga /alumnos/<id> abre la ficha de ese alumno", async () => {
    vi.mocked(listStudents).mockResolvedValueOnce([
      { id: "st1", curp: "CURP1", firstName: "ADA", lastName: "LOVELACE", level: "Primaria", levelId: "lv1", gradeGroup: "4° A", guardianName: "", admissionStatus: "enrolled" },
    ]);
    resetFakeRouter("/alumnos/st1");
    mockMe.mockResolvedValue(OWNER_ME);
    render(
      <Providers flags={{ module_students: true }}>
        <AppShell />
      </Providers>
    );

    expect(await screen.findByRole("dialog", { name: "Ficha del alumno" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "LOVELACE ADA" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Estudiantes" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Cerrar" }));
    await waitFor(() => expect(fakePath()).toBe("/alumnos"));
    expect(screen.queryByRole("dialog", { name: "Ficha del alumno" })).not.toBeInTheDocument();
  });

  // T14 gate: a withdrawn pupil's file is read-only; its edit address hands over to the ficha.
  it("la dirección de edición de un alumno dado de baja lleva a su ficha", async () => {
    vi.mocked(getStudent).mockResolvedValueOnce({
      id: "st1", curp: "CURP1", firstName: "ADA", lastName: "LOVELACE", level: "Primaria", levelId: "lv1",
      gradeGroup: "4° A", guardianName: "", admissionStatus: "withdrawn",
    });
    resetFakeRouter("/alumnos/st1/editar");
    mockMe.mockResolvedValue({ ...OWNER_ME, permissions: [...OWNER_ME.permissions, "students:update"] });
    render(
      <Providers flags={{ module_students: true }}>
        <AppShell />
      </Providers>
    );

    await waitFor(() => expect(fakePath()).toBe("/alumnos/st1"));
    expect(screen.queryByLabelText("Nombre(s)")).not.toBeInTheDocument();
  });

  it("sin students:update, la dirección de edición no pinta la forma", async () => {
    resetFakeRouter("/alumnos/st1/editar");
    mockMe.mockResolvedValue(OWNER_ME);
    render(
      <Providers flags={{ module_students: true }}>
        <AppShell />
      </Providers>
    );

    expect(await screen.findByRole("heading", { name: "Estudiantes" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Editar ficha" })).not.toBeInTheDocument();
  });

  it("la carga académica de un docente tiene su propia dirección", async () => {
    resetFakeRouter("/docentes/t1/carga");
    mockMe.mockResolvedValue(OWNER_ME);
    render(
      <Providers flags={{ module_teachers: true }}>
        <AppShell />
      </Providers>
    );

    expect(await screen.findByRole("heading", { name: "Carga académica de Ada Lovelace" })).toBeInTheDocument();
  });

  it("con Docentes apagado, la dirección de la carga no pinta la página", async () => {
    resetFakeRouter("/docentes/t1/carga");
    mockMe.mockResolvedValue(OWNER_ME);
    render(
      <Providers flags={{ module_teachers: false }}>
        <AppShell />
      </Providers>
    );

    expect(await screen.findByRole("heading", { level: 1 })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /Carga académica/ })).not.toBeInTheDocument();
  });

  it("pinta la pantalla que dice la dirección, no la última visitada", async () => {
    // Esto es lo que hace que F5 conserve el lugar: la pantalla sale de la URL.
    resetFakeRouter("/finanzas");
    mockMe.mockResolvedValue(OWNER_ME);
    render(
      <Providers flags={{ module_finance: true }}>
        <AppShell />
      </Providers>
    );

    expect(await screen.findByRole("heading", { name: "Finanzas" })).toBeInTheDocument();
  });

  it("sin sesión, un link directo manda a entrar y luego DEVUELVE al módulo pedido", async () => {
    resetFakeRouter("/finanzas");
    mockLogin.mockResolvedValue({ expiresIn: 900, mustChangePassword: false });
    mockMe.mockRejectedValueOnce(new Error("no session")).mockResolvedValue(OWNER_ME);
    const user = userEvent.setup();
    render(
      <Providers flags={{ module_finance: true }}>
        <AppShell />
      </Providers>
    );

    await submitLogin(user);
    expect(await screen.findByRole("heading", { name: "Finanzas" })).toBeInTheDocument();
    expect(fakePath()).toBe("/finanzas");
  });

  it("sin sesión y sin link, entrar aterriza en el tablero", async () => {
    mockLogin.mockResolvedValue({ expiresIn: 900, mustChangePassword: false });
    mockMe.mockRejectedValueOnce(new Error("no session")).mockResolvedValue(OWNER_ME);
    const user = userEvent.setup();
    render(
      <Providers>
        <AppShell />
      </Providers>
    );

    await submitLogin(user);
    await screen.findByRole("heading", { name: /Módulos/ });
    expect(fakePath()).toBe("/");
  });

  it("cerrar sesión no deja el tablero en el historial", async () => {
    // `replace`, no `push`: si "atrás" reviviera el tablero tras cerrar sesión,
    // en la computadora del mostrador sería un problema, no una comodidad.
    resetFakeRouter("/finanzas");
    mockMe.mockResolvedValue(OWNER_ME);
    mockLogout.mockResolvedValue(undefined);
    const user = userEvent.setup();
    const replaced: string[] = [];
    const spy = vi.spyOn(fakeRouter, "replace").mockImplementation((p: string) => {
      replaced.push(p);
      fakeRouter.push(p);
    });
    render(
      <Providers flags={{ module_finance: true }}>
        <AppShell />
      </Providers>
    );

    await screen.findByRole("heading", { name: "Finanzas" });
    await clickLogout(user);

    await screen.findByRole("button", { name: "Entrar al sistema" });
    expect(replaced).toContain("/entrar");
    spy.mockRestore();
  });
});

describe("la guardia de sesión", () => {
  it("un link directo sin sesión NUNCA pinta la pantalla protegida", async () => {
    // `router.replace` actualiza el pathname de forma asíncrona. Si `restoring`
    // se apaga en el mismo tick, el render siguiente trae restoring=false y la
    // dirección VIEJA: /finanzas se pinta entero —barra, módulo y sus llamadas—
    // antes de que aterrice el redirect. Es exactamente el parpadeo que el
    // comentario de arriba decía evitar.
    resetFakeRouter("/finanzas");
    render(
      <Providers flags={{ module_finance: true }}>
        <AppShell />
      </Providers>
    );

    await screen.findByRole("button", { name: "Entrar al sistema" });
    expect(screen.queryByRole("heading", { name: "Finanzas" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Mi cuenta/ })).toBeNull();
  });

  it("tras cerrar sesión, 'atrás' no revive el tablero", async () => {
    // `replace` sólo sustituye la entrada ACTUAL del historial; las anteriores
    // siguen siendo rutas protegidas. Sin una guardia sobre el principal, el
    // botón atrás vuelve a pintar la cáscara del tablero para alguien que ya
    // cerró sesión — en la computadora del mostrador, justo lo que no debe pasar.
    resetFakeRouter("/finanzas");
    mockMe.mockResolvedValue(OWNER_ME);
    mockLogout.mockResolvedValue(undefined);
    const user = userEvent.setup();
    render(
      <Providers flags={{ module_finance: true }}>
        <AppShell />
      </Providers>
    );

    await screen.findByRole("heading", { name: "Finanzas" });
    await clickLogout(user);
    await screen.findByRole("button", { name: "Entrar al sistema" });

    // El "atrás" del navegador: la dirección regresa sola a una ruta protegida.
    act(() => { fakeRouter.push("/finanzas"); });

    expect(screen.queryByRole("heading", { name: "Finanzas" })).toBeNull();
    expect(await screen.findByRole("button", { name: "Entrar al sistema" })).toBeInTheDocument();
  });
});

describe("la guardia de módulos", () => {
  const MAESTRO_ME = {
    userId: "u2",
    email: "maestra@example.com",
    permissions: ["teaching:read", "teaching:write", "attendance_own:read", "attendance_own:write"],
  };
  const ALL_FLAGS = { module_students: true, module_finance: true, module_teachers: true, module_config: true, module_iam: true };

  it.each(["/alumnos", "/finanzas", "/docentes", "/configuracion", "/administracion", "/alumnos/st1/editar"])(
    "un maestro que escribe %s aterriza en el tablero",
    async (path) => {
      resetFakeRouter(path);
      mockMe.mockResolvedValue(MAESTRO_ME);
      render(
        <Providers flags={ALL_FLAGS}>
          <AppShell />
        </Providers>
      );

      expect(await screen.findByRole("heading", { name: /Módulos/ })).toBeInTheDocument();
      expect(fakePath()).toBe("/");
      expect(screen.queryByRole("heading", { name: /Estudiantes|Finanzas|Docentes|Administración|Configuración/ })).toBeNull();
    },
  );

  it.each([
    ["/alumnos", "Estudiantes"],
    ["/finanzas", "Finanzas"],
    ["/docentes", "Docentes"],
  ])("el director sí llega a %s", async (path, heading) => {
    resetFakeRouter(path);
    mockMe.mockResolvedValue(OWNER_ME);
    render(
      <Providers flags={ALL_FLAGS}>
        <AppShell />
      </Providers>
    );

    expect(await screen.findByRole("heading", { name: heading })).toBeInTheDocument();
    expect(fakePath()).toBe(path);
  });
});

describe("eliminar un nivel", () => {
  const DIRECTOR_ME = { ...OWNER_ME, permissions: [...OWNER_ME.permissions, "config:write", "levels:delete"] };

  it("tiene su propia dirección, sólo para quien puede eliminar", async () => {
    resetFakeRouter("/configuracion/niveles/l1/eliminar");
    mockMe.mockResolvedValue(DIRECTOR_ME);
    render(
      <Providers flags={{ module_config: true }}>
        <AppShell />
      </Providers>
    );

    expect(await screen.findByRole("heading", { name: "Eliminar nivel PRIMARIA" })).toBeInTheDocument();
  });

  it("sin levels:delete, la dirección pinta Configuración y no el borrado", async () => {
    resetFakeRouter("/configuracion/niveles/l1/eliminar");
    mockMe.mockResolvedValue(OWNER_ME);
    render(
      <Providers flags={{ module_config: true }}>
        <AppShell />
      </Providers>
    );

    expect(await screen.findByRole("heading", { name: "Configuración académica" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /Eliminar nivel/ })).toBeNull();
  });

  it("al terminar vuelve a Configuración con el aviso de la Papelera", async () => {
    resetFakeRouter("/configuracion/niveles/l1/eliminar");
    mockMe.mockResolvedValue(DIRECTOR_ME);
    const user = userEvent.setup();
    render(
      <Providers flags={{ module_config: true }}>
        <AppShell />
      </Providers>
    );

    await user.click(await screen.findByRole("button", { name: "Continuar" }));
    await user.type(screen.getByLabelText("Escribe el nombre del nivel para confirmar: PRIMARIA"), "primaria");
    await user.click(screen.getByRole("button", { name: "Continuar" }));
    await user.type(screen.getByLabelText("Tu contraseña de director"), "secreto");
    await user.click(screen.getByRole("button", { name: "Eliminar nivel PRIMARIA" }));

    expect(await screen.findByRole("heading", { name: "Configuración académica" })).toBeInTheDocument();
    expect(fakePath()).toBe("/configuracion");
    expect(screen.getByText("Nivel eliminado. Puedes restaurarlo desde la Papelera hasta el 5 de octubre de 2026.")).toBeInTheDocument();
  });
});

describe("los periodos de un nivel (T14)", () => {
  it("tienen su propia dirección dentro de Configuración, de sólo lectura sin config:write", async () => {
    resetFakeRouter("/configuracion/niveles/l1/periodos");
    mockMe.mockResolvedValue(OWNER_ME);
    const user = userEvent.setup();
    render(
      <Providers flags={{ module_config: true }}>
        <AppShell />
      </Providers>
    );

    expect(await screen.findByRole("heading", { name: /^Periodos de evaluación/ })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /volver a configuración/i }));
    expect(await screen.findByRole("heading", { name: "Configuración académica" })).toBeInTheDocument();
    expect(fakePath()).toBe("/configuracion");
  });

  it("con el módulo apagado no se pintan", async () => {
    resetFakeRouter("/configuracion/niveles/l1/periodos");
    mockMe.mockResolvedValue(OWNER_ME);
    render(
      <Providers flags={{}}>
        <AppShell />
      </Providers>
    );

    await waitFor(() => expect(mockMe).toHaveBeenCalled());
    expect(screen.queryByRole("heading", { name: /^Periodos de evaluación/ })).toBeNull();
  });
});

describe("migrar alumnos (ADR 0033)", () => {
  const IMPORTER_ME = { ...OWNER_ME, permissions: [...OWNER_ME.permissions, "students:import"] };
  const ALL_ON = { module_config: true, module_students: true, module_student_import: true };

  it("tiene su propia dirección con el flag encendido y students:import", async () => {
    resetFakeRouter("/configuracion/migrar-alumnos");
    mockMe.mockResolvedValue(IMPORTER_ME);
    render(
      <Providers flags={ALL_ON}>
        <AppShell />
      </Providers>
    );

    expect(await screen.findByRole("heading", { level: 1, name: "Migrar alumnos" })).toBeInTheDocument();
  });

  it("sin students:import, la dirección pinta Configuración y ninguna entrada", async () => {
    resetFakeRouter("/configuracion/migrar-alumnos");
    mockMe.mockResolvedValue(OWNER_ME);
    render(
      <Providers flags={ALL_ON}>
        <AppShell />
      </Providers>
    );

    expect(await screen.findByRole("heading", { name: "Configuración académica" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Migrar alumnos" })).toBeNull();
  });

  it("con el flag apagado tampoco aparece, aunque tenga el permiso", async () => {
    resetFakeRouter("/configuracion/migrar-alumnos");
    mockMe.mockResolvedValue(IMPORTER_ME);
    render(
      <Providers flags={{ module_config: true, module_students: true }}>
        <AppShell />
      </Providers>
    );

    expect(await screen.findByRole("heading", { name: "Configuración académica" })).toBeInTheDocument();
    await waitFor(() => expect(mockMe).toHaveBeenCalled());
    expect(screen.queryByRole("button", { name: "Migrar alumnos" })).toBeNull();
  });

  it("Configuración lleva a la migración por su botón", async () => {
    resetFakeRouter("/configuracion");
    mockMe.mockResolvedValue(IMPORTER_ME);
    const user = userEvent.setup();
    render(
      <Providers flags={ALL_ON}>
        <AppShell />
      </Providers>
    );

    const [first] = await screen.findAllByRole("button", { name: "Migrar alumnos" });
    if (first !== undefined) await user.click(first);

    expect(fakePath()).toBe("/configuracion/migrar-alumnos");
    expect(await screen.findByRole("heading", { level: 1, name: "Migrar alumnos" })).toBeInTheDocument();
  });
});

describe("el alta de alumno es una página (T9)", () => {
  const CREATOR_ME = { ...OWNER_ME, permissions: [...OWNER_ME.permissions, "students:create"] };

  it("tiene su propia dirección con students:create", async () => {
    resetFakeRouter("/alumnos/nuevo");
    mockMe.mockResolvedValue(CREATOR_ME);
    render(
      <Providers flags={{ module_students: true }}>
        <AppShell />
      </Providers>
    );

    expect(await screen.findByRole("heading", { level: 1, name: "Nuevo alumno" })).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("sin students:create, la dirección pinta la lista y ninguna forma", async () => {
    resetFakeRouter("/alumnos/nuevo");
    mockMe.mockResolvedValue(OWNER_ME);
    render(
      <Providers flags={{ module_students: true }}>
        <AppShell />
      </Providers>
    );

    expect(await screen.findByRole("heading", { name: "Estudiantes" })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { level: 1, name: "Nuevo alumno" })).not.toBeInTheDocument();
  });

  it("el «Nuevo alumno» de la lista lleva a la página", async () => {
    resetFakeRouter("/alumnos");
    mockMe.mockResolvedValue(CREATOR_ME);
    const user = userEvent.setup();
    render(
      <Providers flags={{ module_students: true }}>
        <AppShell />
      </Providers>
    );

    const [first] = await screen.findAllByRole("button", { name: "Nuevo alumno" });
    if (first !== undefined) await user.click(first);

    expect(fakePath()).toBe("/alumnos/nuevo");
    expect(await screen.findByRole("heading", { level: 1, name: "Nuevo alumno" })).toBeInTheDocument();
  });

  it("«Crear alumno» de la paleta lleva directo a la página", async () => {
    resetFakeRouter("/");
    mockMe.mockResolvedValue(CREATOR_ME);
    const user = userEvent.setup();
    render(
      <Providers flags={{ module_students: true }}>
        <AppShell />
      </Providers>
    );
    await screen.findByRole("heading", { name: /Módulos/ });

    fireEvent.keyDown(window, { key: "k", metaKey: true });
    await user.click((await screen.findAllByText("Crear alumno"))[0]!);

    expect(await screen.findByRole("heading", { level: 1, name: "Nuevo alumno" })).toBeInTheDocument();
    expect(fakePath()).toBe("/alumnos/nuevo");
  });

  it("Cancelar regresa a la lista de donde salió", async () => {
    // The list keeps its view in the real address bar (T4); jsdom's must match.
    window.history.replaceState(null, "", "/alumnos");
    resetFakeRouter("/alumnos");
    mockMe.mockResolvedValue(CREATOR_ME);
    const user = userEvent.setup();
    render(
      <Providers flags={{ module_students: true }}>
        <AppShell />
      </Providers>
    );
    const [first] = await screen.findAllByRole("button", { name: "Nuevo alumno" });
    if (first !== undefined) await user.click(first);

    await user.click(await screen.findByRole("button", { name: "Cancelar" }));

    expect(await screen.findByRole("heading", { name: "Estudiantes" })).toBeInTheDocument();
    expect(fakePath()).toBe("/alumnos");
    window.history.replaceState(null, "", "/");
  });
});

describe("cambiar la contraseña por voluntad propia (ISSUE-T14-103)", () => {
  async function openDialog(user: ReturnType<typeof userEvent.setup>): Promise<void> {
    await user.click(await screen.findByRole("button", { name: /Mi cuenta/ }));
    await user.click(screen.getByRole("menuitem", { name: "Cambiar contraseña" }));
    await screen.findByRole("dialog", { name: "Cambiar contraseña" });
    await user.type(screen.getByLabelText("Contraseña actual"), "test-owner-pw");
    await user.type(screen.getByLabelText("Nueva contraseña"), "test-new-pw-123");
    await user.type(screen.getByLabelText("Confirmar contraseña"), "test-new-pw-123");
    await user.click(screen.getByRole("button", { name: "Cambiar contraseña" }));
  }

  it("changes it from the account menu and stays signed in when the session is renewed", async () => {
    mockMe.mockResolvedValue(OWNER_ME);
    mockChangePassword.mockResolvedValue({ sessionRenewed: true });
    const user = userEvent.setup();
    render(
      <Providers flags={{}}>
        <AppShell />
      </Providers>
    );
    await openDialog(user);
    expect(mockChangePassword).toHaveBeenCalledWith(expect.any(String), "test-owner-pw", "test-new-pw-123");
    expect(await screen.findByRole("status")).toHaveTextContent("Tu contraseña cambió");
    await user.click(screen.getByRole("button", { name: "Listo" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByRole("button", { name: /Mi cuenta/ })).toHaveFocus();
    expect(mockLogout).not.toHaveBeenCalled();
  });

  it("sends to sign-in with the reason when the server could not renew the session", async () => {
    mockMe.mockResolvedValue(OWNER_ME);
    mockChangePassword.mockResolvedValue({ sessionRenewed: false });
    mockLogout.mockResolvedValue(undefined);
    const user = userEvent.setup();
    render(
      <Providers flags={{}}>
        <AppShell />
      </Providers>
    );
    await openDialog(user);
    expect(await screen.findByRole("button", { name: "Entrar al sistema" })).toBeInTheDocument();
    expect(screen.getByText("Tu contraseña cambió. Entra con la nueva.")).toBeInTheDocument();
    expect(mockLogout).toHaveBeenCalledWith(expect.any(String));
  });

  it("a plain 401 (session gone, not the password) sends to sign-in saying the session ended", async () => {
    mockMe.mockResolvedValue(OWNER_ME);
    mockChangePassword.mockRejectedValue(new HttpError(401, "changePassword: HTTP 401"));
    mockLogout.mockResolvedValue(undefined);
    const user = userEvent.setup();
    render(
      <Providers flags={{}}>
        <AppShell />
      </Providers>
    );
    await openDialog(user);
    expect(await screen.findByRole("button", { name: "Entrar al sistema" })).toBeInTheDocument();
    expect(screen.getByText("Tu sesión terminó. Vuelve a entrar.")).toBeInTheDocument();
  });
});
