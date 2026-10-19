"use client";

import { useState, type CSSProperties, type ReactElement } from "react";

import type { ChargeConcept, UpdateConceptInput } from "../../finance/api";
import { formatCents, formatPesosInput, pesosToCents } from "../../_lib/money";
import { amountProblem, RECURRENCE_LABEL, RECURRENCE_OPTIONS, toRecurrence } from "./conceptCopy";

export interface ConceptRowProps {
  concept: ChargeConcept;
  canWrite: boolean;
  /** Rejects when the server refused; the row then stays in edit mode. */
  onSave: (input: UpdateConceptInput) => Promise<void>;
}

const td: CSSProperties = { padding: "10px 14px", borderBottom: "1px solid var(--border)", verticalAlign: "middle" };
const cell: CSSProperties = { height: 44, width: "100%", minWidth: 0, padding: "0 12px", borderRadius: "var(--r-md)", border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)", fontSize: 16 };
const link: CSSProperties = { minHeight: 44, padding: "0 10px", border: "none", background: "none", color: "var(--forest)", fontWeight: 600, fontSize: 14, cursor: "pointer" };

/** One catalog concept: read, or edited in place (three fields, no drawer). */
export function ConceptRow({ concept, canWrite, onSave }: ConceptRowProps): ReactElement {
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(concept.name);
  const [amount, setAmount] = useState(formatPesosInput(concept.defaultAmountCents));
  const [recurrence, setRecurrence] = useState<string>(concept.recurrence);
  const [error, setError] = useState<string | undefined>(undefined);
  const [saving, setSaving] = useState(false);
  const system = concept.kind !== "custom";

  const save = async (input: UpdateConceptInput): Promise<void> => {
    if (saving) return;
    setSaving(true);
    try {
      await onSave(input);
      setEditing(false);
    } catch {
      // The panel says what failed; what was typed stays.
    } finally {
      setSaving(false);
    }
  };

  const submitEdit = (): void => {
    const problem = name.trim() === "" ? "Falta el nombre." : amount.trim() === "" ? "Falta el monto." : amountProblem(amount);
    setError(problem);
    if (problem !== undefined) return;
    void save({ name: name.trim(), defaultAmountCents: pesosToCents(amount), recurrence: toRecurrence(recurrence), active: concept.active });
  };

  const toggle = (): void => {
    void save({ name: concept.name, defaultAmountCents: concept.defaultAmountCents, recurrence: concept.recurrence, active: !concept.active });
  };

  if (editing) {
    return (
      <tr onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); submitEdit(); } if (e.key === "Escape") setEditing(false); }}>
        <td style={td}><input aria-label={`Nombre de ${concept.name}`} value={name} onChange={(e) => setName(e.target.value.toUpperCase())} style={cell} /></td>
        <td style={td}><input aria-label={`Monto de ${concept.name}`} inputMode="decimal" value={amount} aria-invalid={error !== undefined ? "true" : undefined} onChange={(e) => { setAmount(e.target.value); if (error !== undefined && amountProblem(e.target.value) === undefined) setError(undefined); }} style={cell} /></td>
        <td style={td}>
          <select aria-label={`Se cobra ${concept.name}`} value={recurrence} onChange={(e) => setRecurrence(e.target.value)} style={cell}>
            {RECURRENCE_OPTIONS.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
          </select>
        </td>
        <td style={td}>{error !== undefined && <span role="alert" style={{ color: "var(--bad-fg)", fontSize: 13, fontWeight: 600 }}>{error}</span>}</td>
        <td style={{ ...td, whiteSpace: "nowrap" }}>
          <button type="button" aria-label={`Guardar ${concept.name}`} aria-busy={saving || undefined} onClick={submitEdit} style={link}>{saving ? "Guardando…" : "Guardar"}</button>
          <button type="button" onClick={() => setEditing(false)} style={{ ...link, color: "var(--text-muted)" }}>Cancelar</button>
        </td>
      </tr>
    );
  }

  return (
    <tr style={{ color: concept.active ? "var(--text)" : "var(--text-faint)" }}>
      <td style={{ ...td, fontWeight: 600 }}>{concept.name}</td>
      <td style={td}>{formatCents(concept.defaultAmountCents)}</td>
      <td style={td}>{RECURRENCE_LABEL[concept.recurrence]}</td>
      <td style={td}>
        {concept.active ? "Activo" : "Inactivo"}
        {system && <span title="La facturación lo usa: se puede editar, no desactivar." style={{ marginLeft: 8, padding: "2px 9px", borderRadius: 99, fontSize: 12, fontWeight: 600, background: "var(--surface-3)", color: "var(--text-muted)" }}>Del sistema</span>}
      </td>
      {canWrite && (
        <td style={{ ...td, whiteSpace: "nowrap" }}>
          <button type="button" aria-label={`Editar ${concept.name}`} onClick={() => { setName(concept.name); setAmount(formatPesosInput(concept.defaultAmountCents)); setRecurrence(concept.recurrence); setError(undefined); setEditing(true); }} style={link}>Editar</button>
          {/* CC-404: billing finds a system concept by kind and ignores `active` — no switch that lies. */}
          {(!system || !concept.active) && <button type="button" aria-label={`${concept.active ? "Desactivar" : "Reactivar"} ${concept.name}`} aria-busy={saving || undefined} onClick={toggle} style={{ ...link, color: concept.active ? "var(--text-muted)" : "var(--forest)" }}>{concept.active ? "Desactivar" : "Reactivar"}</button>}
        </td>
      )}
    </tr>
  );
}
