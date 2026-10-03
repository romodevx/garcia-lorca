/**
 * The scholarship form as typed (T18), and the one function that reads it
 * into the contract's body or says, field by field, what is wrong.
 *
 * The value stays TEXT until `readDraft`: a percent becomes basis points and
 * an amount becomes cents only there, so nothing typed is silently a zero.
 */
import { centsToPesosInput, pesosToCents } from "../../_lib/money";
import type { Scholarship, ScholarshipInput, ScholarshipKind } from "../../scholarships/api";
import { basisPointsToPercentInput, percentToBasisPoints } from "../../scholarships/format";

export interface ScholarshipDraft {
  name: string;
  kind: ScholarshipKind;
  /** A percent ("12.5") or pesos ("1,500.00"), per kind. */
  value: string;
  targets: string[];
  active: boolean;
}

export type DraftErrors = Partial<Record<"name" | "value" | "targets", string>>;

export const MAX_NAME = 120;
export const MAX_TARGETS = 50;
const MIN_BASIS_POINTS = 1;
const MAX_BASIS_POINTS = 10000;

const MISSING = "Falta este dato";
const NO_TARGET = "Elige al menos a qué se aplica.";
const TOO_MANY_TARGETS = `Una beca se aplica a ${MAX_TARGETS} cosas como máximo.`;
const LONG_NAME = `El nombre lleva ${MAX_NAME} letras como máximo.`;
const PERCENT_SHAPE = "Escribe el porcentaje con punto decimal y hasta dos decimales.";
const PERCENT_RANGE = "El porcentaje va de 0.01 a 100.";
const MONEY_SHAPE = "Escribe un monto en pesos, por ejemplo 500.00.";
const DISCOUNT_ZERO = "El descuento debe ser mayor a cero.";

export function emptyDraft(): ScholarshipDraft {
  return { name: "", kind: "percent", value: "", targets: [], active: true };
}

export function draftFromScholarship(s: Scholarship): ScholarshipDraft {
  const value = s.kind === "percent" ? basisPointsToPercentInput(s.value) : centsToPesosInput(s.value);
  return { name: s.name, kind: s.kind, value, targets: [...s.targets], active: s.active };
}

/** The value as the contract wants it (basis points or cents), or NaN. */
function wireValue(kind: ScholarshipKind, text: string): number {
  return kind === "percent" ? percentToBasisPoints(text) : pesosToCents(text);
}

/**
 * What is WRONG with a typed value, said on blur and again on save. An empty
 * one is not wrong — it is missing, and that is said on save only.
 */
export function valueError(kind: ScholarshipKind, text: string): string | undefined {
  if (text.trim() === "") return undefined;
  const value = wireValue(kind, text);
  if (kind === "percent") {
    if (Number.isNaN(value)) return PERCENT_SHAPE;
    return value < MIN_BASIS_POINTS || value > MAX_BASIS_POINTS ? PERCENT_RANGE : undefined;
  }
  if (Number.isNaN(value)) return MONEY_SHAPE;
  return kind === "fixed_off" && value <= 0 ? DISCOUNT_ZERO : undefined;
}

function nameError(name: string): string | undefined {
  if (name.trim() === "") return MISSING;
  return name.trim().length > MAX_NAME ? LONG_NAME : undefined;
}

function targetsError(targets: readonly string[]): string | undefined {
  if (targets.length === 0) return NO_TARGET;
  return targets.length > MAX_TARGETS ? TOO_MANY_TARGETS : undefined;
}

export function readDraft(draft: ScholarshipDraft): { input?: ScholarshipInput; errors: DraftErrors } {
  const found: DraftErrors = {
    name: nameError(draft.name),
    value: draft.value.trim() === "" ? MISSING : valueError(draft.kind, draft.value),
    targets: targetsError(draft.targets),
  };
  const errors = Object.fromEntries(Object.entries(found).filter(([, v]) => v !== undefined)) as DraftErrors;
  if (Object.keys(errors).length > 0) return { errors };
  return {
    input: { name: draft.name.trim(), kind: draft.kind, value: wireValue(draft.kind, draft.value), targets: draft.targets, active: draft.active },
    errors,
  };
}
