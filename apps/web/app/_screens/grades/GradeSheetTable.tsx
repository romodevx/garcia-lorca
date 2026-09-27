"use client";

import { useCallback, useMemo, useRef, type ReactElement } from "react";

import { sheetStats, type GradeSheet, type GradeSheetClient } from "../../grades/monthly";
import { EmptyHint } from "../config/parts";
import { GradeSheetRow } from "./GradeSheetRow";
import { SheetCounters } from "./SheetCounters";
import { useCellSaves } from "./useCellSaves";

export interface GradeSheetTableProps {
  sheet: GradeSheet;
  client: GradeSheetClient;
  pending: number;
  track: <T>(work: Promise<T>) => Promise<T>;
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
 * month, so a switch starts with fresh statuses; saves still travelling are
 * counted by the parent's `track`.
 */
export function GradeSheetTable({ sheet, client, pending, track }: GradeSheetTableProps): ReactElement {
  const saves = useCellSaves(client, sheet, track);
  const inputs = useRef<(HTMLInputElement | null)[]>([]);
  const { passing } = sheet.scale;
  const stats = useMemo(() => sheetStats(Object.values(saves.values), passing), [saves.values, passing]);

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
      <SheetCounters stats={stats} pending={pending} />
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
              const confirmed = saves.values[row.enrollmentId] ?? null;
              const failing = confirmed !== null && confirmed < passing;
              if (!sheet.editable) return <ReadOnlyRow key={row.enrollmentId} name={row.name} value={confirmed} failing={failing} />;
              return (
                <GradeSheetRow
                  key={row.enrollmentId}
                  row={row}
                  scale={sheet.scale}
                  status={saves.statusOf(row.enrollmentId)}
                  failing={failing}
                  inputRef={(el) => { inputs.current[index] = el; }}
                  onCommit={(next) => saves.commit(row.enrollmentId, next)}
                  onMove={(step) => move(index, step)}
                  onRetry={() => saves.retry(row.enrollmentId)}
                  onTyping={() => saves.dismissRejection(row.enrollmentId)}
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
