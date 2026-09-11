"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type ReactElement } from "react";
import { usePathname, useRouter } from "next/navigation";
import { useFlag } from "@garcia-lorca/ui/flags";
import { pathToView, viewToPath, type View } from "./routing/routes";
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
import { endSession } from "./auth/logout";
import { loginWithProvider } from "./auth/provider";
import { providerLoginEnabled } from "./auth/providerFlag";
import { login as apiLogin, changePassword as apiChangePassword, logout as apiLogout, me as apiMe, refresh as apiRefresh, type MeResult } from "./auth/api";

import { API_URL } from "./api-url";
import { onSessionLost } from "./auth/authFetch";
import { useSessionTimeout } from "./auth/useSessionTimeout";

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
  const router = useRouter();
  const pathname = usePathname();

  // La pantalla ES la dirección. Hasta septiembre de 2026 vivía en un useState
  // y la web entera respondía en "/", así que el navegador nunca veía una
  // segunda página: el botón "atrás" sólo podía sacar del sitio y F5 devolvía
  // al tablero.
  const view = pathToView(pathname);

  /** Navegar dejando huella: "atrás" regresa aquí. */
  const go = useCallback((next: View): void => { router.push(viewToPath(next)); }, [router]);

  /**
   * Navegar SIN dejar huella. Para las pantallas a las que no vale la pena
   * regresar: el login tras cerrar sesión (en la computadora del mostrador,
   * "atrás" reviviendo el tablero sería un problema, no una comodidad) y el
   * cambio forzado de contraseña, que ya se resolvió.
   */
  const goReplacing = useCallback((next: View): void => { router.replace(viewToPath(next)); }, [router]);

  // La dirección con la que se abrió la pestaña. Se congela al montar: es "a
  // dónde iba esta persona", y no debe moverse cuando navegue después.
  const bootPath = useRef(pathname);

  /**
   * A dónde iba quien tocó un link directo sin sesión viva.
   *
   * Se guarda como View YA RESUELTA, no como el texto de la URL: así el destino
   * sólo puede ser una de las rutas que routes.ts reconoce, y un link de fuera
   * no puede empujar a nadie a otro sitio. Vive en memoria del cliente y nunca
   * en la barra de direcciones, que es donde un ?next= sería copiable.
   */
  const pendingDestination = useRef<View | undefined>(undefined);

  // Mientras se pregunta si hay sesión no se puede pintar ni el tablero (sería
  // un parpadeo de datos a quien no ha entrado) ni la pantalla de entrada (sería
  // un parpadeo de login a quien recarga con la sesión viva).
  const [restoring, setRestoring] = useState(true);
  const [principal, setPrincipal] = useState<MeResult | undefined>(undefined);
  // Distinct from "principal is undefined": that means the answer is still
  // coming, and the dashboard shows a skeleton for it. A FAILURE shown as a
  // skeleton shimmers forever — which is exactly what a broken session looked
  // like in the demo (dev, 2026-09-07) until somebody reported "no veo ningún
  // módulo".
  const [principalFailed, setPrincipalFailed] = useState(false);

  // Loading the caller's permissions, and recording whether it worked. Shared by
  // the login flow and the retry button so both go through the same path.
  const loadPrincipal = async (): Promise<void> => {
    try {
      setPrincipal(await apiMe(API_URL));
      setPrincipalFailed(false);
    } catch {
      // The session may be perfectly valid and this call still fail — the
      // gateway refuses every route while a rotation is pending, for one. The
      // chrome falls back to its static identity, but the dashboard must SAY it
      // could not resolve the permissions rather than pretend to be loading.
      setPrincipalFailed(true);
    }
  };
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

  /**
   * Arranque: las cookies de sesión son HttpOnly (el JavaScript no las ve), así
   * que se le pregunta al gateway quién somos. Una sesión viva deja la pantalla
   * donde la dirección diga — recargar /finanzas se queda en Finanzas. Sin
   * sesión restaurable se va a la entrada, guardando a dónde iba esta persona
   * para devolverla ahí después.
   */
  useEffect(() => {
    let active = true;

    const restore = async (): Promise<boolean> => {
      try {
        const m = await apiMe(API_URL);
        if (active) setPrincipal(m);
        return true;
      } catch {
        // La cookie de acceso es corta y pudo expirar; queda la de refresco,
        // que sí puede acuñar una nueva.
      }
      try {
        await apiRefresh(API_URL);
        const m = await apiMe(API_URL);
        if (active) setPrincipal(m);
        return true;
      } catch {
        return false;
      }
    };

    void restore().then((alive) => {
      if (!active) return;
      const asked = pathToView(bootPath.current);
      if (!alive) {
        // Sólo vale la pena recordar un destino que NO sea el que se ofrece por
        // omisión: mandar a alguien "de vuelta" al tablero no es devolverlo a
        // ningún lado.
        if (asked.screen !== "login" && asked.screen !== "dashboard" && asked.screen !== "changePassword") {
          pendingDestination.current = asked;
        }
        goReplacing({ screen: "login" });
      } else if (asked.screen === "login" || asked.screen === "changePassword") {
        // Con sesión viva, la pantalla de entrada no tiene nada que hacer.
        goReplacing({ screen: "dashboard" });
      }
      setRestoring(false);
    });

    return () => {
      active = false;
    };
    // Corre UNA vez, al montar: bootPath y goReplacing son estables.
  }, []);

  const handleLogin = async (email: string, password: string): Promise<void> => {
    setPending(true);
    setAuthError(undefined);
    try {
      // Which door: the provider when this build is cut over, our own password
      // endpoint otherwise. Same form, same result shape, same HttpOnly cookies
      // either way — the only difference is who checks the password. Flipping
      // the flag back is a deploy, which is the reversibility this needs.
      // The provider SDK is imported LAZILY, and only when this build actually
      // uses it. A static import cost 468 KB of client JavaScript in every
      // build, including the ones with the flag off — measured, not guessed.
      let res;
      if (providerLoginEnabled()) {
        const { firebaseIdToken } = await import("./auth/firebase");
        res = await loginWithProvider(API_URL, email, password, firebaseIdToken);
      } else {
        res = await apiLogin(API_URL, email, password);
      }
      if (res.mustChangePassword) {
        // `replace`: una vez cambiada, esa pantalla ya no es un destino.
        goReplacing({ screen: "changePassword" });
        return;
      }
      // Login only returns session metadata; fetch the principal so the chrome
      // shows the real identity and the dashboard knows what to show. A failure
      // is non-fatal for the chrome but NOT invisible: loadPrincipal records it
      // and the dashboard says so.
      await loadPrincipal();
      // De vuelta al link que trajo aquí a esta persona, si lo hubo.
      const target = pendingDestination.current ?? { screen: "dashboard" as const };
      pendingDestination.current = undefined;
      goReplacing(target);
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
    goReplacing({ screen: "dashboard" });
    setPending(false);
  };

  const logout = (): void => {
    // BOTH halves. Our cookies are only half the session once the provider is in
    // play: while its own session lives, a fresh ID token can be minted with no
    // password and exchanged at /api/v1/auth/google for a complete new set of
    // cookies. On the shared front-desk machine that means "Cerrar sesión" logs
    // nobody out. The idle timeout below routes through here for the same
    // reason.
    //
    // The SDK is imported lazily and only when it is in use — signing out of a
    // session that was never started is not worth 468 KB.
    void endSession(API_URL, apiLogout, providerLoginEnabled(), () => import("./auth/firebase"));
    setPrincipal(undefined);
    goReplacing({ screen: "login" });
  };

  // The school's session rule. The panel runs on a computer at the front desk:
  // somebody walks away to attend a parent, and the next person who sits down
  // must not find an open session holding the school's money.
  //
  // The server enforces it too — a staff session that has sat unused for fifteen
  // minutes is refused, and the cookie expires with it. This is what makes it
  // immediate and explained, rather than a surprise on the next click.
  // The server can get there first. The client timer and the server's refresh
  // window are both fifteen minutes of inactivity, so at the boundary it is a
  // race — and when the server wins, the next click comes back 401 while the
  // app still believes it has a session. Every panel wraps its calls in a
  // blanket catch, so that 401 surfaced as a domain error: the owner was told
  // "No se pudo crear el grado. Revisa que no exista ya con ese orden" for a
  // session that had simply ended, and reasonably asked what "orden" meant
  // (dev, 2026-09-07).
  //
  // A refused refresh is unambiguous, so whoever notices first — the timer or
  // the server — the person now reads the same sentence.
  useEffect(() => onSessionLost(() => {
    logout();
    setPrincipal(undefined);
    setPrincipalFailed(false);
    goReplacing({ screen: "login" });
    setAuthError("Tu sesión terminó. Vuelve a entrar.");
  }), []);

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
  const home = (): void => go({ screen: "dashboard" });
  const open = useCallback((id: string, _intent?: string): void => {
    if (id === "finance") {
      go({ screen: "finance" });
      return;
    }
    if (id === "settings") {
      go({ screen: "config" });
      return;
    }
    if (id === "students") {
      go({ screen: "students" });
      return;
    }
    if (id === "grades") {
      go({ screen: "grades" });
      return;
    }
    if (id === "attendance") {
      go({ screen: "attendance" });
      return;
    }
    if (id === "teachers") {
      go({ screen: "teachers" });
      return;
    }
    if (id === "uniforms") {
      go({ screen: "uniforms" });
      return;
    }
    if (id === "payments") {
      go({ screen: "payments" });
      return;
    }
    if (id === "announcements") {
      go({ screen: "announcements" });
      return;
    }
    if (id === "admin") {
      go({ screen: "admin" });
      return;
    }
    go({ screen: "module", moduleId: id });
  }, [go]);

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

  // Todavía preguntando si hay sesión. Pintar el tablero aquí enseñaría datos a
  // quien no ha entrado; pintar la entrada haría parpadear un login a quien
  // recarga con la sesión viva. Así que no se pinta ninguna de las dos.
  if (restoring) {
    return (
      <div
        className="app-root"
        role="status"
        aria-label="Abriendo el sistema"
        style={{ minHeight: "100vh", display: "grid", placeItems: "center" }}
      >
        <span style={{ color: "var(--text-faint)", fontFamily: "var(--font-display)", fontWeight: 600, fontSize: 15 }}>
          Abriendo el sistema…
        </span>
      </div>
    );
  }

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
      {view.screen === "dashboard" && (
        <Dashboard
          user={user}
          onOpen={open}
          permissions={principal?.permissions}
          permissionsFailed={principalFailed}
          onRetryPermissions={() => void loadPrincipal()}
        />
      )}
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
