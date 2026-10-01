import type { CSSProperties, ReactElement } from "react";

import { formatCents } from "../../_lib/money";
import type { Receipt } from "../../receipts/api";

/*
 * Black on white whatever the theme: this is what a thermal printer burns onto
 * an 80 mm roll (72 mm printable), not a screen surface.
 */
const ink = "#000";
const paper = "#fff";

const ticket: CSSProperties = {
  width: "72mm",
  margin: "0 auto",
  padding: "4mm 0",
  background: paper,
  color: ink,
  fontFamily: "var(--font-ui), system-ui, sans-serif",
  fontSize: "11px",
  lineHeight: 1.35,
};
const center: CSSProperties = { textAlign: "center" };
const rule: CSSProperties = { border: 0, borderTop: `1px dashed ${ink}`, margin: "6px 0" };
const row: CSSProperties = { display: "flex", justifyContent: "space-between", gap: 8 };

/**
 * The 80 mm ticket of a receipt (task 12): what the school hands over at the
 * counter. The browser prints it (`@page { size: 80mm auto }` on its page).
 */
export function ReceiptTicket({ receipt: r }: { receipt: Receipt }): ReactElement {
  const s = r.school;
  return (
    <article style={ticket} aria-label={`Comprobante ${r.folio}`}>
      <header style={center}>
        {s.name !== "" && <p style={{ fontWeight: 700, fontSize: "13px" }}>{s.name}</p>}
        {s.legalName !== "" && <p>{s.legalName}</p>}
        {(s.rfc !== "" || s.cct !== "") && <p>{[s.rfc !== "" ? `RFC ${s.rfc}` : "", s.cct !== "" ? `CCT ${s.cct}` : ""].filter(Boolean).join(" · ")}</p>}
        {s.address !== "" && <p>{s.address}</p>}
        {s.phone !== "" && <p>Tel. {s.phone}</p>}
      </header>
      <hr style={rule} />
      <p style={{ ...center, fontWeight: 700 }}>{r.kind === "sale" ? "Comprobante de venta" : "Comprobante de pago"}</p>
      <p style={{ ...center, fontWeight: 700, fontSize: "13px" }}>{r.folio}</p>
      {r.voided === true && (
        <>
          <p style={{ ...center, fontWeight: 800, fontSize: "14px" }}>CANCELADO</p>
          {r.voidReason !== undefined && r.voidReason !== "" && <p style={center}>Motivo: {r.voidReason}</p>}
        </>
      )}
      <hr style={rule} />
      <p>Fecha de pago: {r.paidOn}</p>
      <p>Alumno: {[r.buyer, r.buyerDetail].filter((x) => x !== undefined && x !== "").join(" · ")}</p>
      <hr style={rule} />
      {r.lines.map((l, i) => (
        <div key={i} style={{ marginBottom: 3 }}>
          <div style={row}>
            <span>{l.description}</span>
            <span style={{ whiteSpace: "nowrap" }}>{formatCents(l.amountCents)}</span>
          </div>
          {l.discountCents !== undefined && l.discountCents > 0 && <p style={{ fontSize: "10px" }}>incluye descuento de {formatCents(l.discountCents)}</p>}
        </div>
      ))}
      <hr style={rule} />
      {r.surchargeCents > 0 && <Total label="Recargos incluidos" cents={r.surchargeCents} />}
      {r.discountCents > 0 && <Total label="Descuentos aplicados" cents={r.discountCents} />}
      {r.creditCents > 0 && <Total label="Saldo a favor" cents={r.creditCents} />}
      <div style={{ ...row, fontWeight: 800, fontSize: "14px", marginTop: 4 }}>
        <span>Total</span>
        <span data-testid="ticket-total">{formatCents(r.totalCents)}</span>
      </div>
      <hr style={rule} />
      {byTender(r) ? (
        // Split across methods, or a credit note (T8): one row per tender.
        <>
          <p style={{ fontWeight: 700 }}>Forma de pago</p>
          {(r.tenders ?? []).map((t, i) => (
            <div key={i} data-testid="ticket-tender" style={row}>
              <span>{[t.method, t.reference !== undefined && t.reference !== "" ? `ref. ${t.reference}` : ""].filter(Boolean).join(" · ")}</span>
              <span style={{ whiteSpace: "nowrap" }}>{formatCents(t.amountCents)}</span>
            </div>
          ))}
        </>
      ) : (
        r.method !== "" && <p>Forma de pago: {[r.method, r.reference !== undefined && r.reference !== "" ? `ref. ${r.reference}` : ""].filter(Boolean).join(" · ")}</p>
      )}
      {r.receivedBy !== undefined && r.receivedBy !== "" && <p>Recibió: {r.receivedBy}</p>}
      {s.legend !== "" && <p style={{ ...center, marginTop: 8 }}>{s.legend}</p>}
    </article>
  );
}

/** The gateway labels each tender already; a credit note is never folded into one line. */
function byTender(r: Receipt): boolean {
  const tenders = r.tenders ?? [];
  return tenders.length > 1 || tenders.some((t) => t.code === "credit_note");
}

function Total({ label, cents }: { label: string; cents: number }): ReactElement {
  return (
    <div style={row}>
      <span>{label}</span>
      <span>{formatCents(cents)}</span>
    </div>
  );
}
