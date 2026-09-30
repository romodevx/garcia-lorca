import type { Cycle } from "../../config/api";
import type { EnrollmentConcept, Fee, PriceTier, TuitionPlan } from "../../finance/api";
import { centsToPesosInput, pesosToCents as strictPesosToCents } from "../../_lib/money";

/**
 * La aritmética del tabulador, sin pantalla.
 *
 * Vive aparte porque es lo único aquí que decide DINERO, y quería poder leerse
 * y probarse sin arrastrar catorce campos y cinco secciones detrás. Salió de un
 * `TuitionPanel.tsx` de mil líneas que hacía las tres cosas a la vez: calcular,
 * dibujar la forma y dibujar la tabla.
 */

/** The calendar's limit, not ours: February has no 29th three years in four. */
export const MAX_DUE_DAY = 28;

export const MONTH_NAMES = [
  "enero", "febrero", "marzo", "abril", "mayo", "junio",
  "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre",
];

/**
 * Lee los pesos que una persona teclea y devuelve centavos enteros, o null.
 *
 * Delega en el lector de `_lib/money.ts` en vez de tener el suyo. El que vivía
 * aquí borraba TODA coma con `replace(/[\s,$]/g, "")`, así que "1850,50" —una
 * coma decimal, que es como se escribe en media Latinoamérica— se convertía en
 * 185050 pesos: un sobrecobro de cien veces, multiplicado después por la
 * generación masiva de cargos. Es el mismo defecto que `_lib/money.ts` ya había
 * corregido y documentado; seguía vivo aquí porque esta pantalla tenía su
 * propia copia del lector.
 *
 * El contrato `number | null` se conserva —lo esperan siete llamadas de este
 * archivo— y devolver null en vez de 0 sigue siendo el punto: un vacío que se
 * volviera cero dejaría un nivel gratis por un año.
 */
export function pesosToCents(raw: string): number | null {
  const cents = strictPesosToCents(raw);
  return Number.isNaN(cents) ? null : cents;
}

/** Cents back into the pesos a person reads, always with both decimals. */
export function centsToPesos(cents: number): string {
  return centsToPesosInput(cents);
}

/**
 * The months of the school year, in the order they are collected, each labelled
 * the way a person says it.
 *
 * Derived from the plan being edited rather than from the calendar: a year that
 * opens in August and bills from September reaches June of the NEXT year, and a
 * picker that offered bare month names would make "junio" ambiguous in exactly
 * the case where getting it wrong costs a family a month.
 */
export function billedMonths(cycleStartsOn: string, firstMonth: number, count: number): BilledMonth[] {
  const start = new Date(`${cycleStartsOn}T00:00:00`);
  let year = start.getFullYear();
  if (start.getMonth() + 1 > firstMonth) {
    // The first billed month already passed this calendar year, so the school
    // means the one after the year turns.
    year++;
  }
  const out: BilledMonth[] = [];
  for (let i = 0; i < count; i++) {
    const month = ((firstMonth - 1 + i) % 12) + 1;
    if (i > 0 && month === 1) year++;
    out.push({ month, year, label: `${MONTH_NAMES[month - 1]} ${year}` });
  }
  return out;
}

/** One month of the year, named the way the school says it. */
export interface BilledMonth {
  month: number;
  year: number;
  label: string;
}

/**
 * What each month ends up costing once the fees are added.
 *
 * This is the answer to the only question the director actually has: "¿cuánto
 * se le cobra en septiembre?". The system does the dividing, so the screen has
 * to show the sum before anything is saved — otherwise the arithmetic is a
 * promise rather than a fact.
 */
export function monthlyTotals(months: BilledMonth[], monthlyCents: number, fees: readonly Fee[]): MonthTotal[] {
  const extra = new Map<number, number>();
  for (const fee of fees) {
    for (const [i, share] of feeShares(fee).entries()) {
      const month = fee.months[i];
      if (month === undefined) continue;
      extra.set(month, (extra.get(month) ?? 0) + share);
    }
  }
  return months.map((m) => ({
    ...m,
    feeCents: extra.get(m.month) ?? 0,
    totalCents: monthlyCents + (extra.get(m.month) ?? 0),
  }));
}

/** A month with what the fees add to it. */
export interface MonthTotal extends BilledMonth {
  feeCents: number;
  totalCents: number;
}

/**
 * A fee divided between its months, losing no cent.
 *
 * The remainder goes to the FIRST payment, exactly as the server does it. Two
 * implementations of one rule is a risk, but the alternative is a screen that
 * promises $333.33 three times while the school is charged $1,000 — and the
 * person deciding needs the true number before they save, not after.
 */
export function feeShares(fee: Fee): number[] {
  const n = fee.months.length;
  if (n === 0) return [];
  const base = Math.floor(fee.amountCents / n);
  const out = new Array<number>(n).fill(base);
  out[0] = base + (fee.amountCents - base * n);
  return out;
}

/**
 * What joining costs on each date the price changes.
 *
 * This is the answer to the director's only real question: "¿cuánto se le cobra
 * si se inscribe en marzo?". Every concept counts in every tier — a flat $4,700
 * is charged in March too — so reading only the steps that were typed would
 * price March at $3,300 instead of $8,850. The system does the arithmetic, so
 * the screen has to show the sum BEFORE anything is saved, or the total is a
 * promise rather than a fact.
 *
 * The rule matches the server's exactly (domain.AmountOn): the step that applies
 * is the last one whose date has arrived; before the first, the first.
 */
export function inscriptionTotals(concepts: readonly EnrollmentConcept[]): InscriptionTotal[] {
  const dates = [...new Set(concepts.flatMap((c) => c.tiers.map((t) => t.effectiveFrom)))]
    .filter((d) => d !== "")
    .sort();
  return dates.map((effectiveFrom) => ({
    effectiveFrom,
    totalCents: concepts.reduce((sum, c) => sum + amountOn(c, effectiveFrom), 0),
  }));
}

/** What the inscription costs on one date, as one line per tier. */
export interface InscriptionTotal {
  effectiveFrom: string;
  totalCents: number;
}

/** One concept's price on a date — the server's rule, in the browser. */
function amountOn(concept: EnrollmentConcept, on: string): number {
  const arrived = concept.tiers.filter((t) => t.effectiveFrom !== "" && t.effectiveFrom <= on);
  if (arrived.length === 0) {
    const earliest = [...concept.tiers].sort((a, b) => a.effectiveFrom.localeCompare(b.effectiveFrom))[0];
    return earliest?.amountCents ?? 0;
  }
  return arrived.reduce((best, t) => (t.effectiveFrom > best.effectiveFrom ? t : best)).amountCents;
}

/**
 * Whether two names are ONE line for the server — the rule it refuses a plan
 * by, in the browser.
 *
 * Two reductions, because the server holds both. The charge's reference is
 * built from the name's letters and digits with every other character DROPPED
 * ("Inscripción" → "inscripcin"), and the billing run recognises a line it
 * already issued by the name with its accents taken OFF ("inscripcion") — so
 * "INSCRIPCION" is the "Inscripción" a family already owes (T1, revisión
 * 2026-09-30). Two names equal under either are refused.
 */
export function sameLineName(a: string, b: string): boolean {
  return slugOf(a) === slugOf(b) || lineKey(a) === lineKey(b);
}

/** What the charge's reference is built from (the server's `Slugify`). */
function slugOf(raw: string): string {
  return raw.toLowerCase().replace(/[^a-z0-9]/g, "");
}

/** The name without accents, case or punctuation (the server's `LineKey`). */
export function lineKey(raw: string): string {
  return raw.normalize("NFD").replace(/\p{M}/gu, "").toLowerCase().replace(/[^a-z0-9]/g, "");
}

/**
 * What the server would refuse, said here first.
 *
 * Every rule below is one the domain enforces (ADR 0011). Repeating them costs a
 * duplicated rule, and the alternative costs the director a save that fails with
 * a message written for a programmer. The server still decides; this only stops
 * the trip.
 */
export function ladderProblem(concepts: readonly EnrollmentConcept[]): string {
  const seen: string[] = [];
  for (const concept of concepts) {
    if (concept.name.trim() === "") {
      return "Cada renglón de la inscripción necesita un nombre.";
    }
    // "Cuota APF" y "Cuota A.P.F." son dos nombres para una persona y UN cargo
    // para el cobro: el segundo renglón nunca se emitiría. Ver `sameLineName`.
    if (seen.some((name) => sameLineName(name, concept.name))) {
      return `Dos renglones se llaman casi igual ("${concept.name}"). Ponles nombres que se distingan.`;
    }
    seen.push(concept.name);

    if (concept.tiers.length === 0) {
      return `Falta decir cuánto cuesta "${concept.name}", y desde cuándo.`;
    }
    let previous: PriceTier | null = null;
    for (const tier of concept.tiers) {
      if (tier.effectiveFrom === "") {
        return `Falta la fecha desde la que aplica un monto de "${concept.name}".`;
      }
      if (tier.amountCents <= 0) {
        return `Falta el monto de "${concept.name}" a partir del ${tier.effectiveFrom}.`;
      }
      if (tier.effectiveTo !== "" && tier.effectiveTo < tier.effectiveFrom) {
        return `Una vigencia de "${concept.name}" termina antes de empezar.`;
      }
      if (previous && previous.effectiveTo !== "" && tier.effectiveFrom <= previous.effectiveTo) {
        // Dos precios para un mismo día: nada aguas abajo podría decir cuál
        // quiso el colegio.
        return `Dos vigencias de "${concept.name}" se enciman.`;
      }
      if (previous && tier.effectiveFrom <= previous.effectiveFrom) {
        return `Las fechas de "${concept.name}" deben ir hacia adelante, una por vigencia.`;
      }
      if (previous && tier.amountCents < previous.amountCents) {
        return `Un escalón de "${concept.name}" cuesta menos que el anterior. La inscripción sólo sube.`;
      }
      previous = tier;
    }
  }
  return "";
}

/**
 * What the form starts with.
 *
 * A plan written before the ladder carries ONE amount and no concepts. The form
 * no longer offers that field, so opening such a plan and pressing save would
 * store an inscription of nothing — the "familia a la que no se le cobra nada"
 * the billing run warns about. So the old amount comes in already converted: one
 * concept, named as the family reads it, effective from the day the cycle opens.
 * The director sees their own number and can split it from there.
 */
export function seedConcepts(plan: TuitionPlan | undefined, cycle: Cycle): EnrollmentConcept[] {
  if (plan === undefined) return [];
  if (plan.enrollmentConcepts.length > 0) return plan.enrollmentConcepts;
  if (plan.enrollmentCents <= 0) return [];
  return [{
    name: "Inscripción",
    tiers: [{ effectiveFrom: cycle.startsOn, effectiveTo: "", amountCents: plan.enrollmentCents }],
  }];
}

/**
 * Un entero de cero para arriba, o null.
 *
 * Existe porque los DÍAS DE PRONTO PAGO se estaban validando con `MAX_DUE_DAY`,
 * que significa "un día del mes que también existe en febrero". Son cosas
 * distintas: el pronto pago es un CONTEO de días hábiles y no tiene por qué caber
 * en un mes. Prestarle aquel tope volvía inguardable cualquier plan con 30.
 */
export function wholeAtLeastZero(raw: string): number | null {
  if (!/^\d+$/.test(raw.trim())) return null;
  return Number(raw.trim());
}

/** Reads a whole number in a range, or null. Same reasoning as pesosToCents. */
export function wholeInRange(raw: string, min: number, max: number): number | null {
  if (!/^\d+$/.test(raw.trim())) return null;
  const n = Number(raw.trim());
  return n >= min && n <= max ? n : null;
}
