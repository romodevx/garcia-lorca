/**
 * How a scholarship reads on screen (T18): its value, what it applies to and
 * the warning when a pupil holds two that overlap. Pure, so the table, the
 * drawer, the assignment page and the receipt say it the same way.
 */
import { formatCents } from "../_lib/money";
import type { ScholarshipKind, ScholarshipOverlap, ScholarshipTarget, ScholarshipTargetGroup } from "./api";

const BASIS_POINTS_PER_PERCENT = 100;
/** A percent typed with at most two decimals: "50", "12.5", "33.33". */
const PERCENT = /^(\d{1,3})(?:\.(\d{1,2}))?$/;

export const KIND_LABEL: Readonly<Record<ScholarshipKind, string>> = {
  percent: "Porcentaje",
  fixed_off: "Monto fijo",
  fixed_price: "Precio fijo",
};

export const TARGET_GROUP_LABEL: Readonly<Record<ScholarshipTargetGroup, string>> = {
  enrollment: "Inscripción",
  tuition: "Colegiatura",
  concept: "Conceptos",
  activity: "Actividades",
};

const GROUP_ORDER: readonly ScholarshipTargetGroup[] = ["enrollment", "tuition", "concept", "activity"];

/** 5000 → "50", 1250 → "12.5": the percent as a person types it. */
export function basisPointsToPercentInput(bps: number): string {
  return String(bps / BASIS_POINTS_PER_PERCENT);
}

/**
 * A typed percent into basis points, or NaN when it is not one. Strict like
 * pesosToCents: no comma decimal, no sign, no third decimal — the whole and
 * fractional parts are summed as integers so no float shifts a point.
 */
export function percentToBasisPoints(text: string): number {
  const match = PERCENT.exec(text.replace(/[\s%]/g, ""));
  if (match === null) return Number.NaN;
  const [, whole = "0", decimals = ""] = match;
  return Number(whole) * BASIS_POINTS_PER_PERCENT + Number(decimals.padEnd(2, "0"));
}

/** "50 %", "$500.00 de descuento", "Paga $1,000.00". */
export function scholarshipValueLabel(kind: ScholarshipKind, value: number): string {
  if (kind === "percent") return `${basisPointsToPercentInput(value)} %`;
  if (kind === "fixed_off") return `${formatCents(value)} de descuento`;
  return `Paga ${formatCents(value)}`;
}

/** "Beca Excelencia" — but a name that already says "Beca" is left alone. */
export function scholarshipTitle(name: string): string {
  const trimmed = name.trim();
  if (trimmed === "") return "Beca";
  return /^beca\b/i.test(trimmed) ? trimmed : `Beca ${trimmed}`;
}

/** What a target no longer offered (a retired concept or activity) is called. */
function retiredLabel(key: string): string {
  return key.startsWith("activity:") ? "Actividad retirada" : "Concepto retirado";
}

/** "Inscripción, Colegiatura, Natación", in the catalog's order. */
export function targetsSummary(keys: readonly string[], catalog: readonly ScholarshipTarget[]): string {
  if (keys.length === 0) return "—";
  const chosen = new Set(keys);
  const known = catalog.filter((t) => chosen.has(t.key)).map((t) => t.label);
  const retired = keys.filter((k) => !catalog.some((t) => t.key === k)).map(retiredLabel);
  return [...known, ...retired].join(", ");
}

export interface TargetGroup {
  group: ScholarshipTargetGroup;
  label: string;
  targets: ScholarshipTarget[];
}

/** The targets under Inscripción / Colegiatura / Conceptos / Actividades; empty groups left out. */
export function groupTargets(targets: readonly ScholarshipTarget[]): TargetGroup[] {
  return GROUP_ORDER.map((group) => ({ group, label: TARGET_GROUP_LABEL[group], targets: targets.filter((t) => t.group === group) }))
    .filter((g) => g.targets.length > 0);
}

/** Beside a holder who has another scholarship on the same charges. */
export function overlapWarning(overlaps: readonly ScholarshipOverlap[]): string {
  const names = overlaps.map((o) => scholarshipTitle(o.name)).join(", ");
  return `También tiene: ${names} — no se acumulan; se aplica la más favorable`;
}
