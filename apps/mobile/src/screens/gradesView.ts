import { generalAverage, subjectAverage } from "@garcia-lorca/domain/grade-average";
import type { EvaluationPeriod, GradeScale, PeriodSubject } from "../api/grades";
import { monthLabel } from "./attendanceView";

export { formatAverage } from "@garcia-lorca/domain/grade-average";

/**
 * Pure presentation helpers for the Calificaciones screen: which period opens
 * first, how a grade is written, whether it fails, and the averages. Averages
 * follow the system's one rule (packages/domain grade-average): the exact mean,
 * truncated to 2 decimals.
 */

const SHORT_LABEL_LENGTH = 3;
const HUNDREDTHS = 100;
const ISO_PAD = 2;

/** The phone's LOCAL date as YYYY-MM-DD: the day the parent is living, not UTC's. */
export function deviceDay(now: Date = new Date()): string {
  const month = String(now.getMonth() + 1).padStart(ISO_PAD, "0");
  const day = String(now.getDate()).padStart(ISO_PAD, "0");
  return `${now.getFullYear()}-${month}-${day}`;
}

/**
 * The period shown first: the one containing today; else the latest that has
 * already started; else the first. Null without periods. Dates compare as
 * YYYY-MM-DD strings, both ends inclusive; `periods` are by ordinal.
 */
export function defaultPeriod(periods: readonly EvaluationPeriod[], today: string): string | null {
  const current = periods.find((p) => p.startsOn <= today && today <= p.endsOn);
  if (current !== undefined) {
    return current.id;
  }
  const started = periods.filter((p) => p.startsOn <= today);
  return (started[started.length - 1] ?? periods[0])?.id ?? null;
}

/**
 * The stored grade as it is — 5.5 stays "5.5": a family must never read a
 * rounded grade. Up to 2 decimals, trailing zeros off.
 */
export function formatGrade(value: number): string {
  return String(Math.round(value * HUNDREDTHS) / HUNDREDTHS);
}

/** A grade strictly below the scale's passing mark fails; without a scale nothing does. */
export function isFailing(value: number, scale: GradeScale | null): boolean {
  return scale !== null && value < scale.passing;
}

/** Whether any subject carries a grade in any period. */
export function hasCapturedGrades(subjects: readonly PeriodSubject[]): boolean {
  return subjects.some((s) => Object.keys(s.grades).length > 0);
}

/** One subject's average across its captured periods; undefined with none. */
export function subjectAverageOf(subject: PeriodSubject): number | undefined {
  return subjectAverage(Object.values(subject.grades));
}

/** The general average: the exact mean of the subjects' exact averages; undefined with none. */
export function generalAverageOf(subjects: readonly PeriodSubject[]): number | undefined {
  return generalAverage(subjects.map((s) => Object.values(s.grades)));
}

/** "2026-09" → "Sep" — a legacy (monthly) boleta's column. */
export function shortMonthLabel(month: string): string {
  return monthLabel(month).slice(0, SHORT_LABEL_LENGTH);
}

/** "2026-09" → "septiembre": the month as it is read aloud, no year. */
export function monthName(month: string): string {
  return monthLabel(month).split(" ")[0]?.toLowerCase() ?? month;
}

const NO_GRADE_SPOKEN = "sin calificación aún";
const FAILED_SPOKEN = "reprobada";

/**
 * What a screen reader says for one subject in one period:
 * "Matemáticas, 1.er trimestre: 9, reprobada". Failing is said in words.
 */
export function gradeLabel(subjectName: string, periodName: string, value: number | undefined, scale: GradeScale | null): string {
  const head = `${subjectName}, ${periodName}`;
  if (value === undefined) {
    return `${head}: ${NO_GRADE_SPOKEN}`;
  }
  return `${head}: ${formatGrade(value)}${isFailing(value, scale) ? `, ${FAILED_SPOKEN}` : ""}`;
}
