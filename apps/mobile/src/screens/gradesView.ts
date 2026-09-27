import type { GradeScale, MonthlyGrades, MonthlySubject } from "../api/grades";
import { monthLabel } from "./attendanceView";

/**
 * Pure presentation helpers for the Calificaciones screen: which month opens
 * first, how a grade is written, whether it fails, and the month's average.
 */

const SHORT_LABEL_LENGTH = 3;

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

/** 9.5 → "9.5", written with the scale's decimals. */
export function formatGrade(value: number, scale: GradeScale): string {
  return value.toFixed(scale.decimals);
}

/** A grade strictly below the scale's passing mark fails. */
export function isFailing(value: number, scale: GradeScale): boolean {
  return value < scale.passing;
}

/** Mean of the month's captured grades, or null when none is captured. */
export function monthAverage(subjects: MonthlySubject[], month: string): number | null {
  const values = subjects.map((s) => s.grades[month]).filter((v): v is number => v !== undefined);
  if (values.length === 0) {
    return null;
  }
  return values.reduce((sum, v) => sum + v, 0) / values.length;
}

/** "2026-09" → "Sep". */
export function shortMonthLabel(month: string): string {
  return monthLabel(month).slice(0, SHORT_LABEL_LENGTH);
}
