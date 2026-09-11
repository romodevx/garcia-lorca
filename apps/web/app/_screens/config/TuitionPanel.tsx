"use client";

import { useCallback, useEffect, useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { SlideOver } from "../../_design/SlideOver";
import type { Cycle, Level } from "../../config/api";
import {
  listTuitionPlans,
  runBilling,
  upsertTuitionPlan,
  type BillingRun,
  type EnrollmentConcept,
  type Fee,
  type PriceTier,
  type TuitionPlan,
} from "../../finance/api";
import { centsToPesosInput, formatCents, pesosToCents as strictPesosToCents } from "../../_lib/money";
import { DateField, MoneyField, NumberField, TextField } from "../../_design/form";
import { EmptyHint, ErrorAlert, Section } from "./parts";

import { API_URL } from "../../api-url";


/** The calendar's limit, not ours: February has no 29th three years in four. */
const MAX_DUE_DAY = 28;

const MONTH_NAMES = [
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
 * What the server would refuse, said here first.
 *
 * Every rule below is one the domain enforces (ADR 0011). Repeating them costs a
 * duplicated rule, and the alternative costs the director a save that fails with
 * a message written for a programmer. The server still decides; this only stops
 * the trip.
 */
export function ladderProblem(concepts: readonly EnrollmentConcept[]): string {
  const seen = new Set<string>();
  for (const concept of concepts) {
    if (concept.name.trim() === "") {
      return "Cada renglón de la inscripción necesita un nombre.";
    }
    // Compared as the SLUG, because that is what the charge's reference is built
    // from: "Cuota APF" and "Cuota A.P.F." are two names for a person and ONE
    // charge for the ledger, and the second line would never be issued.
    const slug = concept.name.toLowerCase().replace(/[^a-z0-9]/g, "");
    if (seen.has(slug)) {
      return `Dos renglones se llaman casi igual ("${concept.name}"). Ponles nombres que se distingan.`;
    }
    seen.add(slug);

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

/** Reads a whole number in a range, or null. Same reasoning as pesosToCents. */
function wholeInRange(raw: string, min: number, max: number): number | null {
  if (!/^\d+$/.test(raw.trim())) return null;
  const n = Number(raw.trim());
  return n >= min && n <= max ? n : null;
}

export interface TuitionPanelProps {
  cycle: Cycle;
  levels: readonly Level[];
}

/**
 * What each level costs, and the button that bills the month.
 *
 * The screen lives in Configuración because that is where a director looks for
 * it; the data belongs to finance, which is what actually issues the charges.
 *
 * A level with no table shows "Sin tabulador", never zeros. A school reading
 * $0.00 down a column cannot tell "free" from "nobody filled this in", and the
 * difference is a year of uncollected tuition.
 */
export function TuitionPanel({ cycle, levels }: TuitionPanelProps): ReactElement {
  const [plans, setPlans] = useState<TuitionPlan[]>([]);
  const [editing, setEditing] = useState<Level | null>(null);
  const [run, setRun] = useState<BillingRun | null>(null);
  const [error, setError] = useState<string | undefined>(undefined);
  const [running, setRunning] = useState(false);

  const reload = useCallback(() => {
    listTuitionPlans(API_URL, cycle.id)
      .then(setPlans)
      .catch(() => setError("No pudimos cargar los tabuladores."));
  }, [cycle.id]);

  useEffect(reload, [reload]);

  const planFor = (levelId: string): TuitionPlan | undefined => plans.find((p) => p.levelId === levelId);

  const generate = async (): Promise<void> => {
    setRunning(true);
    setError(undefined);
    try {
      setRun(await runBilling(API_URL, cycle.id));
      reload();
    } catch {
      setError("No se pudo generar los cargos del mes.");
    } finally {
      setRunning(false);
    }
  };

  return (
    <Section
      title="Colegiaturas"
      subtitle={`Costo por nivel para ${cycle.name}. Los cargos se generan mes a mes y lo no pagado forma adeudo.`}
    >
      {levels.length === 0 ? (
        <EmptyHint>Registra los niveles del colegio para poder ponerles precio.</EmptyHint>
      ) : (
        <table style={{ width: "100%", borderCollapse: "collapse", marginBottom: 16 }}>
          <thead>
            <tr style={{ textAlign: "left", fontSize: 12.5, color: "var(--text-muted)" }}>
              <th style={cell}>Nivel</th>
              <th style={cell}>Inscripción</th>
              <th style={cell}>Mensualidad</th>
              <th style={cell}>Mensualidades</th>
              <th style={cell}>Día límite</th>
              <th style={cell}>Recargo</th>
              <th style={cell} />
            </tr>
          </thead>
          <tbody>
            {levels.map((level) => {
              const plan = planFor(level.id);
              return (
                <tr key={level.id} style={{ borderTop: "1px solid var(--border)", fontSize: 14 }}>
                  <td style={{ ...cell, fontWeight: 600 }}>{level.name}</td>
                  {plan === undefined ? (
                    <td style={{ ...cell, color: "var(--text-faint)" }} colSpan={5}>
                      Sin tabulador · este nivel no se cobra
                    </td>
                  ) : (
                    <>
                      <td style={cell}>${centsToPesos(plan.enrollmentCents)}</td>
                      <td style={cell}>${centsToPesos(plan.monthlyCents)}</td>
                      <td style={cell}>
                        {plan.months} desde {MONTH_NAMES[plan.firstBilledMonth - 1]}
                      </td>
                      <td style={cell}>día {plan.dueDay}</td>
                      <td style={cell}>
                        {plan.policy.dailyMode
                          ? `$${centsToPesos(plan.policy.lateFeeDailyCents)} / día`
                          : plan.lateFeeCents === 0 ? "—" : `$${centsToPesos(plan.lateFeeCents)}`}
                      </td>
                    </>
                  )}
                  <td style={cell}>
                    <button
                      onClick={() => setEditing(level)}
                      style={{ fontSize: 13.5, fontWeight: 600, color: "var(--forest)", textDecoration: "underline" }}
                    >
                      {plan === undefined ? "Definir" : "Editar"}
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}

      {editing !== null && (
        <PlanForm
          cycle={cycle}
          level={editing}
          plan={planFor(editing.id)}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            reload();
          }}
        />
      )}

      <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
        <Btn icon="check" onClick={() => void generate()} disabled={running}>
          {running ? "Generando…" : "Generar cargos del mes"}
        </Btn>
        <span style={{ fontSize: 13, color: "var(--text-muted)" }}>
          Se puede presionar las veces que haga falta: no cobra dos veces lo mismo.
        </span>
      </div>

      {run !== null && <RunSummary run={run} />}
      {error !== undefined && <ErrorAlert message={error} />}
    </Section>
  );
}

/**
 * What the run did, in a sentence a person can act on.
 *
 * "Ya existían" is the number that makes a second press legible instead of
 * frightening — it is the visible face of the run being idempotent.
 */
function RunSummary({ run }: { run: BillingRun }): ReactElement {
  return (
    <div
      role="status"
      style={{ marginTop: 14, padding: "14px 16px", borderRadius: "var(--r-md)", background: "var(--ok-bg)", color: "var(--ok-fg)", fontSize: 14.5, lineHeight: 1.5 }}
    >
      <p>
        Se generaron <strong>{run.tuitionCreated} colegiaturas</strong>,{" "}
        {run.enrollmentCreated} inscripciones, {run.feesCreated} cuotas y{" "}
        {run.lateFeesCreated} recargos. {run.skipped} ya existían.
      </p>
      {run.enrollmentMoved > 0 && (
        <p style={{ marginTop: 8 }}>
          Se corrigió la fecha límite de <strong>{run.enrollmentMoved} inscripciones</strong> que
          ya estaban cobradas.
        </p>
      )}
      {run.levelsWithoutPlan.length > 0 && (
        <p style={{ marginTop: 8, fontWeight: 600 }}>
          Sin tabulador, no se cobró nada: {run.levelsWithoutPlan.join(", ")}.
        </p>
      )}
    </div>
  );
}

function PlanForm({
  cycle,
  level,
  plan,
  onClose,
  onSaved,
}: {
  cycle: Cycle;
  level: Level;
  plan: TuitionPlan | undefined;
  onClose: () => void;
  onSaved: () => void;
}): ReactElement {
  const [monthly, setMonthly] = useState(plan === undefined ? "" : centsToPesos(plan.monthlyCents));
  const [months, setMonths] = useState(String(plan?.months ?? 10));
  const [firstMonth, setFirstMonth] = useState(String(plan?.firstBilledMonth ?? 9));
  const [dueDay, setDueDay] = useState(String(plan?.dueDay ?? 10));
  // The M16 collection rule. Daily mode replaces the fixed monthly surcharge
  // with an amount charged for every day the money is late.
  const [dailyMode, setDailyMode] = useState(plan?.policy.dailyMode ?? false);
  const [dailyFee, setDailyFee] = useState(
    plan === undefined || plan.policy.lateFeeDailyCents === 0 ? "" : centsToPesos(plan.policy.lateFeeDailyCents));
  const [allDays, setAllDays] = useState(plan?.policy.surchargeCountsAllDays ?? true);
  const [earlyAmount, setEarlyAmount] = useState(
    plan === undefined || plan.policy.earlyPaymentCents === 0 ? "" : centsToPesos(plan.policy.earlyPaymentCents));
  const [earlyDays, setEarlyDays] = useState(String(plan?.policy.earlyPaymentDays ?? 0));
  // The inscription's own deadline. It is a separate charge and the school
  // collects it on its own date — in June, for a year that opens in August.
  const [enrollmentDue, setEnrollmentDue] = useState(plan?.enrollmentDueOn ?? "");
  // The extras collected on top of the tuition, each already whole: the school
  // states what it collects and the system divides it.
  const [fees, setFees] = useState<Fee[]>(plan?.fees ?? []);
  // The inscription, as the price list prints it: one line per concept, each
  // with what it costs from a date onward.
  const [concepts, setConcepts] = useState<EnrollmentConcept[]>(() => seedConcepts(plan, cycle));
  const [error, setError] = useState<string | undefined>(undefined);
  const [pending, setPending] = useState(false);

  const submit = async (): Promise<void> => {
    const monthlyCents = pesosToCents(monthly);
    if (monthlyCents === null) {
      setError("Escribe la colegiatura mensual, en pesos.");
      return;
    }
    const day = wholeInRange(dueDay, 1, MAX_DUE_DAY);
    if (day === null) {
      // Checked here as well as at the server: the 31st does not exist in
      // February, and a person should not need a round trip to be told so.
      setError(`El día límite debe estar entre 1 y ${MAX_DUE_DAY}, para que exista también en febrero.`);
      return;
    }
    const count = wholeInRange(months, 1, 12);
    if (count === null) {
      setError("Las mensualidades van de 1 a 12.");
      return;
    }
    const startMonth = wholeInRange(firstMonth, 1, 12);
    if (startMonth === null) {
      setError("El mes de inicio va de 1 a 12.");
      return;
    }

    const dailyCents = pesosToCents(dailyFee) ?? 0;
    if (dailyMode && dailyCents <= 0) {
      // The same rule the database enforces. Said here so the person reads what
      // is missing instead of a constraint violation.
      setError("El recargo diario necesita un monto, o no cobraría nada.");
      return;
    }
    const window = wholeInRange(earlyDays, 0, MAX_DUE_DAY);
    if (window === null) {
      setError(`Los días de pronto pago van de 0 a ${MAX_DUE_DAY}.`);
      return;
    }
    if (concepts.length === 0) {
      setError("Captura la inscripción: al menos un concepto con su monto y vigencia.");
      return;
    }
    const ladder = ladderProblem(concepts);
    if (ladder !== "") {
      // Every one of these is a rule the server enforces too. Said here so the
      // director reads what is wrong instead of a message written for a
      // programmer, three seconds later.
      setError(ladder);
      return;
    }
    // Aquí vivía una validación de formato aaaa-mm-dd y su mensaje. Con
    // calendario, la fecha sólo puede llegar bien formada o vacía, así que ese
    // error dejó de poder ocurrir.

    setPending(true);
    setError(undefined);
    try {
      await upsertTuitionPlan(API_URL, {
        cycleId: cycle.id,
        levelId: level.id,
        // An unset inscription or surcharge means zero, and zero is a real
        // answer here: some schools charge neither.
        // One shape or the other, never both — the server refuses a plan that
        // carries a single amount AND a list of concepts (ADR 0011).
        // Siempre cero: la inscripción vive en los conceptos, y el servidor
        // rechaza un plan que lleve las dos formas a la vez (ADR 0011).
        enrollmentCents: 0,
        enrollmentConcepts: concepts,
        monthlyCents,
        months: count,
        firstBilledMonth: startMonth,
        dueDay: day,
        // The school does not use the fixed monthly surcharge or the level
        // discount, and asked for both to go. Zero is what disables them
        // downstream, so a level that used to carry one is cleared by saving.
        lateFeeCents: 0,
        monthlyDiscountCents: 0,
        // The mes:monto schedule went with them: splitting a payment is what
        // the fees below do now, and saying it twice invited two answers.
        schedule: [],
        active: true,
        fees,
        enrollmentDueOn: enrollmentDue.trim(),
        policy: {
          dailyMode,
          lateFeeDailyCents: dailyCents,
          surchargeCountsAllDays: allDays,
          earlyPaymentCents: pesosToCents(earlyAmount) ?? 0,
          earlyPaymentDays: window,
        },
      });
      onSaved();
    } catch {
      setError("No se pudo guardar el tabulador. Revisa los datos.");
    } finally {
      setPending(false);
    }
  };

  return (
    <SlideOver
      title={`Precios de ${level.name}`}
      subtitle="Inscripción, colegiatura, cobranza y cuotas"
      icon="finance"
      submitLabel="Guardar tabulador"
      pending={pending}
      error={error}
      onClose={onClose}
      onSubmit={() => void submit()}
    >
      <InscriptionSection concepts={concepts} onChange={setConcepts} onError={setError} />

      <p style={{ fontSize: 13.5, fontWeight: 700, color: "var(--text-muted)", marginTop: 18, marginBottom: 10 }}>
        Colegiatura
      </p>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(190px, 1fr))", gap: 12, marginBottom: 12 }}>
        <MoneyField label="Colegiatura mensual" value={monthly} onChange={setMonthly} placeholder="3200.00" />
        <NumberField label="Mensualidades" value={months} onChange={setMonths} placeholder="10" />
        <NumberField label="Mes de inicio" value={firstMonth} onChange={setFirstMonth} placeholder="9" />
        {/*
          Sin min/max nativos a propósito: esta pantalla ya revisa el rango y
          explica por qué ("el 31 no existe en febrero"). El navegador, con
          `max`, bloquearía el envío con un globo suyo, en su idioma, y esa
          frase se perdería.
        */}
        <NumberField label="Día límite" value={dueDay} onChange={setDueDay} placeholder="10" />
        <DateField
          label="Fecha límite de inscripción"
          value={enrollmentDue}
          onChange={setEnrollmentDue}
        />
      </div>

      <div style={{ borderTop: "1px solid var(--border)", paddingTop: 14, marginBottom: 12 }}>
        <p style={{ fontSize: 13.5, fontWeight: 700, color: "var(--text-muted)", marginBottom: 10 }}>
          Cobranza
        </p>
        <label style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 12, cursor: "pointer" }}>
          <input
            type="checkbox"
            checked={dailyMode}
            onChange={(e) => setDailyMode(e.target.checked)}
            aria-label="Cobrar recargo por día"
            style={{ width: 18, height: 18 }}
          />
          <span style={{ fontSize: 14, color: "var(--text)" }}>
            Cobrar recargo por día en vez del monto fijo mensual
          </span>
        </label>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(190px, 1fr))", gap: 12 }}>
          <MoneyField label="Recargo por día" value={dailyFee} onChange={setDailyFee} placeholder="10.00" />
          <MoneyField label="Pronto pago (monto)" value={earlyAmount} onChange={setEarlyAmount} placeholder="100.00" />
          <NumberField label="Pronto pago (días hábiles)" value={earlyDays} onChange={setEarlyDays} placeholder="5" />
        </div>
        <label style={{ display: "flex", alignItems: "center", gap: 10, marginTop: 12, cursor: "pointer" }}>
          <input
            type="checkbox"
            checked={allDays}
            onChange={(e) => setAllDays(e.target.checked)}
            aria-label="Contar sábados, domingos y feriados"
            style={{ width: 18, height: 18 }}
          />
          <span style={{ fontSize: 14, color: "var(--text)" }}>
            Contar sábados, domingos y feriados en el recargo
          </span>
        </label>
      </div>
      <FeesSection
        cycle={cycle}
        firstMonth={wholeInRange(firstMonth, 1, 12) ?? 9}
        months={wholeInRange(months, 1, 12) ?? 10}
        monthlyCents={pesosToCents(monthly) ?? 0}
        fees={fees}
        onChange={setFees}
        onError={setError}
      />

    </SlideOver>
  );
}

const cell = { padding: "10px 8px" } as const;

/**
 * The inscription, as the school's price list prints it: one line per concept,
 * each with what it costs from a date onward.
 *
 * The totals table is the point of the whole section. The director's question is
 * never "¿cuánto vale el escalón de marzo?" — it is "¿cuánto se le cobra a
 * alguien que se inscribe en marzo?", which is the sum of every line at that
 * date, flat ones included. Making them add it up by hand is how a school
 * publishes a flyer that its own system contradicts.
 */
function InscriptionSection({ concepts, onChange, onError }: InscriptionSectionProps): ReactElement {
  const [name, setName] = useState("");

  const addConcept = (): void => {
    if (name.trim() === "") {
      onError("Escribe el nombre del concepto.");
      return;
    }
    const slug = slugOf(name);
    if (concepts.some((c) => slugOf(c.name) === slug)) {
      onError(`Ya existe un renglón que se llama casi igual que "${name.trim()}".`);
      return;
    }
    onError(undefined);
    setName("");
    onChange([...concepts, { name: name.trim(), tiers: [] }]);
  };

  const patch = (index: number, tiers: PriceTier[]): void =>
    onChange(concepts.map((c, i) => (i === index ? { ...c, tiers } : c)));

  const totals = inscriptionTotals(concepts);
  const problem = ladderProblem(concepts);

  return (
    <div style={{ marginBottom: 12 }}>
      <p style={{ fontSize: 13.5, fontWeight: 700, color: "var(--text-muted)", marginBottom: 4 }}>
        Inscripción
      </p>
      <p style={{ fontSize: 12.5, color: "var(--text-faint)", marginBottom: 12 }}>
        Lo que se cobra por inscribirse, renglón por renglón. Escribe el concepto <b>una vez</b> y
        agrégale cada monto con las fechas entre las que está vigente. Si un concepto no cambia de
        precio, basta un monto sin fecha de fin.
      </p>

      {concepts.map((concept, index) => (
        <ConceptCard
          key={`${concept.name}-${index}`}
          concept={concept}
          onChangeTiers={(tiers) => patch(index, tiers)}
          onRemove={() => onChange(concepts.filter((_, i) => i !== index))}
          onError={onError}
        />
      ))}

      <div style={{ display: "flex", gap: 12, alignItems: "flex-end", flexWrap: "wrap", marginTop: 4 }}>
        <div style={{ flex: "1 1 240px" }}>
          <TextField label="Nuevo concepto" value={name} onChange={setName} uppercase placeholder="GASTOS ADM Y SEGURO" />
        </div>
        <Btn kind="ghost" icon="plus" onClick={addConcept}>Agregar concepto</Btn>
      </div>

      {problem !== "" && (
        <p role="alert" style={{ marginTop: 12, fontSize: 13, fontWeight: 600, color: "var(--danger-fg, #b42318)" }}>
          {problem}
        </p>
      )}

      {totals.length > 0 && (
        <div style={{ marginTop: 16 }}>
          <p style={{ fontSize: 12.5, fontWeight: 600, color: "var(--text-muted)", marginBottom: 8 }}>
            Total a pagar por inscribirse
          </p>
          <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13.5 }}>
            <tbody>
              {totals.map((t, i) => (
                <tr key={t.effectiveFrom} style={{ borderTop: "1px solid var(--border)" }}>
                  <td style={{ padding: "7px 4px", color: "var(--text-muted)" }}>
                    {i === totals.length - 1 ? `Del ${t.effectiveFrom} en adelante` : `Desde el ${t.effectiveFrom}`}
                  </td>
                  <td style={{ padding: "7px 4px", textAlign: "right", fontWeight: 700, color: "var(--text)" }}>
                    {formatCents(t.totalCents)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <p style={{ marginTop: 6, fontSize: 12, color: "var(--text-faint)" }}>
            Pasada la última vigencia se sigue cobrando ese último monto, hasta que captures otro.
          </p>
        </div>
      )}
    </div>
  );
}

/** Compared the way the server builds the charge's reference. */
function slugOf(raw: string): string {
  return raw.toLowerCase().replace(/[^a-z0-9]/g, "");
}

/**
 * One concept and the amounts it has had.
 *
 * The name is typed once and never again: what changes over the year is the
 * MONEY, not what the line is called. Asking for the name with every amount —
 * which is what the first version did — invited two rows for one concept and a
 * validation error instead of the merge the director meant.
 */
function ConceptCard({ concept, onChangeTiers, onRemove, onError }: ConceptCardProps): ReactElement {
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [amount, setAmount] = useState("");

  const addAmount = (): void => {
    const cents = pesosToCents(amount);
    if (cents === null || from.trim() === "") {
      onError(`Para agregar un monto a "${concept.name}": el monto y la fecha desde la que aplica.`);
      return;
    }
    onError(undefined);
    setAmount("");
    setFrom("");
    setTo("");
    onChangeTiers(
      [...concept.tiers, { effectiveFrom: from.trim(), effectiveTo: to.trim(), amountCents: cents }]
        // En orden de fecha, que es como se cobra: así el director teclea en el
        // orden que recuerde y el renglón se lee como la lista de precios.
        .sort((a, b) => a.effectiveFrom.localeCompare(b.effectiveFrom)),
    );
  };

  return (
    <div style={{ borderRadius: "var(--r-md)", border: "1px solid var(--border)", padding: "12px 14px", marginBottom: 12 }}>
      <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 10 }}>
        <p style={{ fontSize: 14.5, fontWeight: 700, color: "var(--text)" }}>{concept.name}</p>
        <div style={{ flex: 1 }} />
        <button
          type="button"
          onClick={onRemove}
          aria-label={`Quitar ${concept.name}`}
          style={{ background: "transparent", border: "none", color: "var(--text-muted)", fontSize: 13.5, fontWeight: 600, cursor: "pointer" }}>
          Quitar concepto
        </button>
      </div>

      {concept.tiers.length === 0 ? (
        <p style={{ fontSize: 12.5, color: "var(--text-faint)", marginBottom: 10 }}>
          Todavía no tiene monto. Agrégale uno abajo.
        </p>
      ) : (
        <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13.5, marginBottom: 10 }}>
          <tbody>
            {concept.tiers.map((tier, i) => (
              <tr key={`${tier.effectiveFrom}-${i}`} style={{ borderTop: i === 0 ? "none" : "1px solid var(--border)" }}>
                <td style={{ padding: "6px 4px", fontWeight: 700, color: "var(--text)" }}>
                  {formatCents(tier.amountCents)}
                </td>
                <td style={{ padding: "6px 4px", color: "var(--text-muted)" }}>
                  {tier.effectiveTo === ""
                    ? `desde el ${tier.effectiveFrom}, sin fecha de fin`
                    : `del ${tier.effectiveFrom} al ${tier.effectiveTo}`}
                </td>
                <td style={{ padding: "6px 4px", textAlign: "right" }}>
                  <button
                    type="button"
                    onClick={() => onChangeTiers(concept.tiers.filter((_, j) => j !== i))}
                    aria-label={`Quitar el monto de ${formatCents(tier.amountCents)} de ${concept.name}`}
                    style={{ background: "transparent", border: "none", color: "var(--text-muted)", fontSize: 13, fontWeight: 600, cursor: "pointer" }}>
                    Quitar
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(150px, 1fr))", gap: 10, marginBottom: 10 }}>
        <MoneyField label="Monto" value={amount} onChange={setAmount} placeholder="2200.00" />
        <DateField label="Vigente desde" value={from} onChange={setFrom} />
        <DateField label="Vigente hasta (opcional)" value={to} onChange={setTo} />
      </div>
      <Btn kind="ghost" icon="plus" onClick={addAmount}>Agregar monto con su vigencia</Btn>
    </div>
  );
}

interface ConceptCardProps {
  concept: EnrollmentConcept;
  onChangeTiers: (tiers: PriceTier[]) => void;
  onRemove: () => void;
  onError: (message: string | undefined) => void;
}

interface InscriptionSectionProps {
  concepts: EnrollmentConcept[];
  onChange: (concepts: EnrollmentConcept[]) => void;
  onError: (message: string | undefined) => void;
}

/**
 * The extras a school collects on top of the tuition.
 *
 * This replaced a text box that asked for "9:3200, 12:1600, 3:4800". The school
 * called that confusing and they were right: it made the director do the
 * arithmetic, express the tuition and the extra as one number, and then read it
 * back a year later with no way to tell which part was which.
 *
 * What it asks for now is what the school actually says out loud: this is the
 * July fee, it is $1,600, collect it in September and October. The dividing is
 * the system's job, and the table underneath shows what each month ends up
 * costing so the sum is a fact before anything is saved rather than a promise.
 */
function FeesSection({
  cycle,
  firstMonth,
  months,
  monthlyCents,
  fees,
  onChange,
  onError,
}: {
  cycle: Cycle;
  firstMonth: number;
  months: number;
  monthlyCents: number;
  fees: Fee[];
  onChange: (fees: Fee[]) => void;
  onError: (message: string) => void;
}): ReactElement {
  const [name, setName] = useState("");
  const [amount, setAmount] = useState("");
  const [picked, setPicked] = useState<number[]>([]);

  const calendar = billedMonths(cycle.startsOn, firstMonth, months);
  const totals = monthlyTotals(calendar, monthlyCents, fees);

  const add = (): void => {
    const cents = pesosToCents(amount);
    if (name.trim() === "" || cents === null || cents <= 0) {
      onError("Una cuota necesita nombre y monto.");
      return;
    }
    if (picked.length === 0) {
      onError("Elige en qué meses se cobra la cuota.");
      return;
    }
    if (cents < picked.length) {
      // Split so thin a share rounds to nothing. The server refuses it too; a
      // person should not need a round trip to learn so.
      onError("La cuota es muy chica para partirla entre esos meses.");
      return;
    }
    // Kept in the order the year collects them, not the order they were
    // clicked: the parts are numbered "1 de 2" on the family's statement, and
    // part 1 falling after part 2 is a support call.
    const order = calendar.map((m) => m.month);
    onChange([...fees, {
      name: name.trim(),
      amountCents: cents,
      months: [...picked].sort((a, b) => order.indexOf(a) - order.indexOf(b)),
    }]);
    setName("");
    setAmount("");
    setPicked([]);
  };

  const toggle = (month: number): void => {
    setPicked((current) =>
      current.includes(month) ? current.filter((m) => m !== month) : [...current, month]);
  };

  const labelOf = (month: number): string =>
    calendar.find((m) => m.month === month)?.label ?? MONTH_NAMES[month - 1] ?? "";

  return (
    <div style={{ borderTop: "1px solid var(--border)", paddingTop: 14, marginBottom: 12 }}>
      <p style={{ fontSize: 13.5, fontWeight: 700, color: "var(--text-muted)", marginBottom: 4 }}>
        Cuotas (opcional)
      </p>
      <p style={{ fontSize: 12.5, color: "var(--text-faint)", marginBottom: 12 }}>
        Lo que se cobra aparte de la colegiatura — la cuota de julio, la graduación.
        Escribe el monto <b>completo</b> y elige en qué meses se cobra: el sistema lo parte
        y lo suma a esos meses.
      </p>

      {fees.length > 0 && (
        <div style={{ borderRadius: "var(--r-md)", border: "1px solid var(--border)", overflow: "hidden", marginBottom: 14 }}>
          {fees.map((fee, index) => (
            <div
              key={`${fee.name}-${index}`}
              style={{ display: "flex", alignItems: "center", gap: 12, padding: "11px 14px", borderTop: index === 0 ? "none" : "1px solid var(--border)" }}>
              <div style={{ minWidth: 0 }}>
                <p style={{ fontSize: 14.5, fontWeight: 600, color: "var(--text)" }}>{fee.name}</p>
                <p style={{ fontSize: 12.5, color: "var(--text-muted)" }}>
                  {formatCents(fee.amountCents)} ·{" "}
                  {feeShares(fee)
                    .map((share, i) => `${formatCents(share)} en ${labelOf(fee.months[i] ?? 0)}`)
                    .join(" · ")}
                </p>
              </div>
              <div style={{ flex: 1 }} />
              <button
                type="button"
                onClick={() => onChange(fees.filter((_, i) => i !== index))}
                aria-label={`Quitar ${fee.name}`}
                style={{ background: "transparent", border: "none", color: "var(--text-muted)", fontSize: 13.5, fontWeight: 600, cursor: "pointer" }}>
                Quitar
              </button>
            </div>
          ))}
        </div>
      )}

      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(190px, 1fr))", gap: 12, marginBottom: 12 }}>
        <TextField label="Nombre de la cuota" value={name} onChange={setName} uppercase placeholder="CUOTA DE JULIO" />
        <MoneyField label="Monto total" value={amount} onChange={setAmount} placeholder="1600.00" />
      </div>
      <p style={{ fontSize: 12.5, fontWeight: 600, color: "var(--text-muted)", marginBottom: 8 }}>
        ¿En qué meses se cobra?
      </p>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 8, marginBottom: 12 }}>
        {calendar.map((m) => {
          const on = picked.includes(m.month);
          return (
            <button
              key={`${m.year}-${m.month}`}
              type="button"
              onClick={() => toggle(m.month)}
              aria-pressed={on}
              style={{ height: 38, padding: "0 14px", borderRadius: 99, fontSize: 13.5, fontWeight: 600, cursor: "pointer", border: `1.5px solid ${on ? "var(--lime)" : "var(--border)"}`, background: on ? "color-mix(in oklab, var(--lime) 12%, var(--surface))" : "var(--surface)", color: on ? "var(--forest)" : "var(--text-muted)" }}>
              {m.label}
            </button>
          );
        })}
      </div>
      <Btn kind="ghost" icon="plus" onClick={add}>Agregar cuota</Btn>

      {fees.length > 0 && (
        <div style={{ marginTop: 16 }}>
          <p style={{ fontSize: 12.5, fontWeight: 600, color: "var(--text-muted)", marginBottom: 8 }}>
            Lo que se cobrará cada mes
          </p>
          <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13.5 }}>
            <tbody>
              {totals.map((m) => (
                <tr key={`${m.year}-${m.month}`} style={{ borderTop: "1px solid var(--border)" }}>
                  <td style={{ padding: "7px 4px", color: "var(--text-muted)" }}>{m.label}</td>
                  <td style={{ padding: "7px 4px", textAlign: "right", color: "var(--text-muted)" }}>
                    {m.feeCents > 0 ? `+ ${formatCents(m.feeCents)}` : ""}
                  </td>
                  <td style={{ padding: "7px 4px", textAlign: "right", fontWeight: 700, color: "var(--text)" }}>
                    {formatCents(m.totalCents)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
