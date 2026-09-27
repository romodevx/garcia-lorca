"use client";

import { useCallback, useRef, type ReactElement } from "react";

import { sheetStats, type GradeSheet, type MonthlyGradeKey } from "../../grades/monthly";
import { EmptyHint } from "../config/parts";
import type { GradeSaveStore } from "./gradeSaveStore";
import { GradeSheetRow } from "./GradeSheetRow";
import { SheetCounters } from "./SheetCounters";
import { useSaveStoreVersion } from "./useGradeSaves";

export interface GradeSheetTableProps {
  sheet: GradeSheet;
  saves: GradeSaveStore;
}

const TH: React.CSSProperties = {
  position: "sticky",
  top: 0,
  zIndex: 1,
  background: "var(--surface)",
  padding: "10px 8px",
  fontSize: 12.5,
  fontWeight: 700,
  letterSpacing: ".04em",
  textTransform: "uppercase",
  color: "var(--text-faint)",
};

/**
 * One month of one class: the counters and the capture table. Mounted per
 * month; every cell's save state lives in the screen's store, so a switch
 * forgets nothing — an unsaved value comes back in its field.
 */
export function GradeSheetTable({ sheet, saves }: GradeSheetTableProps): ReactElement {
  useSaveStoreVersion(saves);
  const inputs = useRef<(HTMLInputElement | null)[]>([]);
  const { passing } = sheet.scale;
  const keyOf = useCallback((enrollmentId: string): MonthlyGradeKey => ({ assignmentId: sheet.assignmentId, month: sheet.month, enrollmentId }), [sheet.assignmentId, sheet.month]);
  // Read fresh on every store change (a class is a few dozen rows).
  const confirmed = sheet.rows.map((r) => saves.confirmedOf(keyOf(r.enrollmentId), r.value));
  const stats = sheetStats(confirmed, passing);

  const move = useCallback((from: number, step: 1 | -1): boolean => {
    const next = inputs.current[from + step];
    if (next === null || next === undefined) return false;
    next.focus();
    next.select();
    return true;
  }, []);

  if (sheet.rows.length === 0) return <EmptyHint>Este grupo aún no tiene alumnos inscritos.</EmptyHint>;

  return (
    <div>
      <SheetCounters stats={stats} pending={saves.pendingCount()} unsaved={saves.unsavedCount()} retryable={saves.retryableCount()} onRetryAll={() => saves.retryAll()} />
      {!sheet.editable && (
        <p role="note" style={{ margin: "0 0 12px", padding: "12px 16px", borderRadius: "var(--r-md)", background: "var(--warn-bg)", color: "var(--warn-fg)", fontSize: 15, fontWeight: 600 }}>
          {sheet.readOnlyReason ?? "Este mes ya no se puede modificar."}
        </p>
      )}
      <div style={{ background: "var(--surface)", border: "1px solid var(--border)", borderRadius: "var(--r-xl)", boxShadow: "var(--shadow-card)", padding: "4px 8px" }}>
        <table aria-label="Calificaciones del mes" style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr>
              <th scope="col" style={{ ...TH, textAlign: "left" }}>Alumno</th>
              <th scope="col" style={{ ...TH, textAlign: "right" }}>Calificación ({sheet.scale.min}–{sheet.scale.max})</th>
            </tr>
          </thead>
          <tbody>
            {sheet.rows.map((row, index) => {
              const key = keyOf(row.enrollmentId);
              const value = confirmed[index] ?? null;
              const failing = value !== null && value < passing;
              if (!sheet.editable) return <ReadOnlyRow key={row.enrollmentId} name={row.name} value={value} failing={failing} />;
              return (
                <GradeSheetRow
                  key={row.enrollmentId}
                  row={row}
                  initialText={saves.draftOf(key)}
                  scale={sheet.scale}
                  status={saves.statusOf(key)}
                  failing={failing}
                  inputRef={(el) => { inputs.current[index] = el; }}
                  onCommit={(next) => saves.commit(key, next, row.value)}
                  onInvalid={(text) => saves.markInvalid(key, text, row.value)}
                  onMove={(step) => move(index, step)}
                  onRetry={() => saves.retry(key)}
                  onTyping={() => saves.dismissRejection(key)}
                />
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function ReadOnlyRow({ name, value, failing }: { name: string; value: number | null; failing: boolean }): ReactElement {
  return (
    <tr style={{ borderTop: "1px solid var(--border)" }}>
      <th scope="row" style={{ textAlign: "left", padding: "14px 8px", fontSize: 16, fontWeight: 600, color: "var(--text)" }}>
        {name}
        {failing && <span style={{ display: "block", fontSize: 13, fontWeight: 600, color: "var(--bad-fg)" }}>Reprobado</span>}
      </th>
      <td style={{ padding: "14px 8px", textAlign: "right", fontFamily: "var(--font-display)", fontSize: 18, fontWeight: 700, color: "var(--text)" }}>
        {value === null ? "—" : String(value)}
      </td>
    </tr>
  );
}
