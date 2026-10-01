"use client";

import { useEffect, useState, type ReactElement } from "react";
import { getIncome, type Income } from "../../finance/api";
import { API_URL } from "../../api-url";
import { formatCents } from "../../_lib/money";

export interface IncomePanelProps {
  /** Both inclusive, "2026-10-01" … "2026-10-31". */
  from: string;
  to: string;
}

/**
 * What the school earned, by what it charged for.
 *
 * The director's question is "how much came in for colegiaturas this month?"
 * and until now the answer was a spreadsheet somebody built by hand from the
 * payment list. Biggest line first, because a report is read from the top.
 */
export function IncomePanel({ from, to }: IncomePanelProps): ReactElement {
  const [income, setIncome] = useState<Income | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let live = true;
    getIncome(API_URL, from, to)
      .then((i) => { if (live) { setIncome(i); } })
      .catch(() => { if (live) { setFailed(true); } });
    return () => { live = false; };
  }, [from, to]);

  if (failed) {
    return <p style={{ color: "var(--text-muted)", fontSize: 15 }}>No se pudo leer el ingreso del periodo.</p>;
  }
  if (income === null) {
    return <p style={{ color: "var(--text-muted)", fontSize: 15 }}>Cargando…</p>;
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      <div style={{ padding: "20px 24px", borderRadius: "var(--r-lg)", background: "var(--surface)", border: "1px solid var(--border)", boxShadow: "var(--shadow-card)" }}>
        <span style={{ fontSize: 14, color: "var(--text-muted)" }}>Ingresos del periodo</span>
        <div data-testid="income-total" style={{ fontFamily: "var(--font-display)", fontWeight: 800, fontSize: 38, color: "var(--text)", marginTop: 4 }}>
          {formatCents(income.totalCents)}
        </div>
        <p style={{ fontSize: 13, color: "var(--text-faint)", marginTop: 6 }}>{from} → {to}</p>
        {income.creditNoteCents > 0 && (
          // Credit notes settled debt in the period but are not income (T8):
          // apart, and outside the total above.
          <div data-testid="income-credit-notes" style={{ display: "flex", justifyContent: "space-between", gap: 12, marginTop: 14, paddingTop: 12, borderTop: "1px dashed var(--border)" }}>
            <span style={{ fontSize: 14, color: "var(--text-muted)" }}>Notas de crédito aplicadas — no es ingreso</span>
            <span style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 16, color: "var(--text-muted)" }}>{formatCents(income.creditNoteCents)}</span>
          </div>
        )}
      </div>

      {income.lines.length === 0
        ? <p style={{ color: "var(--text-muted)", fontSize: 15, margin: 0 }}>No se recibió nada en este periodo.</p>
        : (
          <div style={{ borderRadius: "var(--r-lg)", border: "1px solid var(--border)", background: "var(--surface)", overflow: "hidden" }}>
            {income.lines.map((line) => (
              <div key={line.conceptId} style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12, padding: "13px 20px", borderTop: "1px solid var(--border)" }}>
                <span style={{ fontSize: 15, color: "var(--text)" }}>{line.concept}</span>
                <span style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 15.5, color: "var(--text)" }}>{formatCents(line.amountCents)}</span>
              </div>
            ))}
          </div>
        )}
    </div>
  );
}
