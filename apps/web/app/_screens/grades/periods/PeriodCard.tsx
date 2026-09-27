"use client";

import type { ReactElement } from "react";

import { Btn } from "../../../_design/Controls";
import { Icon } from "../../../_design/Icon";
import type { GradingPeriod } from "../../../grades/periods";
import { dayLabel, monthsLabel, periodTitle } from "./format";
import { ProgressBar } from "./ProgressBar";

export type PeriodAction = "close" | "reopen" | "cards";

export interface PeriodCardProps {
  period: GradingPeriod;
  /** `grades:close`: only then are closing and reopening offered. */
  canClose: boolean;
  onAction: (action: PeriodAction, period: GradingPeriod) => void;
}

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
export function PeriodCard({ period, canClose, onAction }: PeriodCardProps): ReactElement {
  const closed = period.status === "closed";
  const closedLine = closed && period.closedAt !== undefined
    ? `${dayLabel(period.closedAt)}${period.closedByName !== undefined ? ` · por ${period.closedByName}` : ""}`
    : undefined;
  return (
    <article
      aria-label={periodTitle(period)}
      style={{ display: "grid", gap: 12, alignContent: "start", padding: "18px 20px", background: "var(--surface)", border: "1px solid var(--border)", borderRadius: "var(--r-lg)", boxShadow: "var(--shadow-card)" }}
    >
      <header style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
        <h3 style={{ fontSize: 17, fontWeight: 700, color: "var(--text)" }}>{period.termName}</h3>
        <StatusPill period={period} />
      </header>
      {closedLine !== undefined && <p style={{ fontSize: 13, color: "var(--text-faint)" }}>{closedLine}</p>}
      <p style={{ fontSize: 14, color: "var(--text-muted)" }}>
        {period.months.length > 0 ? monthsLabel(period.months) : "Sin meses asignados"}
      </p>
      <ProgressBar captured={period.progress.captured} expected={period.progress.expected} />
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        {canClose && !closed && <Btn kind="forest" icon="lock" onClick={() => onAction("close", period)} style={{ height: 44, fontSize: 15 }}>Revisar y cerrar</Btn>}
        {canClose && closed && <Btn kind="ghost" onClick={() => onAction("reopen", period)} style={{ height: 44, fontSize: 15 }}>Reabrir</Btn>}
        {(closed || period.version > 0) && <Btn kind="outline" icon="documents" onClick={() => onAction("cards", period)} style={{ height: 44, fontSize: 15 }}>Ver boletas</Btn>}
      </div>
    </article>
  );
}
