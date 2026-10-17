"use client";

import { useEffect, useRef, useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { DrawerShell } from "../../_design/DrawerShell";
import { DateField, MoneyField, useFocusFirstInvalid } from "../../_design/form";
import { ClosedCutNotice } from "../../_design/payments/ClosedCutNotice";
import { depositRequest, newAttemptKey } from "../../_design/payments/depositRequest";
import { TenderEditor } from "../../_design/payments/TenderEditor";
import { AMOUNT_ERROR } from "../../_design/payments/tenders";
import { useClosedCut } from "../../_design/payments/useClosedCut";
import { useTenders } from "../../_design/payments/useTenders";
import { useDepositPreview } from "../../_design/payments/useDepositPreview";
import { StudentNotesBeside, useNotesBeside, type NotesAccess } from "../../_design/students/StudentNotesBeside";
import { API_URL } from "../../api-url";
import { registerDeposit, type Charge, type DepositResult } from "../../finance/api";
import { localISODate } from "../../_lib/localDate";
import { formatCents, formatPesosInput, pesosToCents } from "../../_lib/money";
import { paymentDateError } from "./CollectDrawer";
import { ChargeQuote, useChargeQuote } from "./ChargeQuote";
import { DepositPreviewCard, DepositReceipt } from "./DepositReceipt";

/** What is wrong with the amount paid against one charge, if anything: never more than it still owes. */
export function chargeAmountError(cents: number, remainingCents: number): string | undefined {
  if (!Number.isFinite(cents) || cents <= 0) return AMOUNT_ERROR;
  return cents > remainingCents ? `No puede ser mayor que el saldo del cargo (${formatCents(remainingCents)}).` : undefined;
}

export interface PayDrawerProps {
  charge: Charge;
  name: string;
  today?: string;
  notes?: NotesAccess;
  onClose: () => void;
  /** The money is in: the list behind is read again. The drawer stays on the receipt. */
  onPaid: () => void;
}

/**
 * Pay one charge from Finanzas. It is a deposit limited to that charge — the
 * counter's path (ISSUE-T14-003) — so it leaves a folio, a ticket and a line
 * in the cut under who collected it, exactly like the ventanilla.
 */
export function PayDrawer({ charge, name, today, notes, onClose, onPaid }: PayDrawerProps): ReactElement {
  // The pupil's notes on the left, as at the counter (T6).
  const beside = useNotesBeside(charge.studentId, notes);
  const [amount, setAmount] = useState(formatPesosInput(charge.remainingCents));
  const [amountError, setAmountError] = useState<string | undefined>(undefined);
  // The day the family paid (task 7): not always the day it is keyed.
  const day = today ?? localISODate();
  const [paidOn, setPaidOn] = useState(day);
  // Only a payment dated earlier travels with its date (see CollectDrawer).
  const sentDate = paidOn === day ? "" : paidOn;
  const focusFirstInvalid = useFocusFirstInvalid();
  const [dateError, setDateError] = useState<string | undefined>(undefined);
  // The payment enters the cut of the day it is dated: said before it is taken (ISSUE-026).
  const dateOk = paymentDateError(paidOn, day) === undefined;
  const cutClosed = useClosedCut(sentDate, dateOk);
  const [saving, setSaving] = useState(false);
  // A ref and not the state: two presses inside one render both read `saving` as false.
  const inFlight = useRef(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const [receipt, setReceipt] = useState<DepositResult | null>(null);
  // One key per ATTEMPT, kept across retries: a retry after a network error
  // returns the deposit already stored instead of charging the family twice.
  const [attemptKey] = useState(newAttemptKey);
  const [describe] = useState<ReadonlyMap<string, string>>(() => new Map([[charge.id, charge.description]]));
  // pesosToCents is locale-aware and returns NaN for anything ambiguous.
  const cents = pesosToCents(amount);
  // What the charge asks for on that day (review B1): the deposit adds its late
  // fee and applies pronto pago, so its bare balance is neither the amount to
  // prefill nor the most it may take.
  const quote = useChargeQuote(charge.studentId, charge.id, sentDate, dateOk);
  const capCents = quote?.totalCents ?? charge.remainingCents;
  const amountEdited = useRef(false);
  useEffect(() => {
    if (quote !== undefined && !amountEdited.current) setAmount(formatPesosInput(quote.totalCents));
  }, [quote]);
  // What the amount covers, from the planner that takes the money.
  const preview = useDepositPreview({ studentId: charge.studentId, amountCents: cents, paidOn: sentDate, chargeIds: [charge.id], enabled: receipt === null && dateOk });
  // How it is paid, method by method (T8): the rows must add up to the amount.
  const tenders = useTenders(cents);
  // Anything typed makes the drawer dirty: a stray click on the backdrop then
  // stops closing it.
  const dirty = amount.trim() !== "" || tenders.dirty;

  const submit = async (): Promise<void> => {
    if (inFlight.current) return;
    const wrongDate = paymentDateError(paidOn, day);
    const wrongAmount = chargeAmountError(cents, capCents);
    setDateError(wrongDate);
    setAmountError(wrongAmount);
    const tendersOk = wrongAmount === undefined && tenders.validate();
    if (wrongDate !== undefined || !tendersOk) {
      focusFirstInvalid();
      return;
    }
    inFlight.current = true;
    setSaving(true);
    setError(undefined);
    try {
      const result = await registerDeposit(API_URL, depositRequest({
        studentId: charge.studentId,
        amountCents: cents,
        tenders: tenders.toTenders(),
        attemptKey,
        paidOn: sentDate,
        chargeIds: [charge.id],
      }));
      setReceipt(result);
      onPaid();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo registrar el pago.");
      inFlight.current = false;
      setSaving(false);
    }
  };

  if (receipt !== null) {
    return (
      <DrawerShell title="Registrar pago" subtitle={`${name} · ${charge.description}`} icon="card" size="md" onClose={onClose} padded={false}>
        <DepositReceipt result={receipt} describe={describe} onClose={onClose} />
      </DrawerShell>
    );
  }

  return (
    <DrawerShell
      title="Registrar pago"
      subtitle={`${name} · ${charge.description}`}
      icon="card"
      // With the form started, the backdrop and Esc stop closing it; the ✕ and
      // Cancelar still do (DrawerShell uses `onClose` for all four).
      dismissible={!dirty && !beside.dirty}
      size={beside.wide ? "lg" : "md"}
      onClose={onClose}
      footer={
        <>
          <Btn kind="ghost" onClick={onClose}>Cancelar</Btn>
          <div style={{ flex: 1 }} />
          <Btn icon="check" onClick={() => void submit()} busy={saving} style={{ height: 52, fontSize: 16 }}>{saving ? "Registrando…" : "Registrar pago"}</Btn>
        </>
      }
    >
      <StudentNotesBeside beside={beside} studentId={charge.studentId}>
        <ChargeQuote line={quote} balanceCents={charge.remainingCents} />
        <MoneyField
          label="Monto a pagar"
          value={amount}
          onChange={(v) => {
            amountEdited.current = true;
            setAmount(v);
            if (chargeAmountError(pesosToCents(v), capCents) === undefined) setAmountError(undefined);
          }}
          onBlur={() => setAmountError(chargeAmountError(cents, capCents))}
          error={amountError}
          help="Lo que la familia paga ahora de este cargo: puede ser un abono, nunca más que su saldo."
          required
        />
        {preview !== null && <DepositPreviewCard preview={preview} describe={describe} fallback={`Recargo · ${charge.description}`} />}
        <TenderEditor tenders={tenders} />
        <DateField
          label="Fecha de pago"
          value={paidOn}
          max={day}
          onChange={(v) => {
            setPaidOn(v);
            if (paymentDateError(v, day) === undefined) setDateError(undefined);
          }}
          onBlur={() => setDateError(paymentDateError(paidOn, day))}
          error={dateError}
          help="El día en que la familia pagó. Si capturas tarde, pon ese día: el recargo y el pronto pago se calculan con él."
        />
        <ClosedCutNotice closed={cutClosed} day={sentDate} />
        {error !== undefined && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 14, margin: 0 }}>{error}</p>}
      </StudentNotesBeside>
    </DrawerShell>
  );
}
