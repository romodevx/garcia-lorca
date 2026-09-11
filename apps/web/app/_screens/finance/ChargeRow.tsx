"use client";

import { useState, type ReactElement } from "react";
import { useTranslations } from "next-intl";
import { Icon } from "../../_design/Icon";
import type { Charge } from "../../finance/api";
import { ChargeActions } from "./ChargeActions";
import { formatCents } from "../../_lib/money";
import { StatusPill } from "./parts";

/** Grid shared by the charge table's header and its rows. */
export const CHARGE_GRID = "2.1fr 1.4fr 1fr 1fr 1fr 1.05fr auto";

/** Width of the trailing action cell, shared with the table header. */
export const ACTION_COL_WIDTH = 216;

export interface ChargeRowProps {
  charge: Charge;
  studentName: string;
  /** Opens the window (ventanilla) form with this charge preselected. */
  onPay: (charge: Charge) => void;
  onDiscount: (chargeId: string, discountCents: number, reason: string) => Promise<void>;
  onCancel: (chargeId: string) => Promise<void>;
}

/**
 * One charge in the debts table: what is owed, what was paid, what is left, and
 * its actions. Amounts arrive in integer cents and are formatted to MXN here.
 *
 * A charge with nothing left to collect is `settled`: it shows its own status
 * (never a blanket "Pagado" — a CANCELLED charge was not paid) and offers no
 * actions at all. Everything else offers the primary action, collect, plus the
 * secondary discount/cancel pair behind a disclosure.
 */
export function ChargeRow({ charge, studentName, onPay, onDiscount, onCancel }: ChargeRowProps): ReactElement {
  const t = useTranslations("finance");
  const [hover, setHover] = useState(false);
  const [actionsOpen, setActionsOpen] = useState(false);
  const settled = charge.remainingCents <= 0 || charge.status === "cancelled";
  const cancelled = charge.status === "cancelled";

  return (
    <div onMouseEnter={() => setHover(true)} onMouseLeave={() => setHover(false)}>
      <div
        style={{
          display: "grid",
          gridTemplateColumns: CHARGE_GRID,
          alignItems: "center",
          gap: 16,
          padding: "14px 20px",
          borderRadius: "var(--r-md)",
          background: hover ? "var(--surface-2)" : "transparent",
          transition: "background .15s var(--ease)",
        }}
      >
        <div style={{ minWidth: 0 }}>
          <div style={{ fontWeight: 600, fontSize: 15.5, color: "var(--text)", overflow: "hidden", textOverflow: "ellipsis" }}>
            {charge.description}
          </div>
          <div style={{ marginTop: 4 }}>
            <StatusPill status={charge.status} label={t(`status.${charge.status}`)} />
          </div>
        </div>
        <div style={{ fontSize: 14, color: "var(--text-muted)", overflow: "hidden", textOverflow: "ellipsis" }}>{studentName}</div>
        <div style={{ fontSize: 14.5, color: "var(--text-muted)" }}>{formatCents(charge.amountCents)}</div>
        <div style={{ fontSize: 14.5, color: "var(--text-muted)" }}>{formatCents(charge.paidCents)}</div>
        <div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 16, color: "var(--text)" }}>
          {formatCents(charge.remainingCents)}
        </div>
        <div style={{ fontSize: 14, color: settled ? "var(--text-muted)" : "var(--bad-fg)", fontWeight: settled ? 400 : 600 }}>
          {charge.dueDate}
        </div>
        <div style={{ display: "flex", justifyContent: "flex-end", alignItems: "center", gap: 8, width: ACTION_COL_WIDTH }}>
          {settled ? (
            <span
              style={{
                display: "inline-flex",
                alignItems: "center",
                gap: 6,
                color: cancelled ? "var(--text-muted)" : "var(--forest)",
                fontSize: 13.5,
                fontWeight: 600,
              }}
            >
              {!cancelled && <Icon name="check" size={18} stroke={2.5} />}
              {t(`status.${charge.status}`)}
            </span>
          ) : (
            <>
              <button
                onClick={() => onPay(charge)}
                aria-label={t("action.payFor", { description: charge.description })}
                style={{
                  display: "inline-flex",
                  alignItems: "center",
                  gap: 7,
                  height: 40,
                  padding: "0 16px",
                  borderRadius: 99,
                  background: "var(--primary)",
                  color: "var(--primary-ink)",
                  fontFamily: "var(--font-display)",
                  fontWeight: 600,
                  fontSize: 14,
                  boxShadow: "0 3px 10px color-mix(in oklab, var(--lime) 36%, transparent)",
                }}
              >
                <Icon name="card" size={17} />
                {t("action.pay")}
              </button>
              <button
                onClick={() => setActionsOpen((open) => !open)}
                aria-label={t("action.more", { description: charge.description })}
                aria-expanded={actionsOpen}
                style={{
                  width: 40,
                  height: 40,
                  flexShrink: 0,
                  display: "grid",
                  placeItems: "center",
                  borderRadius: 99,
                  background: actionsOpen ? "var(--surface-3)" : "transparent",
                  border: "1px solid var(--border)",
                  color: "var(--text-muted)",
                }}
              >
                <Icon name="more" size={18} />
              </button>
            </>
          )}
        </div>
      </div>
      {actionsOpen && !settled && <ChargeActions charge={charge} onDiscount={onDiscount} onCancel={onCancel} />}
    </div>
  );
}
