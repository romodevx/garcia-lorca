"use client";

import { useEffect, useState, type ReactElement } from "react";
import { Icon } from "../../_design/Icon";
import { getCashUp, type CashUp, type Deposit } from "../../finance/api";
import { API_URL } from "../../api-url";
import { localISODate, shortDayLabel } from "../../_lib/localDate";
import { ticketPath } from "../../receipts/api";
import { formatCents } from "../../_lib/money";

export interface CashUpPanelProps {
  /** The day the money changed hands, "2026-10-10". Empty means today. */
  day: string;
}

/**
 * The corte de caja: what came in today, and in what form.
 *
 * This is the artefact that gets the notebook put away. The office closes the
 * day on one number it did not have to add up, with the list behind it right
 * there for when a figure is disputed.
 *
 * Cancelled deposits are shown, counted as nothing. Hiding them would leave a
 * gap between the receipts and the drawer that nobody could account for.
 */
export function CashUpPanel({ day }: CashUpPanelProps): ReactElement {
  const [cut, setCut] = useState<CashUp | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let live = true;
    getCashUp(API_URL, day)
      .then((c) => { if (live) { setCut(c); } })
      .catch(() => { if (live) { setFailed(true); } });
    return () => { live = false; };
  }, [day]);

  if (failed) {
    return <p style={{ color: "var(--text-muted)", fontSize: 15 }}>No se pudo leer el corte del día.</p>;
  }
  if (cut === null) {
    return <p style={{ color: "var(--text-muted)", fontSize: 15 }}>Cargando…</p>;
  }

  const methods: { key: string; label: string; cents: number }[] = [
    { key: "cash", label: "Efectivo", cents: cut.cashCents },
    { key: "card", label: "Tarjeta", cents: cut.cardCents },
    { key: "transfer", label: "Transferencia", cents: cut.transferCents },
  ];

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 18 }}>
      <div style={{ padding: "20px 24px", borderRadius: "var(--r-lg)", background: "var(--surface)", border: "1px solid var(--border)", boxShadow: "var(--shadow-card)" }}>
        <span style={{ fontSize: 14, color: "var(--text-muted)" }}>{day === "" ? "Recibido hoy" : `Recibido el ${shortDayLabel(day)}`}</span>
        <div data-testid="cash-up-total" style={{ fontFamily: "var(--font-display)", fontWeight: 800, fontSize: 40, color: "var(--text)", marginTop: 4 }}>
          {formatCents(cut.totalCents)}
        </div>
        <div style={{ display: "flex", gap: 22, flexWrap: "wrap", marginTop: 14 }}>
          {methods.map((m) => (
            <div key={m.key}>
              <div style={{ fontSize: 12.5, color: "var(--text-faint)", fontWeight: 600, textTransform: "uppercase", letterSpacing: ".04em" }}>{m.label}</div>
              <div data-testid={`cash-up-${m.key}`} style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 19, color: "var(--text)" }}>
                {formatCents(m.cents)}
              </div>
            </div>
          ))}
        </div>
      </div>

      {cut.deposits.length === 0 && cut.cancelled.length === 0 && cut.payments.length === 0 && (
        <p style={{ color: "var(--text-muted)", fontSize: 15, margin: 0 }}>No se recibió nada este día.</p>
      )}

      {cut.deposits.length > 0 && (
        <div style={{ borderRadius: "var(--r-lg)", border: "1px solid var(--border)", background: "var(--surface)", overflow: "hidden" }}>
          {cut.deposits.map((d) => (
            <div key={d.id} data-testid="cash-up-row" style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12, padding: "13px 20px", borderTop: "1px solid var(--border)" }}>
              <span style={{ fontSize: 14, color: "var(--text-muted)" }}>
                {/* A back-dated payment carries a day, not an hour: none is shown. */}
                {capturedLate(d) ? "" : `${hourOf(d.paidAt)} · `}
                {methodLabel(d.method)}
                {d.receivedByName !== undefined && d.receivedByName !== "" && ` · ${d.receivedByName}`}
                {d.folio !== undefined && d.folio !== "" && ` · ${d.folio}`}
                {capturedLate(d) && (
                  // A payment dated the day the family paid but keyed later:
                  // the counterweight to back-dating (task 7).
                  <span style={{ marginLeft: 8, padding: "2px 8px", borderRadius: 99, background: "var(--warn-bg)", color: "var(--warn-fg)", fontSize: 12, fontWeight: 700 }}>
                    capturado el {dayLabel(d.capturedAt ?? "")}
                  </span>
                )}
              </span>
              <span style={{ display: "flex", alignItems: "center", gap: 10 }}>
                <span style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 15.5, color: "var(--text)" }}>{formatCents(d.amountCents)}</span>
                {/* Task 12: the ticket, printed again on its own 80 mm page. */}
                <button
                  type="button"
                  aria-label={`Reimprimir ${d.folio ?? "comprobante"}`}
                  title="Reimprimir comprobante"
                  onClick={() => window.open(ticketPath("payment", d.id), "_blank", "noopener")}
                  style={{ minWidth: 44, minHeight: 44, display: "grid", placeItems: "center", borderRadius: "var(--r-md)", color: "var(--text-muted)" }}
                >
                  <Icon name="documents" size={18} />
                </button>
              </span>
            </div>
          ))}
        </div>
      )}

      {cut.payments.length > 0 && (
        // Taken outside a deposit: a counter sale, paid straight against its
        // charge. It is in the drawer, so it is in the total and listed here.
        <div style={{ borderRadius: "var(--r-lg)", border: "1px solid var(--border)", background: "var(--surface)", overflow: "hidden" }}>
          {cut.payments.map((p) => (
            <div key={p.id} data-testid="cash-up-row" style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12, padding: "13px 20px", borderTop: "1px solid var(--border)" }}>
              <span style={{ fontSize: 14, color: "var(--text-muted)" }}>
                {`${hourOf(p.paidAt)} · ${methodLabel(p.method)} · ${p.description}`}
              </span>
              <span style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 15.5, color: "var(--text)" }}>{formatCents(p.amountCents)}</span>
            </div>
          ))}
        </div>
      )}

      {cut.cancelled.length > 0 && (
        <div style={{ borderRadius: "var(--r-lg)", border: "1px solid var(--border)", background: "var(--surface)", overflow: "hidden" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "10px 20px", background: "var(--surface-2)", fontSize: 13, fontWeight: 700, color: "var(--text-muted)", fontFamily: "var(--font-display)" }}>
            <Icon name="warning" size={16} />Cancelados · no cuentan
          </div>
          {cut.cancelled.map((d) => (
            <div key={d.id} style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12, padding: "13px 20px", borderTop: "1px solid var(--border)" }}>
              <span style={{ fontSize: 14, color: "var(--text-muted)" }}>{capturedLate(d) ? "" : `${hourOf(d.paidAt)} · `}{d.voidReason ?? "sin motivo"}</span>
              <span style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 15.5, color: "var(--text-faint)", textDecoration: "line-through" }}>
                {formatCents(d.amountCents)}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

/**
 * Keyed on a later day than the one it is dated. A back-dated payment carries
 * a calendar date, stored as midnight UTC, so its day is read as written
 * (UTC); the moment it was keyed is read on the office's clock.
 */
function capturedLate(d: Deposit): boolean {
  if (d.capturedAt === undefined || d.capturedAt === "") return false;
  const keyed = new Date(d.capturedAt);
  if (Number.isNaN(keyed.getTime()) || d.paidAt.length < 10) return false;
  return localISODate(keyed) > d.paidAt.slice(0, 10);
}

/** "13 oct": the day a late capture was keyed. */
function dayLabel(iso: string): string {
  const at = new Date(iso);
  return Number.isNaN(at.getTime()) ? "—" : at.toLocaleDateString("es-MX", { day: "numeric", month: "short" });
}

/** The hour a deposit was taken, for a person scanning the day. */
function hourOf(iso: string): string {
  const at = new Date(iso);
  return Number.isNaN(at.getTime()) ? "—" : at.toLocaleTimeString("es-MX", { hour: "2-digit", minute: "2-digit" });
}

/** How the money arrived, as the office says it. */
function methodLabel(method: string): string {
  return method === "cash" ? "Efectivo" : method === "card" ? "Tarjeta" : "Transferencia";
}
