"use client";

import { useCallback, useEffect, useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import type { Cycle, Level } from "../../config/api";
import { listTuitionPlans, runBilling, type BillingRun, type TuitionPlan } from "../../finance/api";
import { EmptyHint, ErrorAlert, Section } from "./parts";
import { MONTH_NAMES, centsToPesos, inscriptionTotals } from "./tuition-math";
import { TuitionPage } from "./TuitionPage";

import { API_URL } from "../../api-url";

/**
 * Este archivo llegó a tener MIL líneas haciendo tres trabajos: la aritmética
 * del dinero, la forma de catorce campos y la tabla del panel. Se partió cuando
 * el tabulador dejó de ser un cajón:
 *
 *   - `tuition-math.ts`     lo que decide dinero, sin pantalla
 *   - `tuition-sections.tsx` la inscripción y las cuotas
 *   - `TuitionPage.tsx`     la forma, que ahora es una página
 *
 * Aquí queda lo que siempre fue este panel: la tabla de qué cuesta cada nivel y
 * el botón que factura el mes.
 */

export interface TuitionPanelProps {
  cycle: Cycle;
  levels: readonly Level[];
  /**
   * El ciclo anterior, para poder ofrecer "copiar de". Sin él la página sigue
   * funcionando: simplemente no aparece el atajo.
   */
  previousCycle?: Cycle | undefined;
  /**
   * El nivel cuyo tabulador se está capturando, y cómo avisar que cambió.
   *
   * Vive ARRIBA, en `Catalog`, y no aquí: mientras se capturan precios la página
   * tiene que ocupar la pantalla. Cuando este estado era interno, la "página" se
   * dibujaba entre la barra de ciclos y el calendario, con todo el catálogo
   * alrededor — y con los chips de ciclo clicables, así que cambiar de año a
   * media captura re-apuntaba el guardado sin limpiar lo escrito y los precios
   * del año viejo se escribían encima del nuevo.
   */
  pricingLevelId?: string | undefined;
  onPrice?: ((levelId: string | undefined) => void) | undefined;
}

const cell = { padding: "10px 8px" } as const;

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
export function TuitionPanel({
  cycle,
  levels,
  previousCycle,
  pricingLevelId,
  onPrice,
}: TuitionPanelProps): ReactElement {
  const [plans, setPlans] = useState<TuitionPlan[]>([]);
  // Los del ciclo anterior existen sólo para ofrecer "copiar de". Se piden una
  // vez y sin bloquear: si fallan, el atajo no aparece y todo lo demás sigue.
  const [previousPlans, setPreviousPlans] = useState<TuitionPlan[]>([]);
  const [run, setRun] = useState<BillingRun | null>(null);
  const [error, setError] = useState<string | undefined>(undefined);
  const [running, setRunning] = useState(false);

  const reload = useCallback(() => {
    listTuitionPlans(API_URL, cycle.id)
      .then(setPlans)
      .catch(() => setError("No pudimos cargar los tabuladores."));
  }, [cycle.id]);

  useEffect(reload, [reload]);

  useEffect(() => {
    if (previousCycle === undefined) {
      setPreviousPlans([]);
      return;
    }
    listTuitionPlans(API_URL, previousCycle.id)
      .then(setPreviousPlans)
      .catch(() => setPreviousPlans([]));
  }, [previousCycle]);

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

  const pricing = levels.find((l) => l.id === pricingLevelId);
  if (pricing !== undefined) {
    return (
      <TuitionPage
        cycle={cycle}
        level={pricing}
        plan={planFor(pricing.id)}
        previousPlan={previousPlans.find((p) => p.levelId === pricing.id)}
        previousCycleName={previousCycle?.name}
        onClose={() => onPrice?.(undefined)}
        onSaved={() => {
          onPrice?.(undefined);
          reload();
        }}
      />
    );
  }

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
                      {/*
                        Se SUMAN los conceptos; `enrollmentCents` no se lee.
                        Desde ADR 0011 ese campo viaja siempre en cero —el monto
                        vive en los tramos de cada concepto— así que leerlo
                        anunciaba como gratis una inscripción de miles de pesos.
                        Es el primer escalón: lo que paga quien se inscribe en
                        cuanto abre el periodo, que es el número que el colegio
                        publica.
                      */}
                      <td style={cell}>
                        ${centsToPesos(inscriptionTotals(plan.enrollmentConcepts)[0]?.totalCents ?? 0)}
                      </td>
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
                      onClick={() => onPrice?.(level.id)}
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
