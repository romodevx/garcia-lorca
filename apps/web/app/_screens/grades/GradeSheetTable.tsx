"use client";

import { useCallback, useRef, type ReactElement } from "react";

import { letterLabel, letterStats, markOf, sheetStats, type GradeMark, type PeriodGradeKey, type PeriodGradeRow, type PeriodSheet } from "../../grades/periodSheet";
import { EmptyHint } from "../config/parts";
import type { CellStatus, GradeSaveStore } from "./gradeSaveStore";
import { GradeSheetRow } from "./GradeSheetRow";
import { LetterSheetRow } from "./LetterSheetRow";
import { LockedGradeRow } from "./LockedGradeRow";
import { LetterCounters, SheetCounters } from "./SheetCounters";
import { useSaveStoreVersion } from "./useGradeSaves";

export interface GradeSheetTableProps {
  sheet: PeriodSheet;
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
 * One evaluation period of one class: the counters and the capture table.
 * Mounted per period; every cell's save state lives in the screen's store, so a switch
 * forgets nothing — an unsaved value comes back in its field.
 */
export function GradeSheetTable({ sheet, saves }: GradeSheetTableProps): ReactElement {
  useSaveStoreVersion(saves);
  const inputs = useRef<(HTMLInputElement | null)[]>([]);
  const { passing } = sheet.scale;
  // A sheet without a period has no rows to key: the parent shows why instead.
  const periodId = sheet.periodId ?? "";
  const keyOf = useCallback((enrollmentId: string): PeriodGradeKey => ({ assignmentId: sheet.assignmentId, periodId, enrollmentId }), [sheet.assignmentId, periodId]);
  // Read fresh on every store change (a class is a few dozen rows).
  const confirmed = sheet.rows.map((r) => saves.confirmedOf(keyOf(r.enrollmentId), markOf(r)));
  const letters = sheet.gradingLetters === true;

  /** Focus the next EDITABLE student `step` away; read-only rows leave no input here. */
  const move = useCallback((from: number, step: 1 | -1): boolean => {
    for (let i = from + step; i >= 0 && i < inputs.current.length; i += step) {
      const next = inputs.current[i];
      if (next === null || next === undefined) continue;
      next.focus();
      next.select();
      return true;
    }
    return false;
  }, []);
  const notice = saves.sheetNotice({ assignmentId: sheet.assignmentId, periodId });

  // No grades at all: counters, a "(0–0)" scale and a column of dashes would say nothing.
  if (!sheet.gradingEnabled && !letters) return <EmptyHint>{sheet.readOnlyReason ?? NOT_GRADED_FALLBACK}</EmptyHint>;
  if (sheet.rows.length === 0) return <EmptyHint>Este grupo aún no tiene alumnos inscritos.</EmptyHint>;

  return (
    <div>
      {letters ? (
        <LetterCounters stats={letterStats(confirmed)} pending={saves.pendingCount()} unsaved={saves.unsavedCount()} retryable={saves.retryableCount()} onRetryAll={() => saves.retryAll()} />
      ) : (
        <SheetCounters stats={sheetStats(confirmed, passing)} pending={saves.pendingCount()} unsaved={saves.unsavedCount()} retryable={saves.retryableCount()} onRetryAll={() => saves.retryAll()} />
      )}
      {!sheet.editable && (
        <p role="note" style={{ margin: "0 0 12px", padding: "12px 16px", borderRadius: "var(--r-md)", background: "var(--warn-bg)", color: "var(--warn-fg)", fontSize: 15, fontWeight: 600 }}>
          {sheet.readOnlyReason ?? "Este periodo ya no se puede modificar."}
        </p>
      )}
      {notice !== undefined && (
        <p role="alert" style={{ margin: "0 0 12px", padding: "12px 16px", borderRadius: "var(--r-md)", background: "var(--bad-bg)", color: "var(--bad-fg)", fontSize: 15, fontWeight: 600 }}>
          {notice}
        </p>
      )}
      <div style={{ background: "var(--surface)", border: "1px solid var(--border)", borderRadius: "var(--r-xl)", boxShadow: "var(--shadow-card)", padding: "4px 8px" }}>
        <table aria-label="Calificaciones del periodo" style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr>
              <th scope="col" style={{ ...TH, textAlign: "left" }}>Alumno</th>
              <th scope="col" style={{ ...TH, textAlign: "right" }}>{letters ? "Calificación (A–D)" : `Calificación (${sheet.scale.min}–${sheet.scale.max})`}</th>
            </tr>
          </thead>
          <tbody>
            {sheet.rows.map((row, index) => {
              const key = keyOf(row.enrollmentId);
              const value = confirmed[index] ?? null;
              const failing = typeof value === "number" && value < passing;
              if (!sheet.editable) return <ReadOnlyRow key={row.enrollmentId} name={row.name} value={value} failing={failing} />;
              const status = saves.statusOf(key);
              const lockReason = rowLockReason(row, status);
              if (lockReason !== undefined) {
                return <LockedGradeRow key={row.enrollmentId} name={row.name} value={value} reason={lockReason} failing={failing} />;
              }
              if (letters) {
                return (
                  <LetterSheetRow
                    key={row.enrollmentId}
                    row={row}
                    letter={typeof value === "string" ? value : null}
                    status={status}
                    inputRef={(el) => { inputs.current[index] = el; }}
                    onCommit={(next) => saves.commit(key, next, markOf(row))}
                    onRetry={() => saves.retry(key)}
                  />
                );
              }
              return (
                <GradeSheetRow
                  key={row.enrollmentId}
                  row={row}
                  initialText={saves.draftOf(key)}
                  scale={sheet.scale}
                  status={status}
                  failing={failing}
                  inputRef={(el) => { inputs.current[index] = el; }}
                  onCommit={(next) => saves.commit(key, next, markOf(row))}
                  onInvalid={(text) => saves.markInvalid(key, text, markOf(row))}
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

const NOT_GRADED_FALLBACK = "Este nivel no lleva calificación numérica";
const PREVIOUS_GROUP_FALLBACK = "Este periodo no se puede modificar para este alumno.";

/** Why a row of an open period cannot change: the loaded sheet says so, or a save was just refused for it. */
function rowLockReason(row: PeriodGradeRow, status: CellStatus): string | undefined {
  if (!row.rowEditable) return row.rowReadOnlyReason ?? PREVIOUS_GROUP_FALLBACK;
  return status.kind === "locked" ? status.reason || PREVIOUS_GROUP_FALLBACK : undefined;
}

function ReadOnlyRow({ name, value, failing }: { name: string; value: GradeMark | null; failing: boolean }): ReactElement {
  return (
    <tr style={{ borderTop: "1px solid var(--border)" }}>
      <th scope="row" style={{ textAlign: "left", padding: "14px 8px", fontSize: 16, fontWeight: 600, color: "var(--text)" }}>
        {name}
        {failing && <span style={{ display: "block", fontSize: 13, fontWeight: 600, color: "var(--bad-fg)" }}>Reprobado</span>}
      </th>
      <td style={{ padding: "14px 8px", textAlign: "right", fontFamily: "var(--font-display)", fontSize: 18, fontWeight: 700, color: "var(--text)" }}>
        {value === null ? "—" : typeof value === "string" ? letterLabel(value) : String(value)}
      </td>
    </tr>
  );
}
