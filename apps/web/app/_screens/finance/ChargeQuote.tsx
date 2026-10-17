"use client";

import { useEffect, useState, type CSSProperties, type ReactElement } from "react";

import { API_URL } from "../../api-url";
import { getExpected, type ExpectedLine } from "../../finance/api";
import { formatCents } from "../../_lib/money";

/** One charge's quote for a day, and whether it could be read at all. */
export interface ChargeQuoteState {
  /** Undefined while it is read, or when it could not be. */
  line: ExpectedLine | undefined;
  /** The quote failed or did not list the charge: the bare balance stands in. */
  unquoted: boolean;
}

/**
 * What one charge asks for on the day the family paid (review B1): its balance,
 * the late fee being late has cost it and the pronto-pago reward it earns —
 * the counter's quote, for the one line. Every new day starts from nothing, so
 * the previous day's figure never caps the amount while the new one is read.
 */
export function useChargeQuote(studentId: string, chargeId: string, paidOn: string, enabled: boolean): ChargeQuoteState {
  const [state, setState] = useState<ChargeQuoteState>({ line: undefined, unquoted: false });
  useEffect(() => {
    setState({ line: undefined, unquoted: false });
    if (!enabled) return;
    let live = true;
    getExpected(API_URL, studentId, paidOn, { withBalance: false })
      .then((e) => {
        if (!live) return;
        const line = e.lines.find((l) => l.chargeId === chargeId);
        setState({ line, unquoted: line === undefined });
      })
      .catch(() => { if (live) setState({ line: undefined, unquoted: true }); });
    return () => { live = false; };
  }, [studentId, chargeId, paidOn, enabled]);
  return state;
}

const row: CSSProperties = { display: "flex", justifyContent: "space-between", fontSize: 14, color: "var(--text-muted)" };

/** The quote said before the money is taken: what makes up "a pagar hoy". */
export function ChargeQuote({ quote: { line, unquoted }, balanceCents }: { quote: ChargeQuoteState; balanceCents: number }): ReactElement {
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
      {unquoted && (
        <p role="status" style={{ margin: 0, fontSize: 13, color: "var(--warn-fg)" }}>
          No se pudo cotizar el cargo para ese día: se usa su saldo, sin recargo ni pronto pago.
        </p>
      )}
    </div>
  );
}
