"use client";

import { useCallback, useMemo, useState, type ReactElement } from "react";
import { useFlag } from "@garcia-lorca/ui/flags";
import { CommandPaletteHost } from "./command/CommandPaletteHost";
import { TopBar } from "./_design/TopBar";
import { MODULES, USERS, type ModuleDef, type Role } from "./_design/data";
import { Dashboard } from "./_screens/Dashboard";
import { Finances } from "./_screens/Finances";
import { Catalog } from "./_screens/config/Catalog";
import { Login } from "./_screens/Login";
import { ChangePassword } from "./_screens/ChangePassword";
import { ModulePlaceholder } from "./_screens/ModulePlaceholder";
import { useTheme } from "./theme/ThemeProvider";
import { login as apiLogin, changePassword as apiChangePassword, logout as apiLogout, refresh as apiRefresh } from "./auth/api";

type View =
  | { screen: "login" }
  | { screen: "changePassword" }
  | { screen: "dashboard" }
  | { screen: "finance" }
  | { screen: "config" }
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
  const open = useCallback((id: string): void => {
    if (id === "finance") {
      setView({ screen: "finance" });
      return;
    }
    if (id === "settings") {
      setView({ screen: "config" });
      return;
    }
    setView({ screen: "module", moduleId: id });
  }, []);

  // Command Palette navigation: a command route is `module:<id>`. Navigating IS
  // executing (the palette never mutates), so we just open the target surface.
  const navigate = useCallback(
    (route: string): void => {
      const [kind, id] = route.split(":");
      if (kind === "module" && id !== undefined && id !== "") open(id);
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
          <Finances onBack={home} />
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
      {view.screen === "module" && activeModule && <ModulePlaceholder m={activeModule} onBack={home} />}
    </div>
  );
}
