"use client";

import { useEffect, useRef, useState, type FormEvent, type ReactElement } from "react";

import { DateField } from "../../_design/form";
import { formatCents } from "../../_lib/money";
import { API_URL } from "../../api-url";
import { StudentsError, previewWithdrawal, withdraw, type Enrollment, type WithdrawalPreview } from "../../students/api";
import { todayISO } from "../attendance/sheetModel";

export function WithdrawDialog({ enrollment, onClose, onSuccess }: { enrollment: Enrollment; onClose: () => void; onSuccess: (updated: Enrollment) => void }): ReactElement {
  const dialog = useRef<HTMLDialogElement>(null);
  const [date, setDate] = useState(todayISO());
  const [reason, setReason] = useState("");
  const [preview, setPreview] = useState<WithdrawalPreview | null>(null);
  const [previewRestricted, setPreviewRestricted] = useState(false);
  const [message, setMessage] = useState("");
  const [saving, setSaving] = useState(false);
  const busy = useRef(false);

  useEffect(() => {
    const node = dialog.current;
    if (!node) return;
    if (typeof node.showModal === "function") node.showModal();
    else node.setAttribute("open", "");
  }, []);

  useEffect(() => {
    let alive = true;
    setPreview(null);
    setPreviewRestricted(false);
    void previewWithdrawal(API_URL, enrollment.id, date).then((value) => { if (alive) setPreview(value); }).catch((error: unknown) => {
      if (!alive) return;
      if (error instanceof StudentsError && error.status === 403) setPreviewRestricted(true);
      else setMessage(error instanceof Error ? error.message : "No se pudo cargar la vista previa.");
    });
    return () => { alive = false; };
  }, [date, enrollment.id]);

  const submit = async (event: FormEvent<HTMLFormElement>): Promise<void> => {
    event.preventDefault();
    if (busy.current) return;
    busy.current = true;
    setSaving(true);
    setMessage("");
    try {
      const updated = await withdraw(API_URL, enrollment.id, { withdrawnOn: date, ...(reason.trim() ? { reason: reason.trim() } : {}) });
      onSuccess(updated);
    } catch (error) {
      setMessage(error instanceof StudentsError ? error.detail || error.message : error instanceof Error ? error.message : "No se pudo dar de baja.");
      dialog.current?.querySelector<HTMLElement>("[role=alert]")?.focus();
    } finally {
      busy.current = false;
      setSaving(false);
    }
  };

  return <dialog ref={dialog} className="drawer-sm" aria-labelledby="withdraw-title" onCancel={(event) => { event.preventDefault(); onClose(); }} style={{ border: "1px solid var(--border)", borderRadius: 18, padding: 24, color: "var(--text)", background: "var(--surface)", boxShadow: "var(--shadow-card)" }}>
    <form onSubmit={(event) => void submit(event)} style={{ display: "grid", gap: 16 }}>
      <h2 id="withdraw-title" style={{ margin: 0 }}>Dar de baja</h2>
      {message && <div role="alert" tabIndex={-1} style={{ padding: 12, color: "var(--bad-fg)", background: "var(--bad-bg)", borderRadius: 10 }}>{message}</div>}
      <DateField label="Fecha de baja" required value={date} onChange={setDate} max={todayISO()} />
      <label style={{ display: "grid", gap: 6, color: "var(--text-muted)", fontSize: 14 }}>Motivo<textarea maxLength={500} value={reason} onChange={(event) => setReason(event.currentTarget.value)} onKeyDown={(event) => { if ((event.ctrlKey || event.metaKey) && event.key === "Enter") { event.preventDefault(); event.currentTarget.form?.requestSubmit(); } }} style={{ width: "100%", minHeight: 88, boxSizing: "border-box", padding: 10, font: "inherit", fontSize: 16, color: "var(--text)", background: "var(--surface)", border: "1px solid var(--border)", borderRadius: 10, resize: "vertical" }} /></label>
      <span style={{ marginTop: -12, textAlign: "right", color: "var(--text-faint)", fontSize: 12 }}>{reason.length}/500</span>
      {previewRestricted ? <p>Se cancelarán los cargos que aún no vencen y no tienen pagos</p> : preview ? <section aria-live="polite"><p>Se cancelarán {preview.toCancel.length} cargos por {formatCents(preview.toCancelCents)} que aún no vencen</p><ul>{preview.toCancel.map((item) => <li key={item.id}>{item.description} · vence {item.dueDate} · {formatCents(item.amountCents)}</li>)}</ul><p>Queda un adeudo de {formatCents(preview.remainingDebtCents)}</p></section> : <p aria-live="polite">Cargando vista previa…</p>}
      <footer style={{ display: "flex", justifyContent: "flex-end", gap: 12, alignItems: "center" }}><button type="button" onClick={onClose} style={{ minHeight: 44, border: 0, background: "transparent", color: "var(--text-muted)", cursor: "pointer" }}>Cancelar</button><button type="submit" aria-busy={saving} style={{ minHeight: 44, padding: "0 18px", border: 0, borderRadius: 99, color: "white", background: "var(--red)", cursor: "pointer", opacity: saving ? 0.65 : 1 }}>{saving ? "Guardando…" : "Dar de baja"}</button></footer>
    </form>
  </dialog>;
}
