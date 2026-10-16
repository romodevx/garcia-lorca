"use client";

import { useEffect, useState, type ReactElement } from "react";
import { Icon } from "../../_design/Icon";
import { getCashUp, type CashUp, type Deposit } from "../../finance/api";
import { depositDay } from "../../finance/depositDay";
import { API_URL } from "../../api-url";
import { localISODate, shortDayLabel } from "../../_lib/localDate";
import { ticketPath } from "../../receipts/api";
import { formatCents } from "../../_lib/money";
import { tenderLabel, tenderSummary } from "../../_design/payments/tenders";
import { CashCutClosing, type CutAccess } from "./CashCutClosing";

export interface CashUpPanelProps {
  /** The day the money changed hands, "2026-10-10". Empty means today. */
  day: string;
  /** Opens the correction of a receipt (T9). Absent: the caller may not correct (payments:edit). */
  onCorrect?: (deposit: Deposit) => void;
  /** T10: expenses, expected cash and closing the day. Absent: module_expenses is off. */
  access?: CutAccess;
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
export function CashUpPanel({ day, onCorrect, access }: CashUpPanelProps): ReactElement {
  const [cut, setCut] = useState<CashUp | null>(null);
  const [failed, setFailed] = useState(false);
  // Bumped after closing or reopening: the cut is read again, as the server has it.
  const [reads, setReads] = useState(0);

  useEffect(() => {
    let live = true;
    getCashUp(API_URL, day)
      .then((c) => { if (live) { setCut(c); } })
      .catch(() => { if (live) { setFailed(true); } });
    return () => { live = false; };
  }, [day, reads]);

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
        {cut.creditNoteCents > 0 && (
          // A credit note settles debt but brings nothing into the drawer (T8):
          // it is shown apart, and the total above does not count it.
          <div data-testid="cash-up-credit-note" style={{ display: "flex", justifyContent: "space-between", gap: 12, marginTop: 16, paddingTop: 12, borderTop: "1px dashed var(--border)" }}>
            <span style={{ fontSize: 14, color: "var(--text-muted)" }}>Notas de crédito — no es ingreso</span>
            <span style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 16, color: "var(--text-muted)" }}>{formatCents(cut.creditNoteCents)}</span>
          </div>
        )}
      </div>

      {access?.expenses === true && (
        <CashCutClosing cut={cut} day={day} access={access} onChanged={() => setReads((n) => n + 1)} />
      )}

      {cut.deposits.length === 0 && cut.cancelled.length === 0 && cut.superseded.length === 0 && cut.payments.length === 0 && (
        <p style={{ color: "var(--text-muted)", fontSize: 15, margin: 0 }}>No se recibió nada este día.</p>
      )}

      {cut.deposits.length > 0 && (
        <div style={{ borderRadius: "var(--r-lg)", border: "1px solid var(--border)", background: "var(--surface)", overflow: "hidden" }}>
          {cut.deposits.map((d) => (
            <div key={d.id} data-testid="cash-up-row" style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12, padding: "13px 20px", borderTop: "1px solid var(--border)" }}>
              <span style={{ fontSize: 14, color: "var(--text-muted)" }}>
                {/* A back-dated payment carries a day, not an hour: none is shown. */}
                {capturedLate(d) ? "" : `${hourOf(d.paidAt)} · `}
                {tenderSummary(d.tenders)}
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
                {onCorrect !== undefined && (
                  // T9: the receipt is kept and a corrected one issued.
                  <button
                    type="button"
                    aria-label={`Corregir ${d.folio ?? "pago"}`}
                    title="Corregir pago"
                    onClick={() => onCorrect(d)}
                    style={{ minWidth: 44, minHeight: 44, display: "grid", placeItems: "center", borderRadius: "var(--r-md)", color: "var(--text-muted)" }}
                  >
                    <Icon name="edit" size={18} />
                  </button>
                )}
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
                {`${hourOf(p.paidAt)} · ${tenderLabel(p.method)} · ${p.description}`}
              </span>
              <span style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 15.5, color: "var(--text)" }}>{formatCents(p.amountCents)}</span>
            </div>
          ))}
        </div>
      )}

      {cut.superseded.length > 0 && (
        // T9: replaced by a correction. They count for nothing — the
        // correction counts on the day it is dated — and are shown so the
        // day's receipts still add up.
        <div style={{ borderRadius: "var(--r-lg)", border: "1px solid var(--border)", background: "var(--surface)", overflow: "hidden" }}>
          <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "10px 20px", background: "var(--surface-2)", fontSize: 13, fontWeight: 700, color: "var(--text-muted)", fontFamily: "var(--font-display)" }}>
            <Icon name="edit" size={16} />Sustituidos · no cuentan
          </div>
          {cut.superseded.map((d) => (
            <div key={d.id} style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12, padding: "13px 20px", borderTop: "1px solid var(--border)" }}>
              <span style={{ fontSize: 14, color: "var(--text-muted)" }}>
                {`${d.folio ?? "Pago"} · sustituido por ${d.supersededByFolio ?? "otro comprobante"}`}
              </span>
              <span style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 15.5, color: "var(--text-faint)", textDecoration: "line-through" }}>
                {formatCents(d.amountCents)}
              </span>
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
 * Keyed on a later day than the one it is dated. The deposit's day is its
 * school-local day (depositDay, ADR 0035); the moment it was keyed is read on
 * the school's clock too.
 */
function capturedLate(d: Deposit): boolean {
  if (d.capturedAt === undefined || d.capturedAt === "") return false;
  const keyed = new Date(d.capturedAt);
  const day = depositDay(d);
  if (Number.isNaN(keyed.getTime()) || day.length < 10) return false;
  return localISODate(keyed) > day;
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
