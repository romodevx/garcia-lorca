"use client";

import { useEffect, useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { DateField } from "../../_design/form";
import { API_URL } from "../../api-url";
import { downloadReport, getReport, type FinanceReport, type MethodTotals, type ReportPeriod } from "../../expenses/api";
import { formatCents } from "../../_lib/money";

const PERIODS: readonly { value: ReportPeriod; label: string }[] = [
  { value: "daily", label: "Día" },
  { value: "weekly", label: "Semana" },
  { value: "monthly", label: "Mes" },
  { value: "yearly", label: "Año" },
];

const box = { padding: 20, borderRadius: "var(--r-xl)", background: "var(--surface)", border: "1px solid var(--border)", boxShadow: "var(--shadow-card)" } as const;
const th = { textAlign: "left", fontSize: 12, fontWeight: 700, color: "var(--text-faint)", textTransform: "uppercase", letterSpacing: ".04em", padding: "8px 10px" } as const;
const td = { padding: "10px", fontSize: 14.5, color: "var(--text)", borderTop: "1px solid var(--border)" } as const;
const num = { ...td, textAlign: "right", fontVariantNumeric: "tabular-nums" } as const;

/**
 * The period's money and its reports: a day, a week (Monday–Sunday), a month
 * or a year, with income, expenses and balance by method, and the PDF and
 * Excel exports the gateway renders.
 */
export function ReportsPanel({ today }: { today: string }): ReactElement {
  const [period, setPeriod] = useState<ReportPeriod>("monthly");
  const [date, setDate] = useState(today);
  const [report, setReport] = useState<FinanceReport | null>(null);
  const [error, setError] = useState<string | undefined>(undefined);
  const [exporting, setExporting] = useState<"pdf" | "xlsx" | null>(null);

  useEffect(() => {
    let live = true;
    setError(undefined);
    getReport(API_URL, period, date)
      .then((r) => { if (live) setReport(r); })
      .catch((err: unknown) => { if (live) setError(err instanceof Error ? err.message : "No se pudo leer el reporte."); });
    return () => { live = false; };
  }, [period, date]);

  const exportAs = async (format: "pdf" | "xlsx"): Promise<void> => {
    setExporting(format);
    try {
      const { blob, filename } = await downloadReport(API_URL, period, date, format);
      const href = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = href;
      a.download = filename;
      a.click();
      URL.revokeObjectURL(href);
    } catch (err) {
      setError(err instanceof Error ? err.message : "No se pudo exportar el reporte.");
    } finally {
      setExporting(null);
    }
  };

  const s = report?.summary;
  const methodRow = (label: string, pick: (m: MethodTotals) => number): ReactElement | null =>
    s === undefined ? null : (
      <tr key={label}>
        <td style={td}>{label}</td>
        <td style={num}>{formatCents(pick(s.income))}</td>
        <td style={num}>{formatCents(pick(s.expenses))}</td>
        <td style={{ ...num, fontWeight: 700, color: pick(s.balance) < 0 ? "var(--bad-fg)" : "var(--text)" }}>{formatCents(pick(s.balance))}</td>
      </tr>
    );

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 18 }}>
      <div style={{ ...box, display: "flex", flexWrap: "wrap", gap: 16, alignItems: "flex-end" }}>
        <div role="radiogroup" aria-label="Periodo" style={{ display: "flex", gap: 6, padding: 5, background: "var(--surface-3)", borderRadius: 99, border: "1px solid var(--border)" }}>
          {PERIODS.map((p) => {
            const on = p.value === period;
            return (
              <button key={p.value} type="button" role="radio" aria-checked={on} onClick={() => setPeriod(p.value)}
                style={{ height: 42, padding: "0 18px", borderRadius: 99, background: on ? "var(--surface)" : "transparent", color: on ? "var(--text)" : "var(--text-muted)", fontWeight: 600, boxShadow: on ? "var(--shadow-card)" : "none" }}>
                {p.label}
              </button>
            );
          })}
        </div>
        <div style={{ width: 200 }}>
          <DateField label="Cualquier día del periodo" value={date} max={today} onChange={(v) => v !== "" && setDate(v)} />
        </div>
        <div style={{ flex: 1 }} />
        <Btn kind="ghost" icon="download" disabled={exporting !== null} onClick={() => void exportAs("pdf")}>{exporting === "pdf" ? "Generando…" : "PDF"}</Btn>
        <Btn kind="ghost" icon="download" disabled={exporting !== null} onClick={() => void exportAs("xlsx")}>{exporting === "xlsx" ? "Generando…" : "Excel"}</Btn>
      </div>

      {error !== undefined && <p role="alert" style={{ margin: 0, color: "var(--bad-fg)", fontSize: 14 }}>{error}</p>}

      {report !== null && s !== undefined && (
        <>
          <div style={box}>
            <h2 style={{ fontSize: 18, fontWeight: 800, color: "var(--text)", margin: "0 0 12px" }}>{report.label}</h2>
            <table style={{ width: "100%", borderCollapse: "collapse" }}>
              <thead>
                <tr><th style={th}>Forma de pago</th><th style={{ ...th, textAlign: "right" }}>Ingresos</th><th style={{ ...th, textAlign: "right" }}>Egresos</th><th style={{ ...th, textAlign: "right" }}>Balance</th></tr>
              </thead>
              <tbody>
                {methodRow("Efectivo", (m) => m.cashCents)}
                {methodRow("Tarjeta", (m) => m.cardCents)}
                {methodRow("Transferencia", (m) => m.transferCents)}
                {methodRow("Total", (m) => m.totalCents)}
              </tbody>
            </table>
            <p style={{ margin: "12px 0 0", fontSize: 14, color: "var(--text-muted)" }}>
              {`Notas de crédito aplicadas (no es ingreso): ${formatCents(s.creditNoteCents)}`}
            </p>
          </div>

          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(min(340px, 100%), 1fr))", gap: 18 }}>
            <div style={box}>
              <h3 style={{ fontSize: 15, fontWeight: 700, margin: "0 0 8px", color: "var(--text)" }}>Ingresos por concepto</h3>
              <table style={{ width: "100%", borderCollapse: "collapse" }}>
                <tbody>
                  {report.incomeByConcept.lines.map((l) => (
                    <tr key={l.conceptId + l.concept}><td style={td}>{l.concept}</td><td style={num}>{formatCents(l.amountCents)}</td></tr>
                  ))}
                  <tr><td style={{ ...td, fontWeight: 700 }}>Total aplicado</td><td style={{ ...num, fontWeight: 700 }}>{formatCents(report.incomeByConcept.totalCents)}</td></tr>
                </tbody>
              </table>
            </div>
            <div style={box}>
              <h3 style={{ fontSize: 15, fontWeight: 700, margin: "0 0 8px", color: "var(--text)" }}>Egresos por categoría</h3>
              <table style={{ width: "100%", borderCollapse: "collapse" }}>
                <tbody>
                  {s.expensesByCategory.length === 0 && <tr><td style={td}>Sin egresos en el periodo.</td></tr>}
                  {s.expensesByCategory.map((c) => (
                    <tr key={c.categoryId}><td style={td}>{`${c.categoryName} (${c.count})`}</td><td style={num}>{formatCents(c.amountCents)}</td></tr>
                  ))}
                  <tr><td style={{ ...td, fontWeight: 700 }}>Total</td><td style={{ ...num, fontWeight: 700 }}>{formatCents(s.expenses.totalCents)}</td></tr>
                </tbody>
              </table>
            </div>
          </div>
        </>
      )}
    </div>
  );
}
