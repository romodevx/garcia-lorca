"use client";

import { useEffect, useRef, type ReactElement } from "react";

import { Btn } from "../../../_design/Controls";
import { Icon } from "../../../_design/Icon";
import type { GradingPeriod } from "../../../grades/periods";
import { dateRangeLabel, dayLabel, periodTitle } from "./format";
import { ProgressBar } from "./ProgressBar";

export type PeriodAction = "close" | "reopen" | "cards";

export interface PeriodCardProps {
  period: GradingPeriod;
  /** `grades:close`: only then are closing and reopening offered. */
  canClose: boolean;
  onAction: (action: PeriodAction, period: GradingPeriod) => void;
  /**
   * Bumped after this period was closed or reopened: the button that opened the
   * dialog is gone, so the focus comes to the card's heading instead of the body.
   */
  focusToken?: number;
}

export const NO_DATES_TEXT = "Sin fechas: ningún grado tiene este periodo";
export const NO_DATES_HINT = "Dale fechas en Configuración › Periodos por grado";

function StatusPill({ period }: { period: GradingPeriod }): ReactElement {
  const closed = period.status === "closed";
  return (
    <span style={{
      display: "inline-flex", alignItems: "center", gap: 6, padding: "4px 10px", borderRadius: "var(--r-pill)", fontSize: 13, fontWeight: 700,
      background: closed ? "var(--ok-bg)" : "var(--warn-bg)", color: closed ? "var(--ok-fg)" : "var(--warn-fg)",
    }}>
      {closed ? <Icon name="lock" size={14} /> : <span aria-hidden style={{ width: 7, height: 7, borderRadius: "50%", background: "currentColor" }} />}
      {closed ? `Cerrado v${period.version}` : "Abierto"}
    </span>
  );
}

/** One trimester of one level: where its capture stands and what the director can do with it. */
export function PeriodCard({ period, canClose, onAction, focusToken }: PeriodCardProps): ReactElement {
  const heading = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    if (focusToken !== undefined) heading.current?.focus();
  }, [focusToken]);

  const closed = period.status === "closed";
  // No grade of the level has this period yet: there is nothing to close.
  const dates = dateRangeLabel(period.startsOn, period.endsOn);
  const closedLine = closed && period.closedAt !== undefined
    ? `${dayLabel(period.closedAt)}${period.closedByName !== undefined ? ` · por ${period.closedByName}` : ""}`
    : undefined;
  // A card's actions repeat on every card: the accessible name carries which
  // level and period, after the visible words (label in name, CC-312).
  const title = periodTitle(period);
  return (
    <article
      aria-label={title}
      style={{ display: "grid", gap: 12, alignContent: "start", padding: "18px 20px", background: "var(--surface)", border: "1px solid var(--border)", borderRadius: "var(--r-lg)", boxShadow: "var(--shadow-card)" }}
    >
      <header style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
        <h3 ref={heading} tabIndex={-1} style={{ fontSize: 17, fontWeight: 700, color: "var(--text)", outlineOffset: 3 }}>{period.termName}</h3>
        <StatusPill period={period} />
      </header>
      {closedLine !== undefined && <p style={{ fontSize: 13, color: "var(--text-faint)" }}>{closedLine}</p>}
      <p style={{ fontSize: 14, color: "var(--text-muted)" }}>
        {dates ?? NO_DATES_TEXT}
      </p>
      <ProgressBar captured={period.progress.captured} expected={period.progress.expected} />
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        {canClose && !closed && dates === undefined && <p style={{ fontSize: 14, color: "var(--warn-fg)", fontWeight: 600 }}>{NO_DATES_HINT}</p>}
        {canClose && !closed && dates !== undefined && <Btn kind="forest" icon="lock" aria-label={`Revisar y cerrar ${title}`} onClick={() => onAction("close", period)} style={{ height: 44, fontSize: 15 }}>Revisar y cerrar</Btn>}
        {canClose && closed && <Btn kind="ghost" onClick={() => onAction("reopen", period)} style={{ height: 44, fontSize: 15 }}>Reabrir</Btn>}
        {(closed || period.version > 0) && <Btn kind="outline" icon="documents" aria-label={`Ver boletas ${title}`} onClick={() => onAction("cards", period)} style={{ height: 44, fontSize: 15 }}>Ver boletas</Btn>}
      </div>
    </article>
  );
}
