"use client";

import { useMemo, useRef, useState, type ReactElement, type ReactNode } from "react";

import { Btn } from "../../_design/Controls";
import { CreatePage } from "../../_design/CreatePage";
import { DateField, MoneyField, NumberField } from "../../_design/form";
import { Icon } from "../../_design/Icon";
import type { Cycle, Level } from "../../config/api";
import { upsertTuitionPlan, type Fee, type EnrollmentConcept, type TuitionPlan } from "../../finance/api";
import { formatCents } from "../../_lib/money";
import { API_URL } from "../../api-url";
import { ErrorAlert } from "./parts";
import {
  MAX_DUE_DAY,
  billedMonths,
  centsToPesos,
  inscriptionTotals,
  ladderProblem,
  monthlyTotals,
  pesosToCents,
  seedConcepts,
  wholeAtLeastZero,
  wholeInRange,
} from "./tuition-math";
import { copyPlanToCycle } from "./tuition-copy";
import { FeesSection, InscriptionSection, type DraftCheck } from "./tuition-sections";

/**
 * El tabulador de un nivel: lo que cuesta el año.
 *
 * Es una PÁGINA, y eso es el arreglo, no un detalle de estilo. Catorce campos en
 * cinco secciones se capturaban por un cajón que —para que cupieran— fluía en
 * dos columnas tipo periódico: el corte caía donde alcanzara la altura, así que
 * agregar un concepto de inscripción hacía brincar de columna a "Cobranza"
 * entera mientras alguien escribía, y el renglón de error acababa al pie de la
 * segunda columna hablando de una sección que estaba arriba en la primera.
 *
 * Un solo hilo, sin pasos. Los pasos cobran navegación justo en el caso normal
 * del colegio —copiar el del año pasado y subir el monto— y, peor, separan el
 * total del año de los números que lo producen: quien captura acabaría yendo y
 * viniendo para entender de dónde salió el precio que va a cobrar.
 */
export interface TuitionPageProps {
  cycle: Cycle;
  level: Level;
  plan: TuitionPlan | undefined;
  /**
   * El tabulador de ESTE nivel en el ciclo anterior, si lo hay. Es lo que hace
   * que el caso normal sean tres clics en vez de catorce campos en blanco.
   */
  previousPlan?: TuitionPlan | undefined;
  previousCycleName?: string | undefined;
  onClose: () => void;
  onSaved: () => void;
}

export function TuitionPage({
  cycle,
  level,
  plan,
  previousPlan,
  previousCycleName,
  onClose,
  onSaved,
}: TuitionPageProps): ReactElement {
  const [monthly, setMonthly] = useState(plan === undefined ? "" : centsToPesos(plan.monthlyCents));
  const [months, setMonths] = useState(String(plan?.months ?? 10));
  const [firstMonth, setFirstMonth] = useState(String(plan?.firstBilledMonth ?? 9));
  const [dueDay, setDueDay] = useState(String(plan?.dueDay ?? 10));
  // La regla de cobranza de M16: el modo diario sustituye el recargo fijo
  // mensual por un monto que se cobra por cada día de atraso.
  const [dailyMode, setDailyMode] = useState(plan?.policy.dailyMode ?? false);
  const [dailyFee, setDailyFee] = useState(
    plan === undefined || plan.policy.lateFeeDailyCents === 0 ? "" : centsToPesos(plan.policy.lateFeeDailyCents));
  const [allDays, setAllDays] = useState(plan?.policy.surchargeCountsAllDays ?? true);
  const [earlyAmount, setEarlyAmount] = useState(
    plan === undefined || plan.policy.earlyPaymentCents === 0 ? "" : centsToPesos(plan.policy.earlyPaymentCents));
  const [earlyDays, setEarlyDays] = useState(String(plan?.policy.earlyPaymentDays ?? 0));
  // El plazo propio de la inscripción. Es un cargo aparte y el colegio lo cobra
  // en su fecha — en junio, para un año que abre en agosto.
  const [enrollmentDue, setEnrollmentDue] = useState(plan?.enrollmentDueOn ?? "");
  const [fees, setFees] = useState<Fee[]>(plan?.fees ?? []);
  const [concepts, setConcepts] = useState<EnrollmentConcept[]>(() => seedConcepts(plan, cycle));
  const [error, setError] = useState<string | undefined>(undefined);
  const [pending, setPending] = useState(false);
  // Cobranza y Cuotas nacen plegadas; un error de guardado las abre.
  const [openCollection, setOpenCollection] = useState(false);
  const [openFees, setOpenFees] = useState(false);
  /**
   * Lo que cada sección tiene escrito y sin agregar.
   *
   * En refs y no en estado: la comprobación corre al guardar, no en cada tecla,
   * y ponerla en estado repintaría catorce campos por letra.
   */
  const inscriptionDraft = useRef<DraftCheck>(() => undefined);
  const feesDraft = useRef<DraftCheck>(() => undefined);

  /** Se queja Y abre la sección donde vive el campo, para no señalar al vacío. */
  const complain = (message: string, section?: "collection" | "fees"): void => {
    if (section === "collection") setOpenCollection(true);
    if (section === "fees") setOpenFees(true);
    setError(message);
  };

  const copyPrevious = (): void => {
    if (previousPlan === undefined) return;
    const copy = copyPlanToCycle(previousPlan, cycle);
    setMonthly(centsToPesos(copy.monthlyCents));
    setMonths(String(copy.months));
    setFirstMonth(String(copy.firstBilledMonth));
    setDueDay(String(copy.dueDay));
    setDailyMode(copy.policy.dailyMode);
    setDailyFee(copy.policy.lateFeeDailyCents === 0 ? "" : centsToPesos(copy.policy.lateFeeDailyCents));
    setAllDays(copy.policy.surchargeCountsAllDays);
    setEarlyAmount(copy.policy.earlyPaymentCents === 0 ? "" : centsToPesos(copy.policy.earlyPaymentCents));
    setEarlyDays(String(copy.policy.earlyPaymentDays));
    setEnrollmentDue(copy.enrollmentDueOn);
    setFees(copy.fees);
    setConcepts(copy.enrollmentConcepts);
    setError(undefined);
  };

  const submit = async (): Promise<void> => {
    const monthlyCents = pesosToCents(monthly);
    if (monthlyCents === null) {
      setError("Escribe la colegiatura mensual, en pesos.");
      return;
    }
    const day = wholeInRange(dueDay, 1, MAX_DUE_DAY);
    if (day === null) {
      // Revisado aquí además de en el servidor: el 31 no existe en febrero, y
      // nadie debería necesitar un viaje de ida y vuelta para que se lo digan.
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
    /*
     * Los montos escritos SE LEEN O SE RECLAMAN; nunca se vuelven cero solos.
     *
     * `pesosToCents` devuelve null para "100.oo", "$100" o "1,00". El `?? 0` que
     * vivía aquí guardaba cero sin una palabra y redibujaba el campo vacío: nadie
     * podía saber que alguna vez se tecleó, y las familias perdían el pronto pago
     * el ciclo completo.
     */
    const dailyCents = dailyFee.trim() === "" ? 0 : pesosToCents(dailyFee);
    if (dailyCents === null) {
      complain("El recargo por día no se entiende. Escríbelo en pesos, por ejemplo 10.00", "collection");
      return;
    }
    if (dailyMode && dailyCents <= 0) {
      // La misma regla que impone la base. Dicha aquí para que se lea qué falta
      // en vez de una violación de restricción.
      complain("El recargo diario necesita un monto, o no cobraría nada.", "collection");
      return;
    }
    const earlyCents = earlyAmount.trim() === "" ? 0 : pesosToCents(earlyAmount);
    if (earlyCents === null) {
      complain("El monto de pronto pago no se entiende. Escríbelo en pesos, por ejemplo 100.00", "collection");
      return;
    }
    // Sin el tope de MAX_DUE_DAY: eso es "un día que existe en febrero", y esto
    // es un conteo de días hábiles.
    const window = wholeAtLeastZero(earlyDays);
    if (window === null) {
      complain("Los días de pronto pago son un número entero de días, desde 0.", "collection");
      return;
    }
    if (concepts.length === 0) {
      setError("Captura la inscripción: al menos un concepto con su monto y vigencia.");
      return;
    }
    const ladder = ladderProblem(concepts);
    if (ladder !== "") {
      setError(ladder);
      return;
    }
    /*
     * Nada tecleado se tira por no haberlo "agregado": se dice y se detiene.
     *
     * Cada ref se consulta UNA vez. Consultarlas dos veces —una para el mensaje y
     * otra para decidir qué sección abrir— hacía que, con borrador pendiente en
     * las dos, el mensaje fuera el de Inscripción y la sección que se abría fuera
     * Cuotas: señalando un lugar del que el mensaje no habla.
     */
    const inscripcionPendiente = inscriptionDraft.current();
    const cuotaPendiente = feesDraft.current();
    if (inscripcionPendiente !== undefined) {
      complain(inscripcionPendiente);
      return;
    }
    if (cuotaPendiente !== undefined) {
      complain(cuotaPendiente, "fees");
      return;
    }

    setPending(true);
    setError(undefined);
    try {
      await upsertTuitionPlan(API_URL, {
        cycleId: cycle.id,
        levelId: level.id,
        // Siempre cero: la inscripción vive en los conceptos, y el servidor
        // rechaza un plan que lleve las dos formas a la vez (ADR 0011).
        enrollmentCents: 0,
        enrollmentConcepts: concepts,
        monthlyCents,
        months: count,
        firstBilledMonth: startMonth,
        dueDay: day,
        // El colegio no usa el recargo fijo mensual ni el descuento por nivel y
        // pidió quitar los dos. Cero es lo que los apaga río abajo.
        lateFeeCents: 0,
        monthlyDiscountCents: 0,
        schedule: [],
        active: true,
        fees,
        enrollmentDueOn: enrollmentDue.trim(),
        policy: {
          dailyMode,
          lateFeeDailyCents: dailyCents,
          surchargeCountsAllDays: allDays,
          earlyPaymentCents: earlyCents,
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

  const collectionSummary = dailyMode
    ? `recargo por día${dailyFee === "" ? "" : ` · $${dailyFee}`}`
    : "recargo fijo";
  const feesSummary = fees.length === 0 ? "ninguna" : `${fees.length}`;

  return (
    <CreatePage
      breadcrumb={["Configuración", "Colegiaturas"]}
      title={`Precios de ${level.name} · ${cycle.name}`}
      subtitle="Inscripción, colegiatura, cobranza y cuotas"
      aside={
        <YearSummary
          cycle={cycle}
          monthly={monthly}
          months={months}
          firstMonth={firstMonth}
          fees={fees}
          concepts={concepts}
          previousPlan={previousPlan}
          previousCycleName={previousCycleName}
        />
      }
      footer={
        <>
          <Btn kind="ghost" onClick={onClose}>Cancelar</Btn>
          <div style={{ flex: 1 }} />
          {/* Nunca se apaga por una forma incompleta: al enviar se explica qué
              falta. Un botón muerto que no dice nada se lee como una app rota. */}
          <Btn icon="check" onClick={() => void submit()} disabled={pending}>
            {pending ? "Guardando…" : "Guardar tabulador"}
          </Btn>
        </>
      }
    >
      {previousPlan !== undefined && (
        <CopyCard cycleName={previousCycleName ?? "el ciclo anterior"} onCopy={copyPrevious} />
      )}

      <FormSection n={1} title="Colegiatura" note="lo que se cobra cada mes">
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(180px, 1fr))", gap: 12 }}>
          <MoneyField label="Colegiatura mensual" value={monthly} onChange={setMonthly} placeholder="3200.00" />
          <NumberField label="Mensualidades" value={months} onChange={setMonths} placeholder="10" />
          <NumberField label="Mes de inicio" value={firstMonth} onChange={setFirstMonth} placeholder="9" />
          {/*
            Sin min/max nativos a propósito: esta pantalla ya revisa el rango y
            explica por qué ("el 31 no existe en febrero"). El navegador, con
            `max`, bloquearía el envío con un globo suyo y esa frase se perdería.
          */}
          <NumberField label="Día límite" value={dueDay} onChange={setDueDay} placeholder="10" />
        </div>
      </FormSection>

      <FormSection n={2} title="Inscripción" note={`${concepts.length} concepto${concepts.length === 1 ? "" : "s"}`}>
        <InscriptionSection
          concepts={concepts}
          onChange={setConcepts}
          onError={setError}
          draftRef={inscriptionDraft}
        />
        <DateField
          label="Fecha límite de inscripción"
          value={enrollmentDue}
          onChange={setEnrollmentDue}
        />
      </FormSection>

      <FormSection
        n={3}
        title="Cobranza"
        note={collectionSummary}
        collapsible
        open={openCollection}
        onOpenChange={setOpenCollection}
      >
        <label style={{ display: "flex", alignItems: "center", gap: 10, cursor: "pointer" }}>
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
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(180px, 1fr))", gap: 12 }}>
          <MoneyField label="Recargo por día" value={dailyFee} onChange={setDailyFee} placeholder="10.00" />
          <MoneyField label="Pronto pago (monto)" value={earlyAmount} onChange={setEarlyAmount} placeholder="100.00" />
          <NumberField label="Pronto pago (días hábiles)" value={earlyDays} onChange={setEarlyDays} placeholder="5" />
        </div>
        <label style={{ display: "flex", alignItems: "center", gap: 10, cursor: "pointer" }}>
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
      </FormSection>

      <FormSection
        n={4}
        title="Cuotas"
        note={feesSummary}
        collapsible
        open={openFees}
        onOpenChange={setOpenFees}
      >
        <FeesSection
          cycle={cycle}
          firstMonth={wholeInRange(firstMonth, 1, 12) ?? 9}
          months={wholeInRange(months, 1, 12) ?? 10}
          monthlyCents={pesosToCents(monthly) ?? 0}
          fees={fees}
          onChange={setFees}
          onError={setError}
          draftRef={feesDraft}
        />
      </FormSection>

      {error !== undefined && <ErrorAlert message={error} />}
    </CreatePage>
  );
}

/**
 * Una sección de la forma, numerada.
 *
 * `collapsible` es divulgación progresiva, no un adorno: Cobranza y Cuotas las
 * deja como vienen la mayoría de los niveles, así que empiezan cerradas con su
 * estado dicho en una línea. Lo que aparece de entrada le comunica a quien
 * captura qué importa.
 */
function FormSection({
  n,
  title,
  note,
  collapsible = false,
  open,
  onOpenChange,
  children,
}: {
  n: number;
  title: string;
  note: string;
  collapsible?: boolean;
  /**
   * Plegado CONTROLADO desde la página, no guardado aquí dentro.
   *
   * Hace falta porque al guardar puede fallar un campo que vive en una sección
   * plegada: el mensaje señalaba algo que ni siquiera estaba en el DOM, y quien
   * captura apretaba Guardar, no veía cambiar nada y concluía que el botón
   * estaba roto. La página abre la sección culpable antes de quejarse.
   */
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  children: ReactNode;
}): ReactElement {
  const isOpen = open ?? !collapsible;
  const setOpen = (next: boolean): void => onOpenChange?.(next);

  const badge = (
    <span
      style={{
        width: 22,
        height: 22,
        borderRadius: "50%",
        display: "grid",
        placeItems: "center",
        background: isOpen ? "var(--forest)" : "var(--surface-3)",
        color: isOpen ? "#fff" : "var(--text-muted)",
        fontSize: 12,
        fontWeight: 800,
        flex: "none",
      }}
    >
      {n}
    </span>
  );

  const heading = (
    <>
      {badge}
      <span style={{ fontSize: 14.5, fontWeight: 800, color: "var(--text)" }}>{title}</span>
      <span style={{ marginLeft: "auto", fontSize: 12.5, color: "var(--text-muted)" }}>{note}</span>
    </>
  );

  return (
    <section
      style={{
        background: "var(--surface)",
        border: "1px solid var(--border)",
        borderRadius: "var(--r-xl)",
        overflow: "hidden",
      }}
    >
      {collapsible ? (
        <button
          type="button"
          aria-expanded={isOpen}
          onClick={() => setOpen(!isOpen)}
          style={{
            display: "flex",
            alignItems: "center",
            gap: 10,
            width: "100%",
            padding: "13px 16px",
            background: "transparent",
            border: "none",
            cursor: "pointer",
            textAlign: "left",
          }}
        >
          {heading}
          {/* El sistema tiene UN chevron. Se gira en vez de inventar un segundo
              ícono, que es cómo nacen dos verdades sobre la misma flecha. */}
          <Icon
            name="chevron"
            size={17}
            style={{ transform: isOpen ? "rotate(180deg)" : "none", transition: "transform .18s var(--ease)" }}
          />
        </button>
      ) : (
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: 10,
            padding: "13px 16px",
            borderBottom: "1px solid var(--border)",
          }}
        >
          {heading}
        </div>
      )}

      {isOpen && (
        <div style={{ padding: "14px 16px", display: "flex", flexDirection: "column", gap: 14 }}>
          {children}
        </div>
      )}
    </section>
  );
}

/**
 * Copiar el tabulador del año pasado.
 *
 * Es el atajo que convierte el caso normal del colegio —"los precios del año
 * pasado, con el monto más alto"— en tres clics. Sólo aparece cuando hay de
 * dónde copiar; una tarjeta que no puede hacer nada es ruido.
 */
function CopyCard({ cycleName, onCopy }: { cycleName: string; onCopy: () => void }): ReactElement {
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: 14,
        padding: "13px 16px",
        borderRadius: "var(--r-xl)",
        border: "1.5px dashed var(--lime)",
        background: "color-mix(in oklab, var(--lime) 8%, var(--surface))",
      }}
    >
      <div style={{ flex: 1, minWidth: 0 }}>
        <p style={{ fontSize: 14.5, fontWeight: 800, color: "var(--forest)" }}>
          Copiar de {cycleName}
        </p>
        <p style={{ fontSize: 12.5, color: "var(--text-muted)", marginTop: 2 }}>
          Trae la estructura y los montos tal cual, con las fechas recorridas un año.
          Después sólo cambias lo que subió.
        </p>
      </div>
      <Btn kind="ghost" onClick={onCopy} style={{ height: 40, fontSize: 14 }}>
        Copiar
      </Btn>
    </div>
  );
}

/**
 * Lo que cuesta el año, mientras se captura.
 *
 * Es el dato que la directora quiere revisar ANTES de guardar, y es la razón de
 * que esta página no tenga pasos: separarlo de los números que lo producen
 * obliga a navegar de ida y vuelta para entender de dónde salió.
 */
function YearSummary({
  cycle,
  monthly,
  months,
  firstMonth,
  fees,
  concepts,
  previousPlan,
  previousCycleName,
}: {
  cycle: Cycle;
  monthly: string;
  months: string;
  firstMonth: string;
  fees: readonly Fee[];
  concepts: readonly EnrollmentConcept[];
  previousPlan: TuitionPlan | undefined;
  previousCycleName: string | undefined;
}): ReactElement {
  const total = useMemo(() => {
    const monthlyCents = pesosToCents(monthly) ?? 0;
    const count = wholeInRange(months, 1, 12) ?? 0;
    const start = wholeInRange(firstMonth, 1, 12) ?? 9;
    const tuition = monthlyTotals(billedMonths(cycle.startsOn, start, count), monthlyCents, fees)
      .reduce((sum, m) => sum + m.totalCents, 0);
    // La inscripción del año es la del PRIMER escalón: lo que paga quien se
    // inscribe en cuanto abre el periodo.
    const enrollment = inscriptionTotals(concepts)[0]?.totalCents ?? 0;
    return { tuition, enrollment, all: tuition + enrollment };
  }, [cycle.startsOn, monthly, months, firstMonth, fees, concepts]);

  const before = useMemo(() => {
    if (previousPlan === undefined) return undefined;
    const tuition = monthlyTotals(
      billedMonths(cycle.startsOn, previousPlan.firstBilledMonth, previousPlan.months),
      previousPlan.monthlyCents,
      previousPlan.fees,
    ).reduce((sum, m) => sum + m.totalCents, 0);
    const enrollment = inscriptionTotals(previousPlan.enrollmentConcepts)[0]?.totalCents ?? 0;
    return tuition + enrollment;
  }, [cycle.startsOn, previousPlan]);

  const delta = before === undefined ? undefined : total.all - before;

  return (
    <div
      style={{
        background: "var(--surface)",
        border: "1px solid var(--border)",
        borderRadius: "var(--r-xl)",
        padding: "14px 16px",
      }}
    >
      <p style={{ fontSize: 11, fontWeight: 800, letterSpacing: ".05em", textTransform: "uppercase", color: "var(--text-muted)" }}>
        Año completo
      </p>
      <p style={{ fontSize: 24, fontWeight: 800, color: "var(--forest)", lineHeight: 1.1, margin: "3px 0 11px" }}>
        {formatCents(total.all)}
      </p>

      <Line label="Inscripción" value={formatCents(total.enrollment)} />
      <Line label="Colegiatura" value={formatCents(total.tuition)} />

      {delta !== undefined && (
        <p
          style={{
            marginTop: 11,
            padding: "6px 8px",
            borderRadius: "var(--r-md)",
            background: "var(--surface-3)",
            fontSize: 12,
            fontWeight: 700,
            color: "var(--text-muted)",
            textAlign: "center",
          }}
        >
          {delta === 0
            ? `Igual que ${previousCycleName ?? "el año pasado"}`
            : `${delta > 0 ? "▲" : "▼"} ${formatCents(Math.abs(delta))} vs. ${previousCycleName ?? "el año pasado"}`}
        </p>
      )}
    </div>
  );
}

function Line({ label, value }: { label: string; value: string }): ReactElement {
  return (
    <div
      style={{
        display: "flex",
        justifyContent: "space-between",
        gap: 10,
        padding: "5px 0",
        borderTop: "1px solid var(--border)",
        fontSize: 12.5,
        color: "var(--text-muted)",
      }}
    >
      <span>{label}</span>
      <b style={{ color: "var(--text)" }}>{value}</b>
    </div>
  );
}
