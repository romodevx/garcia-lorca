"use client";

import type { ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import type { Activity } from "../../activities/api";
import { optionsSummary, PERIODICITY_LABEL, scheduleSummary } from "../../activities/format";

export type ActiveFilter = "active" | "inactive";

const th = { textAlign: "left", fontSize: 12, fontWeight: 700, color: "var(--text-faint)", textTransform: "uppercase", letterSpacing: ".04em", padding: "12px 14px" } as const;
const td = { padding: "14px", fontSize: 14.5, color: "var(--text)", borderTop: "1px solid var(--border)", verticalAlign: "middle" } as const;
const COLUMNS = 7;

export interface ActivitiesTableProps {
  rows: readonly Activity[];
  loading: boolean;
  filter: ActiveFilter;
  counts: Readonly<Record<ActiveFilter, number>>;
  onFilter: (f: ActiveFilter) => void;
  canWrite: boolean;
  onEdit: (a: Activity) => void;
  onEnroll: (a: Activity) => void;
}

/** The activities, one per row: week, periodicity, prices, pupils and the row's actions. */
export function ActivitiesTable({ rows, loading, filter, counts, onFilter, canWrite, onEdit, onEnroll }: ActivitiesTableProps): ReactElement {
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
        <table style={{ width: "100%", borderCollapse: "collapse", minWidth: 860 }}>
          <thead>
            <tr>
              <th style={th}>Actividad</th><th style={th}>Horario</th><th style={th}>Cobro</th><th style={th}>Precios</th>
              <th style={{ ...th, textAlign: "right" }}>Inscritos</th><th style={th}>Estado</th><th style={th} aria-label="Acciones" />
            </tr>
          </thead>
          <tbody>
            {loading && <tr><td style={td} colSpan={COLUMNS}>Cargando…</td></tr>}
            {!loading && rows.length === 0 && (
              <tr><td style={{ ...td, color: "var(--text-muted)" }} colSpan={COLUMNS}>
                {filter === "active" ? "Todavía no hay actividades activas." : "No hay actividades inactivas."}
              </td></tr>
            )}
            {rows.map((a) => (
              <tr key={a.id} data-testid="activity-row" style={{ boxShadow: `inset 3px 0 0 ${a.active ? "var(--forest)" : "var(--red)"}` }}>
                <td style={{ ...td, fontWeight: 700 }}>{a.name}</td>
                <td style={td}>{scheduleSummary(a.schedule)}</td>
                <td style={td}>{PERIODICITY_LABEL[a.periodicity]}</td>
                <td style={{ ...td, color: "var(--text-muted)" }}>{optionsSummary(a.options)}</td>
                <td style={{ ...td, textAlign: "right", fontWeight: 700 }}>{a.enrolledCount}</td>
                <td style={td}><StatusPill active={a.active} /></td>
                <td style={{ ...td, whiteSpace: "nowrap" }}>
                  <span style={{ display: "inline-flex", gap: 8 }}>
                    {canWrite && <Btn kind="ghost" icon="edit" onClick={() => onEdit(a)} style={{ height: 44 }}>Editar</Btn>}
                    <Btn kind="ghost" icon="students" onClick={() => onEnroll(a)} style={{ height: 44 }}>{canWrite ? "Inscribir alumnos" : "Ver inscritos"}</Btn>
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
