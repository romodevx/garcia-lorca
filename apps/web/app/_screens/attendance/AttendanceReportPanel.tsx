"use client";

import { useEffect, useState, type CSSProperties, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { DateField } from "../../_design/form";
import { GroupChain, NO_GROUP_CHOICE, type GroupChoice } from "../../_design/students/GroupChain";
import type { FilterCatalog } from "../../_design/students/studentFilter";
import { downloadBlob } from "../../_lib/downloadBlob";
import type { AttendanceCounts, AttendanceReport, AttendanceReportQuery, ReportFormat } from "../../attendance/report";
import { EmptyHint } from "../config/parts";

export interface AttendanceReportPanelProps {
  /** The cycle's levels, grades and groups the chips come from. */
  catalog: FilterCatalog;
  /** The school's today (`YYYY-MM-DD`): the range ends here by default. */
  today: string;
  load: (q: AttendanceReportQuery) => Promise<AttendanceReport>;
  download: (q: AttendanceReportQuery, format: ReportFormat) => Promise<{ blob: Blob; filename: string }>;
}

const box: CSSProperties = { padding: 20, borderRadius: "var(--r-xl)", background: "var(--surface)", border: "1px solid var(--border)", boxShadow: "var(--shadow-card)" };
const th: CSSProperties = { textAlign: "right", fontSize: 12, fontWeight: 700, color: "var(--text-faint)", textTransform: "uppercase", letterSpacing: ".04em", padding: "8px 10px", whiteSpace: "nowrap" };
const td: CSSProperties = { padding: "10px", fontSize: 14.5, color: "var(--text)", borderTop: "1px solid var(--border)", textAlign: "right", fontVariantNumeric: "tabular-nums" };
const nameCell: CSSProperties = { ...td, textAlign: "left", fontVariantNumeric: "normal", overflowWrap: "anywhere" };

/** The columns, in the pass list's own words and order. Retardos get their orange. */
const COLUMNS: readonly { key: keyof AttendanceCounts; label: string; tone?: string }[] = [
  { key: "present", label: "Presentes" },
  { key: "absent", label: "Ausentes" },
  { key: "late", label: "Retardos", tone: "var(--orange)" },
  { key: "excused", label: "Justificadas" },
];

const firstOfMonth = (day: string): string => `${day.slice(0, 7)}-01`;

function CountsRow({ label, counts, strong = false }: { label: string; counts: AttendanceCounts; strong?: boolean }): ReactElement {
  const weight = strong ? 700 : undefined;
  return (
    <tr>
      <td style={{ ...nameCell, fontWeight: weight }}>{label}</td>
      {COLUMNS.map((c) => (
        <td key={c.key} style={{ ...td, fontWeight: weight, color: c.tone !== undefined && counts[c.key] > 0 ? c.tone : td.color }}>{counts[c.key]}</td>
      ))}
    </tr>
  );
}

/**
 * The attendance report (T12): one group — chosen level → grade → group — over
 * a range of days, with each pupil's present, absent, late and excused counts
 * and the totals, exportable to PDF and Excel (the gateway renders both).
 */
export function AttendanceReportPanel({ catalog, today, load, download }: AttendanceReportPanelProps): ReactElement {
  const [choice, setChoice] = useState<GroupChoice>(NO_GROUP_CHOICE);
  const [from, setFrom] = useState(firstOfMonth(today));
  const [to, setTo] = useState(today);
  const [report, setReport] = useState<AttendanceReport | null>(null);
  const [error, setError] = useState<string | undefined>(undefined);
  const [exporting, setExporting] = useState<ReportFormat | null>(null);
  const { groupId } = choice;
  const rangeOk = from !== "" && to !== "" && from <= to;

  useEffect(() => {
    setReport(null);
    setError(undefined);
    if (groupId === "" || !rangeOk) return;
    let live = true;
    load({ groupId, from, to })
      .then((r) => live && setReport(r))
      .catch((err: unknown) => live && setError(err instanceof Error ? err.message : "No se pudo leer el reporte."));
    return () => {
      live = false;
    };
  }, [groupId, from, to, rangeOk, load]);

  const exportAs = async (format: ReportFormat): Promise<void> => {
    setExporting(format);
    try {
      const { blob, filename } = await download({ groupId, from, to }, format);
      downloadBlob(blob, filename);
    } catch (err) {
      setError(err instanceof Error ? err.message : "No se pudo exportar el reporte.");
    } finally {
      setExporting(null);
    }
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 18 }}>
      <div style={{ ...box, display: "grid", gap: 16 }}>
        <GroupChain catalog={catalog} value={choice} onChange={setChoice} />
        <div style={{ display: "flex", flexWrap: "wrap", gap: 16, alignItems: "flex-end" }}>
          <div style={{ width: 200, maxWidth: "100%" }}>
            <DateField label="Desde" value={from} max={to !== "" ? to : today} onChange={setFrom} />
          </div>
          <div style={{ width: 200, maxWidth: "100%" }}>
            <DateField label="Hasta" value={to} min={from} max={today} onChange={setTo} />
          </div>
          <div style={{ flex: 1 }} />
          {report !== null && (
            <>
              <Btn kind="ghost" icon="download" onClick={() => exporting === null && void exportAs("pdf")}>{exporting === "pdf" ? "Generando…" : "PDF"}</Btn>
              <Btn kind="ghost" icon="download" onClick={() => exporting === null && void exportAs("xlsx")}>{exporting === "xlsx" ? "Generando…" : "Excel"}</Btn>
            </>
          )}
        </div>
      </div>

      {groupId === "" && <EmptyHint>Elige nivel, grado y grupo para ver su reporte.</EmptyHint>}
      {groupId !== "" && !rangeOk && <EmptyHint>La fecha final no puede ser anterior a la inicial.</EmptyHint>}
      {error !== undefined && <p role="alert" style={{ margin: 0, color: "var(--bad-fg)", fontSize: 14 }}>{error}</p>}

      {report !== null && (
        <div style={box}>
          <h2 style={{ fontSize: 18, fontWeight: 800, color: "var(--text)", margin: 0 }}>{report.groupLabel}</h2>
          <p style={{ margin: "4px 0 12px", fontSize: 14, color: "var(--text-muted)" }}>{report.listsTaken === 1 ? "1 lista tomada" : `${report.listsTaken} listas tomadas`}</p>
          {report.rows.length === 0 ? (
            <EmptyHint>El grupo no tiene alumnos activos.</EmptyHint>
          ) : (
            // A phone scrolls the table inside its card, never the page.
            <div style={{ overflowX: "auto" }}>
              <table aria-label="Asistencia por alumno" style={{ width: "100%", borderCollapse: "collapse", minWidth: 480 }}>
                <thead>
                  <tr>
                    <th style={{ ...th, textAlign: "left" }}>Alumno</th>
                    {COLUMNS.map((c) => <th key={c.key} style={th}>{c.label}</th>)}
                  </tr>
                </thead>
                <tbody>
                  {report.rows.map((r) => <CountsRow key={r.enrollmentId} label={r.active ? r.name : `${r.name} (baja)`} counts={r} />)}
                  <CountsRow label="Total" counts={report.totals} strong />
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
