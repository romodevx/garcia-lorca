"use client";

import { useId, useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { inputStyle } from "../../_design/form";
import type { ImportRow } from "../../studentImport/api";
import { rowReasons } from "../../studentImport/copy";
import { PAGE_SIZE, REVIEW_TABS, TABS, pageOf, tabCounts, visibleRows, type ImportTab } from "../../studentImport/view";
import { ImportTableRow, type PreviewReview } from "./ImportReviewRow";
import { cardStyle } from "./parts";

export interface ImportTableProps {
  rows: readonly ImportRow[];
  tab: ImportTab;
  onTab: (tab: ImportTab) => void;
  /** module_import_review: per-row decisions; absent, the table is read-only as before. */
  review?: PreviewReview;
}

const th = { textAlign: "left", padding: "10px 12px", fontSize: 12.5, fontWeight: 700, textTransform: "uppercase", letterSpacing: ".04em", color: "var(--text-faint)" } as const;
const HEADERS = ["# fila", "Alumno", "CURP", "Destino", "Estado", "Motivo"];

/** Every row of the file: tabs by fate, search by name or CURP, 50 per page; cards under 640 px. */
export function ImportTable({ rows, tab, onTab, review }: ImportTableProps): ReactElement {
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(0);
  const searchId = useId();
  const counts = tabCounts(rows);
  const shown = visibleRows(rows, tab, query);
  const pages = Math.max(1, Math.ceil(shown.length / PAGE_SIZE));
  const current = Math.min(page, pages - 1);
  const first = shown.length === 0 ? 0 : current * PAGE_SIZE + 1;
  const last = Math.min((current + 1) * PAGE_SIZE, shown.length);
  const tabs = review === undefined ? TABS : [...TABS, ...REVIEW_TABS];
  const headers = review === undefined ? HEADERS : [...HEADERS, "Acciones"];

  return (
    <section aria-label="Filas del archivo" style={cardStyle}>
      <div role="tablist" aria-label="Filtrar filas" style={{ display: "flex", flexWrap: "wrap", gap: 4, padding: 4, borderRadius: "var(--r-pill)", background: "var(--surface-3)", justifySelf: "start" }}>
        {tabs.map((t) => (
          <button
            key={t.id}
            role="tab"
            type="button"
            aria-selected={t.id === tab}
            onClick={() => { onTab(t.id); setPage(0); }}
            style={{ minHeight: 44, padding: "0 14px", borderRadius: "var(--r-pill)", fontSize: 14, fontWeight: 600, background: t.id === tab ? "var(--surface)" : "transparent", boxShadow: t.id === tab ? "var(--shadow-card)" : "none", color: t.id === tab ? "var(--text)" : "var(--text-muted)" }}
          >
            {`${t.label} · ${counts[t.id]}`}
          </button>
        ))}
      </div>
      <div style={{ display: "grid", gap: 6, maxWidth: 420 }}>
        <label htmlFor={searchId} style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)" }}>Buscar por nombre o CURP</label>
        <input id={searchId} type="search" value={query} onChange={(e) => { setQuery(e.target.value); setPage(0); }} style={inputStyle(false)} />
      </div>
      <table className="import-table" style={{ width: "100%", borderCollapse: "collapse" }}>
        <thead>
          <tr>{headers.map((h) => <th key={h} scope="col" style={th}>{h}</th>)}</tr>
        </thead>
        <tbody>
          {pageOf(shown, current).map((r) => (
            <ImportTableRow key={r.row} row={r} review={review} reasons={<Reasons reasons={rowReasons(r)} />} />
          ))}
        </tbody>
      </table>
      {shown.length === 0 && <p style={{ color: "var(--text-muted)", margin: 0 }}>Ninguna fila en esta vista.</p>}
      <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 10, borderTop: "1px solid var(--border)", paddingTop: 12 }}>
        <p style={{ fontSize: 14, color: "var(--text-muted)", margin: 0, flex: "1 1 auto" }}>{`Mostrando ${first}–${last} de ${shown.length}`}</p>
        <Btn kind="ghost" onClick={() => setPage(Math.max(0, current - 1))} style={{ height: 44 }}>Anterior</Btn>
        <span style={{ fontSize: 14, color: "var(--text-muted)" }}>{`${current + 1} / ${pages}`}</span>
        <Btn kind="ghost" onClick={() => setPage(Math.min(pages - 1, current + 1))} style={{ height: 44 }}>Siguiente</Btn>
      </div>
    </section>
  );
}

/** The first reason, and "+N" that unfolds the rest. */
function Reasons({ reasons }: { reasons: readonly string[] }): ReactElement | null {
  const [open, setOpen] = useState(false);
  const [first, ...rest] = reasons;
  if (first === undefined) return null;
  return (
    <div style={{ display: "grid", gap: 4 }}>
      <span>{first}</span>
      {rest.length > 0 && !open && (
        <button type="button" onClick={() => setOpen(true)} aria-expanded={false} style={{ justifySelf: "start", minHeight: 44, minWidth: 44, color: "var(--forest)", fontWeight: 700 }}>{`+${rest.length}`}</button>
      )}
      {open && rest.map((r) => <span key={r}>{r}</span>)}
    </div>
  );
}
