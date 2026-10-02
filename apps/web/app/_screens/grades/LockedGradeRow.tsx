"use client";

import type { ReactElement } from "react";

export interface LockedGradeRowProps {
  name: string;
  value: number | null;
  /** Why this row cannot change, e.g. the period belongs to the pupil's previous group. */
  reason: string;
  failing: boolean;
}

const fieldStyle: React.CSSProperties = {
  width: 96,
  height: 48,
  padding: "0 12px",
  borderRadius: "var(--r-md)",
  border: "1.5px dashed var(--border)",
  background: "var(--surface-3)",
  color: "var(--text-muted)",
  fontFamily: "var(--font-display)",
  fontSize: 18,
  fontWeight: 700,
  textAlign: "right",
};

/**
 * A student whose grade this sheet shows but cannot change. The field stays
 * (readable, announced as read-only) but is out of the Enter/Tab capture path.
 */
export function LockedGradeRow({ name, value, reason, failing }: LockedGradeRowProps): ReactElement {
  return (
    <tr style={{ borderTop: "1px solid var(--border)" }}>
      <th scope="row" style={{ textAlign: "left", padding: "10px 8px", fontSize: 16, fontWeight: 600, color: "var(--text)", wordBreak: "break-word" }}>
        {name}
        <span style={{ display: "block", fontSize: 13, fontWeight: 500, color: "var(--text-muted)" }}>{reason}</span>
        {failing && <span style={{ display: "block", fontSize: 13, fontWeight: 600, color: "var(--bad-fg)" }}>Reprobado</span>}
      </th>
      <td style={{ padding: "10px 8px", textAlign: "right", verticalAlign: "top" }}>
        <input
          type="text"
          readOnly
          aria-readonly="true"
          tabIndex={-1}
          aria-label={`Calificación de ${name}`}
          value={value === null ? "" : String(value)}
          style={fieldStyle}
        />
      </td>
    </tr>
  );
}
