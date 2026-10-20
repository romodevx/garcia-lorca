"use client";

import { useId, useState, type KeyboardEvent, type ReactElement, type Ref } from "react";

import { Icon } from "../../_design/Icon";
import { parseGradeInput, validateGrade, type GradeScale, type PeriodGradeRow } from "../../grades/periodSheet";
import type { CellStatus } from "./gradeSaveStore";

export interface GradeSheetRowProps {
  row: PeriodGradeRow;
  /** An unsaved value kept from before a period/assignment switch; else the loaded grade. */
  initialText: string | undefined;
  scale: GradeScale;
  status: CellStatus;
  /** The confirmed grade is below the passing mark. */
  failing: boolean;
  inputRef: Ref<HTMLInputElement>;
  onCommit: (next: number | null) => void;
  /** The field was left with text the scale refuses: it stays unsaved. */
  onInvalid: (text: string) => void;
  /** Focus the student `step` rows away; false when there is none. */
  onMove: (step: 1 | -1) => boolean;
  onRetry: () => void;
  onTyping: () => void;
}

const inputStyle = (invalid: boolean): React.CSSProperties => ({
  width: 96,
  height: 48,
  padding: "0 12px",
  borderRadius: "var(--r-md)",
  border: `1.5px solid ${invalid ? "var(--bad-fg)" : "var(--border)"}`,
  background: "var(--surface)",
  color: "var(--text)",
  fontFamily: "var(--font-display)",
  fontSize: 18,
  fontWeight: 700,
  textAlign: "right",
});

/** One student: the name, one grade field, and what happened to its last save. */
export function GradeSheetRow({ row, initialText, scale, status, failing, inputRef, onCommit, onInvalid, onMove, onRetry, onTyping }: GradeSheetRowProps): ReactElement {
  const [text, setText] = useState(initialText ?? (row.value === null ? "" : String(row.value)));
  const [error, setError] = useState<string | undefined>(() => (initialText === undefined ? undefined : validateGrade(initialText, scale)));
  const errorId = useId();
  const shownError = error ?? (status.kind === "rejected" ? status.message : undefined);
  // "Guardado" speaks for the field's text: a value the scale refused was never sent.
  const shownStatus: CellStatus = error !== undefined && status.kind === "saved" ? { kind: "idle" } : status;

  /** Checks and saves the cell; false when it is wrong and must be fixed first. */
  const settle = (): boolean => {
    const problem = validateGrade(text, scale);
    setError(problem);
    if (problem !== undefined) {
      onInvalid(text);
      return false;
    }
    const parsed = parseGradeInput(text);
    onCommit(parsed.kind === "value" ? parsed.value : null);
    return true;
  };

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>): void => {
    if (e.key === "Enter") {
      e.preventDefault();
      if (settle()) onMove(1);
      return;
    }
    // Leaving by Tab saves through the blur it causes; the next student gets the focus —
    // unless this cell failed: then Tab must reach its retry button.
    if (e.key === "Tab" && !(status.kind === "failed" && !e.shiftKey) && onMove(e.shiftKey ? -1 : 1)) e.preventDefault();
  };

  const change = (next: string): void => {
    setText(next);
    onTyping();
    if (error !== undefined && validateGrade(next, scale) === undefined) setError(undefined);
  };

  return (
    <tr style={{ borderTop: "1px solid var(--border)" }}>
      <th scope="row" style={{ textAlign: "left", padding: "10px 8px", fontSize: 16, fontWeight: 600, color: "var(--text)", wordBreak: "break-word" }}>
        {row.name}
        {failing && <span style={{ display: "block", fontSize: 13, fontWeight: 600, color: "var(--bad-fg)" }}>Reprobado</span>}
      </th>
      <td style={{ padding: "10px 8px", textAlign: "right", verticalAlign: "top" }}>
        <input
          ref={inputRef}
          type="text"
          inputMode="numeric"
          lang="es"
          autoComplete="off"
          aria-label={`Calificación de ${row.name}`}
          aria-invalid={shownError !== undefined}
          aria-describedby={shownError !== undefined ? errorId : undefined}
          value={text}
          onChange={(e) => change(e.target.value)}
          onBlur={() => void settle()}
          onKeyDown={onKeyDown}
          style={inputStyle(shownError !== undefined)}
        />
        {shownError !== undefined && <p id={errorId} role="alert" style={{ margin: "4px 0 0", fontSize: 13.5, fontWeight: 600, color: "var(--bad-fg)" }}>{shownError}</p>}
        <CellStatusLine status={shownStatus} onRetry={onRetry} />
      </td>
    </tr>
  );
}

export function CellStatusLine({ status, onRetry }: { status: CellStatus; onRetry: () => void }): ReactElement {
  const line: React.CSSProperties = { display: "inline-flex", alignItems: "center", gap: 4, marginTop: 4, fontSize: 13.5, fontWeight: 600 };
  return (
    <div aria-live="polite" style={{ minHeight: 20 }}>
      {status.kind === "saving" && <span style={{ ...line, color: "var(--text-muted)" }}><Icon name="clock" size={14} /><span>Guardando…</span></span>}
      {status.kind === "saved" && <span style={{ ...line, color: "var(--ok-fg)" }}><Icon name="check" size={14} stroke={2.6} /><span>Guardado</span></span>}
      {status.kind === "failed" && (
        <button type="button" onClick={onRetry} style={{ ...line, minHeight: 44, color: "var(--bad-fg)", textDecoration: "underline", cursor: "pointer" }}>
          Error — reintentar
        </button>
      )}
    </div>
  );
}
