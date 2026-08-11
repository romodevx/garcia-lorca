"use client";

import { useCallback, useEffect, useMemo, useState, type ReactElement } from "react";
import { useFlag } from "@garcia-lorca/ui/flags";
import { CommandPaletteHost } from "./command/CommandPaletteHost";
import { TopBar } from "./_design/TopBar";
import { MODULES, USERS, type ModuleDef, type Role } from "./_design/data";
import { Dashboard } from "./_screens/Dashboard";
import { Attendance } from "./_screens/attendance/Attendance";
import { FinanceScreen } from "./_screens/finance/FinanceScreen";
import { Catalog } from "./_screens/config/Catalog";
import { Admin } from "./_screens/admin/Admin";
import { StudentsScreen } from "./_screens/students/StudentsScreen";
import { GradeCapture } from "./_screens/grades/GradeCapture";
import { Login } from "./_screens/Login";
import { ChangePassword } from "./_screens/ChangePassword";
import { ModulePlaceholder } from "./_screens/ModulePlaceholder";
import { useTheme } from "./theme/ThemeProvider";
import { login as apiLogin, changePassword as apiChangePassword, logout as apiLogout, me as apiMe, refresh as apiRefresh } from "./auth/api";

type View =
  | { screen: "login" }
  | { screen: "changePassword" }
  | { screen: "dashboard" }
  | { screen: "finance" }
  | { screen: "config" }
  | { screen: "students" }
  | { screen: "grades" }
  | { screen: "attendance" }
  | { screen: "admin" }
  | { screen: "module"; moduleId: string };

// Gateway base URL for the browser (public env; matches compose NEXT_PUBLIC_API_URL).
const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

// Single-user demo chrome: the dashboard/top-bar identity is fixed to the owner
// until per-user profiles land. Login is now credential-driven (no role picker).
const CHROME_ROLE: Role = "admin";

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
  const [authError, setAuthError] = useState<string | undefined>(undefined);
  const [pending, setPending] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);

  // The single flag read for the finance surface (UX-only; the gateway is the
  // security boundary). Hooks must run unconditionally, so we read it here.
  const financeEnabled = useFlag("module_finance");
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

  // Restore the session on boot: the auth cookies are HttpOnly (unreadable by
  // JS), so we ask the gateway who we are. A live session lands straight on the
  // dashboard — without this, every reload / deep-link / bookmark drops the user
  // back to login even though the cookie is still valid. A rejection (no session,
  // or a pending forced change) simply leaves the initial login screen in place.
  useEffect(() => {
    let active = true;
    const restore = async (): Promise<void> => {
      try {
        await apiMe(API_URL);
        if (active) setView({ screen: "dashboard" });
        return;
      } catch {
        // The short-lived access cookie may have expired; fall through to the
        // longer-lived refresh cookie, which can mint a fresh access token.
      }
      try {
        await apiRefresh(API_URL);
        await apiMe(API_URL);
        if (active) setView({ screen: "dashboard" });
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
      setView(res.mustChangePassword ? { screen: "changePassword" } : { screen: "dashboard" });
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
      // Rotate the session cookie so it carries the cleared must_change_password
      // flag (mcp=false); otherwise the stale token keeps protected routes at 403.
      await apiRefresh(API_URL);
      setView({ screen: "dashboard" });
    } catch {
      setAuthError("No se pudo cambiar la contraseña. Verifica la actual.");
    } finally {
      setPending(false);
    }
  };

  const logout = (): void => {
    void apiLogout(API_URL).catch(() => {});
    setView({ screen: "login" });
  };
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

  const user = USERS[CHROME_ROLE];
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
      <CommandPaletteHost open={paletteOpen} onOpenChange={setPaletteOpen} onNavigate={navigate} />
      {view.screen === "dashboard" && <Dashboard user={user} onOpen={open} />}
      {view.screen === "finance" &&
        (financeEnabled ? (
          <FinanceScreen onBack={home} />
        ) : (
          // Flag OFF: do not render the finance surface; show the friendly stub
          // instead so navigation never dead-ends. (Backend 404 is authoritative.)
          <ModulePlaceholder m={MODULES.find((m) => m.id === "finance") as ModuleDef} onBack={home} />
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
          <GradeCapture onBack={home} />
        ) : (
          // Flag OFF: never render the grade-capture surface; show the friendly stub
          // so navigation never dead-ends. (Backend 404 is authoritative.)
          <ModulePlaceholder m={MODULES.find((m) => m.id === "grades") as ModuleDef} onBack={home} />
        ))}
      {view.screen === "attendance" &&
        (attendanceEnabled ? (
          <Attendance onBack={home} />
        ) : (
          // Flag OFF: never render the pass list; show the friendly stub so
          // navigation never dead-ends. (Backend 404 is authoritative.)
          <ModulePlaceholder m={MODULES.find((m) => m.id === "attendance") as ModuleDef} onBack={home} />
        ))}
      {view.screen === "admin" &&
        (adminEnabled ? (
          <Admin onBack={home} />
        ) : (
          // Flag OFF: never render the admin surface; show the friendly stub so
          // navigation never dead-ends. (Backend 404 is authoritative.)
          <ModulePlaceholder m={MODULES.find((m) => m.id === "admin") as ModuleDef} onBack={home} />
        ))}
      {view.screen === "module" && activeModule && <ModulePlaceholder m={activeModule} onBack={home} />}
    </div>
  );
}
