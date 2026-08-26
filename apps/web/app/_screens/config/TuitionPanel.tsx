"use client";

import { useCallback, useEffect, useState, type FormEvent, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import type { Cycle, Level } from "../../config/api";
import {
  listTuitionPlans,
  runBilling,
  upsertTuitionPlan,
  type BillingRun,
  type TuitionPlan,
} from "../../finance/api";
import { EmptyHint, ErrorAlert, Section, TextField } from "./parts";

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

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
                        {plan.lateFeeCents === 0 ? "—" : `$${centsToPesos(plan.lateFeeCents)}`}
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
  const [error, setError] = useState<string | undefined>(undefined);
  const [pending, setPending] = useState(false);

  const submit = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
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
      });
      onSaved();
    } catch {
      setError("No se pudo guardar el tabulador. Revisa los datos.");
    } finally {
      setPending(false);
    }
  };

  return (
    <form
      aria-label="Tabulador del nivel"
      onSubmit={(e) => void submit(e)}
      style={{ padding: 16, borderRadius: "var(--r-md)", background: "var(--surface-2)", marginBottom: 16 }}
    >
      <p style={{ fontWeight: 700, marginBottom: 12 }}>{level.name}</p>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(190px, 1fr))", gap: 12, marginBottom: 12 }}>
        <TextField label="Inscripción" value={enrollment} onChange={setEnrollment} placeholder="5000" />
        <TextField label="Colegiatura mensual" value={monthly} onChange={setMonthly} placeholder="3200" />
        <TextField label="Mensualidades" value={months} onChange={setMonths} placeholder="10" />
        <TextField label="Mes de inicio" value={firstMonth} onChange={setFirstMonth} placeholder="9" />
        <TextField label="Día límite" value={dueDay} onChange={setDueDay} placeholder="10" />
        <TextField label="Recargo por mes vencido" value={lateFee} onChange={setLateFee} placeholder="150" />
      </div>
      <div style={{ display: "flex", gap: 10 }}>
        <Btn type="submit" icon="check" disabled={pending}>
          Guardar tabulador
        </Btn>
        <Btn kind="ghost" onClick={onClose}>
          Cancelar
        </Btn>
      </div>
      {error !== undefined && <ErrorAlert message={error} />}
    </form>
  );
}

const cell = { padding: "10px 8px" } as const;
