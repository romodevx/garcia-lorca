"use client";

import type { ReactElement } from "react";

import { StatCards, type StatCardDef } from "../../_design/StatCards";
import type { ImportSummary as Summary } from "../../studentImport/api";
import type { ImportTab } from "../../studentImport/view";

/**
 * The counts, which reconcile to the file: Nuevos + Ya estaban + No se importan
 * (+ Pendientes) = the rows. "Con avisos" is a subset of Nuevos, and says so.
 * A click on a card opens that tab of the table.
 */
export function ImportSummary({ summary, onPick }: { summary: Summary; onPick: (tab: ImportTab) => void }): ReactElement {
  const cards: StatCardDef[] = [
    { icon: "plus", color: "forest", label: "Nuevos", value: String(summary.new), sub: `se importan · ${summary.warnings} con avisos`, onClick: () => onPick("new") },
    { icon: "warning", color: "yellow", label: "Con avisos", value: String(summary.warnings), sub: "incluidos en Nuevos", onClick: () => onPick("warning") },
    { icon: "user", color: "neutral", label: "Ya estaban", value: String(summary.already), sub: "misma CURP, no se tocan", onClick: () => onPick("already") },
    { icon: "close", color: "red", label: "No se importan", value: String(summary.skipped), sub: "ver motivo en la tabla", onClick: () => onPick("skipped") },
  ];
  if (summary.pending > 0) {
    cards.push({ icon: "clock", color: "orange", label: "Pendientes", value: String(summary.pending), sub: "falta elegir su nivel", onClick: () => onPick("pending") });
  }
  return (
    <div role="group" aria-label="Resumen del archivo" style={{ display: "grid", gap: 8 }}>
      <StatCards cards={cards} />
      <p style={{ fontSize: 13.5, color: "var(--text-muted)", margin: 0 }}>{`${summary.total} filas en el archivo`}</p>
    </div>
  );
}
