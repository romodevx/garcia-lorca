"use client";

import type { ReactElement } from "react";

import { formatCents } from "../../_lib/money";
import { shortDayLabel } from "../../_lib/localDate";
import type { ExpectedLine } from "../../finance/api";

export interface DebtListProps {
  lines: readonly ExpectedLine[];
  /** Charges the cashier took off the list; everything else is chosen. */
  unchosen: ReadonlySet<string>;
  onToggle: (chargeId: string) => void;
}

/**
 * What the family owes, concept by concept (task 8): the cashier sees what the
 * money is paying and can leave a concept out. Everything starts chosen, which
 * is the rule as it always was — the oldest debt first.
 */
export function DebtList({ lines, unchosen, onToggle }: DebtListProps): ReactElement | null {
  if (lines.length === 0) {
    return null;
  }
  const total = lines.reduce((sum, l) => sum + l.totalCents, 0);
  const selected = lines.reduce((sum, l) => sum + (unchosen.has(l.chargeId) ? 0 : l.totalCents), 0);
  return (
    <div style={{ borderRadius: "var(--r-md)", border: "1px solid var(--border)", overflow: "hidden" }}>
      <div style={{ padding: "10px 16px", background: "var(--surface-2)", fontSize: 13.5, fontWeight: 700, color: "var(--text-muted)", fontFamily: "var(--font-display)" }}>
        Adeudo
      </div>
      {lines.map((l) => (
        <label
          key={l.chargeId}
          data-testid={`debt-row-${l.chargeId}`}
          style={{ display: "flex", alignItems: "center", gap: 12, minHeight: 52, padding: "8px 16px", borderTop: "1px solid var(--border)", cursor: "pointer" }}
        >
          <input
            type="checkbox"
            checked={!unchosen.has(l.chargeId)}
            onChange={() => onToggle(l.chargeId)}
            aria-label={l.description}
            style={{ width: 20, height: 20, accentColor: "var(--forest)", flexShrink: 0 }}
          />
          <span style={{ flex: 1, minWidth: 0 }}>
            <span style={{ display: "block", fontSize: 14.5, fontWeight: 600, color: "var(--text)" }}>{l.description}</span>
            <span style={{ display: "block", fontSize: 12.5, color: "var(--text-muted)" }}>
              {l.due ? "vence" : "por vencer"} {shortDayLabel(l.dueDate)} · {formatCents(l.owedCents)}
              {l.surchargeCents > 0 && ` + recargo ${formatCents(l.surchargeCents)}`}
              {l.discountCents > 0 && ` − pronto pago ${formatCents(l.discountCents)}`}
            </span>
          </span>
          <span style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 15, color: "var(--text)" }}>{formatCents(l.totalCents)}</span>
        </label>
      ))}
      <div style={{ display: "grid", gap: 4, padding: "10px 16px", borderTop: "1px solid var(--border)", background: "var(--surface-2)", fontSize: 14 }}>
        <span style={{ display: "flex", justifyContent: "space-between" }}>
          <span style={{ color: "var(--text-muted)" }}>Total del adeudo</span>
          <b data-testid="debt-total">{formatCents(total)}</b>
        </span>
        <span style={{ display: "flex", justifyContent: "space-between" }}>
          <span style={{ color: "var(--text-muted)" }}>Total seleccionado</span>
          <b data-testid="debt-selected">{formatCents(selected)}</b>
        </span>
      </div>
    </div>
  );
}
