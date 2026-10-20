"use client";

import { type ReactElement, type Ref } from "react";

import { GRADE_LETTERS, type PeriodGradeRow } from "../../grades/periodSheet";
import type { CellStatus } from "./gradeSaveStore";
import { CellStatusLine } from "./GradeSheetRow";

export interface LetterSheetRowProps {
  row: PeriodGradeRow;
  /** The letter the server holds (or the one being saved). */
  letter: string | null;
  status: CellStatus;
  /** The first letter button, for moving between pupils. */
  inputRef: Ref<HTMLInputElement>;
  onCommit: (next: string | null) => void;
  onRetry: () => void;
}

const option = (checked: boolean): React.CSSProperties => ({
  display: "inline-flex",
  alignItems: "center",
  gap: 6,
  minHeight: 44,
  padding: "0 12px",
  borderRadius: 99,
  cursor: "pointer",
  fontSize: 14.5,
  fontWeight: 600,
  border: `1.5px solid ${checked ? "var(--forest)" : "var(--border)"}`,
  background: checked ? "color-mix(in oklab, var(--forest) 12%, var(--surface))" : "var(--surface)",
  color: "var(--text)",
});

/**
 * One pupil on a letters sheet (preescolar): A–D, each with its meaning, as a
 * radio group — four choices, one tap, saved at once (no field to leave). A
 * chosen letter can be cleared.
 */
export function LetterSheetRow({ row, letter, status, inputRef, onCommit, onRetry }: LetterSheetRowProps): ReactElement {
  return (
    <tr style={{ borderTop: "1px solid var(--border)" }}>
      <th scope="row" style={{ textAlign: "left", padding: "10px 8px", fontSize: 16, fontWeight: 600, color: "var(--text)", wordBreak: "break-word" }}>
        {row.name}
      </th>
      <td style={{ padding: "10px 8px", textAlign: "right", verticalAlign: "top" }}>
        <div role="radiogroup" aria-label={`Calificación de ${row.name}`} style={{ display: "inline-flex", flexWrap: "wrap", justifyContent: "flex-end", gap: 6 }}>
          {GRADE_LETTERS.map(({ letter: l, meaning }, i) => (
            <label key={l} style={option(letter === l)}>
              <input
                ref={i === 0 ? inputRef : undefined}
                type="radio"
                name={`letter-${row.enrollmentId}`}
                value={l}
                checked={letter === l}
                onChange={() => onCommit(l)}
                aria-label={`${l} · ${meaning}`}
                style={{ accentColor: "var(--forest)" }}
              />
              <span aria-hidden><strong>{l}</strong> {meaning}</span>
            </label>
          ))}
        </div>
        {letter !== null && (
          <div>
            <button type="button" onClick={() => onCommit(null)} aria-label={`Quitar la calificación de ${row.name}`}
              style={{ minHeight: 44, marginTop: 2, border: "none", background: "none", color: "var(--text-muted)", fontSize: 13.5, fontWeight: 600, textDecoration: "underline", cursor: "pointer" }}>
              Quitar
            </button>
          </div>
        )}
        <CellStatusLine status={status} onRetry={onRetry} />
      </td>
    </tr>
  );
}
