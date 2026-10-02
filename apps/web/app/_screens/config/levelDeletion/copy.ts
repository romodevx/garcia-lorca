import type { LevelDeletionCounts, RestoreSkipped } from "../../../config/api";

/**
 * The words of deleting (and restoring) a level. Pure, so every sentence the
 * director reads before the most destructive action in Configuración is
 * pinned by a test and not by a screenshot.
 */

type Noun = readonly [singular: string, plural: string];

/** What goes with the level, in the order a school thinks about it. */
const DELETED: ReadonlyArray<readonly [keyof LevelDeletionCounts, Noun]> = [
  ["gradeLevels", ["grado", "grados"]],
  ["groups", ["grupo", "grupos"]],
  ["subjects", ["asignatura", "asignaturas"]],
  ["terms", ["trimestre", "trimestres"]],
  ["teachingAssignments", ["asignación de carga académica", "asignaciones de carga académica"]],
  ["enrollments", ["inscripción a grupo", "inscripciones a grupos"]],
  ["enrollmentTransfers", ["cambio de grupo", "cambios de grupo"]],
  ["attendanceSessions", ["lista de asistencia", "listas de asistencia"]],
  ["attendanceMarks", ["registro de asistencia", "registros de asistencia"]],
  ["periodGrades", ["calificación por periodo", "calificaciones por periodo"]],
  ["monthlyGrades", ["calificación mensual (anterior)", "calificaciones mensuales (anteriores)"]],
  ["gradeEntries", ["calificación por trimestre (anterior)", "calificaciones por trimestre (anteriores)"]],
  ["periodClosures", ["periodo cerrado", "periodos cerrados"]],
  ["reportCards", ["boleta", "boletas"]],
  ["tuitionPlans", ["tabulador de colegiaturas", "tabuladores de colegiaturas"]],
  ["schoolEvents", ["evento del nivel", "eventos del nivel"]],
];

const STUDENTS: Noun = ["alumno", "alumnos"];
const UNKNOWN_KIND: Noun = ["registro", "registros"];
const TRASH_SUMMARY_ITEMS = 3;
const MS_PER_DAY = 86_400_000;
const SCHOOL_TIME_ZONE = "America/Mexico_City";
const LONG_DAY = new Intl.DateTimeFormat("es-MX", { day: "numeric", month: "long", year: "numeric", timeZone: SCHOOL_TIME_ZONE });

function counted(n: number, [one, many]: Noun): string {
  return `${n} ${n === 1 ? one : many}`;
}

function nounOf(kind: string): Noun {
  if (kind === "students") return STUDENTS;
  return DELETED.find(([key]) => key === kind)?.[1] ?? UNKNOWN_KIND;
}

/** "6 grados", "1 grupo"… — only what the delete takes away, zeros hidden. */
export function deletedLines(counts: LevelDeletionCounts): string[] {
  return DELETED.filter(([key]) => (counts[key] ?? 0) > 0).map(([key, noun]) => counted(counts[key] ?? 0, noun));
}

/** What survives the delete: the money, the pupils and the uniform stock. */
export function keptLines(counts: LevelDeletionCounts, keeps: { charges: number; payments: number }): string[] {
  const lines: string[] = [];
  if (keeps.charges > 0 || keeps.payments > 0) {
    lines.push(`${counted(keeps.charges, ["cargo", "cargos"])} y ${counted(keeps.payments, ["pago", "pagos"])} en Finanzas`);
  }
  if (counts.students === 1) {
    lines.push("1 alumno quedará sin nivel, grado ni grupo para reasignarlo");
  } else if (counts.students > 1) {
    lines.push(`${counts.students} alumnos quedarán sin nivel, grado ni grupo para reasignarlos`);
  }
  if (counts.uniformItems > 0) {
    lines.push(`${counted(counts.uniformItems, ["prenda", "prendas"])} de uniforme quedará${counts.uniformItems === 1 ? "" : "n"} sin nivel`);
  }
  return lines;
}

/** Case-insensitive, trimmed: the point is reading the name, not matching its capitals. */
export function namesMatch(typed: string, levelName: string): boolean {
  const norm = (s: string): string => s.trim().toLocaleUpperCase("es-MX");
  return typed.trim() !== "" && norm(typed) === norm(levelName);
}

/** "5 de octubre de 2026", on the school's calendar. */
export function longDay(iso: string): string {
  return LONG_DAY.format(new Date(iso));
}

/** Whole days left to restore, rounded up; 0 once the window closed. */
export function daysLeft(untilIso: string, now: Date): number {
  const ms = new Date(untilIso).getTime() - now.getTime();
  return ms <= 0 ? 0 : Math.ceil(ms / MS_PER_DAY);
}

/** "PRIMARIA restaurado: 6 grados, 120 alumnos." */
export function restoredSummary(levelName: string, restored: Partial<LevelDeletionCounts>): string {
  const parts = [...DELETED, ["students", STUDENTS] as const]
    .filter(([key]) => (restored[key] ?? 0) > 0)
    .map(([key, noun]) => counted(restored[key] ?? 0, noun));
  return parts.length === 0 ? `${levelName} restaurado.` : `${levelName} restaurado: ${parts.join(", ")}.`;
}

/** "3 inscripciones a grupos no se restauraron: los alumnos ya están en otro grupo". */
export function skippedLine(s: RestoreSkipped): string {
  const verb = s.count === 1 ? "no se restauró" : "no se restauraron";
  return `${counted(s.count, nounOf(s.kind))} ${verb}: ${s.reason}`;
}

/** The trash row's one-liner: the first few things plus the pupils. */
export function trashSummary(counts: LevelDeletionCounts): string {
  const parts = deletedLines(counts).slice(0, TRASH_SUMMARY_ITEMS);
  if (counts.students > 0) parts.push(counted(counts.students, STUDENTS));
  return parts.length === 0 ? "Nivel vacío" : parts.join(" · ");
}

/** A deletion of this level is already running (or failed half-way and waits in the trash). */
export const DELETION_IN_PROGRESS = "Ya se está eliminando este nivel; espera un momento y revisa la Papelera.";

/** The trash row's state line, when the batch is not a plain deletion. */
export function trashStatusLine(status: string): string | undefined {
  if (status === "failed_partial") return "Eliminación incompleta — Restaurar para recuperar lo que se alcanzó a borrar";
  if (status === "restored_partial") return "Restaurado en parte — lo que no cupo sigue aquí; Restaurar lo intenta de nuevo";
  if (status === "restoring") return "Restaurando… espera un momento y vuelve a abrir la Papelera";
  return undefined;
}
