"use client";

import { useEffect, useRef, useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { DateField, MoneyField, TextField } from "../../_design/form";
import type { Cycle } from "../../config/api";
import type { EnrollmentConcept, Fee, PriceTier } from "../../finance/api";
import { formatCents } from "../../_lib/money";
import {
  MONTH_NAMES,
  billedMonths,
  feeShares,
  inscriptionTotals,
  ladderProblem,
  monthlyTotals,
  pesosToCents,
  sameLineName,
} from "./tuition-math";

/**
 * Las dos secciones grandes de la forma del tabulador: la inscripción con sus
 * conceptos y tramos de precio, y las cuotas.
 *
 * Salieron de `TuitionPanel.tsx` cuando el tabulador dejó de ser un cajón: ese
 * archivo hacía tres trabajos a la vez —calcular dinero, dibujar la forma y
 * dibujar la tabla del panel— en mil líneas.
 */

/**
 * Lo que una sección tiene escrito y todavía no agregado.
 *
 * Existe porque el monto y la fecha de un tramo viven en el estado local de su
 * tarjeta hasta que alguien aprieta "Agregar". Con el botón de guardar siempre a
 * la vista, escribir el precio y guardar directo es el gesto natural, y ese
 * precio se iba a la basura sin una palabra: el tabulador quedaba con un escalón
 * de menos y cobraba mal todo el año.
 *
 * Devuelve el reclamo, o `undefined` si no hay nada pendiente. Se registra en un
 * ref y no en estado para no repintar la forma en cada tecla.
 */
export type DraftCheck = () => string | undefined;

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
export function InscriptionSection({ concepts, onChange, onError, draftRef }: InscriptionSectionProps): ReactElement {
  const [name, setName] = useState("");
  const cardDrafts = useRef(new Map<string, DraftCheck>());

  /*
   * Al desmontarse deja de reclamar.
   *
   * `FormSection` sólo dibuja sus hijos mientras está abierta, así que plegar
   * "Cuotas" desmonta esta sección y dejaba en el ref un cierre viejo sobre lo
   * que se había tecleado. El siguiente "Guardar tabulador" se quejaba de una
   * cuota que ya no existía —y al abrir la sección los campos estaban vacíos,
   * porque el estado se había reiniciado—; el segundo clic sí guardaba. Se leía
   * como un botón que a veces no hace nada.
   */
  useEffect(() => {
    const ref = draftRef;
    return () => {
      if (ref !== undefined) ref.current = () => undefined;
    };
  }, [draftRef]);

  const registerDraft = (concept: string, check: DraftCheck | undefined): void => {
    if (check === undefined) cardDrafts.current.delete(concept);
    else cardDrafts.current.set(concept, check);
  };

  // El nombre de un concepto escrito y sin agregar cuenta igual: es un renglón
  // de la lista de precios que nadie va a cobrar.
  if (draftRef !== undefined) {
    draftRef.current = (): string | undefined => {
      if (name.trim() !== "") {
        return `Tienes el concepto "${name.trim()}" escrito sin agregar. Aprieta "Agregar concepto" o borra lo escrito.`;
      }
      for (const check of cardDrafts.current.values()) {
        const problem = check();
        if (problem !== undefined) return problem;
      }
      return undefined;
    };
  }

  const addConcept = (): void => {
    if (name.trim() === "") {
      onError("Escribe el nombre del concepto.");
      return;
    }
    if (concepts.some((c) => sameLineName(c.name, name))) {
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
        /*
          La llave va por NOMBRE, no por índice. Con el índice, quitar el
          concepto #1 remontaba todos los de abajo y tiraba el monto que
          tuvieran tecleado sin agregar — justo lo que `DraftCheck` existe para
          proteger. El nombre es único: lo garantiza el guard de slug al agregar.
        */
        <ConceptCard
          key={concept.name}
          concept={concept}
          onChangeTiers={(tiers) => patch(index, tiers)}
          onRemove={() => onChange(concepts.filter((_, i) => i !== index))}
          onError={onError}
          registerDraft={registerDraft}
        />
      ))}

      <div style={{ display: "flex", gap: 12, alignItems: "flex-end", flexWrap: "wrap", marginTop: 4 }}>
        <div style={{ flex: "1 1 240px" }}>
          <TextField label="Nuevo concepto" value={name} onChange={setName} uppercase help="Cómo se llamará este cobro en el estado de cuenta de la familia." />
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

/**
 * One concept and the amounts it has had.
 *
 * The name is typed once and never again: what changes over the year is the
 * MONEY, not what the line is called. Asking for the name with every amount —
 * which is what the first version did — invited two rows for one concept and a
 * validation error instead of the merge the director meant.
 */
function ConceptCard({ concept, onChangeTiers, onRemove, onError, registerDraft }: ConceptCardProps): ReactElement {
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [amount, setAmount] = useState("");

  // Se registra en cada render: la comprobación tiene que leer los valores de
  // AHORA, y un registro que sólo corriera al montar leería los del principio.
  useEffect(() => {
    registerDraft(concept.name, () =>
      amount.trim() !== "" || from.trim() !== "" || to.trim() !== ""
        ? `Tienes un monto escrito en "${concept.name}" sin agregar. Aprieta "Agregar monto con su vigencia" o borra lo escrito.`
        : undefined,
    );
    return () => registerDraft(concept.name, undefined);
  });

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
        <MoneyField label="Monto" value={amount} onChange={setAmount} help="Lo que se cobra por este concepto, por alumno." />
        <DateField label="Vigente desde" value={from} onChange={setFrom} />
        <DateField label="Vigente hasta (opcional)" value={to} onChange={setTo} />
      </div>
      <Btn kind="ghost" icon="plus" onClick={addAmount}>Agregar monto con su vigencia</Btn>
    </div>
  );
}

interface ConceptCardProps {
  /** Cómo avisar qué tiene escrito y sin agregar; `undefined` la da de baja. */
  registerDraft: (name: string, check: DraftCheck | undefined) => void;
  concept: EnrollmentConcept;
  onChangeTiers: (tiers: PriceTier[]) => void;
  onRemove: () => void;
  onError: (message: string | undefined) => void;
}

export interface InscriptionSectionProps {
  /** Dónde dejar la comprobación de lo escrito y sin agregar. */
  draftRef?: { current: DraftCheck } | undefined;
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
export function FeesSection({
  cycle,
  firstMonth,
  months,
  monthlyCents,
  fees,
  onChange,
  onError,
  draftRef,
}: {
  cycle: Cycle;
  firstMonth: number;
  months: number;
  monthlyCents: number;
  fees: Fee[];
  onChange: (fees: Fee[]) => void;
  onError: (message: string) => void;
  /** Dónde dejar la comprobación de lo escrito y sin agregar. */
  draftRef?: { current: DraftCheck } | undefined;
}): ReactElement {
  const [name, setName] = useState("");
  const [amount, setAmount] = useState("");
  const [picked, setPicked] = useState<number[]>([]);

  // Ver InscriptionSection: plegar la sección la desmonta, y un cierre viejo en
  // el ref bloqueaba el siguiente guardado con una queja sobre algo inexistente.
  useEffect(() => {
    const ref = draftRef;
    return () => {
      if (ref !== undefined) ref.current = () => undefined;
    };
  }, [draftRef]);

  if (draftRef !== undefined) {
    draftRef.current = (): string | undefined =>
      name.trim() !== "" || amount.trim() !== "" || picked.length > 0
        ? `Tienes la cuota "${name.trim() === "" ? "sin nombre" : name.trim()}" escrita sin agregar. Aprieta "Agregar cuota" o borra lo escrito.`
        : undefined;
  }

  const calendar = billedMonths(cycle.startsOn, firstMonth, months);
  const totals = monthlyTotals(calendar, monthlyCents, fees);

  const add = (): void => {
    const cents = pesosToCents(amount);
    if (name.trim() === "" || cents === null || cents <= 0) {
      onError("Una cuota necesita nombre y monto.");
      return;
    }
    if (fees.some((f) => sameLineName(f.name, name) && f.months.some((m) => picked.includes(m)))) {
      // In a shared month they would bill as ONE fee and the second would never
      // reach the statement (the month is part of a fee's identity server-side,
      // so the same name in other months is fine). See sameLineName.
      onError(`Ya existe una cuota que se llama casi igual que "${name.trim()}".`);
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
        <TextField label="Nombre de la cuota" value={name} onChange={setName} uppercase help="Cómo se llamará esta cuota en el estado de cuenta de la familia." />
        <MoneyField label="Monto total" value={amount} onChange={setAmount} help="El total de la cuota. Abajo eliges en cuántas partes se parte." />
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

