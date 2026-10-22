"use client";

import { useRef, useState, type CSSProperties, type ReactElement, type RefObject } from "react";

import { Btn } from "../../_design/Controls";
import type { ImportRow, RowAction, RowDecision, RowFieldError, RowOverrides } from "../../studentImport/api";
import { issueLine, reasonText, rowErrorText, rowReasons } from "../../studentImport/copy";
import { needsReview, type Decisions } from "../../studentImport/review";
import { RowCorrection } from "./RowCorrection";
import { StatusPill } from "./parts";

/** What the preview needs to let the office decide each row (module_import_review). */
export interface PreviewReview {
  decisions: Decisions;
  /** The server's refusals of the current decisions (422 IMPORT_ROWS_INVALID). */
  rowErrors: readonly RowFieldError[];
  onAction: (row: number, action: RowAction | undefined) => void;
  onCorrect: (row: number, overrides: RowOverrides) => void;
  /** The school's day, aaaa-mm-dd: a birth date cannot be after it. */
  today: string;
}

export const cellStyle: CSSProperties = { padding: "10px 12px", borderTop: "1px solid var(--border)", fontSize: 14.5, color: "var(--text)", verticalAlign: "top", overflowWrap: "anywhere" };
const COLUMNS_WITH_ACTIONS = 7;
const rowAction: CSSProperties = { height: 44, padding: "0 12px", fontSize: 13.5 };
const refusalStyle: CSSProperties = { color: "var(--bad-fg)", fontWeight: 600 };

/** One row of the file; with the review on, its issues, its actions and its inline correction. */
export function ImportTableRow({ row, review, reasons }: { row: ImportRow; review?: PreviewReview; reasons: ReactElement }): ReactElement {
  const [editing, setEditing] = useState(false);
  const [corrected, setCorrected] = useState(false);
  const editButton = useRef<HTMLButtonElement>(null);
  const rowHeader = useRef<HTMLTableCellElement>(null);
  const decision = review?.decisions[row.row];
  const errors = review?.rowErrors.filter((e) => e.row === row.row) ?? [];
  const reviewed = review !== undefined && needsReview(row, decision);

  const close = (target: RefObject<HTMLElement | null>): void => {
    setEditing(false);
    // After the editor unmounts, so the target is there to take focus.
    queueMicrotask(() => target.current?.focus());
  };

  return (
    <>
      <tr>
        <td data-label="Fila" style={cellStyle}>{row.row}</td>
        {/* The focus target after a save: the re-preview may replace the row's buttons, never its header. */}
        <th ref={rowHeader} tabIndex={-1} scope="row" style={{ ...cellStyle, fontWeight: 600, textAlign: "left" }}>{row.name}</th>
        <td data-label="CURP" style={cellStyle}>{row.curp}</td>
        <td data-label="Destino" style={cellStyle}>{row.destination}</td>
        <td style={cellStyle}><StatusPill status={row.status} /></td>
        <td style={cellStyle}>
          {reviewed ? <ReviewNotes row={row} errors={errors} /> : reasons}
          {corrected && <span role="status" style={{ display: "block", marginTop: 4, color: "var(--ok-fg)", fontWeight: 600 }}>{`Fila ${row.row} corregida`}</span>}
        </td>
        {review !== undefined && (
          <td data-label="Acciones" style={cellStyle}>
            {reviewed && <ReviewActions row={row} decision={decision} onAction={review.onAction} onEdit={() => setEditing(true)} editButton={editButton} />}
          </td>
        )}
      </tr>
      {review !== undefined && editing && (
        <tr>
          <td colSpan={COLUMNS_WITH_ACTIONS} style={{ ...cellStyle, background: "var(--surface-2)" }}>
            <RowCorrection
              row={row}
              decision={decision}
              errors={errors}
              today={review.today}
              onSave={(overrides) => {
                const changed = Object.keys(overrides).length > 0;
                if (changed) review.onCorrect(row.row, overrides);
                setCorrected(changed);
                close(changed ? rowHeader : editButton);
              }}
              onCancel={() => close(editButton)}
            />
          </td>
        </tr>
      )}
    </>
  );
}

/** Why the row is as it is: the skip reason, each issue with the file's value, and the server's refusals. */
function ReviewNotes({ row, errors }: { row: ImportRow; errors: readonly RowFieldError[] }): ReactElement {
  const lines = [
    ...(row.reason !== undefined && row.reason !== "" ? [{ key: `reason-${row.reason}`, text: reasonText(row.reason) }] : []),
    ...(row.issues ?? []).map((i) => ({ key: `${i.field}-${i.code}`, text: issueLine(i) })),
  ];
  const shown = lines.length > 0 ? lines : rowReasons(row).map((text, at) => ({ key: `warning-${at}`, text }));
  return (
    <div style={{ display: "grid", gap: 4 }}>
      {shown.map((line) => <span key={line.key}>{line.text}</span>)}
      {errors.map((e) => <span key={`${e.row}-${e.field}-${e.code}`} style={refusalStyle}>{rowErrorText(e)}</span>)}
    </div>
  );
}

function ReviewActions({ row, decision, onAction, onEdit, editButton }: {
  row: ImportRow;
  decision: RowDecision | undefined;
  onAction: (row: number, action: RowAction | undefined) => void;
  onEdit: () => void;
  editButton: RefObject<HTMLButtonElement | null>;
}): ReactElement {
  const n = row.row;
  if (decision?.action === "omit") {
    return (
      <div style={{ display: "grid", gap: 6, justifyItems: "start" }}>
        <span style={{ color: "var(--text-muted)", fontWeight: 600 }}>Omitida</span>
        <Btn kind="ghost" onClick={() => onAction(n, undefined)} aria-label={`Incluir de nuevo la fila ${n}`} style={rowAction}>Incluir de nuevo</Btn>
      </div>
    );
  }
  const drafted = decision?.action === "draft";
  return (
    <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
      {(row.issues ?? []).length > 0 && <Btn ref={editButton} kind="ghost" onClick={onEdit} aria-label={`Corregir fila ${n}`} style={rowAction}>Corregir</Btn>}
      <Btn kind="ghost" onClick={() => onAction(n, "omit")} aria-label={`Omitir fila ${n}`} style={rowAction}>Omitir</Btn>
      {row.canDraft === true && (
        drafted
          ? <Btn kind="ghost" onClick={() => onAction(n, undefined)} aria-label={`Quitar borrador de la fila ${n}`} style={rowAction}>Quitar borrador</Btn>
          : <Btn kind="ghost" onClick={() => onAction(n, "draft")} aria-label={`Importar fila ${n} como borrador`} style={rowAction}>Importar como borrador</Btn>
      )}
    </div>
  );
}
