"use client";

import { useCallback, useEffect, useState, type CSSProperties, type ReactElement } from "react";

import { API_URL } from "../../api-url";
import { Btn } from "../../_design/Controls";
import { InlineCreateRow, type InlineColumn } from "../../_design/InlineCreateRow";
import { createConcept, listConcepts, updateConcept, type ChargeConcept, type UpdateConceptInput } from "../../finance/api";
import { pesosToCents } from "../../_lib/money";
import { ConceptRow } from "./ConceptRow";
import { amountProblem, RECURRENCE_OPTIONS, toRecurrence } from "./conceptCopy";

export interface ConceptsPanelProps {
  /** finance:write — may add, edit and deactivate. UX only: the gateway decides. */
  canWrite: boolean;
}

type Load = { state: "loading" } | { state: "failed" } | { state: "ready"; concepts: ChargeConcept[] };

const COLUMNS: readonly InlineColumn[] = [
  { key: "name", label: "Nombre", kind: "text", uppercase: true, required: true, help: "Cómo aparece en el estado de cuenta y en el comprobante." },
  { key: "amount", label: "Monto", kind: "text", required: true, width: 150, help: "Lo que se propone al cobrarlo; cada cargo puede llevar otro." },
  { key: "recurrence", label: "Se cobra", kind: "select", width: 140, options: RECURRENCE_OPTIONS },
];
const RULES = { amount: amountProblem };
const DEFAULTS = { recurrence: "once" };

const SR_ONLY: CSSProperties = { position: "absolute", width: 1, height: 1, padding: 0, margin: -1, overflow: "hidden", clip: "rect(0 0 0 0)", whiteSpace: "nowrap", border: 0 };
export const th: CSSProperties = { padding: "10px 14px", textAlign: "left", fontSize: 12.5, fontWeight: 700, color: "var(--text-faint)", textTransform: "uppercase", letterSpacing: ".04em", borderBottom: "1px solid var(--border)" };

/**
 * The school's catalog of loose charges (CC-111): credencial, examen
 * extraordinario… Three fields, entered now and then — so an inline row at the
 * foot of the list, never a drawer. Deactivating is reversible, so it happens
 * at once and the row itself keeps "Reactivar": no confirmation.
 */
export function ConceptsPanel({ canWrite }: ConceptsPanelProps): ReactElement {
  const [load, setLoad] = useState<Load>({ state: "loading" });
  const [notice, setNotice] = useState("");
  const [problem, setProblem] = useState<string | undefined>(undefined);

  const read = useCallback((): void => {
    setLoad({ state: "loading" });
    listConcepts(API_URL, true)
      .then((concepts) => setLoad({ state: "ready", concepts }))
      .catch(() => setLoad({ state: "failed" }));
  }, []);
  useEffect(() => { read(); }, [read]);

  const put = (saved: ChargeConcept): void => setLoad((prev) => prev.state !== "ready" ? prev : {
    state: "ready",
    concepts: prev.concepts.some((c) => c.id === saved.id)
      ? prev.concepts.map((c) => (c.id === saved.id ? saved : c))
      : [...prev.concepts, saved],
  });

  const create = async (values: Record<string, string>): Promise<void> => {
    setProblem(undefined);
    try {
      const saved = await createConcept(API_URL, {
        name: (values.name ?? "").trim(),
        defaultAmountCents: pesosToCents(values.amount ?? ""),
        recurrence: toRecurrence(values.recurrence ?? ""),
      });
      put(saved);
      setNotice(`${saved.name} quedó en el catálogo.`);
    } catch (cause) {
      setProblem("No se pudo guardar el concepto. Revisa que el nombre no exista ya e intenta de nuevo.");
      throw cause;
    }
  };

  const update = async (concept: ChargeConcept, input: UpdateConceptInput): Promise<void> => {
    setProblem(undefined);
    try {
      const saved = await updateConcept(API_URL, concept.id, input);
      put(saved);
      if (concept.active !== saved.active) {
        setNotice(saved.active ? `${saved.name} se ofrece otra vez.` : `${saved.name} ya no se ofrece para cargos nuevos. Puedes reactivarlo desde su renglón.`);
      }
    } catch (cause) {
      setProblem(`No se pudo guardar ${concept.name}. Intenta de nuevo.`);
      throw cause;
    }
  };

  return (
    <section aria-labelledby="concepts-title" style={{ background: "var(--surface)", border: "1px solid var(--border)", borderRadius: "var(--r-xl)", boxShadow: "var(--shadow-card)", overflow: "hidden" }}>
      <h3 id="concepts-title" style={{ padding: "16px 20px 6px", fontSize: 17, fontWeight: 700, color: "var(--text)" }}>Conceptos de cobro</h3>
      <p style={{ padding: "0 20px 12px", margin: 0, fontSize: 14, color: "var(--text-muted)" }}>Lo que se puede cargar a un alumno fuera de la colegiatura: credencial, examen extraordinario…</p>
      {problem !== undefined && <p role="alert" style={{ margin: "0 20px 12px", color: "var(--bad-fg)", fontWeight: 600, fontSize: 14 }}>{problem}</p>}
      {load.state === "loading" && <p style={{ padding: "16px 20px", color: "var(--text-muted)" }}>Cargando conceptos…</p>}
      {load.state === "failed" && (
        <div role="alert" style={{ padding: "16px 20px", display: "grid", gap: 10, justifyItems: "start" }}>
          <span style={{ color: "var(--bad-fg)" }}>No se pudieron leer los conceptos.</span>
          <Btn kind="ghost" onClick={read}>Reintentar</Btn>
        </div>
      )}
      {load.state === "ready" && load.concepts.length === 0 && (
        <p style={{ padding: "16px 20px", color: "var(--text-muted)" }}>Aún no hay conceptos. {canWrite ? "Agrega el primero abajo." : ""}</p>
      )}
      {load.state === "ready" && load.concepts.length > 0 && (
        <div style={{ overflowX: "auto" }}>
          <table aria-labelledby="concepts-title" style={{ width: "100%", borderCollapse: "collapse", fontSize: 14.5 }}>
            <thead><tr><th style={th}>Concepto</th><th style={th}>Monto</th><th style={th}>Se cobra</th><th style={th}>Estado</th>{canWrite && <th style={th}><span style={SR_ONLY}>Acciones</span></th>}</tr></thead>
            <tbody>{load.concepts.map((c) => <ConceptRow key={c.id} concept={c} canWrite={canWrite} onSave={(input) => update(c, input)} />)}</tbody>
          </table>
        </div>
      )}
      {canWrite && load.state === "ready" && <InlineCreateRow columns={COLUMNS} rules={RULES} defaults={DEFAULTS} addLabel="Agregar concepto" onCreate={create} />}
      <div role="status" style={{ padding: notice === "" ? 0 : "10px 20px", fontSize: 14, color: "var(--ok-fg)" }}>{notice}</div>
    </section>
  );
}
