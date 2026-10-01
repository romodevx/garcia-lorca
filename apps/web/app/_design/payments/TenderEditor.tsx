"use client";

import type { CSSProperties, ReactElement } from "react";

import type { CardType, PaymentMethod } from "../../finance/api";
import { formatCents } from "../../_lib/money";
import { Btn } from "../Controls";
import { Icon } from "../Icon";
import { MoneyField, TextField } from "../form";
import { MAX_TENDERS, TENDER_METHODS, coveredCents, remainingLabel, tenderLabel, type TenderRow } from "./tenders";
import type { TendersState } from "./useTenders";

/** What the (?) of Referencia explains, per method: a credit note has a folio, a card a voucher. */
const REFERENCE_HELP: Record<PaymentMethod, string> = {
  cash: "",
  card: "El folio del voucher: lo que te permita reconocer este pago después. Puede quedar vacío.",
  transfer: "El número de la transferencia: lo que te permita reconocer este pago después. Puede quedar vacío.",
  credit_note: "El folio de la nota de crédito, si lo tiene. Puede quedar vacío.",
};

const CARD_TYPES: readonly [CardType, string][] = [["debit", "Débito"], ["credit", "Crédito"]];

/** A one-tap choice, the counter's pill. */
function chip(on: boolean): CSSProperties {
  return {
    minHeight: 44, padding: "0 16px", borderRadius: 99, fontSize: 14.5, fontWeight: 600,
    fontFamily: "var(--font-display)", cursor: "pointer",
    border: `1.5px solid ${on ? "var(--lime)" : "var(--border)"}`,
    background: on ? "color-mix(in oklab, var(--lime) 12%, var(--surface))" : "var(--surface)",
    color: on ? "var(--forest)" : "var(--text-muted)",
  };
}

/**
 * How a payment is paid (T8): one row per method, a credit note among them.
 *
 * The school does not automate a family's credit: the cashier picks "Nota de
 * crédito" and types what it covers, then completes the rest with another
 * method. The bar under the rows says, all the time, what is covered and what
 * is left — the arithmetic a person would otherwise do on the receipt pad.
 */
export function TenderEditor({ tenders }: { tenders: TendersState }): ReactElement {
  const { rows } = tenders;
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
      {rows.map((row, i) => (
        <TenderRowEditor key={row.key} row={row} n={i + 1} removable={rows.length > 1} tenders={tenders} />
      ))}
      {rows.length < MAX_TENDERS && (
        <div>
          <Btn kind="ghost" icon="plus" onClick={tenders.add} style={{ height: 44, padding: "0 14px", fontSize: 14.5 }}>
            Agregar método
          </Btn>
        </div>
      )}
      <CoverageBar totalCents={tenders.totalCents} covered={coveredCents(rows)} />
    </div>
  );
}

function TenderRowEditor({ row, n, removable, tenders }: { row: TenderRow; n: number; removable: boolean; tenders: TendersState }): ReactElement {
  const name = `Forma de pago ${n}`;
  return (
    <div
      role="group"
      aria-label={name}
      style={{ display: "flex", flexDirection: "column", gap: 12, padding: "14px 16px", borderRadius: "var(--r-md)", border: "1px solid var(--border)", background: "var(--surface)" }}
    >
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8 }}>
        <span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)" }}>{name}</span>
        {removable && (
          <button
            type="button"
            aria-label={`Quitar forma de pago ${n}`}
            onClick={() => tenders.remove(row.key)}
            style={{ minWidth: 44, minHeight: 44, display: "grid", placeItems: "center", borderRadius: "var(--r-md)", color: "var(--text-faint)" }}
          >
            <Icon name="close" size={18} />
          </button>
        )}
      </div>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        {TENDER_METHODS.map((method) => (
          <button key={method} type="button" aria-pressed={row.method === method} onClick={() => tenders.setMethod(row.key, method)} style={chip(row.method === method)}>
            {tenderLabel(method)}
          </button>
        ))}
      </div>
      {row.method === "card" && (
        <div role="group" aria-label="Tipo de tarjeta" style={{ display: "flex", gap: 8 }}>
          {CARD_TYPES.map(([value, label]) => (
            <button key={value} type="button" aria-pressed={row.cardType === value} onClick={() => tenders.setCardType(row.key, value)} style={chip(row.cardType === value)}>
              {label}
            </button>
          ))}
        </div>
      )}
      <MoneyField
        label="Monto"
        value={row.amount}
        onChange={(v) => tenders.setAmount(row.key, v)}
        onBlur={() => tenders.blurAmount(row.key)}
        error={tenders.errors[row.key]}
        required
      />
      {row.method !== "cash" && (
        <TextField
          label="Referencia"
          value={row.reference}
          onChange={(v) => tenders.setReference(row.key, v)}
          help={REFERENCE_HELP[row.method]}
        />
      )}
    </div>
  );
}

/** Total a pagar · Cubierto · Restante, visible the whole time the rows are edited. */
function CoverageBar({ totalCents, covered }: { totalCents: number; covered: number }): ReactElement {
  const total = Number.isFinite(totalCents) ? totalCents : 0;
  const settled = covered === total;
  const figure = (label: string, testId: string, value: string): ReactElement => (
    <div style={{ minWidth: 0 }}>
      <div style={{ fontSize: 12.5, color: "var(--text-faint)", fontWeight: 600 }}>{label}</div>
      <div data-testid={testId} style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 16.5, color: "var(--text)" }}>{value}</div>
    </div>
  );
  return (
    <div
      aria-live="polite"
      style={{ display: "flex", gap: 18, flexWrap: "wrap", justifyContent: "space-between", padding: "12px 16px", borderRadius: "var(--r-md)", background: settled ? "var(--ok-bg)" : "var(--warn-bg)" }}
    >
      {figure("Total a pagar", "tender-total", formatCents(total))}
      {figure("Cubierto", "tender-covered", formatCents(covered))}
      {figure("Restante", "tender-remaining", remainingLabel(total, covered))}
    </div>
  );
}
