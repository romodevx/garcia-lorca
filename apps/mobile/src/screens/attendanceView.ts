import type { AttendanceDay, AttendanceStatus } from "../api/attendance";
import type { ChipKind } from "../data/mock";
import type { IconName } from "../icons";

/**
 * Pure presentation helpers for the Asistencia screen: Spanish date labels built
 * from fixed tables (Hermes and Node disagree on Intl output, a table does not),
 * month arithmetic on "YYYY-MM" strings, and the words each status is shown with.
 * Every status carries TEXT — color is only ever a second signal.
 */

const WEEKDAYS = ["Dom", "Lun", "Mar", "Mié", "Jue", "Vie", "Sáb"];
const MONTHS_SHORT = ["ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"];
const MONTHS_LONG = [
  "Enero", "Febrero", "Marzo", "Abril", "Mayo", "Junio",
  "Julio", "Agosto", "Septiembre", "Octubre", "Noviembre", "Diciembre",
];
const MONTHS_PER_YEAR = 12;

/** Statuses in which the child was in the classroom. */
const ATTENDED: ReadonlySet<AttendanceStatus> = new Set(["present", "late"]);

function parseDay(iso: string): { year: number; month: number; day: number } {
  const [year = 0, month = 1, day = 1] = iso.split("-").map(Number);
  return { year, month, day };
}

/** "2026-09-28" → "Lun 28 sep". */
export function dayLabel(iso: string): string {
  const { year, month, day } = parseDay(iso);
  const weekday = new Date(year, month - 1, day).getDay();
  return `${WEEKDAYS[weekday]} ${day} ${MONTHS_SHORT[month - 1]}`;
}

/** "2026-09" → "Septiembre 2026". */
export function monthLabel(month: string): string {
  const { year, month: m } = parseDay(month);
  return `${MONTHS_LONG[m - 1]} ${year}`;
}

/** Moves a "YYYY-MM" month by `delta` months. */
export function shiftMonth(month: string, delta: number): string {
  const { year, month: m } = parseDay(month);
  const index = year * MONTHS_PER_YEAR + (m - 1) + delta;
  const nextYear = Math.floor(index / MONTHS_PER_YEAR);
  const nextMonth = (index % MONTHS_PER_YEAR) + 1;
  return `${nextYear}-${String(nextMonth).padStart(2, "0")}`;
}

/** "2026-09-28" → "2026-09". */
export function monthOf(isoDay: string): string {
  return isoDay.slice(0, 7);
}

/** The phone's current month, used only until the server tells us its today. */
export function deviceMonth(now: Date = new Date()): string {
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}`;
}

export interface Headline {
  text: string;
  icon: IconName;
  kind: ChipKind;
}

/** The big line of the "Hoy" card. A day with no record reads as "not taken yet". */
export function todayHeadline(day: AttendanceDay | undefined): Headline {
  switch (day?.status) {
    case "present":
      return { text: "Asistió", icon: "check-circle", kind: "ok" };
    case "absent":
      return { text: "Faltó", icon: "x-circle", kind: "bad" };
    case "late":
      return { text: "Llegó tarde", icon: "clock", kind: "warn" };
    case "excused":
      return { text: "Falta justificada", icon: "document", kind: "warn" };
    case "partial":
      return { text: partialText(day), icon: "alert", kind: "warn" };
    default:
      return { text: "Lista aún no tomada", icon: "clock", kind: "neutral" };
  }
}

function partialText(day: AttendanceDay): string {
  if (day.classes.length === 0) {
    return "Asistió a algunas clases";
  }
  const attended = day.classes.filter((c) => ATTENDED.has(c.status)).length;
  return `Asistió a ${attended} de ${day.classes.length} clases`;
}

export interface Pill {
  label: string;
  kind: ChipKind;
}

const PILLS: Record<AttendanceStatus, Pill> = {
  present: { label: "Asistió", kind: "ok" },
  absent: { label: "Faltó", kind: "bad" },
  late: { label: "Retardo", kind: "warn" },
  excused: { label: "Justificada", kind: "purple" },
  partial: { label: "Parcial", kind: "warn" },
  not_taken: { label: "Sin lista", kind: "neutral" },
};

/** The short pill a day or a class is tagged with in the list. */
export function statusPill(status: AttendanceStatus): Pill {
  return PILLS[status] ?? PILLS.not_taken;
}
