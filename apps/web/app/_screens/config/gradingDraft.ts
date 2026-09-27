import type { LevelGrading } from "../../config/api";

/** Most Mexican levels grade 5–10 with one decimal and pass at 6. */
export const DEFAULT_GRADING: LevelGrading = { enabled: true, min: 5, max: 10, decimals: 1, passing: 6 };

/** The form's text, as typed. */
export interface GradingDraft {
  min: string;
  max: string;
  decimals: string;
  passing: string;
}

export type GradingField = "min" | "max" | "passing";
export type GradingErrors = Partial<Record<GradingField, string>>;

export function toDraft(g: LevelGrading): GradingDraft {
  return { min: String(g.min), max: String(g.max), decimals: String(g.decimals), passing: String(g.passing) };
}

const number = (text: string): number => (text.trim() === "" ? Number.NaN : Number(text.replace(",", ".")));

/** min < passing ≤ max; each field names its own problem. */
export function gradingErrors(d: GradingDraft): GradingErrors {
  const [min, max, passing] = [number(d.min), number(d.max), number(d.passing)];
  const errors: GradingErrors = {};
  if (Number.isNaN(min)) errors.min = "Escribe la calificación mínima.";
  if (Number.isNaN(max) || !(max > min)) errors.max = "La máxima debe ser mayor que la mínima.";
  if (Number.isNaN(passing) || !(passing > min) || passing > max) {
    errors.passing = "El mínimo aprobatorio debe ser mayor que la mínima y no pasar de la máxima.";
  }
  return errors;
}

export function fromDraft(d: GradingDraft, enabled: boolean): LevelGrading {
  return { enabled, min: number(d.min), max: number(d.max), decimals: Number(d.decimals), passing: number(d.passing) };
}
