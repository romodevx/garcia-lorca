"use client";

import { useCallback, useEffect, useMemo, useState, type ReactElement } from "react";
import { useFlag } from "@garcia-lorca/ui/flags";
import { CommandPaletteHost } from "./command/CommandPaletteHost";
import { TopBar } from "./_design/TopBar";
import { MODULES, USERS, type ModuleDef, type Role, type UserProfile } from "./_design/data";
import { Dashboard } from "./_screens/Dashboard";
import { AttendanceScreen } from "./_screens/attendance/AttendanceScreen";
import { TeachersScreen } from "./_screens/teachers/TeachersScreen";
import { UniformsScreen } from "./_screens/uniforms/UniformsScreen";
import { FinanceScreen } from "./_screens/finance/FinanceScreen";
import { Catalog } from "./_screens/config/Catalog";
import { AdminScreen } from "./_screens/admin/AdminScreen";
import { StudentsScreen } from "./_screens/students/StudentsScreen";
import { GradesScreen } from "./_screens/grades/GradesScreen";
import { Login } from "./_screens/Login";
import { ChangePassword } from "./_screens/ChangePassword";
import { ModulePlaceholder } from "./_screens/ModulePlaceholder";
import { AnnouncementsScreen } from "./_screens/announcements/AnnouncementsScreen";
import { PaymentsScreen } from "./_screens/payments/PaymentsScreen";
import { useTheme } from "./theme/ThemeProvider";
import { login as apiLogin, changePassword as apiChangePassword, logout as apiLogout, me as apiMe, refresh as apiRefresh, type MeResult } from "./auth/api";

import { API_URL } from "./api-url";
import { useSessionTimeout } from "./auth/useSessionTimeout";

type View =
  | { screen: "login" }
  | { screen: "changePassword" }
  | { screen: "dashboard" }
  | { screen: "finance" }
  | { screen: "announcements" }
  | { screen: "payments" }
  | { screen: "config" }
  | { screen: "students" }
  | { screen: "grades" }
  | { screen: "attendance" }
  | { screen: "teachers" }
  | { screen: "uniforms" }
  | { screen: "admin" }
  | { screen: "module"; moduleId: string };


// Chrome fallback: color + the institution subtitle used until a real per-user
// profile lands. The identity itself now comes from the authenticated principal
// (`me()`), not this static row.
const CHROME_ROLE: Role = "admin";

// Derive a display name from the email local-part (dots/underscores/hyphens →
// spaces, title-cased): "patricia.lugo@…" → "Patricia Lugo". Used as the fallback
// when the gateway has no real display name to return (directory outage or an
// identity with none set).
function displayName(email: string): string {
  const local = email.split("@")[0] ?? email;
  return local
    .split(/[._-]+/)
    .filter((part) => part.length > 0)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

// Build the top-bar identity from the real principal, falling back to the static
// chrome row only before `me()` resolves (or if it fails). The bold name line
// prefers the gateway's real display name; when absent it falls back to a name
// derived from the email local-part. The real email stays the identity subtitle,
// and the name drives the greeting + avatar initials.
function chromeUser(principal: MeResult | undefined): UserProfile {
  const base = USERS[CHROME_ROLE];
  if (principal === undefined) {
    return base;
  }
  const name = principal.displayName && principal.displayName.length > 0
    ? principal.displayName
    : displayName(principal.email);
  // `name` is the full identity (bold line + avatar); `first` is only the greeting
  // token, so take the leading word — "Patricia Lugo Reyes" greets as "Patricia".
  const first = name.split(/\s+/)[0] || name;
  return { name, first, role: principal.email, color: base.color };
}

/**
 * Navigation shell — real auth flow: login → (forced change-password, if the
 * gateway reports it) → dashboard → finance/module → back home; logout → login.
 * Kept as client-side state (not Next routes) so the demo flow is instant. Real
 * module surfaces will graduate to App Router routes gated by `FeatureGate` when
 * their backends land.
 */
export function AppShell(): ReactElement {
  const { theme, toggle } = useTheme();
  const [view, setView] = useState<View>({ screen: "login" });
  const [principal, setPrincipal] = useState<MeResult | undefined>(undefined);
  const [authError, setAuthError] = useState<string | undefined>(undefined);
  const [pending, setPending] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);

  // The single flag read for the finance surface (UX-only; the gateway is the
  // security boundary). Hooks must run unconditionally, so we read it here.
  const financeEnabled = useFlag("module_finance");
  // UX only, like every other flag read here: the gateway's 404 is what actually
  // keeps a disabled module unreachable.
  const commsEnabled = useFlag("module_comms");
  // The academic-catalog surface is gated by module_config (UX-only; the gateway
  // is the boundary). The "settings" module (Configuración) opens this catalog.
  const configEnabled = useFlag("module_config");
  // The enrollment surface is gated by module_students (UX-only; the gateway is
  // the boundary). The "students" module (Estudiantes) opens this surface.
  const studentsEnabled = useFlag("module_students");
  // The grade-capture surface is gated by module_grades (UX-only; the gateway is
  // the boundary). The "grades" module (Calificaciones) opens this surface.
  const gradesEnabled = useFlag("module_grades");
  // The daily pass-list surface is gated by module_attendance (UX-only; the
  // gateway is the boundary). The "attendance" module (Asistencia) opens it.
  const attendanceEnabled = useFlag("module_attendance");
  // The admin surface (users/roles/teacher-link) is gated by module_iam (UX-only;
  // the gateway is the boundary). The "admin" module (Administración) opens it.
  const adminEnabled = useFlag("module_iam");
  const teachersEnabled = useFlag("module_teachers");
  const uniformsEnabled = useFlag("module_uniforms");

  // Restore the session on boot: the auth cookies are HttpOnly (unreadable by
  // JS), so we ask the gateway who we are. A live session lands straight on the
  // dashboard — without this, every reload / deep-link / bookmark drops the user
  // back to login even though the cookie is still valid. A rejection (no session,
  // or a pending forced change) simply leaves the initial login screen in place.
  useEffect(() => {
    let active = true;
    const restore = async (): Promise<void> => {
      try {
        const m = await apiMe(API_URL);
        if (active) {
          setPrincipal(m);
          setView({ screen: "dashboard" });
        }
        return;
      } catch {
        // The short-lived access cookie may have expired; fall through to the
        // longer-lived refresh cookie, which can mint a fresh access token.
      }
      try {
        await apiRefresh(API_URL);
        const m = await apiMe(API_URL);
        if (active) {
          setPrincipal(m);
          setView({ screen: "dashboard" });
        }
      } catch {
        // No restorable session — stay on login.
      }
    };
    void restore();
    return () => {
      active = false;
    };
  }, []);

  const handleLogin = async (email: string, password: string): Promise<void> => {
    setPending(true);
    setAuthError(undefined);
    try {
      const res = await apiLogin(API_URL, email, password);
      if (res.mustChangePassword) {
        setView({ screen: "changePassword" });
        return;
      }
      // Login only returns session metadata; fetch the principal so the chrome
      // shows the real identity. A failure here is non-fatal — the static
      // fallback chrome renders and the dashboard still opens.
      try {
        setPrincipal(await apiMe(API_URL));
      } catch {
        // Keep the fallback chrome; the session is still valid.
      }
      setView({ screen: "dashboard" });
    } catch {
      setAuthError("Correo o contraseña incorrectos.");
    } finally {
      setPending(false);
    }
  };

  const handleChangePassword = async (current: string, next: string): Promise<void> => {
    setPending(true);
    setAuthError(undefined);
    try {
      await apiChangePassword(API_URL, current, next);
    } catch {
      // Only the change itself can be blamed on the current password.
      setAuthError("No se pudo cambiar la contraseña. Verifica la actual.");
      setPending(false);
      return;
    }
    // The password IS changed. The gateway revoked the old refresh token and
    // re-issued a fresh session in the SAME change-password response (new
    // gl_access / gl_refresh cookies, must_change cleared), so we just land on
    // the dashboard — no extra refresh round-trip, and the user is never trapped
    // re-typing a password that no longer exists.
    try {
      setPrincipal(await apiMe(API_URL));
    } catch {
      // Keep the fallback chrome; the fresh session is still valid.
    }
    setView({ screen: "dashboard" });
    setPending(false);
  };

  const logout = (): void => {
    void apiLogout(API_URL).catch(() => {});
    setPrincipal(undefined);
    setView({ screen: "login" });
  };

  // The school's session rule. The panel runs on a computer at the front desk:
  // somebody walks away to attend a parent, and the next person who sits down
  // must not find an open session holding the school's money.
  //
  // The server enforces it too — a staff session that has sat unused for fifteen
  // minutes is refused, and the cookie expires with it. This is what makes it
  // immediate and explained, rather than a surprise on the next click.
  useSessionTimeout({
    onExpire: () => {
      if (principal === undefined) {
        // Nobody is signed in; there is nothing to close.
        return;
      }
      logout();
      setAuthError("Cerramos la sesión por inactividad. Vuelve a entrar.");
    },
    // Renews while somebody is working, so a cashier entering payments for an
    // hour never meets a login screen mid-entry.
    onKeepAlive: () => { void apiRefresh(API_URL).catch(() => {}); },
  });
  const home = (): void => setView({ screen: "dashboard" });
  const open = useCallback((id: string, _intent?: string): void => {
    if (id === "finance") {
      setView({ screen: "finance" });
      return;
    }
    if (id === "settings") {
      setView({ screen: "config" });
      return;
    }
    if (id === "students") {
      setView({ screen: "students" });
      return;
    }
    if (id === "grades") {
      setView({ screen: "grades" });
      return;
    }
    if (id === "attendance") {
      setView({ screen: "attendance" });
      return;
    }
    if (id === "teachers") {
      setView({ screen: "teachers" });
      return;
    }
    if (id === "uniforms") {
      setView({ screen: "uniforms" });
      return;
    }
    if (id === "payments") {
      setView({ screen: "payments" });
      return;
    }
    if (id === "announcements") {
      setView({ screen: "announcements" });
      return;
    }
    if (id === "admin") {
      setView({ screen: "admin" });
      return;
    }
    setView({ screen: "module", moduleId: id });
  }, []);

  // Command Palette navigation: a command route is `module:<id>` plus an optional
  // intent (`module:finance:payment` lands on the ventanilla form). Navigating IS
  // executing (the palette never mutates), so we just open the target surface.
  const navigate = useCallback(
    (route: string): void => {
      const [kind, id, intent] = route.split(":");
      if (kind === "module" && id !== undefined && id !== "") open(id, intent);
    },
    [open],
  );

  const user = chromeUser(principal);
  const activeModule: ModuleDef | undefined = useMemo(
    () => (view.screen === "module" ? MODULES.find((m) => m.id === view.moduleId) : undefined),
    [view]
  );

  if (view.screen === "login") {
    return <Login theme={theme} onToggle={toggle} onSubmit={handleLogin} error={authError} pending={pending} />;
  }
  if (view.screen === "changePassword") {
    return <ChangePassword theme={theme} onToggle={toggle} onSubmit={handleChangePassword} error={authError} pending={pending} />;
  }

  return (
    <div className="app-root" style={{ minHeight: "100vh" }}>
      <TopBar user={user} theme={theme} onToggle={toggle} onHome={home} onLogout={logout} onOpenPalette={() => setPaletteOpen(true)} />
      <CommandPaletteHost open={paletteOpen} onOpenChange={setPaletteOpen} onNavigate={navigate} permissions={principal?.permissions} />
      {view.screen === "dashboard" && <Dashboard user={user} onOpen={open} permissions={principal?.permissions} />}
      {view.screen === "finance" &&
        (financeEnabled ? (
          <FinanceScreen onBack={home} />
        ) : (
          // Flag OFF: do not render the finance surface; show the friendly stub
          // instead so navigation never dead-ends. (Backend 404 is authoritative.)
          <ModulePlaceholder m={MODULES.find((m) => m.id === "finance") as ModuleDef} onBack={home} />
        ))}
      {view.screen === "payments" &&
        (financeEnabled ? (
          <PaymentsScreen onBack={home} />
        ) : (
          <ModulePlaceholder m={MODULES.find((m) => m.id === "payments") as ModuleDef} onBack={home} />
        ))}
      {view.screen === "announcements" &&
        (commsEnabled ? (
          <AnnouncementsScreen onBack={home} />
        ) : (
          // Flag OFF: never render the send surface; the friendly stub keeps
          // navigation from dead-ending. (Backend 404 is authoritative.)
          <ModulePlaceholder m={MODULES.find((m) => m.id === "announcements") as ModuleDef} onBack={home} />
        ))}
      {view.screen === "config" &&
        (configEnabled ? (
          <Catalog onBack={home} />
        ) : (
          // Flag OFF: never render the catalog; show the friendly stub so
          // navigation never dead-ends. (Backend 404 is authoritative.)
          <ModulePlaceholder m={MODULES.find((m) => m.id === "settings") as ModuleDef} onBack={home} />
        ))}
      {view.screen === "students" &&
        (studentsEnabled ? (
          <StudentsScreen onBack={home} />
        ) : (
          // Flag OFF: never render the enrollment surface; show the friendly stub
          // so navigation never dead-ends. (Backend 404 is authoritative.)
          <ModulePlaceholder m={MODULES.find((m) => m.id === "students") as ModuleDef} onBack={home} />
        ))}
      {view.screen === "grades" &&
        (gradesEnabled ? (
          <GradesScreen onBack={home} permissions={principal?.permissions} />
        ) : (
          // Flag OFF: never render the grade-capture surface; show the friendly stub
          // so navigation never dead-ends. (Backend 404 is authoritative.)
          <ModulePlaceholder m={MODULES.find((m) => m.id === "grades") as ModuleDef} onBack={home} />
        ))}
      {view.screen === "attendance" &&
        (attendanceEnabled ? (
          <AttendanceScreen onBack={home} permissions={principal?.permissions} />
        ) : (
          // Flag OFF: never render the pass list; show the friendly stub so
          // navigation never dead-ends. (Backend 404 is authoritative.)
          <ModulePlaceholder m={MODULES.find((m) => m.id === "attendance") as ModuleDef} onBack={home} />
        ))}
      {view.screen === "teachers" &&
        (teachersEnabled ? (
          <TeachersScreen onBack={home} />
        ) : (
          // Flag OFF: never render the roster; show the friendly stub so
          // navigation never dead-ends. (Backend 404 is authoritative.)
          <ModulePlaceholder m={MODULES.find((m) => m.id === "teachers") as ModuleDef} onBack={home} />
        ))}
      {view.screen === "uniforms" &&
        (uniformsEnabled ? (
          <UniformsScreen onBack={home} permissions={principal?.permissions} />
        ) : (
          // Flag OFF: never render the shop; show the friendly stub so navigation
          // never dead-ends. (Backend 404 is authoritative.)
          <ModulePlaceholder m={MODULES.find((m) => m.id === "uniforms") as ModuleDef} onBack={home} />
        ))}
      {view.screen === "admin" &&
        (adminEnabled ? (
          <AdminScreen onBack={home} />
        ) : (
          // Flag OFF: never render the admin surface; show the friendly stub so
          // navigation never dead-ends. (Backend 404 is authoritative.)
          <ModulePlaceholder m={MODULES.find((m) => m.id === "admin") as ModuleDef} onBack={home} />
        ))}
      {view.screen === "module" && activeModule && <ModulePlaceholder m={activeModule} onBack={home} />}
    </div>
  );
}
