"use client";

import { useEffect, useState, type ReactElement } from "react";

import { Icon } from "../../_design/Icon";
import { Toast } from "../../_design/Toast";
import { DateField } from "../../_design/form";
import type { AttendanceStatus } from "../../attendance/api";
import { EmptyHint, ErrorAlert, Section } from "../config/parts";
import { PassSheetRow, STATUS_META } from "./PassSheetRow";
import { dateWindow, sessionLine, type StatusCounts } from "./sheetModel";
import { useAttendanceSheet, type LoadSheet, type SaveSheet } from "./useAttendanceSheet";
import { confirmDiscard } from "./useLeaveGuard";

export interface PassSheetProps {
  /** What the list is of: "1° SECUNDARIA · A · Matemáticas". */
  title: string;
  groupId: string;
  /** Only for a per-class list. */
  subjectId?: string;
  /** The local today: where the list opens, and the top of a teacher's window. */
  today: string;
  /** A teacher may open only today…today−window; the director any day. */
  restrictDates: boolean;
  load: LoadSheet;
  save: SaveSheet;
  /** Tells the screen whether leaving would lose marks. */
  onDirtyChange?: (dirty: boolean) => void;
}

const DEFAULT_WINDOW_DAYS = 7;
const COUNTERS: readonly { status: AttendanceStatus; label: string }[] = [
  { status: "present", label: "Presentes" },
  { status: "absent", label: "Ausentes" },
  { status: "late", label: "Retardos" },
  { status: "excused", label: "Justificadas" },
];

function Counters({ counts }: { counts: StatusCounts }): ReactElement {
  return (
    <ul aria-label="Resumen de la lista" style={{ display: "flex", flexWrap: "wrap", gap: 8, listStyle: "none", margin: 0, padding: 0 }}>
      {COUNTERS.map(({ status, label }) => (
        <li key={status} style={{ display: "inline-flex", alignItems: "center", gap: 6, padding: "6px 12px", borderRadius: "var(--r-pill)", border: "1px solid var(--border)", background: "var(--surface)", fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)" }}>
          <Icon name={STATUS_META[status].icon} size={15} stroke={2.4} style={{ color: STATUS_META[status].tone }} />
          {label}
          <strong style={{ color: "var(--text)", fontFamily: "var(--font-display)" }}>{counts[status]}</strong>
        </li>
      ))}
    </ul>
  );
}

/**
 * The pass list shared by the teacher and the director: everyone opens
 * present, one tap per absence, "Más" for the rare cases, and one sticky
 * "Guardar lista" that sends only the exceptions. Mobile-first: one column,
 * everything wraps, no fixed widths — it is used standing, on a phone.
 */
export function PassSheet({ title, groupId, subjectId, today, restrictDates, load, save, onDirtyChange }: PassSheetProps): ReactElement {
  const [date, setDate] = useState(today);
  const list = useAttendanceSheet({ groupId, subjectId, date }, load, save);
  const { sheet, rows, dirty } = list;
  const editable = sheet?.editable ?? false;
  const windowDays = sheet?.windowDays ?? DEFAULT_WINDOW_DAYS;
  const bounds = dateWindow(today, windowDays, restrictDates);

  useEffect(() => onDirtyChange?.(dirty), [dirty, onDirtyChange]);
  useEffect(() => () => onDirtyChange?.(false), [onDirtyChange]);

  const changeDate = (next: string): void => {
    if (confirmDiscard(dirty)) setDate(next);
  };

  return (
    <Section title={`Pase de lista · ${title}`} subtitle="Todos empiezan presentes: toca solo a quien faltó.">
      <div style={{ display: "flex", flexWrap: "wrap", alignItems: "flex-end", gap: 12, marginBottom: 14 }}>
        <div style={{ flex: "0 1 220px", minWidth: 0 }}>
          <DateField label="Fecha" value={date} onChange={changeDate} {...bounds} />
        </div>
        {sheet !== undefined && <p style={{ margin: 0, padding: "10px 0", fontSize: 14, fontWeight: 600, color: sheet.session === null ? "var(--text-muted)" : "var(--forest)" }}>{sessionLine(sheet.session)}</p>}
      </div>

      {list.loading ? (
        <EmptyHint>Cargando lista…</EmptyHint>
      ) : list.loadError !== undefined ? (
        <ErrorAlert message={list.loadError} />
      ) : sheet === undefined ? null : rows.length === 0 ? (
        <EmptyHint>Este grupo no tiene alumnos inscritos.</EmptyHint>
      ) : (
        <>
          {!editable && (
            <p style={{ margin: "0 0 12px", padding: "10px 14px", borderRadius: "var(--r-md)", background: "var(--warn-bg)", color: "var(--warn-fg)", fontSize: 14, fontWeight: 600 }}>
              Solo lectura: {restrictDates ? `solo puedes corregir hasta ${windowDays} días atrás.` : "esta lista ya no se puede modificar."}
            </p>
          )}
          <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: 10, marginBottom: 12 }}>
            <Counters counts={list.counts} />
            {editable && (
              <button type="button" onClick={list.allPresent} style={{ minHeight: 44, padding: "0 16px", borderRadius: "var(--r-pill)", border: "1.5px solid var(--border)", background: "var(--surface)", color: "var(--text)", fontWeight: 700, fontSize: 14, cursor: "pointer" }}>
                Todos presentes
              </button>
            )}
          </div>
          <ul aria-label="Pase de lista" style={{ display: "grid", gap: 8, listStyle: "none", margin: 0, padding: 0 }}>
            {rows.map((row) => (
              <PassSheetRow key={row.enrollmentId} row={row} editable={editable} onStatus={list.setStatus} onNote={list.setNote} />
            ))}
          </ul>
          {editable && (
            <div style={{ position: "sticky", bottom: 0, zIndex: 5, marginTop: 14, padding: "12px 0", display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "flex-end", gap: 12, background: "var(--surface)", borderTop: "1px solid var(--border)" }}>
              {dirty && <span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--warn-fg)" }}>Cambios sin guardar</span>}
              <button type="button" aria-busy={list.saving} onClick={() => void list.save()} style={{ minHeight: 52, flex: "1 1 200px", maxWidth: 320, padding: "0 24px", borderRadius: "var(--r-pill)", background: "var(--primary)", color: "var(--primary-ink)", fontWeight: 800, fontSize: 16, cursor: "pointer" }}>
                {list.saving ? "Guardando…" : "Guardar lista"}
              </button>
            </div>
          )}
        </>
      )}

      {list.saveError !== undefined && <ErrorAlert message={list.saveError} />}
      {list.toast !== undefined && <Toast>{list.toast}</Toast>}
    </Section>
  );
}
