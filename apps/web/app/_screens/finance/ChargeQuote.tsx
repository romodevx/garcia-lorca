"use client";

import { useEffect, useState, type CSSProperties, type ReactElement } from "react";

import { API_URL } from "../../api-url";
import { getExpected, type ExpectedLine } from "../../finance/api";
import { formatCents } from "../../_lib/money";

/**
 * What one charge asks for on the day the family paid (review B1): its balance,
 * the late fee being late has cost it and the pronto-pago reward it earns —
 * the counter's quote, for the one line. Undefined until read, or when the
 * quote does not list it; the caller falls back to the charge's own balance.
 */
export function useChargeQuote(studentId: string, chargeId: string, paidOn: string, enabled: boolean): ExpectedLine | undefined {
  const [line, setLine] = useState<ExpectedLine | undefined>(undefined);
  useEffect(() => {
    if (!enabled) return;
    let live = true;
    getExpected(API_URL, studentId, paidOn, { withBalance: false })
      .then((e) => { if (live) setLine(e.lines.find((l) => l.chargeId === chargeId)); })
      .catch(() => { if (live) setLine(undefined); });
    return () => { live = false; };
  }, [studentId, chargeId, paidOn, enabled]);
  return line;
}

const row: CSSProperties = { display: "flex", justifyContent: "space-between", fontSize: 14, color: "var(--text-muted)" };

/** The quote said before the money is taken: what makes up "a pagar hoy". */
export function ChargeQuote({ line, balanceCents }: { line: ExpectedLine | undefined; balanceCents: number }): ReactElement {
  const owed = line?.owedCents ?? balanceCents;
  const total = line?.totalCents ?? balanceCents;
  return (
    <div data-testid="pay-quote" style={{ display: "flex", flexDirection: "column", gap: 6, padding: "14px 16px", borderRadius: "var(--r-md)", background: "var(--surface-3)", border: "1px solid var(--border)" }}>
      <div style={row}><span>Saldo del cargo</span><span>{formatCents(owed)}</span></div>
      {line !== undefined && line.surchargeCents > 0 && <div style={row}><span>Recargo</span><span>{formatCents(line.surchargeCents)}</span></div>}
      {line !== undefined && line.discountCents > 0 && <div style={row}><span>Pronto pago</span><span>−{formatCents(line.discountCents)}</span></div>}
      <div style={{ ...row, color: "var(--text)", fontWeight: 700, fontSize: 16, fontFamily: "var(--font-display)" }}>
        <span>A pagar hoy</span><span>{formatCents(total)}</span>
      </div>
    </div>
  );
}
