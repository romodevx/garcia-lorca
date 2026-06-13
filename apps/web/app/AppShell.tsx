"use client";

import { useMemo, useState, type ReactElement } from "react";
import { useFlag } from "@garcia-lorca/ui/flags";
import { TopBar } from "./_design/TopBar";
import { MODULES, USERS, type ModuleDef, type Role } from "./_design/data";
import { Dashboard } from "./_screens/Dashboard";
import { Finances } from "./_screens/Finances";
import { Login } from "./_screens/Login";
import { ModulePlaceholder } from "./_screens/ModulePlaceholder";
import { useTheme } from "./theme/ThemeProvider";

type View =
  | { screen: "login" }
  | { screen: "dashboard" }
  | { screen: "finance" }
  | { screen: "module"; moduleId: string };

/**
 * Navigation shell — replicates the prototype's SPA flow (project/app.jsx):
 * login → dashboard → finance/module → back home; logout → login. Kept as
 * client-side state (not Next routes) so the demo flow is instant and matches
 * the handoff exactly. Real module surfaces will graduate to App Router routes
 * gated by `FeatureGate` when their backends land.
 */
export function AppShell(): ReactElement {
  const { theme, toggle } = useTheme();
  const [view, setView] = useState<View>({ screen: "login" });
  const [role, setRole] = useState<Role>("admin");
  const [search, setSearch] = useState("");

  // The single flag read for the finance surface (UX-only; the gateway is the
  // security boundary). Hooks must run unconditionally, so we read it here.
  const financeEnabled = useFlag("module_finance");

  const login = (r: Role): void => {
    setRole(r);
    setView({ screen: "dashboard" });
  };
  const logout = (): void => setView({ screen: "login" });
  const home = (): void => setView({ screen: "dashboard" });
  const open = (id: string): void =>
    setView(id === "finance" ? { screen: "finance" } : { screen: "module", moduleId: id });

  const user = USERS[role];
  const activeModule: ModuleDef | undefined = useMemo(
    () => (view.screen === "module" ? MODULES.find((m) => m.id === view.moduleId) : undefined),
    [view]
  );

  if (view.screen === "login") {
    return <Login theme={theme} onToggle={toggle} onLogin={login} />;
  }

  return (
    <div className="app-root" style={{ minHeight: "100vh" }}>
      <TopBar user={user} theme={theme} onToggle={toggle} onHome={home} onLogout={logout} search={search} onSearch={setSearch} />
      {view.screen === "dashboard" && <Dashboard user={user} onOpen={open} />}
      {view.screen === "finance" &&
        (financeEnabled ? (
          <Finances onBack={home} />
        ) : (
          // Flag OFF: do not render the finance surface; show the friendly stub
          // instead so navigation never dead-ends. (Backend 404 is authoritative.)
          <ModulePlaceholder m={MODULES.find((m) => m.id === "finance") as ModuleDef} onBack={home} />
        ))}
      {view.screen === "module" && activeModule && <ModulePlaceholder m={activeModule} onBack={home} />}
    </div>
  );
}
