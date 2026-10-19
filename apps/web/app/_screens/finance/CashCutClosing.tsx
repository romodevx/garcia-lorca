"use client";

import { useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { Icon } from "../../_design/Icon";
import { MoneyField, TextField, useRequired } from "../../_design/form";
import { API_URL } from "../../api-url";
import { closeCashCut, reopenCashCut, type CashCutClosure, type Expense } from "../../expenses/api";
import type { CashUp } from "../../finance/api";
import { formatCents, pesosToCents } from "../../_lib/money";

/** What the signed-in person may do with the day's cut (T10). UX only: the gateway decides. */
export interface CutAccess {
  /** module_expenses is on: the cut shows what went out and can be closed. */
  expenses: boolean;
  /** expenses:write — may close the day. */
  canClose: boolean;
  /** finance:write — the director, who may reopen a closed day. */
  canReopen: boolean;
}

const METHOD_LABEL: Record<string, string> = { cash: "Efectivo", card: "Tarjeta", transfer: "Transferencia" };

const card = {
  padding: "18px 22px",
  borderRadius: "var(--r-lg)",
  background: "var(--surface)",
  border: "1px solid var(--border)",
  boxShadow: "var(--shadow-card)",
} as const;

/** Red when money is missing, green when it is over, neutral when it squares. */
function differenceColor(cents: number): string {
  if (cents < 0) return "var(--bad-fg)";
  if (cents > 0) return "var(--ok-fg)";
  return "var(--text)";
}

/**
 * The second half of the corte (T10): what went out, the cash the drawer should
 * hold, and closing the day with what was counted. A closed day shows its
 * snapshot; a day that changed afterwards says so, and the director may reopen
 * it.
 */
export function CashCutClosing({
  cut,
  day,
  access,
  onChanged,
}: {
  cut: CashUp;
  /** The day as the cut was asked for: "" is today, the server's day. */
  day: string;
  access: CutAccess;
  onChanged: () => void;
}): ReactElement {
  const expenses = cut.expenses ?? [];
  // Close and reopen THE DAY ON SCREEN. "Today" was asked as "", and the
  // server's day may roll over (UTC) between reading the cut and the POST.
  const cutDay = day !== "" ? day : cut.day ?? "";
  const expected = cut.expectedCashCents ?? cut.cashCents;
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 18 }}>
      <ExpensesOfTheDay expenses={expenses} totalCents={cut.expensesByMethod?.totalCents ?? 0} />
      <div style={{ ...card, display: "flex", justifyContent: "space-between", alignItems: "center", gap: 12 }}>
        <span style={{ fontSize: 14.5, color: "var(--text-muted)" }}>Efectivo esperado en caja (efectivo recibido − egresos en efectivo)</span>
        <span data-testid="cash-up-expected" style={{ fontFamily: "var(--font-display)", fontWeight: 800, fontSize: 22, color: "var(--text)" }}>
          {formatCents(expected)}
        </span>
      </div>
      {cut.closure !== undefined ? (
        <ClosedCut closure={cut.closure} modified={cut.modifiedAfterClose === true} day={cutDay} canReopen={access.canReopen} onChanged={onChanged} />
      ) : (
        access.canClose && <CloseForm day={cutDay} expectedCents={expected} onChanged={onChanged} />
      )}
      {(cut.reopened ?? []).length > 0 && (
        <p style={{ fontSize: 13.5, color: "var(--text-muted)", margin: 0 }}>
          {`Este día se reabrió ${(cut.reopened ?? []).length === 1 ? "una vez" : `${(cut.reopened ?? []).length} veces`}: `}
          {(cut.reopened ?? []).map((r) => `${r.reopenedByName ?? "—"} (${r.reopenReason ?? ""})`).join(" · ")}
        </p>
      )}
    </div>
  );
}

function ExpensesOfTheDay({ expenses, totalCents }: { expenses: readonly Expense[]; totalCents: number }): ReactElement {
  return (
    <div data-testid="cash-up-expenses" style={{ borderRadius: "var(--r-lg)", border: "1px solid var(--border)", background: "var(--surface)", overflow: "hidden" }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8, padding: "10px 20px", background: "var(--surface-2)", fontSize: 13, fontWeight: 700, color: "var(--text-muted)", fontFamily: "var(--font-display)" }}>
        <span style={{ display: "inline-flex", alignItems: "center", gap: 8 }}><Icon name="trend" size={16} />Egresos del día</span>
        <span>{formatCents(totalCents)}</span>
      </div>
      {expenses.length === 0 && <p style={{ margin: 0, padding: "13px 20px", fontSize: 14, color: "var(--text-muted)" }}>No se registraron egresos este día.</p>}
      {expenses.map((e) => {
        const cancelled = e.status === "cancelled";
        return (
          <div key={e.id} style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12, padding: "13px 20px", borderTop: "1px solid var(--border)" }}>
            <span style={{ fontSize: 14, color: "var(--text-muted)" }}>
              {`${METHOD_LABEL[e.method] ?? e.method} · ${e.categoryName}${e.description !== "" ? ` · ${e.description}` : ""}`}
              {cancelled && ` · cancelado: ${e.cancelReason ?? ""}`}
            </span>
            <span style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 15.5, color: cancelled ? "var(--text-faint)" : "var(--text)", textDecoration: cancelled ? "line-through" : "none" }}>
              {formatCents(e.amountCents)}
            </span>
          </div>
        );
      })}
    </div>
  );
}

function CloseForm({ day, expectedCents, onChanged }: { day: string; expectedCents: number; onChanged: () => void }): ReactElement {
  const [counted, setCounted] = useState("");
  const [note, setNote] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const required = useRequired(["counted"]);
  const countedCents = pesosToCents(counted);
  const typed = counted.trim() !== "" && !Number.isNaN(countedCents);

  const submit = async (): Promise<void> => {
    if (!required.check({ counted })) return;
    if (Number.isNaN(countedCents)) {
      setError("Escribe el efectivo contado como un monto, por ejemplo 2,850.00");
      return;
    }
    setPending(true);
    setError(undefined);
    try {
      await closeCashCut(API_URL, { day, countedCashCents: countedCents, note: note.trim() });
      onChanged();
    } catch (err) {
      setError(err instanceof Error ? err.message : "No se pudo cerrar el corte.");
    } finally {
      setPending(false);
    }
  };

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        void submit();
      }}
      style={{ ...card, display: "flex", flexDirection: "column", gap: 14 }}
    >
      <strong style={{ fontFamily: "var(--font-display)", fontSize: 16, color: "var(--text)" }}>Cerrar el corte</strong>
      <MoneyField
        label="Efectivo contado en caja"
        required
        value={counted}
        onChange={setCounted}
        error={required.errors.counted}
        onClearError={() => required.clear("counted")}
        help="Cuenta los billetes y monedas del cajón y escribe el total."
      />
      {typed && (
        <div style={{ display: "flex", justifyContent: "space-between", fontSize: 14.5 }}>
          <span style={{ color: "var(--text-muted)" }}>Diferencia (contado − esperado)</span>
          <span data-testid="cash-up-difference" style={{ fontWeight: 700, color: differenceColor(countedCents - expectedCents) }}>
            {formatCents(countedCents - expectedCents)}
          </span>
        </div>
      )}
      <TextField label="Nota" value={note} onChange={setNote} help="Opcional: por qué no cuadra, quién entregó el efectivo." />
      {error !== undefined && (
        <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 14, margin: 0 }}>{error}</p>
      )}
      <div>
        <Btn icon="lock" type="submit" disabled={pending}>{pending ? "Cerrando…" : "Cerrar corte"}</Btn>
      </div>
    </form>
  );
}

function ClosedCut({ closure, modified, day, canReopen, onChanged }: {
  closure: CashCutClosure;
  modified: boolean;
  day: string;
  canReopen: boolean;
  onChanged: () => void;
}): ReactElement {
  const [reopening, setReopening] = useState(false);
  const [reason, setReason] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const required = useRequired(["reason"]);
  const closedAt = new Date(closure.closedAt);

  const reopen = async (): Promise<void> => {
    if (!required.check({ reason: reason.trim() })) return;
    setPending(true);
    setError(undefined);
    try {
      await reopenCashCut(API_URL, day === "" ? closure.day : day, reason.trim());
      onChanged();
    } catch (err) {
      setError(err instanceof Error ? err.message : "No se pudo reabrir el corte.");
    } finally {
      setPending(false);
    }
  };

  const row = (label: string, value: string, testId?: string, color = "var(--text)"): ReactElement => (
    <div style={{ display: "flex", justifyContent: "space-between", gap: 12, fontSize: 14.5 }}>
      <span style={{ color: "var(--text-muted)" }}>{label}</span>
      <span data-testid={testId} style={{ fontWeight: 700, color }}>{value}</span>
    </div>
  );

  return (
    <div data-testid="cash-up-closure" style={{ ...card, display: "flex", flexDirection: "column", gap: 10, borderColor: modified ? "var(--warn-fg)" : "var(--border)" }}>
      <div style={{ display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
        <span style={{ display: "inline-flex", alignItems: "center", gap: 6, padding: "3px 10px", borderRadius: 99, background: "var(--ok-bg)", color: "var(--ok-fg)", fontSize: 12.5, fontWeight: 700 }}>
          <Icon name="lock" size={14} />Corte cerrado
        </span>
        {modified && (
          <span role="status" style={{ display: "inline-flex", alignItems: "center", gap: 6, padding: "3px 10px", borderRadius: 99, background: "var(--warn-bg)", color: "var(--warn-fg)", fontSize: 12.5, fontWeight: 700 }}>
            <Icon name="warning" size={14} />Modificado después del cierre
          </span>
        )}
      </div>
      <p style={{ margin: 0, fontSize: 13.5, color: "var(--text-muted)" }}>
        {`Cerrado por ${closure.closedByName ?? "—"}${Number.isNaN(closedAt.getTime()) ? "" : ` el ${closedAt.toLocaleString("es-MX", { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" })}`}`}
        {closure.note !== undefined && closure.note !== "" && ` · ${closure.note}`}
      </p>
      {row("Efectivo esperado al cerrar", formatCents(closure.expectedCashCents))}
      {row("Efectivo contado", formatCents(closure.countedCashCents))}
      {row("Diferencia", formatCents(closure.differenceCents), "cash-up-closure-difference", differenceColor(closure.differenceCents))}
      {modified && (
        <p style={{ margin: 0, fontSize: 13.5, color: "var(--warn-fg)" }}>
          Algo de este día cambió después de cerrar: un cobro nuevo, una corrección, una cancelación o un egreso. Las cifras de arriba son las de hoy; el cierre conserva las de entonces.
        </p>
      )}
      {canReopen && !reopening && (
        <div>
          <Btn kind="ghost" icon="edit" onClick={() => setReopening(true)}>Reabrir corte</Btn>
        </div>
      )}
      {canReopen && reopening && (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void reopen();
          }}
          style={{ display: "flex", flexDirection: "column", gap: 12 }}
        >
          <TextField
            label="Motivo de la reapertura"
            required
            value={reason}
            onChange={setReason}
            error={required.errors.reason}
            onClearError={() => required.clear("reason")}
          />
          {error !== undefined && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 14, margin: 0 }}>{error}</p>}
          <div style={{ display: "flex", gap: 10 }}>
            <Btn kind="ghost" onClick={() => setReopening(false)}>Cancelar</Btn>
            <Btn type="submit" disabled={pending}>{pending ? "Reabriendo…" : "Confirmar reapertura"}</Btn>
          </div>
        </form>
      )}
    </div>
  );
}
