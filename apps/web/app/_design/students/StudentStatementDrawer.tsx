"use client";

import { useCallback, useEffect, useRef, useState, type CSSProperties, type ReactElement } from "react";

import { API_URL } from "../../api-url";
import { dayWithYearLabel } from "../../_lib/localDate";
import { formatBalance, formatCents } from "../../_lib/money";
import {
  downloadStatementPdf,
  getStatement,
  isDebit,
  type Statement,
  type StatementMovement,
} from "../../finance/statement";
import { Btn } from "../Controls";
import { DrawerShell } from "../DrawerShell";
import { Icon } from "../Icon";
import { CorrectPaymentDrawer } from "../payments/CorrectPaymentDrawer";
import { VoidDepositDrawer } from "../payments/VoidDepositDrawer";

export interface StudentStatementDrawerProps {
  studentId: string;
  /** The pupil's name as the list shows it (surnames first). */
  name: string;
  subtitle?: string;
  /** May correct a registered payment (payments:edit, T9). UX only: the gateway decides. */
  canCorrect?: boolean;
  /** May void a registered payment (finance:write, CC-203). UX only: the gateway decides. */
  canVoid?: boolean;
  onClose: () => void;
}

const METHOD_LABEL: Record<string, string> = { cash: "Efectivo", transfer: "Transferencia", card: "Tarjeta", credit_note: "Nota de crédito" };
const CARD_LABEL: Record<string, string> = { debit: "Tarjeta de débito", credit: "Tarjeta de crédito" };

function methodLabel(m: StatementMovement): string {
  const card = m.method === "card" && m.cardType !== undefined ? CARD_LABEL[m.cardType] : undefined;
  if (card !== undefined) return card;
  return m.method === undefined ? "" : (METHOD_LABEL[m.method] ?? m.method);
}

/** The concept column's second line: how the money came, and its folio. */
function detailOf(m: StatementMovement): string {
  if (m.kind !== "payment" && m.kind !== "credit_note") return "";
  return [m.folio, methodLabel(m), m.reference === undefined || m.reference === "" ? "" : `ref. ${m.reference}`]
    .filter((part) => part !== undefined && part !== "")
    .join(" · ");
}

type Load = { state: "loading" } | { state: "failed"; message: string } | { state: "ready"; statement: Statement };

/**
 * A pupil's account statement, opened from the list (T7). It shows finance's
 * ledger as it comes — every movement, its running balance, the totals and
 * what the counter asks for today — and computes nothing: the parents' app and
 * the PDF read the same numbers.
 */
export function StudentStatementDrawer({ studentId, name, subtitle, canCorrect = false, canVoid = false, onClose }: StudentStatementDrawerProps): ReactElement {
  const [load, setLoad] = useState<Load>({ state: "loading" });
  // The receipt being corrected (T9). One dialog at a time: while it is open
  // the statement steps aside, and comes back re-read.
  const [correcting, setCorrecting] = useState<string | undefined>(undefined);
  // The receipt being voided (CC-203), the same way: the statement steps aside.
  const [voiding, setVoiding] = useState<ReceiptRef | undefined>(undefined);
  const [notice, setNotice] = useState("");
  const [downloading, setDownloading] = useState(false);
  const [pdfProblem, setPdfProblem] = useState<string | undefined>(undefined);
  const request = useRef(0);

  const read = useCallback((): void => {
    const id = (request.current += 1);
    setLoad({ state: "loading" });
    getStatement(API_URL, studentId)
      .then((statement) => { if (id === request.current) setLoad({ state: "ready", statement }); })
      .catch((cause: unknown) => {
        if (id !== request.current) return;
        setLoad({ state: "failed", message: cause instanceof Error ? cause.message : "No se pudo leer el estado de cuenta." });
      });
  }, [studentId]);

  useEffect(() => { read(); }, [read]);

  const download = async (): Promise<void> => {
    setPdfProblem(undefined);
    setDownloading(true);
    try {
      await downloadStatementPdf(API_URL, studentId);
    } catch {
      setPdfProblem("No se pudo descargar el PDF. Intenta de nuevo.");
    } finally {
      setDownloading(false);
    }
  };

  const footer = (
    <div style={{ display: "flex", gap: 12, alignItems: "center", justifyContent: "flex-end", flexWrap: "wrap" }}>
      {pdfProblem !== undefined && <span role="alert" style={{ color: "var(--red)", fontSize: 14 }}>{pdfProblem}</span>}
      <Btn icon="download" onClick={() => void download()} disabled={downloading || load.state !== "ready"}>Descargar PDF</Btn>
    </div>
  );

  if (correcting !== undefined) {
    return (
      <CorrectPaymentDrawer
        depositId={correcting}
        studentId={studentId}
        onClose={() => { setCorrecting(undefined); read(); }}
      />
    );
  }

  if (voiding !== undefined) {
    return (
      <VoidDepositDrawer
        depositId={voiding.depositId}
        folio={voiding.folio}
        onCancel={() => setVoiding(undefined)}
        onVoided={(message) => { setVoiding(undefined); setNotice(message); read(); }}
      />
    );
  }

  return (
    <DrawerShell title={`Estado de cuenta · ${name}`} subtitle={subtitle} icon="finance" size="lg" onClose={onClose} footer={footer}>
      {load.state === "loading" && <p style={{ color: "var(--text-muted)" }}>Cargando estado de cuenta…</p>}
      {load.state === "failed" && (
        <div role="alert" style={{ display: "grid", gap: 12, justifyItems: "start" }}>
          <p style={{ color: "var(--red)", margin: 0 }}>{load.message}</p>
          <Btn kind="ghost" onClick={read}>Reintentar</Btn>
        </div>
      )}
      <div role="status" style={{ fontSize: 14.5, fontWeight: 600, color: "var(--ok-fg)", marginBottom: notice === "" ? 0 : 12 }}>{notice}</div>
      {load.state === "ready" && <Ledger statement={load.statement} onCorrect={canCorrect ? setCorrecting : undefined} onVoid={canVoid ? setVoiding : undefined} />}
    </DrawerShell>
  );
}

/** One receipt, as the void needs it. */
interface ReceiptRef {
  depositId: string;
  folio: string;
}

function Ledger({ statement, onCorrect, onVoid }: { statement: Statement; onCorrect?: (depositId: string) => void; onVoid?: (receipt: ReceiptRef) => void }): ReactElement {
  const { totals } = statement;
  // A receipt paid by several methods is several lines: it is corrected once,
  // from its first line. A cancelled or replaced receipt is not corrected.
  const correctable = new Set<number>();
  const seen = new Set<string>();
  statement.movements.forEach((m, i) => {
    if (m.depositId === undefined || m.voided || m.superseded === true || seen.has(m.depositId)) return;
    seen.add(m.depositId);
    correctable.add(i);
  });
  return (
    <div style={{ display: "grid", gap: 20 }}>
      <Totals statement={statement} />
      {statement.movements.length === 0 ? (
        <div style={{ textAlign: "center", padding: "28px 0", color: "var(--text-muted)" }}>
          <div style={{ fontWeight: 600, fontSize: 16, color: "var(--text)" }}>Sin movimientos</div>
          <div style={{ fontSize: 14 }}>Aquí aparecerán los cargos y pagos del alumno.</div>
        </div>
      ) : (
        <div style={{ overflowX: "auto" }}>
          <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 14 }}>
            <thead>
              <tr style={{ textAlign: "left", color: "var(--text-muted)", fontSize: 12.5, textTransform: "uppercase", letterSpacing: 0.4 }}>
                <th style={th}>Fecha</th>
                <th style={th}>Concepto</th>
                <th style={{ ...th, textAlign: "right" }}>Cargo</th>
                <th style={{ ...th, textAlign: "right" }}>Abono</th>
                <th style={{ ...th, textAlign: "right" }}>Saldo</th>
              </tr>
            </thead>
            <tbody>
              {statement.movements.map((m, i) => (
                <MovementRow
                  key={`${m.kind}-${m.chargeId ?? ""}-${m.depositId ?? ""}-${i}`}
                  m={m}
                  index={i}
                  onCorrect={onCorrect !== undefined && correctable.has(i) && m.depositId !== undefined ? () => onCorrect(m.depositId ?? "") : undefined}
                  onVoid={onVoid !== undefined && correctable.has(i) && m.depositId !== undefined ? () => onVoid({ depositId: m.depositId ?? "", folio: m.folio ?? "" }) : undefined}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}
      {totals.notYetDueCents > 0 && (
        <p style={{ margin: 0, fontSize: 13.5, color: "var(--text-muted)" }}>
          Aún no exigible (fuera del saldo): {formatCents(totals.notYetDueCents)}
        </p>
      )}
    </div>
  );
}

const th: CSSProperties = { padding: "8px 10px", borderBottom: "1px solid var(--border)", fontWeight: 600 };
const td: CSSProperties = { padding: "10px", borderBottom: "1px solid var(--border)", verticalAlign: "top" };

const actionStyle: CSSProperties = { display: "inline-flex", alignItems: "center", gap: 6, minHeight: 44, marginTop: 2, padding: "0 4px", fontSize: 13.5, fontWeight: 600, color: "var(--forest)", background: "none", border: "none", cursor: "pointer", textDecoration: "none" };

function MovementRow({ m, index, onCorrect, onVoid }: { m: StatementMovement; index: number; onCorrect?: () => void; onVoid?: () => void }): ReactElement {
  const struck = m.voided || m.superseded === true;
  const muted = struck || m.notYetDue;
  const amount = formatCents(m.amountCents);
  const detail = detailOf(m);
  return (
    <tr style={{ color: muted ? "var(--text-faint)" : "var(--text)", textDecoration: struck ? "line-through" : "none" }}>
      <td style={{ ...td, whiteSpace: "nowrap" }}>{dayWithYearLabel(m.date)}</td>
      <td style={td}>
        <div style={{ fontWeight: 600 }}>{m.description}</div>
        {detail !== "" && <div style={{ fontSize: 12.5, color: "var(--text-muted)" }}>{detail}</div>}
        {m.voided && <div style={{ fontSize: 12.5, color: "var(--red)" }}>{`Cancelado${m.voidReason ? `: ${m.voidReason}` : ""}`}</div>}
        {m.superseded === true && <div style={{ fontSize: 12.5, color: "var(--text-muted)" }}>{`Sustituido${m.supersededByFolio ? ` por ${m.supersededByFolio}` : ""}`}</div>}
        {m.correctsFolio !== undefined && m.correctsFolio !== "" && <div style={{ fontSize: 12.5, color: "var(--text-muted)" }}>{`Corrige a ${m.correctsFolio}`}</div>}
        {onCorrect !== undefined && (
          <button
            type="button"
            onClick={onCorrect}
            aria-label={`Corregir pago ${m.folio ?? ""}`.trim()}
            style={{ display: "inline-flex", alignItems: "center", gap: 6, minHeight: 44, marginTop: 2, padding: "0 4px", fontSize: 13.5, fontWeight: 600, color: "var(--forest)", background: "none", border: "none", cursor: "pointer", textDecoration: "none" }}
          >
            <Icon name="edit" size={15} />Corregir
          </button>
        )}
        {onVoid !== undefined && (
          <button type="button" onClick={onVoid} aria-label={`Anular pago ${m.folio ?? ""}`.trim()} style={{ ...actionStyle, color: "var(--bad-fg)" }}>
            <Icon name="trash" size={15} />Anular
          </button>
        )}
        {m.notYetDue && <Tag>Aún no exigible</Tag>}
        {m.accrued && <Tag>Acumulado a hoy</Tag>}
      </td>
      <td style={{ ...td, textAlign: "right", whiteSpace: "nowrap" }}>{isDebit(m.kind) ? amount : ""}</td>
      <td style={{ ...td, textAlign: "right", whiteSpace: "nowrap", color: muted ? undefined : "var(--forest)" }}>{isDebit(m.kind) ? "" : amount}</td>
      <td data-testid={`running-balance-${index}`} style={{ ...td, textAlign: "right", whiteSpace: "nowrap", fontWeight: 600 }}>{formatBalance(m.balanceCents)}</td>
    </tr>
  );
}

function Tag({ children }: { children: string }): ReactElement {
  return (
    <span style={{ display: "inline-block", marginTop: 4, padding: "2px 9px", borderRadius: 99, fontSize: 12, fontWeight: 600, background: "var(--surface-3)", color: "var(--text-muted)", textDecoration: "none" }}>
      {children}
    </span>
  );
}

function Totals({ statement }: { statement: Statement }): ReactElement {
  const { totals } = statement;
  const credit = totals.balanceCents < 0;
  const cards: { label: string; cents: number; strong?: boolean }[] = [
    { label: "Total cargado", cents: totals.chargedCents },
    ...(totals.discountCents > 0 ? [{ label: "Descuentos y becas", cents: totals.discountCents }] : []),
    { label: "Total pagado", cents: totals.paidCents },
    { label: credit ? "Saldo a favor" : "Saldo pendiente", cents: Math.abs(totals.balanceCents), strong: true },
    { label: statement.asOf === "" ? "A pagar hoy" : `A pagar hoy (${dayWithYearLabel(statement.asOf)})`, cents: statement.dueTodayCents },
  ];
  return (
    <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(150px, 1fr))", gap: 12 }}>
      {cards.map((c) => (
        <div key={c.label} style={{ padding: "14px 16px", borderRadius: "var(--r-md)", background: c.strong ? "color-mix(in oklab, var(--forest) 10%, var(--surface))" : "var(--surface-2)", border: "1px solid var(--border)" }}>
          <div style={{ fontSize: 12.5, color: "var(--text-muted)", fontWeight: 600 }}>{c.label}</div>
          <div style={{ fontFamily: "var(--font-display)", fontSize: c.strong ? 22 : 18, fontWeight: 700, color: "var(--text)" }}>{formatCents(c.cents)}</div>
        </div>
      ))}
    </div>
  );
}
