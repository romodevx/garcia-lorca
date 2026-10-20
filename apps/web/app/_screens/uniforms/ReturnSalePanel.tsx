"use client";

import { useState, type FormEvent, type ReactElement } from "react";

import { API_URL } from "../../api-url";
import { Btn } from "../../_design/Controls";
import { Icon } from "../../_design/Icon";
import { TextField } from "../../_design/form";
import { returnCounterSale, UniformsError, type ReturnSale, type UniformOrder } from "../../uniforms/api";
import { money } from "./parts";

/** The school's day, whatever the browser's zone (ADR 0035). */
const SCHOOL_ZONE = "America/Mexico_City";
const schoolDay = new Intl.DateTimeFormat("en-CA", { timeZone: SCHOOL_ZONE, year: "numeric", month: "2-digit", day: "2-digit" });

/**
 * Whether a sale made at `soldAt` is of the school's today. At 19:00 in Morelia
 * the UTC date is already tomorrow, so the comparison is in the school's zone.
 * The server decides for real; this only chooses what the panel asks.
 */
export function soldOnSchoolDay(soldAt: string | undefined, now: Date): boolean {
  if (soldAt === undefined) return true;
  const at = new Date(soldAt);
  if (Number.isNaN(at.getTime())) return true;
  return schoolDay.format(at) === schoolDay.format(now);
}

type RefundMethod = NonNullable<ReturnSale["refundMethod"]>;
const METHODS: readonly { value: RefundMethod; label: string }[] = [
  { value: "cash", label: "Efectivo" },
  { value: "card", label: "Tarjeta" },
  { value: "transfer", label: "Transferencia" },
];
const MISSING_REASON = "Escribe el motivo de la devolución.";
const FAILED = "No se pudo devolver la venta. Intenta de nuevo.";

export interface ReturnSalePanelProps {
  order: UniformOrder;
  /** Injected for tests; the real clock otherwise. */
  now?: () => Date;
  /** The order as the server left it, and what to tell the person. */
  onDone: (updated: UniformOrder, message: string) => void;
  onClose: () => void;
}

/**
 * Returning a counter sale, whole (C2-015). A reason, which garments came back
 * damaged and — only when the sale is of an earlier day — how the money goes
 * back: at most four decisions, so an inline panel under the row, not a drawer
 * (house rule). The day of the sale its charge is cancelled; another day the
 * refund is an expense of today, and the sale's own cash-up is left alone.
 */
export function ReturnSalePanel({ order, now = () => new Date(), onDone, onClose }: ReturnSalePanelProps): ReactElement {
  const sameDay = soldOnSchoolDay(order.createdAt, now());
  const [reason, setReason] = useState("");
  const [reasonError, setReasonError] = useState<string | undefined>(undefined);
  const [damaged, setDamaged] = useState<ReadonlySet<string>>(new Set());
  const [method, setMethod] = useState<RefundMethod>("cash");
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const [saving, setSaving] = useState(false);
  const sold = order.lines.filter((l) => l.status === "sold");

  const toggle = (id: string): void => {
    setDamaged((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const submit = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    if (saving) return;
    if (reason.trim() === "") {
      setReasonError(MISSING_REASON);
      return;
    }
    setSaving(true);
    setProblem(undefined);
    const body: ReturnSale = { reason: reason.trim(), damagedLineIds: sold.filter((l) => damaged.has(l.id)).map((l) => l.id) };
    if (!sameDay) body.refundMethod = method;
    try {
      onDone(await returnCounterSale(API_URL, order.id, body), `Venta ${order.folio} devuelta.`);
    } catch (cause) {
      setProblem(cause instanceof UniformsError ? cause.message : FAILED);
    } finally {
      setSaving(false);
    }
  };

  return (
    <form onSubmit={(e) => void submit(e)} aria-label={`Devolver la venta ${order.folio}`} style={{ margin: "4px 8px 10px", padding: "16px 18px", display: "grid", gap: 14, borderRadius: "var(--r-md)", background: "var(--surface-2)", border: "1px solid var(--border)" }}>
      <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
        <span style={{ flex: 1, fontSize: 15, fontWeight: 700 }}>Devolver {order.folio} · {money(order.totalCents)}</span>
        <button type="button" aria-label="Cerrar la devolución" onClick={onClose} style={{ minWidth: 44, minHeight: 44, display: "grid", placeItems: "center", border: "none", background: "none", color: "var(--text-muted)", cursor: "pointer" }}>
          <Icon name="close" size={18} />
        </button>
      </div>

      <p style={{ margin: 0, display: "flex", gap: 8, fontSize: 14.5, color: "var(--text-muted)" }}>
        <Icon name="warning" size={18} />
        {sameDay
          ? "Es de hoy: se cancela el cobro y el corte de hoy deja de contarla. Regresa el dinero a la familia."
          : "Es de otro día: su corte no se toca y el reembolso queda como egreso de hoy."}
      </p>

      <fieldset style={{ border: "none", margin: 0, padding: 0, display: "grid", gap: 8 }}>
        <legend style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)", marginBottom: 4 }}>Prendas que vuelven (marca las que vienen dañadas: no se vuelven a vender)</legend>
        {sold.map((l) => (
          <label key={l.id} style={{ display: "flex", alignItems: "center", gap: 10, minHeight: 44, fontSize: 14.5 }}>
            <input type="checkbox" checked={damaged.has(l.id)} onChange={() => toggle(l.id)} aria-label={`${l.itemName} · talla ${l.sizeLabel} × ${l.qty} viene dañada`} />
            {l.itemName} · talla {l.sizeLabel} × {l.qty}
            {damaged.has(l.id) && <span style={{ color: "var(--bad-fg)", fontWeight: 600 }}>· merma</span>}
          </label>
        ))}
      </fieldset>

      {!sameDay && (
        <div role="radiogroup" aria-label="Cómo se regresa el dinero" style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
          {METHODS.map((m) => (
            <label key={m.value} style={{ display: "inline-flex", alignItems: "center", gap: 6, minHeight: 44, padding: "0 14px", borderRadius: 99, cursor: "pointer", fontWeight: 600, fontSize: 14, background: method === m.value ? "var(--surface)" : "transparent", border: "1px solid var(--border)" }}>
              <input type="radio" name={`refund-${order.id}`} value={m.value} checked={method === m.value} onChange={() => setMethod(m.value)} />
              {m.label}
            </label>
          ))}
        </div>
      )}

      <TextField
        label="Motivo"
        required
        value={reason}
        onChange={(v) => {
          setReason(v);
          if (v.trim() !== "") setReasonError(undefined);
        }}
        onClearError={() => setReasonError(undefined)}
        error={reasonError}
        help="Queda en el historial de la venta y en el comprobante."
      />
      {problem !== undefined && <p role="alert" style={{ margin: 0, color: "var(--bad-fg)", fontWeight: 600 }}>{problem}</p>}
      <div style={{ display: "flex", justifyContent: "flex-end", gap: 12 }}>
        <Btn kind="ghost" onClick={onClose}>Volver</Btn>
        <Btn type="submit" busy={saving} danger icon="back">Devolver la venta</Btn>
      </div>
    </form>
  );
}
