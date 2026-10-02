"use client";

import { useState, type ReactElement } from "react";

import { Btn } from "../../../_design/Controls";
import { Icon } from "../../../_design/Icon";
import { DateField, TextField } from "../../../_design/form";
import type { Cycle, GradeLevel } from "../../../config/api";
import type { PeriodDraft, PeriodField, PeriodProblem } from "../../../config/evaluationPeriods";
import { EmptyHint, ErrorAlert, SelectChip } from "../parts";

export interface GradePeriodsCardProps {
  grade: GradeLevel;
  /** The level's other grades: where this grade's periods can be copied. */
  others: readonly GradeLevel[];
  cycle: Cycle;
  rows: readonly PeriodDraft[];
  problems: readonly PeriodProblem[];
  error: string | undefined;
  busy: boolean;
  canWrite: boolean;
  onEdit: (key: string, field: PeriodField, value: string) => void;
  onBlur: (key: string, field: PeriodField) => void;
  onAdd: () => void;
  onRemove: (key: string) => void;
  onSave: () => void;
  onCopy: (toGradeIds: string[]) => Promise<boolean>;
}

const NO_TARGET = "Elige al menos un grado.";

/** The input of a row's field, for moving focus to it (see focusPeriodField). */
export function periodFieldSelector(key: string, field: PeriodField): string {
  return `[data-period-key="${CSS.escape(key)}"] [data-field="${field}"] input`;
}

/**
 * One grade's evaluation periods: a row per period (name, start, end — each
 * with its label above), edited in place and saved as a whole set. Copying to
 * other grades is an inline choice of grades, not a dialog: one decision.
 * The primary button is never disabled; saving says what is wrong instead.
 */
export function GradePeriodsCard(props: GradePeriodsCardProps): ReactElement {
  const { grade, others, cycle, rows, problems, error, busy, canWrite } = props;
  const problemOf = (key: string, field: PeriodField): string | undefined =>
    problems.find((p) => p.key === key && p.field === field)?.message;

  return (
    <section
      role="region"
      aria-label={`Periodos de ${grade.name}`}
      style={{ display: "grid", gap: 14, padding: "18px 20px", borderRadius: "var(--r-xl)", background: "var(--surface)", border: "1px solid var(--border)", boxShadow: "var(--shadow-card)", minWidth: 0 }}
    >
      <h2 style={{ fontSize: 19, fontWeight: 700, color: "var(--text)" }}>{grade.name}</h2>

      {rows.length === 0 && <EmptyHint>Este grado aún no tiene periodos en el ciclo {cycle.name}.</EmptyHint>}

      {rows.map((row, i) => (
        <div
          key={row.key}
          role="group"
          aria-label={`Periodo ${i + 1}`}
          data-period-key={row.key}
          style={{ display: "flex", flexWrap: "wrap", alignItems: "flex-start", gap: 12, paddingBottom: 12, borderBottom: "1px solid var(--border)" }}
        >
          <span aria-hidden="true" style={{ width: 32, height: 44, marginTop: 22, display: "grid", placeItems: "center", fontWeight: 700, color: "var(--text-muted)" }}>{i + 1}</span>
          <div data-field="name" style={{ flex: "2 1 200px", minWidth: 0 }}>
            <TextField
              label="Nombre"
              uppercase
              required
              value={row.name}
              disabled={!canWrite}
              error={problemOf(row.key, "name")}
              onChange={(v) => props.onEdit(row.key, "name", v)}
              onBlur={() => props.onBlur(row.key, "name")}
            />
          </div>
          <div data-field="startsOn" style={{ flex: "1 1 160px", minWidth: 0 }}>
            <DateField
              label="Inicio"
              required
              value={row.startsOn}
              min={cycle.startsOn}
              max={cycle.endsOn}
              disabled={!canWrite}
              error={problemOf(row.key, "startsOn")}
              onChange={(v) => props.onEdit(row.key, "startsOn", v)}
              onBlur={() => props.onBlur(row.key, "startsOn")}
            />
          </div>
          <div data-field="endsOn" style={{ flex: "1 1 160px", minWidth: 0 }}>
            <DateField
              label="Fin"
              required
              value={row.endsOn}
              min={cycle.startsOn}
              max={cycle.endsOn}
              disabled={!canWrite}
              error={problemOf(row.key, "endsOn")}
              onChange={(v) => props.onEdit(row.key, "endsOn", v)}
              onBlur={() => props.onBlur(row.key, "endsOn")}
            />
          </div>
          {canWrite && (
            <button
              type="button"
              aria-label={`Quitar periodo ${i + 1}`}
              title="Quitar periodo"
              onClick={() => props.onRemove(row.key)}
              style={{ width: 44, height: 44, marginTop: 22, borderRadius: "var(--r-md)", display: "grid", placeItems: "center", color: "var(--text-muted)", border: "1px solid var(--border)", background: "var(--surface)", cursor: "pointer" }}
            >
              <Icon name="trash" size={18} />
            </button>
          )}
        </div>
      ))}

      {error !== undefined && <ErrorAlert message={error} />}

      {canWrite && (
        <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 10 }}>
          <Btn icon="check" onClick={props.onSave} style={{ height: 44 }}>{busy ? "Guardando…" : "Guardar periodos"}</Btn>
          <Btn kind="ghost" icon="plus" onClick={props.onAdd} style={{ height: 44 }}>Agregar periodo</Btn>
          {others.length > 0 && <CopyToGrades others={others} onCopy={props.onCopy} />}
        </div>
      )}
    </section>
  );
}

function CopyToGrades({ others, onCopy }: { others: readonly GradeLevel[]; onCopy: (ids: string[]) => Promise<boolean> }): ReactElement {
  const [open, setOpen] = useState(false);
  const [chosen, setChosen] = useState<ReadonlySet<string>>(new Set());
  const [error, setError] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);

  const run = async (ids: string[]): Promise<void> => {
    if (busy) return;
    if (ids.length === 0) {
      setError(NO_TARGET);
      return;
    }
    setBusy(true);
    try {
      if (await onCopy(ids)) {
        setOpen(false);
        setChosen(new Set());
      }
    } finally {
      setBusy(false);
    }
  };

  const toggle = (id: string): void => {
    setError(undefined);
    setChosen((prev) => {
      const next = new Set(prev);
      if (!next.delete(id)) next.add(id);
      return next;
    });
  };

  const wholeLevel = (
    <Btn kind="ghost" icon="graduation" onClick={() => void run(others.map((g) => g.id))} style={{ height: 44 }}>
      Aplicar a todo el nivel
    </Btn>
  );

  if (!open) {
    return (
      <>
        <Btn kind="ghost" icon="more" onClick={() => setOpen(true)} style={{ height: 44 }}>Copiar a…</Btn>
        {wholeLevel}
      </>
    );
  }
  return (
    <div style={{ flexBasis: "100%", display: "grid", gap: 10, padding: 14, borderRadius: "var(--r-md)", background: "var(--surface-2)" }}>
      <div role="group" aria-label="Grados destino" style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
        {others.map((g) => <SelectChip key={g.id} selected={chosen.has(g.id)} onClick={() => toggle(g.id)}>{g.name}</SelectChip>)}
      </div>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 10, alignItems: "center" }}>
        <Btn icon="check" onClick={() => void run(others.filter((g) => chosen.has(g.id)).map((g) => g.id))} style={{ height: 44 }}>
          {busy ? "Copiando…" : "Copiar periodos"}
        </Btn>
        {wholeLevel}
        <button type="button" onClick={() => setOpen(false)} style={{ minHeight: 44, color: "var(--text-muted)", fontWeight: 600, cursor: "pointer" }}>Cancelar</button>
      </div>
      {error !== undefined && <ErrorAlert message={error} />}
    </div>
  );
}
