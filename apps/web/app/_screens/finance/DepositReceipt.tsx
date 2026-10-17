"use client";

import type { CSSProperties, ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { tenderLabel } from "../../_design/payments/tenders";
import { groupByCharge } from "../../finance/allocations";
import type { Charge, DepositResult, Tender } from "../../finance/api";
import { formatCents } from "../../_lib/money";
import { ReceiptActions } from "../receipts/ReceiptActions";

const lineRow: CSSProperties = { display: "flex", justifyContent: "space-between", padding: "12px 16px", borderTop: "1px solid var(--border)" };

/**
 * What one allocation covered, in words. A surcharge created by this very
 * payment is not in the account read, so its description comes from the result
 * itself.
 */
export function allocationLabel(chargeId: string, surcharges: readonly Charge[], known: ReadonlyMap<string, string>): string {
  const fresh = surcharges.find((c) => c.id === chargeId);
  if (fresh !== undefined) {
    return fresh.description;
  }
  return known.get(chargeId) ?? "Cargo";
}

/** A breakdown is worth showing when there is more than one method, or a credit note that is not money. */
function showsTenders(tenders: readonly Tender[]): boolean {
  return tenders.length > 1 || tenders.some((t) => t.method === "credit_note");
}

/**
 * The deposit's id, shortened, for a deposit that reached the screen without a
 * folio (a gateway older than task 12). Every stored deposit has one.
 */
function idTail(id: string): string {
  return id.replace(/-/g, "").slice(-8).toUpperCase();
}

export interface DepositReceiptProps {
  result: DepositResult;
  /** Charge id → the concept's name, for the lines the money covered. */
  describe: ReadonlyMap<string, string>;
  onClose: () => void;
}

/**
 * The receipt of a deposit, the same at the counter and in Finanzas
 * (ISSUE-T14-003): a folio a parent can quote, a list they can check, and the
 * ticket and PDF a school hands over.
 */
export function DepositReceipt({ result, describe, onClose }: DepositReceiptProps): ReactElement {
  return (
    <div data-testid="receipt" style={{ flex: 1, display: "flex", flexDirection: "column", minHeight: 0 }}>
      <div style={{ flex: 1, overflowY: "auto", padding: "22px 26px", display: "flex", flexDirection: "column", gap: 18 }}>
        <div style={{ padding: "18px 20px", borderRadius: "var(--r-md)", background: "var(--ok-bg)", border: "1px solid var(--border)" }}>
          <span style={{ fontSize: 14, color: "var(--ok-fg)" }}>Cobrado</span>
          <div style={{ fontFamily: "var(--font-display)", fontWeight: 800, fontSize: 34, color: "var(--ok-fg)", marginTop: 4 }}>
            {formatCents(result.deposit.amountCents)}
          </div>
          <p style={{ fontSize: 13, color: "var(--text-muted)", marginTop: 8 }}>
            Folio {result.deposit.folio ?? idTail(result.deposit.id)}
            {result.replayed && " · ya estaba registrado, no se cobró de nuevo"}
          </p>
        </div>
        <div style={{ borderRadius: "var(--r-md)", border: "1px solid var(--border)", overflow: "hidden" }}>
          {groupByCharge(result.allocations).map((a) => (
            <div key={a.chargeId} style={lineRow}>
              <span style={{ fontSize: 14.5, color: "var(--text)" }}>{allocationLabel(a.chargeId, result.surcharges, describe)}</span>
              <span style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 15 }}>{formatCents(a.amountCents)}</span>
            </div>
          ))}
        </div>
        {showsTenders(result.deposit.tenders) && (
          // Split across methods, or settled with a credit note (T8): the
          // receipt says how much each covered — the ticket does the same.
          <div data-testid="receipt-tenders" style={{ borderRadius: "var(--r-md)", border: "1px solid var(--border)", overflow: "hidden" }}>
            <div style={{ padding: "10px 16px", background: "var(--surface-2)", fontSize: 13, fontWeight: 700, color: "var(--text-muted)", fontFamily: "var(--font-display)" }}>
              Formas de pago
            </div>
            {result.deposit.tenders.map((t, i) => (
              <div key={i} style={lineRow}>
                <span style={{ fontSize: 14.5, color: "var(--text)" }}>
                  {tenderLabel(t.method, t.cardType)}{t.reference !== undefined && ` · ref. ${t.reference}`}
                </span>
                <span style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 15 }}>{formatCents(t.amountCents)}</span>
              </div>
            ))}
          </div>
        )}
        {result.unallocatedCents > 0 && (
          <p style={{ fontSize: 14, color: "var(--text-muted)", margin: 0 }}>
            Quedan {formatCents(result.unallocatedCents)} a favor de la familia.
          </p>
        )}
      </div>
      <div style={{ display: "flex", gap: 12, alignItems: "center", padding: "18px 26px", borderTop: "1px solid var(--border)", background: "var(--surface-2)" }}>
        <ReceiptActions kind="payment" id={result.deposit.id} />
        <div style={{ flex: 1 }} />
        <Btn icon="check" type="button" onClick={onClose}>Listo</Btn>
      </div>
    </div>
  );
}
