"use client";

import type { ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import type { Scholarship, ScholarshipTarget } from "../../scholarships/api";
import { KIND_LABEL, scholarshipValueLabel, targetsSummary } from "../../scholarships/format";

export type ActiveFilter = "active" | "inactive";

const th = { textAlign: "left", fontSize: 12, fontWeight: 700, color: "var(--text-faint)", textTransform: "uppercase", letterSpacing: ".04em", padding: "12px 14px" } as const;
const td = { padding: "14px", fontSize: 14.5, color: "var(--text)", borderTop: "1px solid var(--border)", verticalAlign: "middle" } as const;
const COLUMNS = 6;

export interface ScholarshipsTableProps {
  rows: readonly Scholarship[];
  targets: readonly ScholarshipTarget[];
  loading: boolean;
  filter: ActiveFilter;
  counts: Readonly<Record<ActiveFilter, number>>;
  onFilter: (f: ActiveFilter) => void;
  canWrite: boolean;
  onEdit: (s: Scholarship) => void;
  onAssign: (s: Scholarship) => void;
}

/** The scholarships, one per row, in the drawer's field order: name, kind/value, targets, pupils, status. */
export function ScholarshipsTable({ rows, targets, loading, filter, counts, onFilter, canWrite, onEdit, onAssign }: ScholarshipsTableProps): ReactElement {
  const tabs: { id: ActiveFilter; label: string }[] = [{ id: "active", label: "Activas" }, { id: "inactive", label: "Inactivas" }];
  return (
    <div style={{ borderRadius: "var(--r-xl)", background: "var(--surface)", border: "1px solid var(--border)", boxShadow: "var(--shadow-card)", overflow: "hidden" }}>
      <div role="tablist" aria-label="Estado" style={{ display: "flex", gap: 6, padding: 12 }}>
        {tabs.map((t) => {
          const on = t.id === filter;
          return (
            <button key={t.id} role="tab" aria-selected={on} onClick={() => onFilter(t.id)}
              style={{ minHeight: 44, padding: "0 14px", borderRadius: 99, background: on ? "var(--surface-3)" : "transparent", border: "1px solid var(--border)", color: on ? "var(--text)" : "var(--text-muted)", fontWeight: 600 }}>
              {`${t.label} · ${counts[t.id]}`}
            </button>
          );
        })}
      </div>
      <div style={{ overflowX: "auto" }}>
        <table style={{ width: "100%", borderCollapse: "collapse", minWidth: 780 }}>
          <thead>
            <tr>
              <th style={th}>Beca</th><th style={th}>Tipo y valor</th><th style={th}>Aplica a</th>
              <th style={{ ...th, textAlign: "right" }}>Alumnos</th><th style={th}>Estado</th><th style={th} aria-label="Acciones" />
            </tr>
          </thead>
          <tbody>
            {loading && <tr><td style={td} colSpan={COLUMNS}>Cargando…</td></tr>}
            {!loading && rows.length === 0 && (
              <tr><td style={{ ...td, color: "var(--text-muted)" }} colSpan={COLUMNS}>
                {filter === "active" ? "Todavía no hay becas activas." : "No hay becas inactivas."}
              </td></tr>
            )}
            {rows.map((s) => (
              <tr key={s.id} data-testid="scholarship-row" style={{ boxShadow: `inset 3px 0 0 ${s.active ? "var(--purple)" : "var(--red)"}` }}>
                <td style={{ ...td, fontWeight: 700 }}>{s.name}</td>
                <td style={td}>
                  <span style={{ display: "block", fontWeight: 700 }}>{scholarshipValueLabel(s.kind, s.value)}</span>
                  <span style={{ display: "block", fontSize: 13, color: "var(--text-muted)" }}>{KIND_LABEL[s.kind]}</span>
                </td>
                <td style={{ ...td, color: "var(--text-muted)" }}>{targetsSummary(s.targets, targets)}</td>
                <td style={{ ...td, textAlign: "right", fontWeight: 700 }}>{s.assignedCount}</td>
                <td style={td}><StatusPill active={s.active} /></td>
                <td style={{ ...td, whiteSpace: "nowrap" }}>
                  <span style={{ display: "inline-flex", gap: 8 }}>
                    {canWrite && <Btn kind="ghost" icon="edit" onClick={() => onEdit(s)} style={{ height: 44 }}>Editar</Btn>}
                    <Btn kind="ghost" icon="students" onClick={() => onAssign(s)} style={{ height: 44 }}>{canWrite ? "Asignar alumnos" : "Ver alumnos"}</Btn>
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function StatusPill({ active }: { active: boolean }): ReactElement {
  return (
    <span style={{ display: "inline-flex", alignItems: "center", gap: 6, padding: "4px 10px", borderRadius: 99, fontSize: 13, fontWeight: 600, background: active ? "var(--ok-bg)" : "var(--bad-bg)", color: active ? "var(--ok-fg)" : "var(--bad-fg)" }}>
      <span aria-hidden style={{ width: 7, height: 7, borderRadius: 99, background: "currentColor" }} />
      {active ? "Activa" : "Inactiva"}
    </span>
  );
}
