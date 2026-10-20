import type { GradeLevel, Group, Level } from "../../../../../config/api";

/*
 * What a level card says under its name — what the level holds, and how it is
 * set up — so nobody has to open «Ajustes» to know (PLAN-redesign-estructura).
 */

const plural = (n: number, one: string, many: string): string => `${n} ${n === 1 ? one : many}`;

/** "6 grados · 12 grupos · 1 sin grupos"; the groups are left out while they cannot be told. */
export function levelSummary(grades: readonly GradeLevel[], groupsByGrade: ReadonlyMap<string, readonly Group[]> | undefined): string {
  const parts = [plural(grades.length, "grado", "grados")];
  if (groupsByGrade !== undefined) {
    const count = grades.reduce((n, gr) => n + (groupsByGrade.get(gr.id)?.length ?? 0), 0);
    parts.push(plural(count, "grupo", "grupos"));
    const empty = grades.filter((gr) => (groupsByGrade.get(gr.id)?.length ?? 0) === 0).length;
    if (empty > 0) parts.push(`${empty} sin grupos`);
  }
  return parts.join(" · ");
}

/** "Inscripción por ciclo · Asistencia por día · Calificación 5 a 10". */
export function settingsSummary(level: Level): string {
  const enrollment = level.enrollmentGranularity === "term" ? "Inscripción por periodo" : "Inscripción por ciclo";
  const attendance = level.attendanceMode === "per_class" ? "Asistencia por clase" : "Asistencia por día";
  return [enrollment, attendance, gradingLine(level)].join(" · ");
}

function gradingLine(level: Level): string {
  const grading = level.grading;
  if (grading === undefined) return "Calificación 5 a 10";
  if (grading.enabled) return `Calificación ${grading.min} a ${grading.max}`;
  return grading.letters === true ? "Califica con letras" : "Sin calificaciones";
}
