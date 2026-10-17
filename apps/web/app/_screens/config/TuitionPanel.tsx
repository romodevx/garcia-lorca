"use client";

import { useCallback, useEffect, useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import type { Cycle, Level } from "../../config/api";
import { listTuitionPlans, runBilling, type BillingRun, type TuitionPlan } from "../../finance/api";
import { formatCents } from "../../_lib/money";
import { EmptyHint, ErrorAlert, Section } from "./parts";
import { MONTH_NAMES, inscriptionTotalOn } from "./tuition-math";
import { schoolISODate } from "../../_lib/schoolDay";
import { TuitionPage } from "./TuitionPage";
import { useSecondaryInSetup, useSetupPresentation } from "./SetupPresentation";
import { useLatest } from "./useLatest";

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
  /**
   * Notify-only: the cycle's plans after each reload (entry, "Generar cargos",
   * a saved tabulador), or "failed". The setup view marks its step with it
   * instead of reading the plans twice.
   */
  onPlansLoaded?: (cycleId: string, plans: TuitionPlan[] | "failed") => void;
  /** The school's today (aaaa-mm-dd): the inscription is shown at the price in force; defaults to Morelia's now. */
  today?: string;
}

const cell = { padding: "10px 8px" } as const;

/**
 * Lo que cuesta inscribirse HOY, venga de los conceptos o del monto único viejo.
 * El escalón vigente, no el primero: ése pudo haber vencido (CC-108).
 */
function inscriptionShown(plan: TuitionPlan, today: string): number {
  const porConceptos = inscriptionTotalOn(plan.enrollmentConcepts, today);
  return porConceptos !== undefined && porConceptos > 0 ? porConceptos : plan.enrollmentCents;
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
export function TuitionPanel({
  cycle,
  levels,
  previousCycle,
  pricingLevelId,
  onPrice,
  onPlansLoaded,
  today,
}: TuitionPanelProps): ReactElement {
  const day = today ?? schoolISODate();
  const [plans, setPlans] = useState<TuitionPlan[]>([]);
  // Los del ciclo anterior existen sólo para ofrecer "copiar de". Se piden una
  // vez y sin bloquear: si fallan, el atajo no aparece y todo lo demás sigue.
  const [previousPlans, setPreviousPlans] = useState<TuitionPlan[]>([]);
  /**
   * Tres estados, no dos. "Todavía no sé" y "no se cobra" se ven igual en
   * pantalla y significan lo contrario: la tabla se dibujaba de inmediato con la
   * lista vacía, así que durante la carga los cuatro niveles anunciaban "este
   * nivel no se cobra" — una afirmación sobre dinero, dicha antes de saberla.
   */
  const [loaded, setLoaded] = useState(false);
  const [run, setRun] = useState<BillingRun | null>(null);
  const [error, setError] = useState<string | undefined>(undefined);
  const [running, setRunning] = useState(false);
  const generateKind = useSecondaryInSetup();
  // Inside a setup step a narrow screen stacks each level's row (globals.css .tuition-stack),
  // every cell carrying its column's name, so "Definir" never runs off the side (#23).
  const { inSetup } = useSetupPresentation();
  const label = (column: string): string | undefined => (inSetup ? column : undefined);

  const notify = useLatest(onPlansLoaded);
  /*
   * The panel outlives a cycle switch, so an answer can arrive for a cycle that
   * is no longer on screen. Only the current cycle's answer is kept: switching
   * years with a read in flight used to paint the old year's prices last.
   */
  const activeCycleId = useLatest(cycle.id);

  const reload = useCallback(() => {
    const cycleId = cycle.id;
    const active = (): boolean => activeCycleId.current === cycleId;
    listTuitionPlans(API_URL, cycleId)
      .then((rows) => {
        if (!active()) return;
        setPlans(rows);
        setLoaded(true);
        notify.current?.(cycleId, rows);
      })
      .catch(() => {
        if (!active()) return;
        setError("No pudimos cargar los tabuladores.");
        notify.current?.(cycleId, "failed");
      });
  }, [cycle.id, activeCycleId, notify]);

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
        today={day}
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
        <div style={{ overflowX: "auto", contain: "inline-size", marginBottom: 16 }}>
          <table className={inSetup ? "tuition-stack" : undefined} style={{ width: "100%", borderCollapse: "collapse" }}>
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
                    <td data-label={label("Nivel")} style={{ ...cell, fontWeight: 600 }}>{level.name}</td>
                    {plan === undefined ? (
                      <td style={{ ...cell, color: "var(--text-faint)" }} colSpan={5}>
                        {loaded ? "Sin tabulador · este nivel no se cobra" : "Cargando…"}
                      </td>
                    ) : (
                      <>
                        {/*
                          Los conceptos SI los hay; si no, el monto único.
                          
                          Las dos formas existen y ninguna se puede ignorar. Un plan
                          nuevo (ADR 0011) guarda `enrollmentCents` en cero y el
                          dinero en los tramos de cada concepto: leer el campo
                          anunciaba como gratis una inscripción de miles. Pero un
                          plan escrito ANTES guarda lo contrario —el monto único y
                          ninguna lista— y leer sólo los conceptos anuncia gratis
                          exactamente los mismos miles. Los cuatro tabuladores del
                          colegio sembrado son de ese segundo tipo.
                          
                          De los conceptos se toma el PRIMER escalón: lo que paga
                          quien se inscribe en cuanto abre el periodo, que es el
                          número que el colegio publica.
                        */}
                        <td data-label={label("Inscripción")} style={cell}>{formatCents(inscriptionShown(plan, day))}</td>
                        <td data-label={label("Mensualidad")} style={cell}>{formatCents(plan.monthlyCents)}</td>
                        <td data-label={label("Mensualidades")} style={cell}>
                          {plan.months} desde {MONTH_NAMES[plan.firstBilledMonth - 1]}
                        </td>
                        <td data-label={label("Día límite")} style={cell}>día {plan.dueDay}</td>
                        <td data-label={label("Recargo")} style={cell}>
                          {plan.policy.dailyMode
                            ? `${formatCents(plan.policy.lateFeeDailyCents)} / día`
                            : plan.lateFeeCents === 0 ? "—" : formatCents(plan.lateFeeCents)}
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
        </div>
      )}

      <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
        <Btn icon="check" kind={generateKind} onClick={() => void generate()} disabled={running}>
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
        {run.lateFeesCreated} recargos.
        {/* CC-213: a first run can skip inscriptions or activities issued
            earlier; a bare "6 ya existían" read as tuitions charged twice. */}
        {run.skipped > 0 && (
          <span>
            {" "}{run.skipped} {run.skipped === 1 ? "cargo ya existía" : "cargos ya existían"} (colegiaturas,
            inscripciones, cuotas o actividades emitidos antes) y no se volvieron a cobrar.
          </span>
        )}
        {/* T14-022: in the same sentence, not as a footnote under it. A moved
            deadline changes what a withdrawal later treats as not yet due. */}
        {run.enrollmentMoved > 0 && (
          <>
            {" "}Se movió la fecha límite de <strong>{run.enrollmentMoved} inscripciones sin pagar</strong> para
            que coincida con el tabulador.
          </>
        )}
        {(run.enrollmentRepriced ?? 0) > 0 && (
          <>
            {" "}Se actualizó el precio de <strong>{run.enrollmentRepriced} inscripciones sin pagar</strong> al
            tramo vigente.
          </>
        )}
      </p>
      {(run.activitiesCreated ?? 0) > 0 && (
        <p style={{ marginTop: 8 }}>
          Además, <strong>{run.activitiesCreated} cargos de actividades</strong>.
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
