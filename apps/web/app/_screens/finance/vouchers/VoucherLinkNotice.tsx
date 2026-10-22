"use client";

import type { ReactElement } from "react";
import { shortDayLabel } from "../../../_lib/localDate";
import { uploadedOn, voucherSummary } from "./labels";
import { NONE_OF_THEM, READ_FAILED, type VoucherChoice } from "./useVoucherChoice";

const box = { padding: "14px 16px", borderRadius: "var(--r-md)", background: "var(--warn-bg)", border: "1px solid var(--border)", display: "flex", flexDirection: "column" as const, gap: 10 };
const option = { display: "flex", alignItems: "center", gap: 10, minHeight: 44, cursor: "pointer", fontSize: 14.5, color: "var(--text)" };

/**
 * A payment by hand for a pupil whose family sent a voucher (ADR 0040 · B).
 * One: the server links it by itself, so the notice says so out loud and lets
 * the cashier say it is another payment. Two or more: the cashier must say
 * which — the server refuses to guess, and so does the counter.
 */
const LOCKED = "Se reintenta con la misma elección: el primer intento pudo haber quedado registrado.";

export function VoucherLinkNotice({ choice }: { choice: VoucherChoice }): ReactElement | null {
  const { pending, locked } = choice;
  const [only] = pending;
  if (choice.readFailed) {
    return <p role="note" style={{ ...box, margin: 0, fontSize: 14, color: "var(--warn-fg)" }}>{READ_FAILED}</p>;
  }
  const lockedNote = locked && <p style={{ margin: 0, fontSize: 13.5, color: "var(--warn-fg)" }}>{LOCKED}</p>;
  if (pending.length === 1 && only !== undefined) {
    return (
      <div role="note" style={box}>
        <p style={{ margin: 0, fontSize: 14.5, color: "var(--warn-fg)", fontWeight: 600 }}>
          {`Este pago se ligará al comprobante que subió la familia el ${shortDayLabel(uploadedOn(only))}`}
        </p>
        <label style={option}>
          <input type="checkbox" checked={choice.declined} disabled={locked} onChange={(e) => choice.setDeclined(e.target.checked)} style={{ width: 18, height: 18 }} />
          Es otro pago: no ligarlo al comprobante
        </label>
        {lockedNote}
      </div>
    );
  }
  if (pending.length < 2) return null;
  const invalid = choice.error !== undefined;
  return (
    <div role="radiogroup" aria-labelledby="voucher-choice-title" aria-describedby={invalid ? "voucher-choice-error" : undefined} style={box}>
      <p id="voucher-choice-title" style={{ margin: 0, fontSize: 14.5, color: "var(--warn-fg)", fontWeight: 700 }}>¿Cuál comprobante es este pago?</p>
      {pending.map((v, i) => (
        <label key={v.id} style={option}>
          <input type="radio" name="voucher-choice" value={v.id} checked={choice.picked === v.id} disabled={locked} onChange={() => choice.pick(v.id)}
            aria-invalid={invalid && i === 0 ? true : undefined} style={{ width: 18, height: 18 }} />
          {voucherSummary(v)}
        </label>
      ))}
      <label style={option}>
        <input type="radio" name="voucher-choice" value={NONE_OF_THEM} checked={choice.picked === NONE_OF_THEM} disabled={locked} onChange={() => choice.pick(NONE_OF_THEM)} style={{ width: 18, height: 18 }} />
        Ninguno: es otro pago
      </label>
      {lockedNote}
      {invalid && <p id="voucher-choice-error" role="alert" style={{ margin: 0, color: "var(--bad-fg)", fontSize: 14 }}>{choice.error}</p>}
    </div>
  );
}
