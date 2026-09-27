"use client";

import { type ReactElement } from "react";

import { Icon, type IconName } from "../../_design/Icon";
import type { SheetStats } from "../../grades/monthly";

export interface SheetCountersProps {
  stats: SheetStats;
  /** Saves still in flight. */
  pending: number;
}

interface Counter {
  icon: IconName;
  color: string;
  label: string;
  value: string;
  /** Text colour of the number: the failing count turns red, with its label saying why. */
  tone?: string;
}

const AVERAGE_DECIMALS = 1;

function pendingText(pending: number): string {
  if (pending === 0) return "Todo guardado";
  return pending === 1 ? "Guardando 1 calificación…" : `Guardando ${pending} calificaciones…`;
}

/** The month at a glance: how many are in, their average, and how many fail. */
export function SheetCounters({ stats, pending }: SheetCountersProps): ReactElement {
  const counters: Counter[] = [
    { icon: "check", color: "lime", label: "Capturadas", value: `${stats.captured} de ${stats.total}` },
    { icon: "star", color: "purple", label: "Promedio", value: stats.average === undefined ? "—" : stats.average.toFixed(AVERAGE_DECIMALS) },
    { icon: "warning", color: "red", label: "Reprobados", value: String(stats.failing), tone: stats.failing > 0 ? "var(--bad-fg)" : undefined },
  ];
  return (
    <div style={{ marginBottom: 14 }}>
      <div role="group" aria-label="Resumen del mes" style={{ display: "flex", flexWrap: "wrap", gap: 10 }}>
        {counters.map((c) => (
          <div key={c.label} style={{ flex: "1 1 140px", minWidth: 0, display: "flex", alignItems: "center", gap: 12, padding: "12px 16px", borderRadius: "var(--r-lg)", background: "var(--surface)", border: "1px solid var(--border)", boxShadow: "var(--shadow-card)" }}>
            <div aria-hidden style={{ width: 38, height: 38, borderRadius: 12, flexShrink: 0, display: "grid", placeItems: "center", background: `color-mix(in oklab, var(--${c.color}) 16%, var(--surface))`, color: `var(--${c.color})` }}>
              <Icon name={c.icon} size={20} />
            </div>
            <div>
              <div style={{ fontSize: 13, color: "var(--text-muted)", fontWeight: 600 }}>{c.label}</div>
              <div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 20, lineHeight: 1.1, color: c.tone ?? "var(--text)" }}>{c.value}</div>
            </div>
          </div>
        ))}
      </div>
      <p role="status" style={{ margin: "8px 2px 0", fontSize: 13.5, fontWeight: 600, color: pending > 0 ? "var(--warn-fg)" : "var(--text-muted)" }}>{pendingText(pending)}</p>
    </div>
  );
}
