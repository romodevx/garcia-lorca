"use client";

import { useState, type FormEvent, type ReactElement } from "react";

import { API_URL } from "../../api-url";
import { Btn } from "../../_design/Controls";
import { Icon } from "../../_design/Icon";
import { MoneyField, TextField, useRequired } from "../../_design/form";
import { applyDiscount, cancelCharge, type Charge } from "../../finance/api";
import { formatCents, formatPesosInput, pesosToCents } from "../../_lib/money";
import { BAD_AMOUNT } from "./conceptCopy";

export interface ChargeAdjustPanelProps {
  charge: Charge;
  /** The pupil, as the list names them. */
  name: string;
  /** The charge as the server left it, and what to tell the person. */
  onDone: (updated: Charge, message: string) => void;
  onClose: () => void;
}

type Mode = "discount" | "cancel";
const MODES: readonly { value: Mode; label: string }[] = [
  { value: "discount", label: "Descuento" },
  { value: "cancel", label: "Cancelar cargo" },
];

/**
 * Adjusting one charge from its row (CC-203): a discount (two fields) or a
 * cancellation (one). Under the house rule that is an inline panel, not a
 * drawer. A discount is replaced by the next one, so it needs no confirmation;
 * a cancellation is terminal, so the panel says so before the button does it.
 */
export function ChargeAdjustPanel({ charge, name, onDone, onClose }: ChargeAdjustPanelProps): ReactElement {
  const [mode, setMode] = useState<Mode>("discount");
  const [amount, setAmount] = useState(charge.discountCents > 0 ? formatPesosInput(charge.discountCents) : "");
  const [reason, setReason] = useState("");
  const [amountError, setAmountError] = useState<string | undefined>(undefined);
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const [saving, setSaving] = useState(false);
  const required = useRequired(mode === "discount" ? ["amount", "reason"] : ["reason"]);
  const capCents = charge.amountCents - (charge.scholarshipCents ?? 0);

  const discountProblem = (): string | undefined => {
    const cents = pesosToCents(amount);
    if (Number.isNaN(cents)) return BAD_AMOUNT;
    return cents > capCents ? `No puede ser mayor que el importe (${formatCents(capCents)}), sin contar la beca.` : undefined;
  };

  const run = async (): Promise<void> => {
    setSaving(true);
    setProblem(undefined);
    try {
      if (mode === "discount") {
        onDone(await applyDiscount(API_URL, charge.id, pesosToCents(amount), reason.trim()), `Descuento aplicado a ${name}.`);
      } else {
        onDone(await cancelCharge(API_URL, charge.id, reason.trim()), `Cargo de ${name} cancelado.`);
      }
    } catch {
      setProblem(mode === "discount" ? "No se pudo aplicar el descuento. Intenta de nuevo." : "No se pudo cancelar el cargo. Puede que ya estuviera cancelado; vuelve a cargar la lista.");
    } finally {
      setSaving(false);
    }
  };

  const submit = (e: FormEvent): void => {
    e.preventDefault();
    if (saving) return;
    if (!required.check({ amount, reason })) return;
    if (mode === "discount") {
      const bad = discountProblem();
      setAmountError(bad);
      if (bad !== undefined) return;
    }
    void run();
  };

  return (
    <form onSubmit={submit} aria-label={`Ajustar cargo de ${name}`} style={{ margin: "4px 8px 10px", padding: "16px 18px", display: "grid", gap: 14, borderRadius: "var(--r-md)", background: "var(--surface-2)", border: "1px solid var(--border)" }}>
      <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
        <div role="radiogroup" aria-label="Qué hacer con el cargo" style={{ display: "flex", gap: 6, padding: 4, background: "var(--surface-3)", borderRadius: 99 }}>
          {MODES.map((m) => (
            <label key={m.value} style={{ display: "inline-flex", alignItems: "center", gap: 6, minHeight: 44, padding: "0 14px", borderRadius: 99, cursor: "pointer", fontWeight: 600, fontSize: 14, background: mode === m.value ? "var(--surface)" : "transparent" }}>
              <input type="radio" name={`adjust-${charge.id}`} value={m.value} checked={mode === m.value} onChange={() => { setMode(m.value); setProblem(undefined); }} />
              {m.label}
            </label>
          ))}
        </div>
        <span style={{ flex: 1, fontSize: 14, color: "var(--text-muted)" }}>{charge.description} · {formatCents(charge.amountCents)}</span>
        <button type="button" aria-label="Cerrar ajustes" onClick={onClose} style={{ minWidth: 44, minHeight: 44, display: "grid", placeItems: "center", border: "none", background: "none", color: "var(--text-muted)", cursor: "pointer" }}><Icon name="close" size={18} /></button>
      </div>
      {mode === "discount" ? (
        <MoneyField label="Monto del descuento" required value={amount} onChange={(v) => { setAmount(v); setAmountError(undefined); }} onClearError={() => required.clear("amount")} error={required.errors.amount ?? amountError} help="El descuento total del cargo: sustituye al que ya tuviera." />
      ) : (
        <p style={{ margin: 0, display: "flex", gap: 8, color: "var(--bad-fg)", fontSize: 14.5, fontWeight: 600 }}>
          <Icon name="warning" size={18} />Cancelar un cargo no se puede deshacer: deja de contar en el estado de cuenta. Lo que ya se pagó sigue registrado.
        </p>
      )}
      <TextField label="Motivo" required value={reason} onChange={setReason} onClearError={() => required.clear("reason")} error={required.errors.reason} help="Queda en el historial del cargo." />
      {problem !== undefined && <p role="alert" style={{ margin: 0, color: "var(--bad-fg)", fontWeight: 600 }}>{problem}</p>}
      <div style={{ display: "flex", justifyContent: "flex-end", gap: 12 }}>
        <Btn kind="ghost" onClick={onClose}>Volver</Btn>
        <Btn type="submit" busy={saving} danger={mode === "cancel"} icon={mode === "cancel" ? "trash" : "check"}>{mode === "discount" ? "Aplicar descuento" : "Cancelar este cargo"}</Btn>
      </div>
    </form>
  );
}
