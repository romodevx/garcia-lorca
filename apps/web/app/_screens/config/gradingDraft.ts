import type { LevelGrading } from "../../config/api";

/** A new level grades 5–10 in whole numbers and passes at 6 (owner decision, T10). */
export const DEFAULT_GRADING: LevelGrading = { enabled: true, min: 5, max: 10, decimals: 0, passing: 6 };

/**
 * Grades are whole numbers only (owner decision): the UI never offers decimals
 * and every scale it saves says so, whatever an older level had stored.
 */
const WHOLE_NUMBERS = 0;

/** The form's text, as typed. */
export interface GradingDraft {
  min: string;
  max: string;
  passing: string;
}

export type GradingField = "min" | "max" | "passing";
export type GradingErrors = Partial<Record<GradingField, string>>;

export function toDraft(g: LevelGrading): GradingDraft {
  return { min: String(g.min), max: String(g.max), passing: String(g.passing) };
}

/** Digits with at most two decimals: no sign (so never below 0), exponent, hex or inner space. */
const STRICT_NUMBER = /^\d+(?:\.\d{1,2})?$/;
const MAX_ALLOWED = 100;
const NOT_A_NUMBER = "Escribe un número, como 5 o 5,5 (hasta 2 decimales).";

/** The typed text as a number, or NaN when it is not one we accept. */
const number = (text: string): number => {
  const normalized = text.trim().replace(",", ".");
  return STRICT_NUMBER.test(normalized) ? Number(normalized) : Number.NaN;
};

/** 0 ≤ min < passing ≤ max ≤ 100; each field names its own problem. */
export function gradingErrors(d: GradingDraft): GradingErrors {
  const [min, max, passing] = [number(d.min), number(d.max), number(d.passing)];
  const errors: GradingErrors = {};
  if (Number.isNaN(min)) errors.min = NOT_A_NUMBER;
  if (Number.isNaN(max)) errors.max = NOT_A_NUMBER;
  else if (max > MAX_ALLOWED) errors.max = `La máxima no puede pasar de ${MAX_ALLOWED}.`;
  else if (!(max > min)) errors.max = "La máxima debe ser mayor que la mínima.";
  if (Number.isNaN(passing)) errors.passing = NOT_A_NUMBER;
  else if (!(passing > min) || passing > max) {
    errors.passing = "El mínimo aprobatorio debe ser mayor que la mínima y no pasar de la máxima.";
  }
  return errors;
}

export function fromDraft(d: GradingDraft, enabled: boolean): LevelGrading {
  return { enabled, min: number(d.min), max: number(d.max), decimals: WHOLE_NUMBERS, passing: number(d.passing) };
}
