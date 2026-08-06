"use client";

import { useState, type ChangeEvent, type CSSProperties, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import type { RosterRow } from "../../grades/api";

/** Numeric grade bounds — mirrors the server's `value in [0, 10]` invariant. */
export const MIN_VALUE = 0;
export const MAX_VALUE = 10;

export interface GradeRowProps {
  row: RosterRow;
  studentName: string;
  /** Persists the captured value; rejects surface as an inline row error. */
  onSave: (enrollmentId: string, value: number) => Promise<void>;
}

const inputStyle: CSSProperties = {
  height: 40,
  padding: "0 12px",
  borderRadius: "var(--r-md)",
  border: "1px solid var(--border)",
  background: "var(--surface)",
  color: "var(--text)",
  fontSize: 15,
  outline: "none",
  width: 96,
};

/**
 * One roster row: the student's name and an editable 0–10 grade input pre-filled
 * with the current value when present. A per-row Save persists via `onSave`. A
 * client-side range guard (0–10) mirrors the server so an out-of-range value never
 * leaves the browser; anything the server still rejects surfaces inline. The
 * gateway is the security boundary — this row is UX only.
 */
export function GradeRow({ row, studentName, onSave }: GradeRowProps): ReactElement {
  const [text, setText] = useState(row.hasValue ? String(row.value) : "");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  const errorId = `grade-error-${row.enrollmentId}`;

  const save = async (): Promise<void> => {
    const value = Number(text);
    if (text.trim() === "" || Number.isNaN(value) || value < MIN_VALUE || value > MAX_VALUE) {
      setError(`La calificación debe estar entre ${MIN_VALUE} y ${MAX_VALUE}.`);
      return;
    }
    setSaving(true);
    setError(undefined);
    try {
      await onSave(row.enrollmentId, value);
    } catch {
      setError("No se pudo guardar la calificación. Intenta de nuevo.");
    } finally {
      setSaving(false);
    }
  };

  return (
    <tr style={{ borderTop: "1px solid var(--border)" }}>
      <td style={{ padding: "12px 8px", fontWeight: 600, fontSize: 15, color: "var(--text)" }}>{studentName}</td>
      <td style={{ padding: "12px 8px" }}>
        <input
          type="number"
          aria-label={`Calificación de ${studentName}`}
          aria-invalid={error !== undefined}
          aria-describedby={error !== undefined ? errorId : undefined}
          min={MIN_VALUE}
          max={MAX_VALUE}
          step={0.01}
          value={text}
          disabled={saving}
          onChange={(e: ChangeEvent<HTMLInputElement>) => setText(e.target.value)}
          style={inputStyle}
        />
        {error !== undefined && (
          <p id={errorId} role="alert" style={{ margin: "6px 0 0", color: "var(--bad-fg)", fontSize: 12.5, fontWeight: 600 }}>
            {error}
          </p>
        )}
      </td>
      <td style={{ padding: "12px 8px" }}>
        <Btn type="button" kind="outline" icon="check" disabled={saving} onClick={save}>
          Guardar
        </Btn>
      </td>
    </tr>
  );
}
