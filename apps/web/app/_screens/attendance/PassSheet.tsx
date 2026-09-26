"use client";

import { useEffect, useState, type ReactElement } from "react";

import { Toast } from "../../_design/Toast";
import { DateField } from "../../_design/form";
import { EmptyHint, ErrorAlert, Section } from "../config/parts";
import { PassSheetRow } from "./PassSheetRow";
import { Counters, ReadOnlyBanner } from "./PassSheetParts";
import { dateWindow, readOnlyText, sessionLine } from "./sheetModel";
import { useAttendanceSheet, type LoadSheet, type SaveSheet } from "./useAttendanceSheet";
import { confirmDiscard } from "./useLeaveGuard";

export interface PassSheetProps {
  /** What the list is of: "1° SECUNDARIA · A · Matemáticas". */
  title: string;
  groupId: string;
  /** Only for a per-class list. */
  subjectId?: string;
  /** The local today: where the list opens; the roster's server day, when sent, bounds the picker. */
  today: string;
  /** A teacher may open only today…today−window; the director any day. */
  restrictDates: boolean;
  load: LoadSheet;
  save: SaveSheet;
  /** Tells the screen whether leaving would lose marks. */
  onDirtyChange?: (dirty: boolean) => void;
}

const DEFAULT_WINDOW_DAYS = 7;
const LOCKED_OPACITY = 0.6;
const UNTAKEN_LINE = "Lista no tomada ese día";
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
  const bounds = dateWindow(sheet?.today ?? today, windowDays, restrictDates);
  /** A past (or future) list nobody took: no statuses, no counters — "Presentes 4" would be a lie. */
  const untaken = sheet !== undefined && !editable && sheet.session === null;
  const locked = list.saving;

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
        {sheet !== undefined && <p style={{ margin: 0, padding: "10px 0", fontSize: 14, fontWeight: 600, color: sheet.session === null ? "var(--text-muted)" : "var(--forest)" }}>{untaken ? UNTAKEN_LINE : sessionLine(sheet.session)}</p>}
      </div>

      {list.loading ? (
        <EmptyHint>Cargando lista…</EmptyHint>
      ) : list.loadError !== undefined ? (
        <ErrorAlert message={list.loadError} />
      ) : sheet === undefined ? null : rows.length === 0 ? (
        <EmptyHint>Este grupo no tiene alumnos inscritos.</EmptyHint>
      ) : (
        <>
          {!editable && <ReadOnlyBanner text={readOnlyText(sheet.readOnlyReason, windowDays, restrictDates)} />}
          <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: 10, marginBottom: 12 }}>
            {!untaken && <Counters counts={list.counts} />}
            {editable && (
              <button type="button" aria-disabled={locked || undefined} onClick={list.allPresent} style={{ minHeight: 44, padding: "0 16px", borderRadius: "var(--r-pill)", border: "1.5px solid var(--border)", background: "var(--surface)", color: "var(--text)", fontWeight: 700, fontSize: 14, cursor: locked ? "progress" : "pointer", opacity: locked ? LOCKED_OPACITY : 1 }}>
                Todos presentes
              </button>
            )}
          </div>
          <ul aria-label="Pase de lista" style={{ display: "grid", gap: 8, listStyle: "none", margin: 0, padding: 0 }}>
            {rows.map((row) => (
              <PassSheetRow key={row.enrollmentId} row={row} editable={editable} locked={locked} showStatus={!untaken} onStatus={list.setStatus} onNote={list.setNote} />
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
