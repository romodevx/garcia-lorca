import type { AttendanceDay, AttendanceMonth, AttendanceStatus } from "../api/attendance";
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

const SATURDAY = 6;
const SUNDAY = 0;

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
  /** What a screen reader says; differs from `text` only where the text is a symbol. */
  spoken: string;
  icon: IconName;
  kind: ChipKind;
}

const headline = (text: string, icon: IconName, kind: ChipKind): Headline => ({ text, spoken: text, icon, kind });

const UNKNOWN_TODAY: Headline = { text: "—", spoken: "Sin datos todavía", icon: "clock", kind: "neutral" };

function isWeekend(isoDay: string): boolean {
  const { year, month, day } = parseDay(isoDay);
  const weekday = new Date(Date.UTC(year, month - 1, day)).getUTCDay();
  return weekday === SATURDAY || weekday === SUNDAY;
}

/**
 * Today's entry, looked up ONLY in the month that contains the school's today.
 * Null means unknown — not loaded yet, no `today` from the server, today outside
 * the fetched month (a midnight rollover), or no entry for it — and is never
 * read as "the list was not taken".
 */
export function findToday(month: AttendanceMonth | null): AttendanceDay | null {
  if (month === null || month.today === "" || month.month !== monthOf(month.today)) {
    return null;
  }
  return month.days.find((d) => d.date === month.today) ?? null;
}

/** The big line of the "Hoy" card. An unknown today reads as a dash, not as a verdict. */
export function todayHeadline(day: AttendanceDay | null): Headline {
  if (day === null) {
    return UNKNOWN_TODAY;
  }
  switch (day.status) {
    case "present":
      return headline("Asistió", "check-circle", "ok");
    case "absent":
      return headline("Faltó", "x-circle", "bad");
    case "late":
      return headline("Llegó tarde", "clock", "late");
    case "excused":
      return headline(excusedText(day), "document", "warn");
    case "partial":
      return headline(partialText(day), "alert", "warn");
    default:
      return isWeekend(day.date)
        ? headline("Sin clases hoy", "calendar", "neutral")
        : headline("Lista aún no tomada", "clock", "neutral");
  }
}

/** An excused per-class day may be excused in only some classes (the rest attended). */
function excusedText(day: AttendanceDay): string {
  const excused = day.classes.filter((c) => c.status === "excused").length;
  if (excused === 0 || excused === day.classes.length) {
    return "Falta justificada";
  }
  return `Falta justificada en ${excused} de ${day.classes.length} clases`;
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
  late: { label: "Retardo", kind: "late" },
  excused: { label: "Justificada", kind: "purple" },
  partial: { label: "Parcial", kind: "warn" },
  not_taken: { label: "Sin lista", kind: "neutral" },
};

/** The short pill a day or a class is tagged with in the list. */
export function statusPill(status: AttendanceStatus): Pill {
  return PILLS[status] ?? PILLS.not_taken;
}
