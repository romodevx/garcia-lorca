"use client";

import type { ReactElement } from "react";

import { formatCents } from "../../_lib/money";
import { shortDayLabel } from "../../_lib/localDate";
import type { ExpectedLine } from "../../finance/api";

export interface DebtListProps {
  lines: readonly ExpectedLine[];
  /**
   * The ONE balance (T5): what the year still owes, surcharge accrued to the
   * day included and credit netted. The debt's total IS this figure.
   */
  balanceCents: number;
  /** The charges the money pays. */
  chosen: ReadonlySet<string>;
  /** Their total, computed once by the drawer (selectedTotal). */
  selectedCents: number;
  onToggle: (chargeId: string) => void;
}

/**
 * What the family owes, concept by concept (task 8): the cashier sees what the
 * money is paying, can leave a concept out, or choose a month not yet due to
 * pay ahead.
 */
export function DebtList({ lines, balanceCents, chosen, selectedCents, onToggle }: DebtListProps): ReactElement | null {
  if (lines.length === 0) {
    return null;
  }
  // The pronto-pago prices are a projection, not a second debt: said apart,
  // as the balance less every early-payment reward still within reach.
  const rewardCents = lines.reduce((sum, l) => sum + l.discountCents, 0);
  const onTimeCents = balanceCents - rewardCents;
  return (
    // flexShrink 0: the drawer's body is a column of fixed height, and a box
    // with overflow hidden shrinks in it to nothing (it measured 2 px).
    <div style={{ borderRadius: "var(--r-md)", border: "1px solid var(--border)", overflow: "hidden", flexShrink: 0 }}>
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
            checked={chosen.has(l.chargeId)}
            onChange={() => onToggle(l.chargeId)}
            aria-label={l.description}
            style={{ width: 20, height: 20, accentColor: "var(--forest)", flexShrink: 0 }}
          />
          <span style={{ flex: 1, minWidth: 0 }}>
            <span style={{ display: "block", fontSize: 14.5, fontWeight: 600, color: "var(--text)" }}>{l.description}</span>
            <span style={{ display: "block", fontSize: 12.5, color: "var(--text-muted)" }}>
              {l.due ? "vence" : "por vencer"} {shortDayLabel(l.dueDate)} · {formatCents(l.owedCents)}
              {l.scholarshipCents > 0 && ` ya con beca −${formatCents(l.scholarshipCents)}`}
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
          <b data-testid="debt-total">{formatCents(balanceCents)}</b>
        </span>
        {rewardCents > 0 && onTimeCents > 0 && (
          <span data-testid="debt-on-time" style={{ display: "flex", justifyContent: "space-between", fontSize: 13, color: "var(--text-muted)" }}>
            <span>Si paga cada mes a tiempo</span>
            <span>{formatCents(onTimeCents)}</span>
          </span>
        )}
        <span style={{ display: "flex", justifyContent: "space-between" }}>
          <span style={{ color: "var(--text-muted)" }}>Total seleccionado</span>
          <b data-testid="debt-selected">{formatCents(selectedCents)}</b>
        </span>
      </div>
    </div>
  );
}
