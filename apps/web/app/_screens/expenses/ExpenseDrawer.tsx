"use client";

import { useEffect, useRef, useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { DateField, Field, MoneyField, SelectField, TextField, useRequired } from "../../_design/form";
import { Icon } from "../../_design/Icon";
import { SlideOver } from "../../_design/SlideOver";
import { API_URL } from "../../api-url";
import {
  cancelExpense,
  createExpense,
  deleteReceipt,
  getExpense,
  getReceiptBlob,
  updateExpense,
  uploadReceipt,
  MAX_RECEIPT_BYTES,
  type Expense,
  type ExpenseCategory,
  type ExpenseEvent,
  type ExpenseMethod,
} from "../../expenses/api";
import { centsToPesosInput, formatCents, pesosToCents } from "../../_lib/money";

export const METHOD_LABEL: Record<ExpenseMethod, string> = { cash: "Efectivo", card: "Tarjeta", transfer: "Transferencia" };
const METHODS: readonly ExpenseMethod[] = ["cash", "card", "transfer"];
const RECEIPT_TYPES = "image/jpeg,image/png,application/pdf";

const EVENT_LABEL: Record<string, string> = {
  created: "Registrado",
  edited: "Editado",
  cancelled: "Cancelado",
  receipt_attached: "Comprobante adjuntado",
  receipt_removed: "Comprobante quitado",
};

export interface ExpenseDrawerProps {
  /** Absent: a new expense. */
  expense?: Expense;
  categories: readonly ExpenseCategory[];
  /** The office's day (aaaa-mm-dd): the default date and the latest allowed. */
  today: string;
  /** expenses:write. Without it the drawer only reads. */
  canWrite: boolean;
  onClose: () => void;
  /** Saved: what to tell the person, and whether to keep the drawer open (save & new). */
  onSaved: (message: string, keepOpen?: boolean) => void;
}

/**
 * One expense: record it (six fields → a drawer, per the forms rule), or open
 * it to edit, attach its receipt, read its history and cancel it. Field order
 * is the table's column order. Defaults make the common case one click: today,
 * cash, the last category used.
 */
export function ExpenseDrawer({ expense, categories, today, canWrite, onClose, onSaved }: ExpenseDrawerProps): ReactElement {
  const editing = expense !== undefined;
  const [spentOn, setSpentOn] = useState(expense?.spentOn ?? today);
  const [categoryId, setCategoryId] = useState(expense?.categoryId ?? "");
  const [amount, setAmount] = useState(expense !== undefined ? centsToPesosInput(expense.amountCents) : "");
  const [method, setMethod] = useState<ExpenseMethod>(expense?.method ?? "cash");
  const [description, setDescription] = useState(expense?.description ?? "");
  const [reason, setReason] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const [events, setEvents] = useState<ExpenseEvent[]>([]);
  const [current, setCurrent] = useState<Expense | undefined>(expense);
  const [cancelling, setCancelling] = useState(false);
  const [cancelReason, setCancelReason] = useState("");
  const required = useRequired(["spentOn", "categoryId", "amount"]);
  const cancelRequired = useRequired(["cancelReason"]);
  const fileInput = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (expense === undefined) return;
    let live = true;
    getExpense(API_URL, expense.id)
      .then((got) => {
        if (!live) return;
        setEvents(got.events);
        setCurrent(got.expense);
      })
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [expense]);

  const cancelled = current?.status === "cancelled";
  const readOnly = !canWrite || cancelled;
  // A retired category stays selectable on the expense that already has it.
  const options = categories
    .filter((c) => c.active || c.id === expense?.categoryId)
    .map((c) => ({ value: c.id, label: c.active ? c.name : `${c.name} (desactivada)` }));

  const reset = (): void => {
    setAmount("");
    setDescription("");
    setFile(null);
    if (fileInput.current !== null) fileInput.current.value = "";
  };

  const save = async (keepOpen: boolean): Promise<void> => {
    if (!required.check({ spentOn, categoryId, amount })) return;
    const amountCents = pesosToCents(amount);
    if (Number.isNaN(amountCents) || amountCents <= 0) {
      setError("Escribe el monto en pesos, por ejemplo 1,250.50");
      return;
    }
    if (file !== null && file.size > MAX_RECEIPT_BYTES) {
      setError("El comprobante pesa más de 5 MB.");
      return;
    }
    setPending(true);
    setError(undefined);
    try {
      const input = { spentOn, categoryId, amountCents, method, description: description.trim() };
      const saved = editing
        ? await updateExpense(API_URL, expense.id, { ...input, reason: reason.trim() })
        : await createExpense(API_URL, input);
      if (file !== null) {
        await uploadReceipt(API_URL, saved.id, file);
      }
      if (keepOpen) reset();
      onSaved(editing ? "Egreso actualizado" : "Egreso registrado", keepOpen);
    } catch (err) {
      setError(err instanceof Error ? err.message : "No se pudo guardar el egreso.");
    } finally {
      setPending(false);
    }
  };

  const confirmCancel = async (): Promise<void> => {
    if (expense === undefined || !cancelRequired.check({ cancelReason: cancelReason.trim() })) return;
    setPending(true);
    try {
      await cancelExpense(API_URL, expense.id, cancelReason.trim());
      onSaved("Egreso cancelado");
    } catch (err) {
      setError(err instanceof Error ? err.message : "No se pudo cancelar el egreso.");
    } finally {
      setPending(false);
    }
  };

  const openReceipt = async (): Promise<void> => {
    if (expense === undefined) return;
    try {
      const blob = await getReceiptBlob(API_URL, expense.id);
      window.open(URL.createObjectURL(blob), "_blank", "noopener");
    } catch (err) {
      setError(err instanceof Error ? err.message : "No se pudo abrir el comprobante.");
    }
  };

  const removeReceipt = async (): Promise<void> => {
    if (expense === undefined) return;
    try {
      setCurrent(await deleteReceipt(API_URL, expense.id));
    } catch (err) {
      setError(err instanceof Error ? err.message : "No se pudo quitar el comprobante.");
    }
  };

  const title = editing ? (cancelled ? "Egreso cancelado" : canWrite ? "Editar egreso" : "Egreso") : "Registrar egreso";

  return (
    <SlideOver
      title={title}
      subtitle={editing ? `${expense.categoryName} · ${formatCents(expense.amountCents)}` : "Lo que salió de caja o de la cuenta"}
      icon="trend"
      submitLabel={editing ? "Guardar cambios" : "Registrar egreso"}
      pending={pending}
      error={error}
      onClose={onClose}
      onSubmit={() => {
        if (readOnly) {
          onClose();
          return;
        }
        void save(false);
      }}
      onSubmitAndNew={!editing && canWrite ? () => void save(true) : undefined}
    >
      {cancelled && (
        <p role="status" style={{ margin: 0, padding: "10px 14px", borderRadius: "var(--r-md)", background: "var(--warn-bg)", color: "var(--warn-fg)", fontSize: 14 }}>
          {`Cancelado: ${current?.cancelReason ?? ""}. Ya no cuenta en el balance ni en el corte.`}
        </p>
      )}
      <DateField label="Fecha" required value={spentOn} max={today} onChange={setSpentOn} disabled={readOnly}
        error={required.errors.spentOn} onClearError={() => required.clear("spentOn")} />
      <SelectField label="Categoría" required value={categoryId} onChange={setCategoryId} options={options} disabled={readOnly}
        error={required.errors.categoryId} onClearError={() => required.clear("categoryId")}
        help="Se configuran en la pestaña Categorías." />
      <MoneyField label="Monto" required value={amount} onChange={setAmount} disabled={readOnly}
        error={required.errors.amount} onClearError={() => required.clear("amount")} />
      <Field label="Forma de pago">
        {(id) => (
          <div id={id} role="radiogroup" aria-label="Forma de pago" style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
            {METHODS.map((m) => {
              const on = m === method;
              return (
                <button key={m} type="button" role="radio" aria-checked={on} disabled={readOnly} onClick={() => setMethod(m)}
                  style={{ minHeight: 44, padding: "0 16px", borderRadius: 99, border: `1.5px solid ${on ? "var(--forest)" : "var(--border)"}`,
                    background: on ? "color-mix(in oklab, var(--forest) 12%, var(--surface))" : "var(--surface)", color: "var(--text)", fontWeight: 600 }}>
                  {METHOD_LABEL[m]}
                </button>
              );
            })}
          </div>
        )}
      </Field>
      <TextField label="Descripción" value={description} onChange={setDescription} disabled={readOnly}
        help="Qué se pagó y a quién: «Tóner para dirección, Papelería López»." />
      {!readOnly && (
        <Field label="Comprobante" help="Foto del ticket o factura: JPG, PNG o PDF de hasta 5 MB.">
          {(id, describedBy) => (
            <input id={id} ref={fileInput} type="file" accept={RECEIPT_TYPES} aria-describedby={describedBy}
              onChange={(e) => setFile(e.target.files?.[0] ?? null)} style={{ fontSize: 15, minHeight: 44 }} />
          )}
        </Field>
      )}
      {editing && current?.receipt !== undefined && (
        <div style={{ display: "flex", gap: 10, alignItems: "center", flexWrap: "wrap" }}>
          <Btn kind="ghost" icon="documents" onClick={() => void openReceipt()}>Ver comprobante</Btn>
          {!readOnly && <Btn kind="ghost" icon="trash" onClick={() => void removeReceipt()}>Quitar comprobante</Btn>}
        </div>
      )}
      {editing && !readOnly && (
        <TextField label="Motivo del cambio" value={reason} onChange={setReason} help="Opcional. Queda en el historial." />
      )}
      {editing && <History events={events} />}
      {editing && !readOnly && !cancelling && (
        <div>
          <Btn kind="ghost" danger icon="close" onClick={() => setCancelling(true)}>Cancelar egreso</Btn>
        </div>
      )}
      {editing && !readOnly && cancelling && (
        <div style={{ display: "flex", flexDirection: "column", gap: 12, padding: 14, borderRadius: "var(--r-md)", border: "1px solid var(--border)" }}>
          <TextField label="Motivo de la cancelación" required value={cancelReason} onChange={setCancelReason}
            error={cancelRequired.errors.cancelReason} onClearError={() => cancelRequired.clear("cancelReason")} />
          <div style={{ display: "flex", gap: 10 }}>
            <Btn kind="ghost" onClick={() => setCancelling(false)}>No cancelar</Btn>
            <Btn danger onClick={() => void confirmCancel()}>Confirmar cancelación</Btn>
          </div>
        </div>
      )}
    </SlideOver>
  );
}

function History({ events }: { events: readonly ExpenseEvent[] }): ReactElement | null {
  if (events.length === 0) return null;
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
      <strong style={{ fontSize: 14, fontFamily: "var(--font-display)", color: "var(--text)", display: "inline-flex", gap: 6, alignItems: "center" }}>
        <Icon name="clock" size={16} />Historial
      </strong>
      {events.map((ev) => {
        const at = new Date(ev.at);
        const when = Number.isNaN(at.getTime()) ? "" : at.toLocaleString("es-MX", { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });
        const amountChanged = ev.before.amountCents !== undefined && ev.after.amountCents !== undefined && ev.before.amountCents !== ev.after.amountCents;
        return (
          <p key={ev.id} style={{ margin: 0, fontSize: 13.5, color: "var(--text-muted)", lineHeight: 1.45 }}>
            {`${EVENT_LABEL[ev.kind] ?? ev.kind} por ${ev.actorName ?? "—"}${when !== "" ? ` · ${when}` : ""}`}
            {amountChanged && ` · ${formatCents(ev.before.amountCents ?? 0)} → ${formatCents(ev.after.amountCents ?? 0)}`}
            {ev.reason !== undefined && ev.reason !== "" && ` · «${ev.reason}»`}
          </p>
        );
      })}
    </div>
  );
}
