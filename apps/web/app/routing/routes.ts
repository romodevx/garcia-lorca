/**
 * El mapa entre la barra de direcciones y la pantalla que se ve.
 *
 * Vive aparte de React a propósito: es la única pieza de navegación que puede
 * probarse sin montar nada, y es la que define qué URLs existen. Hasta
 * septiembre de 2026 la web entera respondía en "/" con la pantalla guardada en
 * un useState, así que el navegador nunca veía una segunda página y el botón
 * "atrás" sólo podía sacar del sitio.
 */

export type View =
  | { screen: "login" }
  | { screen: "changePassword" }
  | { screen: "dashboard" }
  | { screen: "finance" }
  | { screen: "expenses" }
  | { screen: "schedule" }
  | { screen: "activities" }
  | { screen: "scholarships" }
  | { screen: "announcements" }
  | { screen: "payments" }
  | { screen: "config" }
  | { screen: "studentImport" }
  | { screen: "students" }
  | { screen: "studentNew" }
  | { screen: "grades" }
  | { screen: "attendance" }
  | { screen: "teachers" }
  | { screen: "uniforms" }
  | { screen: "admin" }
  | { screen: "module"; moduleId: string }
  | { screen: "studentEdit"; studentId: string }
  | { screen: "teacherLoad"; teacherId: string }
  | { screen: "levelDelete"; levelId: string }
  | { screen: "levelPeriods"; levelId: string };

/** El prefijo de un módulo que aún no tiene pantalla propia. */
const MODULE_PREFIX = "/modulo/";
/** /alumnos/{id}/editar — the full edit page of one pupil's file. */
const STUDENT_EDIT = /^\/alumnos\/([^/]+)\/editar$/;
/** /docentes/{id}/carga — one teacher's teaching load (carga académica). */
const TEACHER_LOAD = /^\/docentes\/([^/]+)\/carga$/;
/** /configuracion/niveles/{id}/eliminar — the three-step deletion of one level. */
const LEVEL_DELETE = /^\/configuracion\/niveles\/([^/]+)\/eliminar$/;
/** /configuracion/niveles/{id}/periodos — the evaluation periods of a level's grades (T14). */
const LEVEL_PERIODS = /^\/configuracion\/niveles\/([^/]+)\/periodos$/;

/**
 * Las pantallas de una sola pieza. Los módulos sin backend van por
 * MODULE_PREFIX, que lleva un id variable y no cabe en esta tabla.
 */
const BY_SCREEN = {
  login: "/entrar",
  changePassword: "/cambiar-contrasena",
  dashboard: "/",
  finance: "/finanzas",
  expenses: "/egresos",
  schedule: "/horarios",
  activities: "/actividades",
  scholarships: "/becas",
  announcements: "/avisos",
  payments: "/pagos",
  config: "/configuracion",
  /** Migrar alumnos from the previous system (ADR 0033). */
  studentImport: "/configuracion/migrar-alumnos",
  students: "/alumnos",
  /** The alta of one pupil: eleven fields, so a page and not a drawer. */
  studentNew: "/alumnos/nuevo",
  grades: "/calificaciones",
  attendance: "/asistencia",
  teachers: "/docentes",
  uniforms: "/uniformes",
  admin: "/administracion",
} as const satisfies Record<Exclude<View["screen"], "module" | "studentEdit" | "teacherLoad" | "levelDelete" | "levelPeriods">, string>;

const BY_PATH: ReadonlyMap<string, View> = new Map(
  Object.entries(BY_SCREEN).map(([screen, path]) => [path, { screen } as View]),
);

/** La dirección que le corresponde a una pantalla. */
export function viewToPath(view: View): string {
  if (view.screen === "module") {
    return `${MODULE_PREFIX}${view.moduleId}`;
  }
  if (view.screen === "studentEdit") {
    return `/alumnos/${encodeURIComponent(view.studentId)}/editar`;
  }
  if (view.screen === "teacherLoad") {
    return `/docentes/${encodeURIComponent(view.teacherId)}/carga`;
  }
  if (view.screen === "levelDelete") {
    return `/configuracion/niveles/${encodeURIComponent(view.levelId)}/eliminar`;
  }
  if (view.screen === "levelPeriods") {
    return `/configuracion/niveles/${encodeURIComponent(view.levelId)}/periodos`;
  }
  return BY_SCREEN[view.screen];
}

/**
 * La pantalla que le corresponde a una dirección.
 *
 * Una dirección que no reconocemos aterriza en el tablero: una pantalla en
 * blanco por un dedazo en la URL es peor que llegar a casa. Eso vuelve a esta
 * función total — para CUALQUIER texto devuelve una pantalla real—, que es lo
 * que permite guardar el destino de un link directo como View ya resuelta y no
 * como texto crudo. Un `?next=//evil.example.com` no tiene por dónde entrar.
 */
export function pathToView(pathname: string): View {
  const clean =
    pathname.length > 1 && pathname.endsWith("/") ? pathname.slice(0, -1) : pathname;

  const known = BY_PATH.get(clean);
  if (known !== undefined) {
    return known;
  }

  if (clean.startsWith(MODULE_PREFIX)) {
    const moduleId = clean.slice(MODULE_PREFIX.length);
    if (moduleId.length > 0 && !moduleId.includes("/")) {
      return { screen: "module", moduleId };
    }
  }

  const edit = STUDENT_EDIT.exec(clean);
  const studentId = edit?.[1] === undefined ? undefined : safeDecode(edit[1]);
  if (studentId !== undefined) {
    return { screen: "studentEdit", studentId };
  }

  const load = TEACHER_LOAD.exec(clean);
  const teacherId = load?.[1] === undefined ? undefined : safeDecode(load[1]);
  if (teacherId !== undefined) {
    return { screen: "teacherLoad", teacherId };
  }

  const del = LEVEL_DELETE.exec(clean);
  const levelId = del?.[1] === undefined ? undefined : safeDecode(del[1]);
  if (levelId !== undefined) {
    return { screen: "levelDelete", levelId };
  }

  const periods = LEVEL_PERIODS.exec(clean);
  const periodsLevelId = periods?.[1] === undefined ? undefined : safeDecode(periods[1]);
  if (periodsLevelId !== undefined) {
    return { screen: "levelPeriods", levelId: periodsLevelId };
  }

  return { screen: "dashboard" };
}

/** A malformed escape (`%E0%A4%A`) is not an id: undefined, never a thrown URIError. */
function safeDecode(segment: string): string | undefined {
  try {
    return decodeURIComponent(segment);
  } catch {
    return undefined;
  }
}
