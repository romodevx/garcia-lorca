"use client";

import { useEffect, useId, useState, type FormEvent, type ReactElement } from "react";

import { API_URL } from "../../api-url";
import { Btn } from "../../_design/Controls";
import { Icon } from "../../_design/Icon";
import { DateField, Field, inputStyle, MoneyField, SelectField, useRequired } from "../../_design/form";
import { createCharge, listConcepts, type Charge, type ChargeConcept } from "../../finance/api";
import { formatPesosInput, pesosToCents } from "../../_lib/money";
import { BAD_AMOUNT } from "./conceptCopy";

/** A pupil as the picker offers it: the label is unique on the list. */
export interface PupilOption {
  id: string;
  label: string;
}

export interface ManualChargePanelProps {
  cycleId: string;
  pupils: readonly PupilOption[];
  /** The school's day, aaaa-mm-dd: the default due date. */
  today: string;
  onCreated: (charge: Charge, message: string) => void;
  onClose: () => void;
}

const FIELDS = ["pupil", "conceptId", "amount", "dueDate"] as const;

/**
 * A loose charge to one pupil (CC-203): pupil, concept, amount, due date. Four
 * fields, so an inline panel above the list, never a drawer. The concept comes
 * from the catalog (CC-111) and brings its amount; the date starts on today.
 */
export function ManualChargePanel({ cycleId, pupils, today, onCreated, onClose }: ManualChargePanelProps): ReactElement {
  const listId = useId();
  const [concepts, setConcepts] = useState<ChargeConcept[] | "loading" | "failed">("loading");
  const [values, setValues] = useState({ pupil: "", conceptId: "", amount: "", dueDate: today });
  const [invalid, setInvalid] = useState<{ pupil?: string; amount?: string }>({});
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const [saving, setSaving] = useState(false);
  const required = useRequired(FIELDS);

  useEffect(() => {
    listConcepts(API_URL).then(setConcepts).catch(() => setConcepts("failed"));
  }, []);

  const set = (key: keyof typeof values, value: string): void => setValues((prev) => ({ ...prev, [key]: value }));
  const pickConcept = (id: string): void => {
    const concept = Array.isArray(concepts) ? concepts.find((c) => c.id === id) : undefined;
    setValues((prev) => ({ ...prev, conceptId: id, amount: concept === undefined ? prev.amount : formatPesosInput(concept.defaultAmountCents) }));
    required.clear("amount");
  };

  const formatProblems = (): { pupil?: string; amount?: string } => {
    const cents = pesosToCents(values.amount);
    return {
      pupil: pupils.some((p) => p.label === values.pupil) ? undefined : "Elige un alumno de la lista.",
      amount: Number.isNaN(cents) ? BAD_AMOUNT : cents <= 0 ? "El monto debe ser mayor que cero." : undefined,
    };
  };

  const submit = (e: FormEvent): void => {
    e.preventDefault();
    if (saving || !required.check(values)) return;
    const found = formatProblems();
    setInvalid(found);
    if (found.pupil !== undefined || found.amount !== undefined) return;
    const pupil = pupils.find((p) => p.label === values.pupil);
    const concept = Array.isArray(concepts) ? concepts.find((c) => c.id === values.conceptId) : undefined;
    if (pupil === undefined || concept === undefined) return;
    setSaving(true);
    setProblem(undefined);
    createCharge(API_URL, { studentId: pupil.id, cycleId, conceptId: concept.id, amountCentsOverride: pesosToCents(values.amount), descriptionOverride: "", dueDate: values.dueDate })
      .then((charge) => onCreated(charge, `Cargo de ${concept.name} creado para ${pupil.label}.`))
      .catch(() => setProblem("No se pudo crear el cargo. Revisa los datos e intenta de nuevo."))
      .finally(() => setSaving(false));
  };

  const options = Array.isArray(concepts) ? concepts.map((c) => ({ value: c.id, label: c.name })) : [];

  return (
    <form onSubmit={submit} aria-label="Nuevo cargo" style={{ marginBottom: 18, padding: "18px 20px", display: "grid", gap: 14, borderRadius: "var(--r-lg)", background: "color-mix(in oklab, var(--lime) 8%, var(--surface))", border: "1px solid var(--border)" }}>
      <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
        <h3 style={{ flex: 1, fontSize: 17, fontWeight: 700 }}>Nuevo cargo</h3>
        <button type="button" aria-label="Cerrar nuevo cargo" onClick={onClose} style={{ minWidth: 44, minHeight: 44, display: "grid", placeItems: "center", border: "none", background: "none", color: "var(--text-muted)", cursor: "pointer" }}><Icon name="close" size={18} /></button>
      </div>
      <Field label="Alumno" required error={required.errors.pupil ?? invalid.pupil} help="Escribe parte del nombre y elígelo de la lista.">
        {(id, describedBy) => (
          <input id={id} list={listId} autoComplete="off" value={values.pupil} aria-describedby={describedBy} aria-required aria-invalid={required.errors.pupil !== undefined || invalid.pupil !== undefined ? true : undefined}
            onChange={(e) => { set("pupil", e.target.value); required.clear("pupil"); setInvalid((p) => ({ ...p, pupil: undefined })); }} style={inputStyle(required.errors.pupil !== undefined || invalid.pupil !== undefined)} />
        )}
      </Field>
      <datalist id={listId}>{pupils.map((p) => <option key={p.id} value={p.label} />)}</datalist>
      {concepts === "failed" && <p role="alert" style={{ margin: 0, color: "var(--bad-fg)" }}>No se pudieron leer los conceptos de cobro.</p>}
      {Array.isArray(concepts) && concepts.length === 0 && <p style={{ margin: 0, color: "var(--text-muted)" }}>Primero da de alta un concepto en Conceptos de cobro, al pie de esta pantalla.</p>}
      <SelectField label="Concepto" required value={values.conceptId} options={options} onChange={pickConcept} onClearError={() => required.clear("conceptId")} error={required.errors.conceptId} help="Sale del catálogo de conceptos de cobro." />
      <MoneyField label="Monto" required value={values.amount} onChange={(v) => { set("amount", v); setInvalid((p) => ({ ...p, amount: undefined })); }} onClearError={() => required.clear("amount")} error={required.errors.amount ?? invalid.amount} help="Propuesto por el concepto; se puede cambiar para este cargo." />
      <DateField label="Vence" required value={values.dueDate} onChange={(v) => set("dueDate", v)} onClearError={() => required.clear("dueDate")} error={required.errors.dueDate} />
      {problem !== undefined && <p role="alert" style={{ margin: 0, color: "var(--bad-fg)", fontWeight: 600 }}>{problem}</p>}
      <div style={{ display: "flex", justifyContent: "flex-end", gap: 12 }}>
        <Btn kind="ghost" onClick={onClose}>Cancelar</Btn>
        <Btn type="submit" icon="plus" busy={saving}>Crear cargo</Btn>
      </div>
    </form>
  );
}
