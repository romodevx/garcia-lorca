"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type ReactElement } from "react";
import { usePathname, useRouter } from "next/navigation";
import { useFlag } from "@garcia-lorca/ui/flags";
import { pathToView, viewToPath, type View } from "./routing/routes";
import { canReachView } from "./routing/moduleAccess";
import { CommandPaletteHost } from "./command/CommandPaletteHost";
import { TopBar } from "./_design/TopBar";
import { MODULES, USERS, type ModuleDef, type Role, type UserProfile } from "./_design/data";
import { Dashboard } from "./_screens/Dashboard";
import { AttendanceScreen } from "./_screens/attendance/AttendanceScreen";
import { TeacherLoadScreen } from "./_screens/teachers/TeacherLoadScreen";
import { TeachersScreen } from "./_screens/teachers/TeachersScreen";
import { canEditTeachingLoad, canGrantTeacherAccess, canListUsers } from "./teachers/permissions";
import { UniformsScreen } from "./_screens/uniforms/UniformsScreen";
import { ExpensesScreen } from "./_screens/expenses/ExpensesScreen";
import { ScheduleEntry } from "./_screens/schedule/ScheduleEntry";
import { requestCoverFocus } from "./schedule/coverFocus";
import { ActivitiesScreen } from "./_screens/activities/ActivitiesScreen";
import { ScholarshipsScreen } from "./_screens/scholarships/ScholarshipsScreen";
import type { CutAccess } from "./_screens/finance/CashCutClosing";
import { FinanceScreen } from "./_screens/finance/FinanceScreen";
import { Catalog } from "./_screens/config/Catalog";
import { LevelDeleteScreen } from "./_screens/config/levelDeletion/LevelDeleteScreen";
import { LevelPeriodsScreen } from "./_screens/config/periods/LevelPeriodsScreen";
import { clearLevelNotice, leaveLevelNotice, peekLevelNotice } from "./_screens/config/levelDeletion/pendingNotice";
import { canDeleteLevels, canWriteConfig } from "./config/permissions";
import { canDeleteUsers } from "./admin/permissions";
import { AdminScreen } from "./_screens/admin/AdminScreen";
import { StudentEditScreen } from "./_screens/students/StudentEditScreen";
import type { NotesAccess } from "./_design/students/StudentNotesBeside";
import { StudentsScreen, markStudentsVisit, studentsListPath } from "./_screens/students/StudentsScreen";
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
import { isRefusal } from "./auth/httpError";
import { useSessionKeepAlive } from "./auth/useSessionKeepAlive";

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
/**
 * Cuántas veces se intenta rotar la sesión al arrancar antes de darse por
 * vencido. Tres, porque un fallo pasajero rara vez sobrevive a dos esperas y
 * seguir insistiendo sólo retrasa la pantalla de entrada de quien de verdad no
 * tiene sesión.
 */
const RESTORE_ATTEMPTS = 3;

/** Espera base entre intentos; crece con cada uno. */
const RESTORE_BACKOFF_MS = 300;

export function AppShell(): ReactElement {
  const { theme, toggle } = useTheme();
  const router = useRouter();
  const pathname = usePathname();

  // La pantalla ES la dirección. Hasta septiembre de 2026 vivía en un useState
  // y la web entera respondía en "/", así que el navegador nunca veía una
  // segunda página: el botón "atrás" sólo podía sacar del sitio y F5 devolvía
  // al tablero.
  const addressedView = useMemo(() => pathToView(pathname), [pathname]);

  /** Navegar dejando huella: "atrás" regresa aquí. */
  // Each deliberate visit to Estudiantes counts: the list is keyed on it, so it
  // starts fresh even when the address does not change screens (Estudiantes
  // from the palette while /alumnos?nivel=… is open must drop the filters the
  // URL just dropped). Other screens do not count: no wasted remounts.
  const [visit, setVisit] = useState(0);
  const go = useCallback((next: View): void => {
    const toStudents = next.screen === "students";
    markStudentsVisit(toStudents);
    if (toStudents) setVisit((v) => v + 1);
    router.push(viewToPath(next));
  }, [router]);

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

  /**
   * Si el arranque terminó sin sesión porque el servidor no contestó, y no
   * porque la haya rechazado. Un ref y no estado: lo escribe `restore` y lo lee
   * el `.then` del mismo efecto, sin necesidad de repintar entre los dos.
   */
  const unreachable = useRef(false);

  /**
   * Si hay una sesión viva. NO es "ya cargó el principal".
   *
   * Son cosas distintas: `me()` puede fallar con la sesión perfectamente buena
   * —el gateway rechaza todo mientras hay una rotación pendiente— y en ese caso
   * el tablero dice que no pudo resolver los permisos y ofrece reintentar. Si la
   * guardia mirara `principal`, esa persona acabaría en la pantalla de entrada
   * sin haber perdido su sesión.
   */
  const [authenticated, setAuthenticated] = useState(false);
  const [principal, setPrincipal] = useState<MeResult | undefined>(undefined);
  // Distinct from "principal is undefined": that means the answer is still
  // coming, and the dashboard shows a skeleton for it. A FAILURE shown as a
  // skeleton shimmers forever — which is exactly what a broken session looked
  // like in the demo (dev, 2026-09-07) until somebody reported "no veo ningún
  // módulo".
  const [principalFailed, setPrincipalFailed] = useState(false);

  // A typed address to a module the principal cannot see (a teacher at
  // /finanzas) lands on the dashboard instead of an empty 403 shell, and the
  // address follows. UX only: the gateway still refuses the data.
  const reachable = canReachView(addressedView, principal?.permissions);
  const view = useMemo<View>(() => (reachable ? addressedView : { screen: "dashboard" }), [reachable, addressedView]);
  useEffect(() => {
    if (!reachable) router.replace(viewToPath({ screen: "dashboard" }));
  }, [reachable, router]);

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
  // The virtual card (task 13): verified and revoked from Alumnos.
  const cardEnabled = useFlag("module_student_card");
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
  // T10: Egresos. Its data lives in finance, so both flags must be on.
  const expensesEnabled = useFlag("module_expenses") && financeEnabled;
  // T16: Horarios. Its timetable lives in config, so both flags must be on.
  const scheduleEnabled = useFlag("module_schedule") && configEnabled;
  // T17: Actividades. Its charges live in finance, so both flags must be on.
  const activitiesEnabled = useFlag("module_activities") && financeEnabled;
  // T18: Becas. Finance computes them on every charge, so both flags must be on.
  const scholarshipsEnabled = useFlag("module_scholarships") && financeEnabled;

  /**
   * Arranque: las cookies de sesión son HttpOnly (el JavaScript no las ve), así
   * que se le pregunta al gateway quién somos. Una sesión viva deja la pantalla
   * donde la dirección diga — recargar /finanzas se queda en Finanzas. Sin
   * sesión restaurable se va a la entrada, guardando a dónde iba esta persona
   * para devolverla ahí después.
   */
  useEffect(() => {
    let active = true;

    // Distingue "no hay sesión" de "no pude preguntar". Ambas terminan en la
    // pantalla de entrada, pero sólo una de las dos es culpa de quien mira.
    const restore = async (): Promise<boolean> => {
      try {
        const m = await apiMe(API_URL);
        if (active) setPrincipal(m);
        return true;
      } catch {
        // La cookie de acceso es corta y pudo expirar; queda la de refresco,
        // que sí puede acuñar una nueva.
      }
      // Reintentar UN FALLO, nunca un rechazo.
      //
      // Que el gateway conteste 401 a la rotación significa que la sesión se
      // acabó, y ahí la pantalla de entrada es la respuesta correcta. Que no
      // conteste —500 de una base ocupada, 503 de una instancia despertando,
      // la red— no dice nada sobre la sesión, y tratarlo igual era mandar a la
      // pantalla de entrada a quien tenía la sesión perfectamente viva.
      for (let attempt = 0; attempt < RESTORE_ATTEMPTS; attempt++) {
        try {
          await apiRefresh(API_URL);
          const m = await apiMe(API_URL);
          if (active) setPrincipal(m);
          return true;
        } catch (err) {
          if (isRefusal(err)) {
            return false;
          }
          if (attempt === RESTORE_ATTEMPTS - 1) {
            // Se acabaron los intentos y NUNCA hubo un rechazo: la sesión sigue
            // viva y lo único que falló fue preguntar. Decirlo, porque la
            // pantalla de entrada muda hace que la persona teclee su
            // contraseña, falle también —el servidor sigue caído— y concluya
            // que la sacamos. Encontrado con la base detenida de verdad
            // (/qa, 12-sep-2026): al volver el servidor, recargar entra al
            // tablero sin teclear nada, que es la prueba de que la sesión
            // nunca se perdió.
            unreachable.current = true;
            return false;
          }
          // Esperar un poco antes de volver a intentar: si el fallo fue por
          // contención, insistir en el mismo instante la empeora.
          await new Promise((resolve) => setTimeout(resolve, RESTORE_BACKOFF_MS * (attempt + 1)));
          if (!active) return false;
        }
      }
      return false;
    };

    void restore().then((alive) => {
      if (!active) return;
      setAuthenticated(alive);
      if (!alive && unreachable.current) {
        setAuthError("No pudimos contactar al servidor. Tu sesión sigue abierta — vuelve a intentar en un momento.");
      }
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
    // Corre UNA vez, al montar. `bootPath` es justamente la dirección CONGELADA
    // con la que se abrió la pestaña, y volver a correr esto al navegar
    // re-preguntaría la sesión en cada clic.
    // eslint-disable-next-line react-hooks/exhaustive-deps
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
        // El destino pendiente se descarta aquí. Sin esto sobrevivía al cambio
        // forzado —esta rama sale antes de la línea que lo limpia— y la
        // SIGUIENTE persona que entrara en la misma pestaña compartida
        // aterrizaba en el módulo que pidió la anterior.
        pendingDestination.current = undefined;
        setAuthenticated(true);
        // `replace`: una vez cambiada, esa pantalla ya no es un destino.
        goReplacing({ screen: "changePassword" });
        return;
      }
      // Login only returns session metadata; fetch the principal so the chrome
      // shows the real identity and the dashboard knows what to show. A failure
      // is non-fatal for the chrome but NOT invisible: loadPrincipal records it
      // and the dashboard says so.
      setAuthenticated(true);
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
    // nobody out.
    //
    // The SDK is imported lazily and only when it is in use — signing out of a
    // session that was never started is not worth 468 KB.
    void endSession(API_URL, apiLogout, providerLoginEnabled(), () => import("./auth/firebase"));
    setPrincipal(undefined);
    setAuthenticated(false);
    goReplacing({ screen: "login" });
  };

  // When the session is REFUSED, this is what the person reads.
  //
  // Without it, the 401 went back to whichever panel made the call, and every
  // panel wraps its calls in a blanket catch that reports a domain error: the
  // owner was told "No se pudo crear el grado. Revisa que no exista ya con ese
  // orden" for a session that had simply ended, and reasonably asked what
  // "orden" meant (dev, 2026-09-07).
  //
  // Only a REFUSAL arrives here — authFetch is what draws the line between a
  // session the server rejected and a request that merely failed, and a failure
  // must not land on this path. It used to, and it was how a database hiccup
  // during a refresh threw somebody out of a session that was perfectly valid.
  useEffect(() => onSessionLost(() => {
    logout();
    setPrincipal(undefined);
    setAuthenticated(false);
    setPrincipalFailed(false);
    goReplacing({ screen: "login" });
    setAuthError("Tu sesión terminó. Vuelve a entrar.");
    // Se suscribe UNA vez. Re-suscribir en cada render dejaría varios oyentes
    // cerrando la misma sesión.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }), []);

  // Renews while somebody is working, so a cashier entering payments for an
  // hour never meets a login screen mid-entry. There is no longer a counterpart
  // that CLOSES the session from here: the day-long window is the server's, and
  // the browser does not get to end it early.
  useSessionKeepAlive({
    onKeepAlive: () => { void apiRefresh(API_URL).catch(() => {}); },
  });
  const home = (): void => go({ screen: "dashboard" });
  // UX only: the gateway refuses PUT /students/{id} without students:update.
  const canEditStudents = principal?.permissions?.includes("students:update") === true;
  const editStudent = (studentId: string): void => go({ screen: "studentEdit", studentId });
  // A pupil's notes beside Registrar pago (T6). UX only: the gateway answers the
  // notes with students:read (and module_students), writes with notes_write.
  const notesAccess = useMemo<NotesAccess>(() => {
    const perms = principal?.permissions ?? [];
    return {
      userId: principal?.userId ?? "",
      canRead: studentsEnabled && perms.includes("students:read"),
      canWrite: studentsEnabled && perms.includes("students:notes_write"),
      canVoidAny: studentsEnabled && perms.includes("students:notes_void"),
    };
  }, [principal, studentsEnabled]);
  const openTeacherLoad = (teacherId: string): void => go({ screen: "teacherLoad", teacherId });
  // T9: correcting a registered payment (payments:edit — the owner's by default,
  // delegable). UX only: the gateway refuses the correction without it.
  const canCorrectPayments = financeEnabled && principal?.permissions?.includes("payments:edit") === true;
  // T10: the cut shows what went out and closes with expenses:write; the
  // director (finance:write) reopens a closed day. UX only.
  const cutAccess = useMemo<CutAccess | undefined>(() => {
    const perms = principal?.permissions ?? [];
    // The gateway sends the cut's expenses and closures only to expenses:read.
    if (!expensesEnabled || !perms.includes("expenses:read")) return undefined;
    return { expenses: true, canClose: perms.includes("expenses:write"), canReopen: perms.includes("finance:write") };
  }, [expensesEnabled, principal]);
  // Owner-only (levels:delete). UX only: the gateway refuses the delete without it.
  const mayDeleteLevels = canDeleteLevels(principal?.permissions);
  const openLevelDelete = (levelId: string): void => go({ screen: "levelDelete", levelId });
  // T14: a level's evaluation periods, grade by grade. UX only: the gateway
  // answers reads with config:read and edits with config:write.
  const openLevelPeriods = (levelId: string): void => go({ screen: "levelPeriods", levelId });
  const mayWriteConfig = canWriteConfig(principal?.permissions);
  // What a finished deletion tells Configuración; it belongs to that one visit.
  // Next remounts this shell on the route change, so the sentence travels in
  // pendingNotice (module scope) and the new shell picks it up once.
  const [configNotice, setConfigNotice] = useState<string | undefined>(peekLevelNotice);
  useEffect(() => {
    if (peekLevelNotice() !== undefined) clearLevelNotice();
  }, []);
  const onConfigScreens = view.screen === "config" || view.screen === "levelDelete" || view.screen === "levelPeriods";
  useEffect(() => {
    if (!onConfigScreens) setConfigNotice(undefined);
  }, [onConfigScreens]);
  const finishLevelDelete = (message: string): void => {
    leaveLevelNotice(message);
    setConfigNotice(message);
    go({ screen: "config" });
  };
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
    if (id === "expenses") {
      go({ screen: "expenses" });
      return;
    }
    if (id === "schedule") {
      go({ screen: "schedule" });
      return;
    }
    if (id === "activities") {
      go({ screen: "activities" });
      return;
    }
    if (id === "scholarships") {
      go({ screen: "scholarships" });
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

  /**
   * LA GUARDIA. Sin sesión no se pinta ninguna pantalla del sistema, diga lo que
   * diga la dirección.
   *
   * Hace falta por dos caminos distintos que terminan igual:
   *
   * 1. `router.replace` mueve el pathname de forma ASÍNCRONA. El render que
   *    sigue a la restauración trae `restoring` ya en false y todavía la
   *    dirección vieja, así que un link directo a /finanzas se pintaba entero
   *    —barra, módulo y sus llamadas— antes de que aterrizara el redirect.
   *
   * 2. `replace` sólo sustituye la entrada ACTUAL del historial. Las anteriores
   *    siguen siendo rutas protegidas, así que después de cerrar sesión el botón
   *    "atrás" volvía a pintar el tablero. En la computadora del mostrador eso
   *    es justo lo que no debe pasar, y es lo que un comentario de este archivo
   *    afirmaba haber evitado usando `replace` — no lo lograba.
   *
   * Esto es comodidad y respeto por quien mira la pantalla, NO un control de
   * seguridad: el gateway sigue siendo la frontera y responde 401 sin cookie.
   */
  const publicScreen = view.screen === "login" || view.screen === "changePassword";
  if (!authenticated && !publicScreen) {
    return <Login theme={theme} onToggle={toggle} onSubmit={handleLogin} error={authError} pending={pending} />;
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
          <FinanceScreen onBack={home} notes={notesAccess} canCorrectPayments={canCorrectPayments} cutAccess={cutAccess} />
        ) : (
          // Flag OFF: do not render the finance surface; show the friendly stub
          // instead so navigation never dead-ends. (Backend 404 is authoritative.)
          <ModulePlaceholder m={MODULES.find((m) => m.id === "finance") as ModuleDef} onBack={home} />
        ))}
      {view.screen === "payments" &&
        (financeEnabled ? (
          <PaymentsScreen onBack={home} canGrantDiscounts={principal?.permissions?.includes("finance:write") === true} notes={notesAccess} canCorrectPayments={canCorrectPayments} cutAccess={cutAccess} />
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
      {view.screen === "levelDelete" && configEnabled && mayDeleteLevels && (
        <LevelDeleteScreen key={view.levelId} levelId={view.levelId} onBack={() => go({ screen: "config" })} onDeleted={finishLevelDelete} />
      )}
      {view.screen === "levelPeriods" && configEnabled && (
        <LevelPeriodsScreen key={view.levelId} levelId={view.levelId} canWrite={mayWriteConfig} onBack={() => go({ screen: "config" })} />
      )}
      {(view.screen === "config" ||
        (view.screen === "levelDelete" && !(configEnabled && mayDeleteLevels)) ||
        (view.screen === "levelPeriods" && !configEnabled)) &&
        (configEnabled ? (
          <Catalog onBack={home} canDeleteLevels={mayDeleteLevels} onDeleteLevel={openLevelDelete} onOpenLevelPeriods={openLevelPeriods} notice={configNotice} />
        ) : (
          // Flag OFF: never render the catalog; show the friendly stub so
          // navigation never dead-ends. (Backend 404 is authoritative.)
          <ModulePlaceholder m={MODULES.find((m) => m.id === "settings") as ModuleDef} onBack={home} />
        ))}
      {view.screen === "studentEdit" && studentsEnabled && canEditStudents && (
        <StudentEditScreen
          key={view.studentId}
          studentId={view.studentId}
          userId={principal?.userId ?? ""}
          permissions={principal?.permissions}
          // Back to the list as it was left: level, grade, group and search (T4).
          onBack={() => router.push(studentsListPath())}
        />
      )}
      {(view.screen === "students" || (view.screen === "studentEdit" && !(studentsEnabled && canEditStudents))) &&
        (studentsEnabled ? (
          <StudentsScreen
            key={visit}
            onBack={home}
            onEdit={canEditStudents ? editStudent : undefined}
            canWriteNotes={principal?.permissions?.includes("students:notes_write") === true}
            canVoidAnyNote={principal?.permissions?.includes("students:notes_void") === true}
            userId={principal?.userId ?? ""}
            canVerifyCard={cardEnabled}
            canRevokeCard={cardEnabled && canEditStudents}
            canReadStatement={financeEnabled && (principal?.permissions?.some((p) => p === "finance:read" || p === "collections:read") === true)}
            canCorrectPayments={canCorrectPayments}
          />
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
          <TeachersScreen onBack={home} onOpenLoad={openTeacherLoad} canGrantAccess={canGrantTeacherAccess(principal?.permissions)} canListUsers={canListUsers(principal?.permissions)}
            // T19: absences live in Horarios — offered only with it on and schedule:write (UX; the gateway decides).
            canRegisterAbsence={scheduleEnabled && (principal?.permissions ?? []).includes("schedule:write")}
            onCoverClasses={(absence) => { requestCoverFocus(absence); go({ screen: "schedule" }); }} />
        ) : (
          // Flag OFF: never render the roster; show the friendly stub so
          // navigation never dead-ends. (Backend 404 is authoritative.)
          <ModulePlaceholder m={MODULES.find((m) => m.id === "teachers") as ModuleDef} onBack={home} />
        ))}
      {view.screen === "teacherLoad" &&
        (teachersEnabled ? (
          <TeacherLoadScreen
            key={view.teacherId}
            teacherId={view.teacherId}
            onBack={() => go({ screen: "teachers" })}
            canWrite={canEditTeachingLoad(principal?.permissions)}
          />
        ) : (
          // Flag OFF: same friendly stub as the roster. (Backend 404 is authoritative.)
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
      {view.screen === "expenses" &&
        (expensesEnabled ? (
          <ExpensesScreen onBack={home} permissions={principal?.permissions} />
        ) : (
          // Flag OFF: never render the module; the friendly stub keeps
          // navigation from dead-ending. (Backend 404 is authoritative.)
          <ModulePlaceholder m={MODULES.find((m) => m.id === "expenses") as ModuleDef} onBack={home} />
        ))}
      {view.screen === "schedule" && (
        // Flag OFF renders the friendly stub (inside ScheduleEntry). Backend 404 is authoritative.
        <ScheduleEntry enabled={scheduleEnabled} permissions={principal?.permissions} onBack={home} />
      )}
      {view.screen === "activities" &&
        (activitiesEnabled ? (
          <ActivitiesScreen onBack={home} permissions={principal?.permissions} />
        ) : (
          // Flag OFF: the friendly stub. Backend 404 is authoritative.
          <ModulePlaceholder m={MODULES.find((m) => m.id === "activities") as ModuleDef} onBack={home} />
        ))}
      {view.screen === "scholarships" &&
        (scholarshipsEnabled ? (
          <ScholarshipsScreen onBack={home} permissions={principal?.permissions} />
        ) : (
          // Flag OFF: the friendly stub. Backend 404 is authoritative.
          <ModulePlaceholder m={MODULES.find((m) => m.id === "scholarships") as ModuleDef} onBack={home} />
        ))}
      {view.screen === "admin" &&
        (adminEnabled ? (
          <AdminScreen onBack={home} canDeleteUsers={canDeleteUsers(principal?.permissions)} currentUserId={principal?.userId} />
        ) : (
          // Flag OFF: never render the admin surface; show the friendly stub so
          // navigation never dead-ends. (Backend 404 is authoritative.)
          <ModulePlaceholder m={MODULES.find((m) => m.id === "admin") as ModuleDef} onBack={home} />
        ))}
      {view.screen === "module" && activeModule && <ModulePlaceholder m={activeModule} onBack={home} />}
    </div>
  );
}
