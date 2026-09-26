"use client";

import { useId, useState, type ReactElement } from "react";

import { Icon, type IconName } from "../../_design/Icon";
import type { AttendanceStatus } from "../../attendance/api";
import type { AttendanceSheetRow } from "../../attendance/sheet";
import { toggled } from "./sheetModel";

export const NOTE_MAX = 500;

/** Never colour alone: every status has its icon and its word too. */
export const STATUS_META: Record<AttendanceStatus, { label: string; icon: IconName; tone: string }> = {
  present: { label: "Presente", icon: "check", tone: "var(--forest)" },
  absent: { label: "Ausente", icon: "close", tone: "var(--red)" },
  late: { label: "Retardo", icon: "clock", tone: "var(--orange)" },
  excused: { label: "Justificada", icon: "documents", tone: "var(--purple)" },
};

const SECONDARY: readonly AttendanceStatus[] = ["late", "excused"];
const LOCKED_OPACITY = 0.6;

export interface PassSheetRowProps {
  row: AttendanceSheetRow;
  editable: boolean;
  /** A save is in flight: controls stay visible but refuse taps (aria-disabled, dimmed). */
  locked?: boolean;
  /** False for a past list nobody took: there is no status to show. */
  showStatus?: boolean;
  onStatus: (enrollmentId: string, status: AttendanceStatus) => void;
  onNote: (enrollmentId: string, note: string) => void;
}

function StatusPill({ status }: { status: AttendanceStatus }): ReactElement {
  const meta = STATUS_META[status];
  return (
    <span style={{ display: "inline-flex", alignItems: "center", gap: 6, flexShrink: 0, padding: "6px 12px", borderRadius: "var(--r-pill)", border: `1.5px solid ${meta.tone}`, background: `color-mix(in oklab, ${meta.tone} 14%, var(--surface))`, color: meta.tone, fontSize: 14, fontWeight: 700 }}>
      <Icon name={meta.icon} size={16} stroke={2.6} />
      {meta.label}
    </span>
  );
}

/**
 * One student: the whole row is the tap (Presente ↔ Ausente), because in a
 * classroom the teacher aims a thumb at a name, not at a small segment. The
 * rarer choices — Retardo, Justificada, a note — sit behind "Más".
 */
export function PassSheetRow({ row, editable, locked = false, showStatus = true, onStatus, onNote }: PassSheetRowProps): ReactElement {
  const [open, setOpen] = useState(false);
  const panelId = useId();
  const meta = STATUS_META[row.status];
  const setStatus = (status: AttendanceStatus): void => {
    if (!locked) onStatus(row.enrollmentId, status);
  };
  const lock = locked ? { "aria-disabled": true, style: { opacity: LOCKED_OPACITY, cursor: "progress" } } : undefined;
  const face = (
    <>
      <span style={{ flex: "1 1 auto", minWidth: 0, fontSize: 16, fontWeight: 600, color: "var(--text)", overflowWrap: "anywhere", textAlign: "left" }}>{row.name}</span>
      {showStatus && <StatusPill status={row.status} />}
    </>
  );

  return (
    <li style={{ borderRadius: "var(--r-md)", background: "var(--surface)", border: "1px solid var(--border)", boxShadow: showStatus ? `inset 4px 0 0 ${meta.tone}` : undefined, overflow: "hidden", ...lock?.style }}>
      <div style={{ display: "flex", alignItems: "center", gap: 4 }}>
        {editable ? (
          <button type="button" aria-label={`${row.name}: ${meta.label}`} aria-disabled={lock?.["aria-disabled"]} onClick={() => setStatus(toggled(row.status))} style={{ flex: "1 1 auto", minWidth: 0, minHeight: 56, display: "flex", flexWrap: "wrap", alignItems: "center", gap: 8, padding: "8px 12px", cursor: locked ? "progress" : "pointer", background: "transparent" }}>
            {face}
          </button>
        ) : (
          <div style={{ flex: "1 1 auto", minWidth: 0, minHeight: 56, display: "flex", flexWrap: "wrap", alignItems: "center", gap: 8, padding: "8px 12px" }}>{face}</div>
        )}
        {editable && (
          <button type="button" aria-label={`Más opciones de ${row.name}`} aria-expanded={open} aria-controls={panelId} onClick={() => setOpen((v) => !v)} style={{ width: 48, height: 48, flexShrink: 0, display: "grid", placeItems: "center", borderRadius: 99, color: "var(--text-muted)", cursor: "pointer" }}>
            <Icon name="more" size={22} />
          </button>
        )}
      </div>
      {row.note !== "" && !open && <p style={{ margin: 0, padding: "0 12px 10px", fontSize: 13.5, color: "var(--text-muted)", overflowWrap: "anywhere" }}>{row.note}</p>}
      {open && (
        <div id={panelId} style={{ display: "grid", gap: 10, padding: "4px 12px 12px" }}>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
            {SECONDARY.map((status) => {
              const on = row.status === status;
              const tone = STATUS_META[status].tone;
              return (
                <button key={status} type="button" aria-pressed={on} aria-disabled={lock?.["aria-disabled"]} onClick={() => setStatus(on ? "present" : status)} style={{ minHeight: 44, display: "inline-flex", alignItems: "center", gap: 6, padding: "0 16px", borderRadius: "var(--r-pill)", fontWeight: 700, fontSize: 14, cursor: "pointer", border: `1.5px solid ${on ? tone : "var(--border)"}`, background: on ? `color-mix(in oklab, ${tone} 14%, var(--surface))` : "var(--surface)", color: on ? tone : "var(--text-muted)" }}>
                  <Icon name={STATUS_META[status].icon} size={16} stroke={2.4} />
                  {STATUS_META[status].label}
                </button>
              );
            })}
          </div>
          <label style={{ display: "grid", gap: 6, fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)" }}>
            Nota
            <textarea aria-label={`Nota de ${row.name}`} value={row.note} maxLength={NOTE_MAX} rows={2} readOnly={locked} onChange={(e) => onNote(row.enrollmentId, e.target.value)} style={{ width: "100%", boxSizing: "border-box", padding: "10px 12px", borderRadius: "var(--r-md)", border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)", fontSize: 16, fontFamily: "var(--font-ui)", resize: "vertical" }} />
          </label>
          <div style={{ display: "flex", flexWrap: "wrap", justifyContent: "space-between", gap: 8, fontSize: 12.5, color: "var(--text-faint)" }}>
            <span>{row.status === "present" && row.note !== "" ? "La nota se guarda solo si no está presente." : ""}</span>
            <span aria-live="polite">{row.note.length}/{NOTE_MAX}</span>
          </div>
        </div>
      )}
    </li>
  );
}
