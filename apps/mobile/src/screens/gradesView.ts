import type { GradeScale, MonthlyGrades, MonthlySubject } from "../api/grades";
import { monthLabel } from "./attendanceView";
import { averageHalfUp } from "./gradeAverage";

/**
 * Pure presentation helpers for the Calificaciones screen: which month opens
 * first, how a grade is written, whether it fails, and the month's average.
 */

const SHORT_LABEL_LENGTH = 3;
const HUNDREDTHS = 100;
const AVERAGE_DECIMALS = 1;

function hasAnyGrade(subjects: MonthlySubject[], month: string): boolean {
  return subjects.some((s) => s.grades[month] !== undefined);
}

/**
 * The month shown first: the latest one with any captured grade; else the
 * current month when the cycle has it; else the cycle's last month.
 */
export function defaultMonth(data: MonthlyGrades, currentMonth: string): string {
  const captured = [...data.months].reverse().find((m) => hasAnyGrade(data.subjects, m));
  if (captured !== undefined) {
    return captured;
  }
  if (data.months.includes(currentMonth)) {
    return currentMonth;
  }
  return data.months[data.months.length - 1] ?? currentMonth;
}

/**
 * The stored grade as it is — 5.5 stays "5.5" even on a whole-number scale:
 * a family must never read a rounded grade. Up to 2 decimals, trailing zeros off.
 */
export function formatGrade(value: number): string {
  return String(Math.round(value * HUNDREDTHS) / HUNDREDTHS);
}

/** An average always reads with one decimal: 8 → "8.0". */
export function formatAverage(average: number): string {
  return average.toFixed(AVERAGE_DECIMALS);
}

/** A grade strictly below the scale's passing mark fails; without a scale nothing does. */
export function isFailing(value: number, scale: GradeScale | null): boolean {
  return scale !== null && value < scale.passing;
}

/** Whether any subject carries a grade in any month. */
export function hasCapturedGrades(data: MonthlyGrades): boolean {
  return data.subjects.some((s) => Object.keys(s.grades).length > 0);
}

/** Mean of the month's captured grades, half-up to one decimal; null when none is captured. */
export function monthAverage(subjects: MonthlySubject[], month: string): number | null {
  const values = subjects.map((s) => s.grades[month]).filter((v): v is number => v !== undefined);
  return averageHalfUp(values) ?? null;
}

/** "2026-09" → "Sep". */
export function shortMonthLabel(month: string): string {
  return monthLabel(month).slice(0, SHORT_LABEL_LENGTH);
}
