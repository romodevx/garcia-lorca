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
  type Installment,
  type TuitionPlan,
} from "../../finance/api";
import { EmptyHint, ErrorAlert, Section, TextField } from "./parts";

import { API_URL } from "../../api-url";


/** The calendar's limit, not ours: February has no 29th three years in four. */
const MAX_DUE_DAY = 28;

const MONTH_NAMES = [
  "enero", "febrero", "marzo", "abril", "mayo", "junio",
  "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre",
];

/**
 * Reads pesos as a person types them and returns integer cents.
 *
 * Everything behind this screen is integer cents, and the person in front of it
 * thinks in pesos. Returning null rather than 0 for nonsense is the whole point:
 * a blank that silently became zero would price a level free for a year.
 */
export function pesosToCents(raw: string): number | null {
  const cleaned = raw.replace(/[\s,$]/g, "");
  if (cleaned === "" || !/^\d+(\.\d{1,2})?$/.test(cleaned)) return null;
  // Rounded, not truncated: 0.1 + 0.2 in floating point is why this multiplies
  // and then rounds instead of trusting the product.
  return Math.round(Number(cleaned) * 100);
}

/** Cents back into the pesos a person reads, always with both decimals. */
export function centsToPesos(cents: number): string {
  return (cents / 100).toFixed(2);
}

/**
 * The payment plan as a person writes it: "9:3200, 12:1600, 3:4800" — month and
 * amount in pesos, in the order they are collected.
 *
 * Text and not a row builder because a school states this once a year and reads
 * it back far more often than it edits it. One line is easier to check against
 * a printed circular than eleven boxes.
 */
export function textToSchedule(raw: string): Installment[] | null {
  const trimmed = raw.trim();
  if (trimmed === "") {
    return [];
  }
  const out: Installment[] = [];
  for (const part of trimmed.split(",")) {
    const [month, amount] = part.split(":");
    if (month === undefined || amount === undefined) {
      return null;
    }
    const m = wholeInRange(month.trim(), 1, 12);
    const cents = pesosToCents(amount.trim());
    if (m === null || cents === null || cents <= 0) {
      return null;
    }
    out.push({ month: m, amountCents: cents });
  }
  return out;
}

/** The same plan, back as text, for the form to show. */
export function scheduleToText(schedule: readonly Installment[]): string {
  return schedule.map((i) => `${i.month}:${centsToPesos(i.amountCents)}`).join(", ");
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
        {run.enrollmentCreated} inscripciones y {run.lateFeesCreated} recargos.{" "}
        {run.skipped} ya existían.
      </p>
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
  const [enrollment, setEnrollment] = useState(plan === undefined ? "" : centsToPesos(plan.enrollmentCents));
  const [monthly, setMonthly] = useState(plan === undefined ? "" : centsToPesos(plan.monthlyCents));
  const [months, setMonths] = useState(String(plan?.months ?? 10));
  const [firstMonth, setFirstMonth] = useState(String(plan?.firstBilledMonth ?? 9));
  const [dueDay, setDueDay] = useState(String(plan?.dueDay ?? 10));
  const [lateFee, setLateFee] = useState(plan === undefined ? "" : centsToPesos(plan.lateFeeCents));
  // The M16 collection rule. Daily mode replaces the fixed monthly surcharge
  // with an amount charged for every day the money is late.
  const [dailyMode, setDailyMode] = useState(plan?.policy.dailyMode ?? false);
  const [dailyFee, setDailyFee] = useState(
    plan === undefined || plan.policy.lateFeeDailyCents === 0 ? "" : centsToPesos(plan.policy.lateFeeDailyCents));
  const [allDays, setAllDays] = useState(plan?.policy.surchargeCountsAllDays ?? true);
  const [earlyAmount, setEarlyAmount] = useState(
    plan === undefined || plan.policy.earlyPaymentCents === 0 ? "" : centsToPesos(plan.policy.earlyPaymentCents));
  const [earlyDays, setEarlyDays] = useState(String(plan?.policy.earlyPaymentDays ?? 0));
  // An explicit payment plan, written as "9:1000, 12:500, 3:1500" — month and
  // amount. Empty keeps the equal year, which is what most schools want.
  const [schedule, setSchedule] = useState(scheduleToText(plan?.schedule ?? []));
  // The school's standing arrangement for this level — maternal, most often.
  const [levelDiscount, setLevelDiscount] = useState(
    plan === undefined || plan.monthlyDiscountCents === 0 ? "" : centsToPesos(plan.monthlyDiscountCents));
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
    const discountCents = pesosToCents(levelDiscount) ?? 0;
    if (discountCents > monthlyCents) {
      // The same rule the database enforces. A charge that owes negative money
      // would pay for itself and then some.
      setError("El descuento no puede ser mayor que la colegiatura del mes.");
      return;
    }
    const plan = textToSchedule(schedule);
    if (plan === null) {
      setError("Las cuotas se escriben mes:monto, por ejemplo 9:3200, 12:1600, 3:4800.");
      return;
    }

    setPending(true);
    setError(undefined);
    try {
      await upsertTuitionPlan(API_URL, {
        cycleId: cycle.id,
        levelId: level.id,
        // An unset inscription or surcharge means zero, and zero is a real
        // answer here: some schools charge neither.
        enrollmentCents: pesosToCents(enrollment) ?? 0,
        monthlyCents,
        months: count,
        firstBilledMonth: startMonth,
        dueDay: day,
        lateFeeCents: pesosToCents(lateFee) ?? 0,
        active: true,
        schedule: plan,
        monthlyDiscountCents: discountCents,
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
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(190px, 1fr))", gap: 12, marginBottom: 12 }}>
        <TextField label="Inscripción" value={enrollment} onChange={setEnrollment} placeholder="5000" />
        <TextField label="Colegiatura mensual" value={monthly} onChange={setMonthly} placeholder="3200" />
        <TextField label="Mensualidades" value={months} onChange={setMonths} placeholder="10" />
        <TextField label="Mes de inicio" value={firstMonth} onChange={setFirstMonth} placeholder="9" />
        <TextField label="Día límite" value={dueDay} onChange={setDueDay} placeholder="10" />
        <TextField label="Recargo por mes vencido" value={lateFee} onChange={setLateFee} placeholder="150" />
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
          <TextField label="Recargo por día" value={dailyFee} onChange={setDailyFee} placeholder="10" />
          <TextField label="Pronto pago (monto)" value={earlyAmount} onChange={setEarlyAmount} placeholder="100" />
          <TextField label="Pronto pago (días hábiles)" value={earlyDays} onChange={setEarlyDays} placeholder="5" />
          <TextField label="Descuento del nivel (por mes)" value={levelDiscount} onChange={setLevelDiscount} placeholder="500" />
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
      <div style={{ borderTop: "1px solid var(--border)", paddingTop: 14, marginBottom: 12 }}>
        <p style={{ fontSize: 13.5, fontWeight: 700, color: "var(--text-muted)", marginBottom: 4 }}>
          Cuotas por período (opcional)
        </p>
        <p style={{ fontSize: 12.5, color: "var(--text-faint)", marginBottom: 10 }}>
          Déjalo vacío para cobrar la mensualidad igual todos los meses. Para partirla,
          escribe mes:monto — por ejemplo <b>9:3200, 12:1600, 3:4800</b> cobra la mitad de
          diciembre en marzo.
        </p>
        <TextField label="Cuotas" value={schedule} onChange={setSchedule} placeholder="9:3200, 10:3200, 12:1600, 3:4800" />
      </div>

    </SlideOver>
  );
}

const cell = { padding: "10px 8px" } as const;
